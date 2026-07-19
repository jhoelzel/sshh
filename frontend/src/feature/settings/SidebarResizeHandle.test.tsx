import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SidebarResizeHandle } from './SidebarResizeHandle'

afterEach(cleanup)

describe('SidebarResizeHandle', () => {
  it('previews and commits a bounded pointer resize', () => {
    const preview = vi.fn()
    const commit = vi.fn()
    render(<SidebarResizeHandle width={272} onPreview={preview} onCommit={commit} />)
    const separator = screen.getByRole('separator', { name: 'Resize sidebar' })

    fireEvent.pointerDown(separator, { pointerId: 4, clientX: 272 })
    fireEvent.pointerMove(separator, { pointerId: 4, clientX: 520 })
    fireEvent.pointerUp(separator, { pointerId: 4, clientX: 520 })

    expect(preview).toHaveBeenLastCalledWith(420)
    expect(commit).toHaveBeenCalledWith(420)
  })

  it('supports precise and bounded keyboard resizing', () => {
    const preview = vi.fn()
    const commit = vi.fn()
    const { rerender } = render(
      <SidebarResizeHandle width={272} onPreview={preview} onCommit={commit} />,
    )
    const separator = screen.getByRole('separator', { name: 'Resize sidebar' })

    fireEvent.keyDown(separator, { key: 'ArrowRight' })
    expect(commit).toHaveBeenLastCalledWith(280)
    rerender(<SidebarResizeHandle width={280} onPreview={preview} onCommit={commit} />)
    fireEvent.keyDown(separator, { key: 'ArrowLeft', shiftKey: true })
    expect(commit).toHaveBeenLastCalledWith(256)
    fireEvent.keyDown(separator, { key: 'Home' })
    expect(commit).toHaveBeenLastCalledWith(220)
    fireEvent.keyDown(separator, { key: 'End' })
    expect(commit).toHaveBeenLastCalledWith(420)
  })

  it('restores the starting width when pointer capture is cancelled', () => {
    const preview = vi.fn()
    const commit = vi.fn()
    render(<SidebarResizeHandle width={300} onPreview={preview} onCommit={commit} />)
    const separator = screen.getByRole('separator', { name: 'Resize sidebar' })

    fireEvent.pointerDown(separator, { pointerId: 7, clientX: 300 })
    fireEvent.pointerMove(separator, { pointerId: 7, clientX: 340 })
    fireEvent.pointerCancel(separator, { pointerId: 7 })

    expect(preview).toHaveBeenLastCalledWith(300)
    expect(commit).not.toHaveBeenCalled()
  })
})
