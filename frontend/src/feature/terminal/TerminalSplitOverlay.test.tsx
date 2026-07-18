import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { splitTerminalWorkspace, createTerminalWorkspace } from './splitLayout'
import { TerminalSplitOverlay } from './TerminalSplitOverlay'

afterEach(cleanup)

describe('TerminalSplitOverlay', () => {
  it('exposes pane focus and bounded keyboard and pointer resizing', () => {
    const activate = vi.fn()
    const resize = vi.fn()
    const workspace = splitTerminalWorkspace(createTerminalWorkspace('one'), 'row', 'two')
    const view = render(
      <TerminalSplitOverlay
        workspace={workspace}
        primary={{ tabId: 'one', title: 'One', state: 'running', attention: false }}
        secondary={{ tabId: 'two', title: 'Two', state: 'failed', attention: true }}
        activeTabId="two"
        onActivate={activate}
        onRatioChange={resize}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Focus One pane' }))
    expect(activate).toHaveBeenCalledWith('one')
    expect(screen.getByRole('button', { name: 'Focus Two pane' }).getAttribute('aria-pressed')).toBe('true')

    const separator = screen.getByRole('separator', { name: 'Resize terminal split' })
    expect(separator.getAttribute('aria-orientation')).toBe('vertical')
    expect(separator.getAttribute('aria-valuenow')).toBe('50')
    fireEvent.keyDown(separator, { key: 'ArrowRight' })
    fireEvent.keyDown(separator, { key: 'Home' })
    fireEvent.doubleClick(separator)
    expect(resize.mock.calls.map(([ratio]) => ratio)).toEqual([0.55, 0.2, 0.5])

    const overlay = separator.parentElement as HTMLDivElement
    vi.spyOn(overlay, 'getBoundingClientRect').mockReturnValue({
      bottom: 620, height: 600, left: 10, right: 1_010, top: 20, width: 1_000, x: 10, y: 20,
      toJSON: () => ({}),
    })
    fireEvent.pointerDown(separator, { pointerId: 1 })
    fireEvent.pointerMove(separator, { clientX: 710, clientY: 200, pointerId: 1 })
    fireEvent.pointerUp(separator, { pointerId: 1 })
    expect(resize).toHaveBeenLastCalledWith(0.7)

    view.rerender(
      <TerminalSplitOverlay
        workspace={{ ...workspace, axis: 'column' }}
        primary={{ tabId: 'one', title: 'One', state: 'running', attention: false }}
        secondary={{ tabId: 'two', title: 'Two', state: 'failed', attention: true }}
        activeTabId="one"
        onActivate={activate}
        onRatioChange={resize}
      />,
    )
    const horizontal = screen.getByRole('separator', { name: 'Resize terminal split' })
    expect(horizontal.getAttribute('aria-orientation')).toBe('horizontal')
    fireEvent.keyDown(horizontal, { key: 'ArrowDown', shiftKey: true })
    expect(resize).toHaveBeenLastCalledWith(0.6)
  })
})
