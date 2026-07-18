import type { CSSProperties } from 'react'
import { terminalPaneBounds, type TerminalPaneSlot, type TerminalWorkspaceState } from './splitLayout'

const paneHeaderHeight = 27

export function terminalPaneBodyStyle(
  workspace: TerminalWorkspaceState,
  pane: TerminalPaneSlot,
): CSSProperties {
  const bounds = terminalPaneBounds(workspace, pane)
  return {
    left: `${bounds.left}%`,
    top: `calc(${bounds.top}% + ${paneHeaderHeight}px)`,
    width: `${bounds.width}%`,
    height: `calc(${bounds.height}% - ${paneHeaderHeight}px)`,
  }
}
