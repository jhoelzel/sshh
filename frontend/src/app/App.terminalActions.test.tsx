import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

interface ControllerDouble {
  clearScrollback: ReturnType<typeof vi.fn>
  dispose: ReturnType<typeof vi.fn>
  resetTerminal: ReturnType<typeof vi.fn>
  sessionId: string
}

const harness = vi.hoisted(() => {
  const lease = { id: 'lease-1', expiresAt: '2026-07-18T23:00:00Z' }
  const profile = {
    id: 'local-1', name: 'Local Shell', protocol: 'local', host: '', port: 0,
    username: '', authentication: 'auto', identityFile: '', shell: '', arguments: [],
    workingDirectory: '', environment: {}, tags: [], group: '', favorite: true,
    endpoint: 'Local shell', connectable: true,
  }
  const settings = {
    terminal: {
      fontFamily: 'system-mono', fontSize: 13, lineHeight: 1.2,
      cursorStyle: 'block', cursorBlink: true, scrollback: 10_000, bell: true,
    },
    connection: {
      connectTimeoutSeconds: 15, keepAliveEnabled: true,
      keepAliveIntervalSeconds: 30, keepAliveMaxFailures: 3,
    },
    notifications: {
      enabled: false, transferCompleted: true,
      unexpectedDisconnect: true, longTransferSeconds: 30,
    },
    transfers: { concurrency: 2, collisionPolicy: 'ask', keepPartialFiles: false },
  }
  let sessionCounter = 0
  const backend = {
    attachFrontend: vi.fn().mockResolvedValue(lease),
    listProfiles: vi.fn().mockResolvedValue([profile]),
    listTunnels: vi.fn().mockResolvedValue([]),
    listSnippets: vi.fn().mockResolvedValue([]),
    listWorkspaceLayouts: vi.fn().mockResolvedValue([]),
    listRemotePathFavorites: vi.fn().mockResolvedValue([]),
    getSettings: vi.fn().mockResolvedValue(settings),
    getBuildInfo: vi.fn().mockResolvedValue({
      version: '0.1.0-dev', commit: 'abcdef123456', buildDate: '2026-07-18T20:00:00Z',
      dirty: true, goVersion: 'go1.26.5', platform: 'darwin/arm64',
    }),
    getNotificationStatus: vi.fn().mockResolvedValue({
      available: true, authorized: false, message: 'Permission is required',
    }),
    listTransfers: vi.fn().mockResolvedValue([]),
    listTunnelStates: vi.fn().mockResolvedValue([]),
    openLocalTerminal: vi.fn(async () => {
      sessionCounter += 1
      return {
        id: `session-${sessionCounter}`, generation: 1, leaseId: lease.id, profileId: profile.id,
        title: 'Local Shell', state: 'running', columns: 100, rows: 30,
        startedAt: '2026-07-18T20:00:00Z',
      }
    }),
    activateTerminal: vi.fn().mockResolvedValue(undefined),
    closeTerminal: vi.fn().mockResolvedValue(undefined),
    renewFrontendLease: vi.fn().mockResolvedValue(lease),
  }

  const listeners = new Map<string, Set<(...data: unknown[]) => void>>()
  const subscribe = (name: string, callback: (...data: unknown[]) => void) => {
    const callbacks = listeners.get(name) ?? new Set()
    callbacks.add(callback)
    listeners.set(name, callbacks)
    return () => callbacks.delete(callback)
  }

  return {
    backend,
    controllerInstances: [] as ControllerDouble[],
    emit: (name: string, ...data: unknown[]) => {
      for (const callback of [...(listeners.get(name) ?? [])]) callback(...data)
    },
    subscribe,
  }
})

vi.mock('../lib/bridge/client', () => ({
  backend: harness.backend,
  onTerminalOutput: (callback: (...data: unknown[]) => void) => harness.subscribe('output', callback),
  onSessionState: (callback: (...data: unknown[]) => void) => harness.subscribe('state', callback),
  onTransfer: (callback: (...data: unknown[]) => void) => harness.subscribe('transfer', callback),
  onTunnel: (callback: (...data: unknown[]) => void) => harness.subscribe('tunnel', callback),
  onSessionLog: (callback: (...data: unknown[]) => void) => harness.subscribe('log', callback),
  onCloseRequested: (callback: (...data: unknown[]) => void) => harness.subscribe('close', callback),
}))

vi.mock('../../wailsjs/runtime/runtime', () => ({ BrowserOpenURL: vi.fn() }))

vi.mock('../feature/terminal/TerminalController', () => ({
  TerminalController: class implements ControllerDouble {
    acceptOutput = vi.fn()
    applySettings = vi.fn()
    attach = vi.fn()
    clearScrollback = vi.fn()
    dispose = vi.fn()
    findNext = vi.fn()
    findPrevious = vi.fn()
    focus = vi.fn()
    ready = vi.fn().mockResolvedValue(undefined)
    resetTerminal = vi.fn()
    selectedText = vi.fn(() => '')
    sessionId: string
    setVisible = vi.fn()
    visibleText = vi.fn(() => '')

    constructor(session: { id: string }) {
      this.sessionId = session.id
      harness.controllerInstances.push(this)
    }
  },
}))

import { App } from './App'

afterEach(cleanup)

describe('terminal tab actions', () => {
  it('clears, resets, duplicates, retries in place, and reconnects in a new tab', async () => {
    render(<App />)
    await screen.findByText('Local Shell')

    fireEvent.keyDown(document, { key: 't', ctrlKey: true, shiftKey: true })
    await waitFor(() => expect(harness.backend.activateTerminal).toHaveBeenCalledTimes(1))
    expect(harness.controllerInstances.map((controller) => controller.sessionId)).toEqual(['session-1'])

    await runTerminalAction('Clear scrollback')
    expect(harness.controllerInstances[0].clearScrollback).toHaveBeenCalledOnce()
    await runTerminalAction('Reset terminal')
    expect(harness.controllerInstances[0].resetTerminal).toHaveBeenCalledOnce()

    await runTerminalAction('Duplicate terminal tab')
    await waitFor(() => expect(harness.backend.activateTerminal).toHaveBeenCalledTimes(2))
    expect(harness.backend.openLocalTerminal).toHaveBeenCalledTimes(2)
    expect(screen.getAllByRole('tab', { name: 'Local Shell' })).toHaveLength(2)

    emitSessionState('session-2', 'exited')
    await expectTerminalActionState('Retry terminal', false)
    await expectTerminalActionState('Reconnect in new tab', false)
    await expectTerminalActionState('Duplicate terminal tab', true)
    closeTerminalActions()

    await runTerminalAction('Retry terminal')
    await waitFor(() => expect(harness.backend.activateTerminal).toHaveBeenCalledTimes(3))
    expect(harness.backend.closeTerminal).toHaveBeenCalledWith('lease-1', 'session-2', 1)
    expect(harness.controllerInstances[1].dispose).toHaveBeenCalledOnce()
    expect(screen.getAllByRole('tab', { name: 'Local Shell' })).toHaveLength(2)

    emitSessionState('session-3', 'failed')
    await runTerminalAction('Reconnect in new tab')
    await waitFor(() => expect(harness.backend.activateTerminal).toHaveBeenCalledTimes(4))
    expect(harness.backend.closeTerminal).toHaveBeenCalledTimes(1)
    expect(harness.controllerInstances[2].dispose).not.toHaveBeenCalled()
    expect(screen.getAllByRole('tab', { name: 'Local Shell' })).toHaveLength(3)

    emitSessionState('session-4', 'closed')
    await runTerminalAction('Retry terminal')
    await waitFor(() => expect(harness.backend.activateTerminal).toHaveBeenCalledTimes(5))
    expect(harness.backend.closeTerminal).toHaveBeenCalledTimes(1)
    expect(harness.controllerInstances[3].dispose).toHaveBeenCalledOnce()
    expect(screen.getAllByRole('tab', { name: 'Local Shell' })).toHaveLength(3)
  })
})

async function runTerminalAction(name: string) {
  fireEvent.click(screen.getByRole('button', { name: 'Terminal actions' }))
  const option = await screen.findByRole('option', { name })
  expect((option as HTMLButtonElement).disabled).toBe(false)
  fireEvent.click(option)
}

async function expectTerminalActionState(name: string, disabled: boolean) {
  if (!screen.queryByRole('dialog', { name: 'Terminal actions' })) {
    fireEvent.click(screen.getByRole('button', { name: 'Terminal actions' }))
  }
  const option = await screen.findByRole('option', { name })
  await waitFor(() => expect((option as HTMLButtonElement).disabled).toBe(disabled))
}

function closeTerminalActions() {
  fireEvent.click(screen.getByRole('button', { name: 'Close command palette' }))
}

function emitSessionState(sessionId: string, state: 'exited' | 'failed' | 'closed') {
  act(() => harness.emit('state', {
    leaseId: 'lease-1', sessionId, generation: 1, title: 'Local Shell', state,
    exitCode: state === 'exited' ? 0 : undefined,
    signal: '', message: state === 'failed' ? 'Connection lost' : '',
  }))
}
