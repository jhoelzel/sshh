import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { FileText, Settings2, TerminalSquare } from 'lucide-react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CommandPalette, type PaletteCommand } from './CommandPalette'

afterEach(cleanup)

function commands(run = vi.fn()): PaletteCommand[] {
  return [
    { id: 'terminal', label: 'New local terminal', group: 'Connections', icon: TerminalSquare, run },
    { id: 'logs', label: 'Start session logging', group: 'Terminal', icon: FileText, run: vi.fn(), disabled: true },
    { id: 'settings', label: 'Go to settings', group: 'Navigation', icon: Settings2, keywords: ['preferences'], run },
  ]
}

describe('CommandPalette', () => {
  it('filters commands by labels, groups, and keywords', () => {
    render(<CommandPalette commands={commands()} onClose={vi.fn()} />)

    fireEvent.change(screen.getByLabelText('Search commands'), { target: { value: 'preferences' } })

    expect(screen.getByRole('option', { name: 'Go to settings' })).toBeTruthy()
    expect(screen.queryByRole('option', { name: 'New local terminal' })).toBeNull()
  })

  it('skips disabled commands during keyboard navigation and executes the selection', () => {
    const run = vi.fn()
    const close = vi.fn()
    render(<CommandPalette commands={commands(run)} onClose={close} />)
    const dialog = screen.getByRole('dialog', { name: 'Command palette' })

    fireEvent.keyDown(dialog, { key: 'ArrowDown' })
    fireEvent.keyDown(dialog, { key: 'Enter' })

    expect(run).toHaveBeenCalledTimes(1)
    expect(close).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('option', { name: 'Start session logging' })).toHaveProperty('disabled', true)
  })

  it('closes on Escape without executing a command', () => {
    const run = vi.fn()
    const close = vi.fn()
    render(<CommandPalette commands={commands(run)} onClose={close} />)

    fireEvent.keyDown(screen.getByRole('dialog', { name: 'Command palette' }), { key: 'Escape' })

    expect(close).toHaveBeenCalledTimes(1)
    expect(run).not.toHaveBeenCalled()
  })
})
