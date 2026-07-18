import { describe, expect, it } from 'vitest'
import {
  captureTerminalWorkspaceSplit,
  closeTerminalSplit,
  createTerminalWorkspace,
  nextTerminalSplitCandidate,
  removeTerminalWorkspaceTab,
  replaceTerminalWorkspaceTab,
  resizeTerminalSplit,
  restoreTerminalWorkspace,
  selectTerminalWorkspaceTab,
  splitTerminalWorkspace,
  terminalPaneBounds,
  terminalWorkspaceActiveTab,
  terminalWorkspacePane,
  terminalWorkspaceVisibleTabs,
} from './splitLayout'

describe('terminal split layout', () => {
  it('assigns hidden tabs to the focused pane and focuses tabs already visible', () => {
    let workspace = createTerminalWorkspace('one')
    workspace = splitTerminalWorkspace(workspace, 'row', 'two')
    expect(terminalWorkspaceActiveTab(workspace)).toBe('two')
    expect(terminalWorkspaceVisibleTabs(workspace)).toEqual(['one', 'two'])

    workspace = selectTerminalWorkspaceTab(workspace, 'three')
    expect(workspace.secondaryTabId).toBe('three')
    expect(terminalWorkspaceVisibleTabs(workspace)).toEqual(['one', 'three'])

    workspace = selectTerminalWorkspaceTab(workspace, 'one')
    expect(workspace.activePane).toBe('primary')
    expect(terminalWorkspacePane(workspace, 'three')).toBe('secondary')
  })

  it('changes orientation, clamps resizing, and keeps the focused pane when closing the split', () => {
    let workspace = splitTerminalWorkspace(createTerminalWorkspace('one'), 'row', 'two')
    workspace = splitTerminalWorkspace(workspace, 'column', 'unused')
    workspace = resizeTerminalSplit(workspace, 0.95)
    expect(workspace).toMatchObject({ axis: 'column', ratio: 0.8, activePane: 'secondary' })
    expect(terminalPaneBounds(workspace, 'primary')).toEqual({ left: 0, top: 0, width: 100, height: 80 })
    expect(terminalPaneBounds(workspace, 'secondary')).toEqual({ left: 0, top: 80, width: 100, height: 20 })

    workspace = closeTerminalSplit(workspace)
    expect(workspace).toMatchObject({ primaryTabId: 'two', activePane: 'primary', ratio: 0.5 })
    expect(workspace.secondaryTabId).toBeUndefined()
  })

  it('collapses a pane deterministically when a visible tab closes or is replaced', () => {
    const split = splitTerminalWorkspace(createTerminalWorkspace('one'), 'row', 'two')
    expect(removeTerminalWorkspaceTab(split, 'one', ['two', 'three'], 'three')).toMatchObject({
      primaryTabId: 'two', activePane: 'primary',
    })
    expect(removeTerminalWorkspaceTab(split, 'two', ['one', 'three'], 'one')).toMatchObject({
      primaryTabId: 'one', activePane: 'primary',
    })
    expect(replaceTerminalWorkspaceTab(split, 'two', 'replacement').secondaryTabId).toBe('replacement')
  })

  it('chooses the next non-visible tab and round-trips saved tab indexes', () => {
    const split = splitTerminalWorkspace(createTerminalWorkspace('one'), 'row', 'three')
    expect(nextTerminalSplitCandidate(['one', 'two', 'three', 'four'], split, 'three')).toBe('four')

    const indexes = new Map([['one', 2], ['three', 0]])
    const captured = captureTerminalWorkspaceSplit(split, indexes, 'three')
    expect(captured).toEqual({ axis: 'row', primaryTab: 2, secondaryTab: 0, ratio: 0.5 })

    const restored = restoreTerminalWorkspace(['three', 'other', 'one'], 0, captured)
    expect(restored).toEqual({
      primaryTabId: 'one', secondaryTabId: 'three', axis: 'row', ratio: 0.5, activePane: 'secondary',
    })
  })
})
