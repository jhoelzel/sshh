import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

interface ControllerDouble {
  acceptOutput: ReturnType<typeof vi.fn>
  attach: ReturnType<typeof vi.fn>
  dispose: ReturnType<typeof vi.fn>
  sessionId: string
  setVisible: ReturnType<typeof vi.fn>
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
        title: `Local ${String(sessionCounter).padStart(2, '0')}`, state: 'running',
        columns: 100, rows: 30, startedAt: '2026-07-18T20:00:00Z',
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

describe('terminal tab stress', () => {
  it('keeps 50 controllers persistent through hidden output and repeated switching, then disposes each once', async () => {
    const view = render(<App />)
    await screen.findByText('Local Shell')
    const newTerminal = screen.getByRole('button', { name: 'New local terminal' }) as HTMLButtonElement

    for (let index = 1; index <= 50; index += 1) {
      fireEvent.click(newTerminal)
      await waitFor(() => {
        expect(harness.backend.activateTerminal).toHaveBeenCalledTimes(index)
        expect(newTerminal.disabled).toBe(false)
      })
    }

    await waitFor(() => expect(screen.getAllByRole('tab')).toHaveLength(50))
    expect(harness.controllerInstances).toHaveLength(50)
    for (const controller of harness.controllerInstances) {
      expect(controller.attach).toHaveBeenCalledOnce()
      expect(controller.dispose).not.toHaveBeenCalled()
    }

    fireEvent.click(screen.getByRole('button', { name: 'Split terminal right' }))
    let separator = screen.getByRole('separator', { name: 'Resize terminal split' })
    expect(separator.getAttribute('aria-orientation')).toBe('vertical')
    expect(screen.getByRole('button', { name: 'Focus Local 50 pane' }).getAttribute('aria-pressed')).toBe('false')
    expect(screen.getByRole('button', { name: 'Focus Local 01 pane' }).getAttribute('aria-pressed')).toBe('true')

    fireEvent.click(screen.getByRole('button', { name: 'Split terminal down' }))
    separator = screen.getByRole('separator', { name: 'Resize terminal split' })
    expect(separator.getAttribute('aria-orientation')).toBe('horizontal')
    fireEvent.keyDown(separator, { key: 'ArrowDown' })
    expect(separator.getAttribute('aria-valuenow')).toBe('55')

    fireEvent.click(screen.getByRole('tab', { name: 'Local 13' }))
    expect(screen.queryByRole('button', { name: 'Focus Local 01 pane' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Focus Local 13 pane' }).getAttribute('aria-pressed')).toBe('true')
    fireEvent.click(screen.getByRole('button', { name: 'Close terminal split' }))
    expect(screen.queryByRole('separator', { name: 'Resize terminal split' })).toBeNull()
    expect(screen.getByRole('tab', { name: 'Local 13' }).getAttribute('aria-selected')).toBe('true')
    for (const controller of harness.controllerInstances) {
      expect(controller.attach).toHaveBeenCalledOnce()
      expect(controller.dispose).not.toHaveBeenCalled()
    }

    const visibilityCallsBeforeOutput = totalVisibilityCalls()
    const hiddenIndexes = [0, 12, 24, 36]
    act(() => {
      for (const controllerIndex of hiddenIndexes) {
        for (let sequence = 1; sequence <= 256; sequence += 1) {
          harness.emit('output', {
            leaseId: 'lease-1', sessionId: `session-${controllerIndex + 1}`, generation: 1,
            sequence, endOffset: sequence, byteCount: 1, payload: 'eA==', final: false,
          })
        }
      }
    })
    for (const controllerIndex of hiddenIndexes) {
      expect(harness.controllerInstances[controllerIndex].acceptOutput).toHaveBeenCalledTimes(256)
    }
    expect(harness.controllerInstances[49].acceptOutput).not.toHaveBeenCalled()
    expect(totalVisibilityCalls()).toBe(visibilityCallsBeforeOutput)

    const tabs = screen.getAllByRole('tab')
    for (let index = 0; index < 137; index += 1) {
      fireEvent.click(tabs[(index * 17) % tabs.length])
    }
    expect(screen.getByRole('tab', { name: 'Local 13' }).getAttribute('aria-selected')).toBe('true')
    expect(totalVisibilityCalls()).toBe(visibilityCallsBeforeOutput + 137 * 2)
    for (const controller of harness.controllerInstances) {
      expect(controller.attach).toHaveBeenCalledOnce()
      expect(controller.dispose).not.toHaveBeenCalled()
    }

    act(() => {
      for (let index = 1; index <= 50; index += 1) {
        harness.emit('state', {
          leaseId: 'lease-1', sessionId: `session-${index}`, generation: 1,
          title: `Local ${String(index).padStart(2, '0')}`, state: 'exited',
          exitCode: 0, signal: '', message: '',
        })
      }
    })
    await waitFor(() => expect(document.querySelectorAll('.state-exited')).toHaveLength(50))

    const closeButtons = screen.getAllByRole('button', { name: /^Close Local \d{2}$/ })
    for (const button of closeButtons) fireEvent.click(button)
    await waitFor(() => expect(harness.backend.closeTerminal).toHaveBeenCalledTimes(50))
    await waitFor(() => expect(screen.queryAllByRole('tab')).toHaveLength(0))
    for (const controller of harness.controllerInstances) {
      expect(controller.dispose).toHaveBeenCalledOnce()
    }

    view.unmount()
    for (const controller of harness.controllerInstances) {
      expect(controller.dispose).toHaveBeenCalledOnce()
    }
  }, 20_000)
})

function totalVisibilityCalls(): number {
  return harness.controllerInstances.reduce(
    (total, controller) => total + controller.setVisible.mock.calls.length,
    0,
  )
}
