import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AppSettings } from '../../lib/bridge/types'
import { SettingsWorkspace } from './SettingsWorkspace'

const settings = {
  terminal: {
    fontFamily: 'system-mono' as const,
    fontSize: 13,
    lineHeight: 1.2,
    cursorStyle: 'block' as const,
    cursorBlink: true,
    scrollback: 10000,
    bell: true,
  },
  connection: {
    connectTimeoutSeconds: 15,
    keepAliveEnabled: true,
    keepAliveIntervalSeconds: 30,
    keepAliveMaxFailures: 3,
  },
  notifications: {
    enabled: false,
    transferCompleted: true,
    unexpectedDisconnect: true,
    longTransferSeconds: 30,
  },
  transfers: {
    concurrency: 2,
    collisionPolicy: 'ask' as const,
    keepPartialFiles: false,
  },
  ui: {
    theme: 'dark' as const,
    sidebarWidth: 272,
    workspace: 'terminals' as const,
  },
}

const notificationStatus = { available: true, authorized: false, message: 'Permission is required' }

const buildInfo = {
  version: '0.1.0-dev',
  commit: '1234567890abcdef',
  buildDate: '2026-07-17T15:30:00Z',
  dirty: true,
  goVersion: 'go1.26.5',
  platform: 'darwin/arm64',
}

const renderSettings = (overrides: Partial<React.ComponentProps<typeof SettingsWorkspace>> = {}) => render(
  <SettingsWorkspace
    settings={settings}
    buildInfo={buildInfo}
    notificationStatus={notificationStatus}
    onSave={vi.fn()}
    onReset={vi.fn()}
    onRequestNotificationPermission={vi.fn()}
    onSendTestNotification={vi.fn()}
    {...overrides}
  />,
)

afterEach(cleanup)

describe('SettingsWorkspace', () => {
  it('saves a validated terminal settings draft', async () => {
    const save = vi.fn(async (value: AppSettings) => value)
    renderSettings({ onSave: save })

    fireEvent.change(screen.getByLabelText('Font size'), { target: { value: '16' } })
    fireEvent.click(screen.getByRole('button', { name: 'Bar' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))
    await waitFor(() => expect(save).toHaveBeenCalledWith({
      terminal: { ...settings.terminal, fontSize: 16, cursorStyle: 'bar' },
      connection: settings.connection,
      notifications: settings.notifications,
      transfers: settings.transfers,
      ui: settings.ui,
    }))
  })

  it('resets through the durable settings callback', async () => {
    const reset = vi.fn(async () => settings)
    renderSettings({ onReset: reset })

    fireEvent.click(screen.getByRole('button', { name: 'Reset' }))
    await waitFor(() => expect(reset).toHaveBeenCalledOnce())
  })

  it('requests system permission only from the explicit action', async () => {
    const requestPermission = vi.fn(async () => ({ ...notificationStatus, authorized: true }))
    renderSettings({ onRequestNotificationPermission: requestPermission })

    expect(requestPermission).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Allow notifications' }))
    await waitFor(() => expect(requestPermission).toHaveBeenCalledOnce())
  })

  it('saves notification categories and the long-transfer threshold', async () => {
    const save = vi.fn(async (value: AppSettings) => value)
    renderSettings({ onSave: save })

    fireEvent.click(screen.getByLabelText('Enable notifications'))
    fireEvent.click(screen.getByLabelText('Unexpected disconnects'))
    fireEvent.change(screen.getByLabelText('Long transfer threshold'), { target: { value: '45' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(save).toHaveBeenCalledWith({
      terminal: settings.terminal,
      connection: settings.connection,
      notifications: {
        enabled: true,
        transferCompleted: true,
        unexpectedDisconnect: false,
        longTransferSeconds: 45,
      },
      transfers: settings.transfers,
      ui: settings.ui,
    }))
  })

  it('saves concurrency, collision, and partial-file preferences', async () => {
    const save = vi.fn(async (value: AppSettings) => value)
    renderSettings({ onSave: save })

    fireEvent.change(screen.getByLabelText('Concurrent transfers'), { target: { value: '4' } })
    fireEvent.change(screen.getByLabelText('Destination collisions'), { target: { value: 'rename' } })
    fireEvent.click(screen.getByLabelText('Keep partial files'))
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(save).toHaveBeenCalledWith({
      terminal: settings.terminal,
      connection: settings.connection,
      notifications: settings.notifications,
      transfers: { concurrency: 4, collisionPolicy: 'rename', keepPartialFiles: true },
      ui: settings.ui,
    }))
  })

  it('saves connection timeout and keepalive preferences', async () => {
    const save = vi.fn(async (value: AppSettings) => value)
    renderSettings({ onSave: save })

    fireEvent.change(screen.getByLabelText('Connection timeout'), { target: { value: '25' } })
    fireEvent.change(screen.getByLabelText('Keepalive interval'), { target: { value: '45' } })
    fireEvent.change(screen.getByLabelText('Keepalive failure threshold'), { target: { value: '5' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(save).toHaveBeenCalledWith({
      terminal: settings.terminal,
      connection: {
        connectTimeoutSeconds: 25,
        keepAliveEnabled: true,
        keepAliveIntervalSeconds: 45,
        keepAliveMaxFailures: 5,
      },
      notifications: settings.notifications,
      transfers: settings.transfers,
      ui: settings.ui,
    }))
  })

  it('saves a selected application theme without changing runtime UI state', async () => {
    const save = vi.fn(async (value: AppSettings) => value)
    renderSettings({ onSave: save })

    fireEvent.click(screen.getByRole('button', { name: 'Light' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(save).toHaveBeenCalledWith({
      ...settings,
      ui: { ...settings.ui, theme: 'light' },
    }))
  })

  it('shows the immutable build identity', () => {
    renderSettings()

    expect(screen.getByText('0.1.0-dev')).toBeTruthy()
    expect(screen.getByText('1234567890ab')).toBeTruthy()
    expect(screen.getByText('Modified source')).toBeTruthy()
    expect(screen.getByText('2026-07-17T15:30:00Z')).toBeTruthy()
    expect(screen.getByText('go1.26.5 · darwin/arm64')).toBeTruthy()
  })
})
