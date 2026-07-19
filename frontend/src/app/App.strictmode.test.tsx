import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
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
  requestLink: (url: string) => void
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
    ui: { theme: 'dark', sidebarWidth: 272, workspace: 'terminals' },
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
    closeTerminal: vi.fn().mockResolvedValue(undefined),
    confirmApplicationClose: vi.fn().mockResolvedValue(undefined),
    renewFrontendLease: vi.fn().mockResolvedValue(lease),
    updateUIPreferences: vi.fn(async (value) => ({ ...settings.ui, ...value })),
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
    settings,
    browserOpenURL: vi.fn(),
    setWindowBackground: vi.fn(),
    setWindowDark: vi.fn(),
    setWindowLight: vi.fn(),
    subscribe,
    disposers,
    controllerInstances: [] as ControllerDouble[],
    emit: (name: string, ...data: unknown[]) => {
      for (const callback of [...(activeListeners.get(name) ?? [])]) {
        callback(...data)
      }
    },
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

vi.mock('../../wailsjs/runtime/runtime', () => ({
  BrowserOpenURL: harness.browserOpenURL,
  WindowSetBackgroundColour: harness.setWindowBackground,
  WindowSetDarkTheme: harness.setWindowDark,
  WindowSetLightTheme: harness.setWindowLight,
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
    requestLink: (url: string) => void
    search = vi.fn()
    selectionText = vi.fn(() => '')
    setVisible = vi.fn()
    visibleText = vi.fn(() => '')

    constructor(
      _session: unknown,
      _settings: unknown,
      callbacks: { onLinkRequested: (url: string) => void },
    ) {
      this.requestLink = callbacks.onLinkRequested
      harness.controllerInstances.push(this)
    }
  },
}))

import { App } from './App'

afterEach(cleanup)

describe('App StrictMode lifecycle', () => {
  it('does not duplicate or retain commands, listeners, controllers, or output delivery', async () => {
    const emitOutputBurst = (count: number) => {
      for (let index = 0; index < count; index += 1) {
        harness.emit('output', {
          leaseId: 'lease-1', sessionId: 'session-1', generation: 1,
          sequence: index + 1, endOffset: index + 1, byteCount: 1,
          payload: 'eA==', final: false,
        })
      }
    }
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

    const appShell = document.querySelector<HTMLElement>('.app-shell')
    expect(appShell?.dataset.theme).toBe('dark')
    expect(appShell?.style.getPropertyValue('--sidebar-width')).toBe('272px')
    const sidebarSeparator = screen.getByRole('separator', { name: 'Resize sidebar' })
    expect(sidebarSeparator.getAttribute('aria-valuenow')).toBe('272')
    fireEvent.keyDown(sidebarSeparator, { key: 'ArrowRight' })
    await waitFor(() => expect(harness.backend.updateUIPreferences).toHaveBeenCalledWith({
      sidebarWidth: 280,
      workspace: 'terminals',
    }))
    expect(appShell?.style.getPropertyValue('--sidebar-width')).toBe('280px')

    fireEvent.keyDown(document, { key: 't', ctrlKey: true, shiftKey: true })
    await waitFor(() => expect(harness.backend.activateTerminal).toHaveBeenCalledOnce())
    expect(harness.backend.openLocalTerminal).toHaveBeenCalledOnce()
    expect(harness.controllerInstances).toHaveLength(1)

    act(() => {
      harness.emit('transfer', {
        id: 'transfer-1', leaseId: 'lease-1', sessionId: 'files-1', direction: 'download',
        source: '/remote/archive.tar', destination: '/local/archive.tar', bytes: 50, total: 100,
        state: 'running', message: '', resumeId: '', resumedFrom: 0,
        startedAt: '2026-07-17T22:01:00Z', finishedAt: '',
      })
      harness.emit('tunnel', {
        configId: 'missing-tunnel', leaseId: 'lease-1', state: 'active',
        boundAddress: '127.0.0.1:4400', message: '',
        startedAt: '2026-07-17T22:01:00Z', updatedAt: '2026-07-17T22:01:00Z',
      })
    })
    fireEvent.click(screen.getByRole('button', { name: /Activity/ }))
    expect(await screen.findByRole('region', { name: 'Workspace activity' })).toBeTruthy()
    expect(screen.getByText('3 active · 0 issues')).toBeTruthy()
    expect(screen.getByText('archive.tar')).toBeTruthy()
    expect(screen.getByText('Unavailable tunnel')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Focus Local Shell' }))
    expect(screen.queryByRole('region', { name: 'Workspace activity' })).toBeNull()
    expect(screen.getByRole('tab', { name: 'Local Shell' }).getAttribute('aria-selected')).toBe('true')
    await waitFor(() => expect(harness.backend.updateUIPreferences).toHaveBeenCalledTimes(3))
    expect(harness.backend.updateUIPreferences).toHaveBeenNthCalledWith(2, {
      sidebarWidth: 280,
      workspace: 'activity',
    })
    expect(harness.backend.updateUIPreferences).toHaveBeenNthCalledWith(3, {
      sidebarWidth: 280,
      workspace: 'terminals',
    })

    fireEvent.click(screen.getByRole('button', { name: 'Find terminal tab' }))
    expect(await screen.findByRole('dialog', { name: 'Find terminal tab' })).toBeTruthy()
    expect(screen.getByRole('option', { name: 'Switch to 1: Local Shell' })).toBeTruthy()
    fireEvent.click(screen.getByRole('option', { name: 'Switch to 1: Local Shell' }))

    harness.controllerInstances[0].requestLink('https://example.com/documentation')
    expect(await screen.findByRole('dialog', { name: 'Open external link?' })).toBeTruthy()
    expect(screen.getByText('https://example.com/documentation')).toBeTruthy()
    expect(harness.browserOpenURL).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Open link' }))
    expect(harness.browserOpenURL).toHaveBeenCalledOnce()
    expect(harness.browserOpenURL).toHaveBeenCalledWith('https://example.com/documentation')

    emitOutputBurst(512)
    expect(harness.controllerInstances[0].acceptOutput).toHaveBeenCalledTimes(512)

    fireEvent.click(screen.getByRole('button', { name: 'Close Local Shell' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Close' }))
    await waitFor(() => expect(harness.backend.closeTerminal).toHaveBeenCalledOnce())
    expect(harness.controllerInstances[0].dispose).toHaveBeenCalledOnce()
    expect(harness.activeListenerCount()).toBe(6)

    emitOutputBurst(1)
    expect(harness.controllerInstances[0].acceptOutput).toHaveBeenCalledTimes(512)

    first.unmount()
    expect(harness.activeListenerCount()).toBe(0)
    expect(harness.controllerInstances[0].dispose).toHaveBeenCalledOnce()
    for (const dispose of harness.disposers) {
      expect(dispose).toHaveBeenCalledOnce()
    }

    harness.settings.ui.theme = 'light'
    harness.settings.ui.sidebarWidth = 340
    harness.settings.ui.workspace = 'activity'
    const second = render(<StrictMode><App /></StrictMode>)
    await screen.findByText('Local Shell')
    await waitFor(() => expect(harness.activeListenerCount()).toBe(6))
    expect(await screen.findByRole('region', { name: 'Workspace activity' })).toBeTruthy()
    const restoredShell = document.querySelector<HTMLElement>('.app-shell')
    expect(restoredShell?.dataset.theme).toBe('light')
    expect(restoredShell?.style.getPropertyValue('--sidebar-width')).toBe('340px')
    expect(harness.setWindowBackground).toHaveBeenLastCalledWith(243, 245, 244, 255)
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

    emitOutputBurst(512)
    expect(harness.controllerInstances[1].acceptOutput).toHaveBeenCalledTimes(512)

    act(() => harness.emit('close'))
    expect(await screen.findByRole('dialog', { name: 'Close running sessions?' })).toBeTruthy()
    expect(screen.getByText('1 active resource will be closed.')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(harness.backend.confirmApplicationClose).not.toHaveBeenCalled()

    act(() => harness.emit('close'))
    fireEvent.click(await screen.findByRole('button', { name: 'Close' }))
    await waitFor(() => expect(harness.backend.confirmApplicationClose).toHaveBeenCalledOnce())
    expect(harness.backend.confirmApplicationClose).toHaveBeenCalledWith('lease-1')

    second.unmount()
    expect(harness.activeListenerCount()).toBe(0)
    expect(harness.controllerInstances[1].dispose).toHaveBeenCalledOnce()
    emitOutputBurst(1)
    expect(harness.controllerInstances[1].acceptOutput).toHaveBeenCalledTimes(512)
    for (const dispose of harness.disposers) {
      expect(dispose).toHaveBeenCalledOnce()
    }
  })
})
