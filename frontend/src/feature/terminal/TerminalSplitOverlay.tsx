import { useRef, type CSSProperties, type KeyboardEvent, type PointerEvent } from 'react'
import type { TerminalPaneBounds, TerminalPaneSlot, TerminalWorkspaceState } from './splitLayout'
import {
  defaultSplitRatio,
  maximumSplitRatio,
  minimumSplitRatio,
  terminalPaneBounds,
} from './splitLayout'

export interface TerminalSplitPaneItem {
  tabId: string
  title: string
  state: string
  attention: boolean
}

interface TerminalSplitOverlayProps {
  workspace: TerminalWorkspaceState
  primary: TerminalSplitPaneItem
  secondary: TerminalSplitPaneItem
  activeTabId?: string
  onActivate: (tabId: string) => void
  onRatioChange: (ratio: number) => void
}

const paneHeaderHeight = 27

export function TerminalSplitOverlay(props: TerminalSplitOverlayProps) {
  const dragging = useRef(false)
  const verticalDivider = props.workspace.axis === 'row'

  const updateFromPointer = (event: PointerEvent<HTMLDivElement>) => {
    if (!dragging.current) return
    const bounds = event.currentTarget.parentElement?.getBoundingClientRect()
    if (!bounds || bounds.width <= 0 || bounds.height <= 0) return
    const ratio = verticalDivider
      ? (event.clientX - bounds.left) / bounds.width
      : (event.clientY - bounds.top) / bounds.height
    props.onRatioChange(ratio)
  }

  const stopDragging = (event: PointerEvent<HTMLDivElement>) => {
    dragging.current = false
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }

  const resizeWithKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    const step = event.shiftKey ? 0.1 : 0.05
    let ratio: number | undefined
    if (event.key === 'Home') ratio = minimumSplitRatio
    if (event.key === 'End') ratio = maximumSplitRatio
    if (verticalDivider && event.key === 'ArrowLeft') ratio = props.workspace.ratio - step
    if (verticalDivider && event.key === 'ArrowRight') ratio = props.workspace.ratio + step
    if (!verticalDivider && event.key === 'ArrowUp') ratio = props.workspace.ratio - step
    if (!verticalDivider && event.key === 'ArrowDown') ratio = props.workspace.ratio + step
    if (ratio === undefined) return
    event.preventDefault()
    props.onRatioChange(ratio)
  }

  return (
    <div className="terminal-split-overlay" aria-hidden="false">
      <PaneHeader
        pane="primary"
        item={props.primary}
        active={props.primary.tabId === props.activeTabId}
        workspace={props.workspace}
        onActivate={props.onActivate}
      />
      <PaneHeader
        pane="secondary"
        item={props.secondary}
        active={props.secondary.tabId === props.activeTabId}
        workspace={props.workspace}
        onActivate={props.onActivate}
      />
      <div
        className={`terminal-split-divider is-${verticalDivider ? 'vertical' : 'horizontal'}`}
        style={dividerStyle(props.workspace)}
        role="separator"
        aria-label="Resize terminal split"
        aria-orientation={verticalDivider ? 'vertical' : 'horizontal'}
        aria-valuemin={minimumSplitRatio * 100}
        aria-valuemax={maximumSplitRatio * 100}
        aria-valuenow={Math.round(props.workspace.ratio * 100)}
        tabIndex={0}
        title="Resize terminal split"
        onDoubleClick={() => props.onRatioChange(defaultSplitRatio)}
        onKeyDown={resizeWithKeyboard}
        onPointerDown={(event) => {
          dragging.current = true
          event.currentTarget.setPointerCapture?.(event.pointerId)
        }}
        onPointerMove={updateFromPointer}
        onPointerCancel={stopDragging}
        onPointerUp={stopDragging}
      />
    </div>
  )
}

interface PaneHeaderProps {
  pane: TerminalPaneSlot
  item: TerminalSplitPaneItem
  active: boolean
  workspace: TerminalWorkspaceState
  onActivate: (tabId: string) => void
}

function PaneHeader(props: PaneHeaderProps) {
  return (
    <div
      className={`terminal-pane-header${props.active ? ' is-active' : ''}`}
      style={paneStyle(terminalPaneBounds(props.workspace, props.pane))}
    >
      <button
        type="button"
        aria-label={`Focus ${props.item.title} pane`}
        aria-pressed={props.active}
        onClick={() => props.onActivate(props.item.tabId)}
      >
        <span
          className={`state-dot state-${props.item.state}${props.item.attention ? ' has-attention' : ''}`}
          aria-hidden="true"
        />
        <span>{props.item.title}</span>
      </button>
    </div>
  )
}

function paneStyle(bounds: TerminalPaneBounds): CSSProperties {
  return {
    left: `${bounds.left}%`,
    top: `${bounds.top}%`,
    width: `${bounds.width}%`,
    height: `${paneHeaderHeight}px`,
  }
}

function dividerStyle(workspace: TerminalWorkspaceState): CSSProperties {
  const position = `${workspace.ratio * 100}%`
  return workspace.axis === 'row' ? { left: position } : { top: position }
}
