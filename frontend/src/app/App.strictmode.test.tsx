import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

interface ControllerDouble {
  acceptOutput: ReturnType<typeof vi.fn>
  activate: ReturnType<typeof vi.fn>
  applySettings: ReturnType<typeof vi.fn>
  attach: ReturnType<typeof vi.fn>
  clearSearch: ReturnType<typeof vi.fn>
  dispose: ReturnType<typeof vi.fn>
  focus: ReturnType<typeof vi.fn>
  ready: ReturnType<typeof vi.fn>
  search: ReturnType<typeof vi.fn>
  selectionText: ReturnType<typeof vi.fn>
  setVisible: ReturnType<typeof vi.fn>
  visibleText: ReturnType<typeof vi.fn>
}

const harness = vi.hoisted(() => {
  const lease = { id: 'lease-1', expiresAt: '2026-07-17T23:00:00Z' }
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
  const session = {
    id: 'session-1', generation: 1, leaseId: lease.id, profileId: profile.id,
    title: 'Local Shell', state: 'running', columns: 100, rows: 30,
    startedAt: '2026-07-17T22:00:00Z',
  }
  const backend = {
    attachFrontend: vi.fn().mockResolvedValue(lease),
    listProfiles: vi.fn().mockResolvedValue([profile]),
    listTunnels: vi.fn().mockResolvedValue([]),
    listSnippets: vi.fn().mockResolvedValue([]),
    listWorkspaceLayouts: vi.fn().mockResolvedValue([]),
    listRemotePathFavorites: vi.fn().mockResolvedValue([]),
    getSettings: vi.fn().mockResolvedValue(settings),
    getBuildInfo: vi.fn().mockResolvedValue({
      version: '0.1.0-dev', commit: 'abcdef123456', buildDate: '2026-07-17T20:00:00Z',
      dirty: true, goVersion: 'go1.26.5', platform: 'darwin/arm64',
    }),
    getNotificationStatus: vi.fn().mockResolvedValue({
      available: true, authorized: false, message: 'Permission is required',
    }),
    listTransfers: vi.fn().mockResolvedValue([]),
    listTunnelStates: vi.fn().mockResolvedValue([]),
    openLocalTerminal: vi.fn().mockResolvedValue(session),
    activateTerminal: vi.fn().mockResolvedValue(undefined),
    renewFrontendLease: vi.fn().mockResolvedValue(lease),
  }

  const activeListeners = new Map<string, Set<(...data: unknown[]) => void>>()
  const disposers: Array<ReturnType<typeof vi.fn>> = []
  const subscribe = vi.fn((name: string, callback: (...data: unknown[]) => void) => {
    const callbacks = activeListeners.get(name) ?? new Set()
    callbacks.add(callback)
    activeListeners.set(name, callbacks)
    let active = true
    const dispose = vi.fn(() => {
      if (!active) return
      active = false
      callbacks.delete(callback)
    })
    disposers.push(dispose)
    return dispose
  })

  return {
    backend,
    subscribe,
    disposers,
    controllerInstances: [] as ControllerDouble[],
    activeListenerCount: () =>
      [...activeListeners.values()].reduce((total, callbacks) => total + callbacks.size, 0),
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

vi.mock('../feature/terminal/TerminalController', () => ({
  TerminalController: class implements ControllerDouble {
    acceptOutput = vi.fn()
    activate = vi.fn()
    applySettings = vi.fn()
    attach = vi.fn()
    clearSearch = vi.fn()
    dispose = vi.fn()
    focus = vi.fn()
    ready = vi.fn().mockResolvedValue(undefined)
    search = vi.fn()
    selectionText = vi.fn(() => '')
    setVisible = vi.fn()
    visibleText = vi.fn(() => '')

    constructor() {
      harness.controllerInstances.push(this)
    }
  },
}))

import { App } from './App'

afterEach(cleanup)

describe('App StrictMode lifecycle', () => {
  it('does not duplicate startup commands, listeners, controllers, or keyboard actions', async () => {
    const first = render(<StrictMode><App /></StrictMode>)

    await screen.findByText('Local Shell')
    await waitFor(() => expect(harness.activeListenerCount()).toBe(6))
    for (const command of [
      harness.backend.attachFrontend,
      harness.backend.listProfiles,
      harness.backend.listTunnels,
      harness.backend.listSnippets,
      harness.backend.listWorkspaceLayouts,
      harness.backend.listRemotePathFavorites,
      harness.backend.getSettings,
      harness.backend.getBuildInfo,
      harness.backend.getNotificationStatus,
    ]) {
      expect(command).toHaveBeenCalledOnce()
    }
    expect(harness.backend.listTransfers).toHaveBeenCalledOnce()
    expect(harness.backend.listTunnelStates).toHaveBeenCalledOnce()
    expect(harness.controllerInstances).toHaveLength(0)

    fireEvent.keyDown(document, { key: 't', ctrlKey: true, shiftKey: true })
    await waitFor(() => expect(harness.backend.activateTerminal).toHaveBeenCalledOnce())
    expect(harness.backend.openLocalTerminal).toHaveBeenCalledOnce()
    expect(harness.controllerInstances).toHaveLength(1)

    first.unmount()
    expect(harness.activeListenerCount()).toBe(0)
    expect(harness.controllerInstances[0].dispose).toHaveBeenCalledOnce()
    for (const dispose of harness.disposers) {
      expect(dispose).toHaveBeenCalledOnce()
    }

    const second = render(<StrictMode><App /></StrictMode>)
    await screen.findByText('Local Shell')
    await waitFor(() => expect(harness.activeListenerCount()).toBe(6))
    for (const command of [
      harness.backend.attachFrontend,
      harness.backend.listProfiles,
      harness.backend.listTunnels,
      harness.backend.listSnippets,
      harness.backend.listWorkspaceLayouts,
      harness.backend.listRemotePathFavorites,
      harness.backend.getSettings,
      harness.backend.getBuildInfo,
      harness.backend.getNotificationStatus,
      harness.backend.listTransfers,
      harness.backend.listTunnelStates,
    ]) {
      expect(command).toHaveBeenCalledTimes(2)
    }

    fireEvent.keyDown(document, { key: 't', ctrlKey: true, shiftKey: true })
    await waitFor(() => expect(harness.backend.activateTerminal).toHaveBeenCalledTimes(2))
    expect(harness.backend.openLocalTerminal).toHaveBeenCalledTimes(2)
    expect(harness.controllerInstances).toHaveLength(2)

    second.unmount()
    expect(harness.activeListenerCount()).toBe(0)
    expect(harness.controllerInstances[1].dispose).toHaveBeenCalledOnce()
    for (const dispose of harness.disposers) {
      expect(dispose).toHaveBeenCalledOnce()
    }
  })
})
