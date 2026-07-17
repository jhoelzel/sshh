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
}

afterEach(cleanup)

describe('SettingsWorkspace', () => {
  it('saves a validated terminal settings draft', async () => {
    const save = vi.fn(async (value: AppSettings) => value)
    render(<SettingsWorkspace settings={settings} onSave={save} onReset={vi.fn()} />)

    fireEvent.change(screen.getByLabelText('Font size'), { target: { value: '16' } })
    fireEvent.click(screen.getByRole('button', { name: 'Bar' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }))
    await waitFor(() => expect(save).toHaveBeenCalledWith({
      terminal: { ...settings.terminal, fontSize: 16, cursorStyle: 'bar' },
    }))
  })

  it('resets through the durable settings callback', async () => {
    const reset = vi.fn(async () => settings)
    render(<SettingsWorkspace settings={settings} onSave={vi.fn()} onReset={reset} />)

    fireEvent.click(screen.getByRole('button', { name: 'Reset' }))
    await waitFor(() => expect(reset).toHaveBeenCalledOnce())
  })
})
