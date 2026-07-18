import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TerminalTabs, type TerminalTabItem } from './TerminalTabs'

afterEach(cleanup)

const tabs: TerminalTabItem[] = [
  { id: 'one', title: 'One', state: 'running', attention: false },
  { id: 'two', title: 'Two', state: 'failed', attention: true },
  { id: 'three', title: 'Three', state: 'disconnected', attention: false },
]

describe('TerminalTabs', () => {
  it('uses roving tab semantics and keyboard focus navigation', () => {
    const select = vi.fn()
    render(<TerminalTabs tabs={tabs} activeId="one" onSelect={select} onClose={vi.fn()} onReorder={vi.fn()} />)

    const first = screen.getByRole('tab', { name: 'One' })
    const second = screen.getByRole('tab', { name: 'Two' })
    const third = screen.getByRole('tab', { name: 'Three' })
    expect(first).toHaveProperty('tabIndex', 0)
    expect(second).toHaveProperty('tabIndex', -1)
    expect(screen.getByRole('button', { name: 'Close One' })).toHaveProperty('tabIndex', 0)
    expect(screen.getByRole('button', { name: 'Close Two' })).toHaveProperty('tabIndex', -1)

    first.focus()
    fireEvent.keyDown(first, { key: 'ArrowRight' })
    expect(document.activeElement).toBe(second)
    fireEvent.keyDown(second, { key: 'End' })
    expect(document.activeElement).toBe(third)
    fireEvent.keyDown(third, { key: 'ArrowRight' })
    expect(document.activeElement).toBe(first)
    expect(select).not.toHaveBeenCalled()

    fireEvent.click(second)
    expect(select).toHaveBeenCalledWith('two')
  })

  it('reports accessible close and pointer reorder actions', () => {
    const close = vi.fn()
    const reorder = vi.fn()
    const data = new Map<string, string>()
    const dataTransfer = {
      dropEffect: 'none',
      effectAllowed: 'none',
      getData: (type: string) => data.get(type) ?? '',
      setData: (type: string, value: string) => data.set(type, value),
    }
    render(<TerminalTabs tabs={tabs} activeId="one" onSelect={vi.fn()} onClose={close} onReorder={reorder} />)

    fireEvent.click(screen.getByRole('button', { name: 'Close Two' }))
    expect(close).toHaveBeenCalledWith('two')

    const source = screen.getByRole('tab', { name: 'Three' })
    const target = screen.getByRole('tab', { name: 'One' }).closest('.tab') as HTMLDivElement
    vi.spyOn(target, 'getBoundingClientRect').mockReturnValue({
      bottom: 30, height: 30, left: 0, right: 100, top: 0, width: 100, x: 0, y: 0,
      toJSON: () => ({}),
    })
    fireEvent.dragStart(source, { dataTransfer })
    fireEvent.dragOver(target, { dataTransfer })
    expect(target.classList.contains('is-drop-after')).toBe(true)
    fireEvent.drop(target, { dataTransfer })

    expect(reorder).toHaveBeenCalledWith('three', 'one', 'after')
  })
})
