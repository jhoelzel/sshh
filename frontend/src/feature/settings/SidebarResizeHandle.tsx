import { useRef, type KeyboardEvent, type PointerEvent } from 'react'
import {
  clampSidebarWidth,
  maximumSidebarWidth,
  minimumSidebarWidth,
} from './uiPreferences'

interface SidebarResizeHandleProps {
  width: number
  onPreview: (width: number) => void
  onCommit: (width: number) => void
}

interface ActiveDrag {
  pointerId: number
  startX: number
  startWidth: number
}

export function SidebarResizeHandle({ width, onPreview, onCommit }: SidebarResizeHandleProps) {
  const activeDrag = useRef<ActiveDrag | undefined>(undefined)
  const previewWidth = useRef(width)

  const move = (event: PointerEvent<HTMLDivElement>) => {
    const active = activeDrag.current
    if (!active || active.pointerId !== event.pointerId) return
    const next = clampSidebarWidth(active.startWidth + event.clientX - active.startX)
    previewWidth.current = next
    onPreview(next)
  }

  const finish = (event: PointerEvent<HTMLDivElement>) => {
    if (activeDrag.current?.pointerId !== event.pointerId) return
    activeDrag.current = undefined
    event.currentTarget.releasePointerCapture?.(event.pointerId)
    onCommit(previewWidth.current)
  }

  const cancel = (event: PointerEvent<HTMLDivElement>) => {
    const active = activeDrag.current
    if (!active || active.pointerId !== event.pointerId) return
    activeDrag.current = undefined
    previewWidth.current = active.startWidth
    onPreview(active.startWidth)
  }

  const resizeWithKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    let next: number | undefined
    if (event.key === 'ArrowLeft') next = width - (event.shiftKey ? 24 : 8)
    if (event.key === 'ArrowRight') next = width + (event.shiftKey ? 24 : 8)
    if (event.key === 'Home') next = minimumSidebarWidth
    if (event.key === 'End') next = maximumSidebarWidth
    if (next === undefined) return
    event.preventDefault()
    const clamped = clampSidebarWidth(next)
    previewWidth.current = clamped
    onPreview(clamped)
    onCommit(clamped)
  }

  return (
    <div
      className="sidebar-resize-handle"
      role="separator"
      aria-label="Resize sidebar"
      aria-controls="application-sidebar"
      aria-orientation="vertical"
      aria-valuemin={minimumSidebarWidth}
      aria-valuemax={maximumSidebarWidth}
      aria-valuenow={width}
      tabIndex={0}
      title="Resize sidebar"
      onPointerDown={(event) => {
        activeDrag.current = { pointerId: event.pointerId, startX: event.clientX, startWidth: width }
        previewWidth.current = width
        event.currentTarget.setPointerCapture?.(event.pointerId)
      }}
      onPointerMove={move}
      onPointerUp={finish}
      onPointerCancel={cancel}
      onKeyDown={resizeWithKeyboard}
    />
  )
}
