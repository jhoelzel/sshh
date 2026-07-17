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
}

const notificationStatus = { available: true, authorized: false, message: 'Permission is required' }

const renderSettings = (overrides: Partial<React.ComponentProps<typeof SettingsWorkspace>> = {}) => render(
  <SettingsWorkspace
    settings={settings}
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
      notifications: settings.notifications,
      transfers: settings.transfers,
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
      notifications: {
        enabled: true,
        transferCompleted: true,
        unexpectedDisconnect: false,
        longTransferSeconds: 45,
      },
      transfers: settings.transfers,
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
      notifications: settings.notifications,
      transfers: { concurrency: 4, collisionPolicy: 'rename', keepPartialFiles: true },
    }))
  })
})
