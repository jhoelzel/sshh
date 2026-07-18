import type { WorkspaceSplit } from '../../lib/bridge/types'

export type SplitAxis = 'row' | 'column'
export type TerminalPaneSlot = 'primary' | 'secondary'

export interface TerminalWorkspaceState {
  primaryTabId?: string
  secondaryTabId?: string
  axis: SplitAxis
  ratio: number
  activePane: TerminalPaneSlot
}

export interface TerminalPaneBounds {
  left: number
  top: number
  width: number
  height: number
}

export const minimumSplitRatio = 0.2
export const maximumSplitRatio = 0.8
export const defaultSplitRatio = 0.5

export function createTerminalWorkspace(tabId?: string): TerminalWorkspaceState {
  return {
    primaryTabId: tabId,
    axis: 'row',
    ratio: defaultSplitRatio,
    activePane: 'primary',
  }
}

export function selectTerminalWorkspaceTab(
  workspace: TerminalWorkspaceState,
  tabId: string,
): TerminalWorkspaceState {
  if (workspace.primaryTabId === tabId) {
    return workspace.activePane === 'primary' ? workspace : { ...workspace, activePane: 'primary' }
  }
  if (workspace.secondaryTabId === tabId) {
    return workspace.activePane === 'secondary' ? workspace : { ...workspace, activePane: 'secondary' }
  }
  if (workspace.activePane === 'secondary' && workspace.secondaryTabId) {
    return { ...workspace, secondaryTabId: tabId }
  }
  return { ...workspace, primaryTabId: tabId, activePane: 'primary' }
}

export function splitTerminalWorkspace(
  workspace: TerminalWorkspaceState,
  axis: SplitAxis,
  candidateTabId?: string,
): TerminalWorkspaceState {
  if (workspace.secondaryTabId) {
    return workspace.axis === axis ? workspace : { ...workspace, axis }
  }
  if (!workspace.primaryTabId) {
    return candidateTabId ? { ...workspace, primaryTabId: candidateTabId, axis } : workspace
  }
  if (!candidateTabId || candidateTabId === workspace.primaryTabId) return workspace
  return {
    ...workspace,
    secondaryTabId: candidateTabId,
    axis,
    ratio: defaultSplitRatio,
    activePane: 'secondary',
  }
}

export function closeTerminalSplit(workspace: TerminalWorkspaceState): TerminalWorkspaceState {
  if (!workspace.secondaryTabId) return workspace
  const primaryTabId = workspace.activePane === 'secondary'
    ? workspace.secondaryTabId
    : workspace.primaryTabId
  return {
    ...workspace,
    primaryTabId,
    secondaryTabId: undefined,
    ratio: defaultSplitRatio,
    activePane: 'primary',
  }
}

export function resizeTerminalSplit(
  workspace: TerminalWorkspaceState,
  ratio: number,
): TerminalWorkspaceState {
  if (!workspace.secondaryTabId || !Number.isFinite(ratio)) return workspace
  const bounded = Math.min(maximumSplitRatio, Math.max(minimumSplitRatio, ratio))
  const normalized = Math.round(bounded * 1_000) / 1_000
  return workspace.ratio === normalized ? workspace : { ...workspace, ratio: normalized }
}

export function removeTerminalWorkspaceTab(
  workspace: TerminalWorkspaceState,
  tabId: string,
  remainingTabIds: string[],
  preferredTabId?: string,
): TerminalWorkspaceState {
  const fallback = preferredTabId && remainingTabIds.includes(preferredTabId)
    ? preferredTabId
    : remainingTabIds[0]
  if (workspace.primaryTabId === tabId) {
    const promoted = workspace.secondaryTabId && remainingTabIds.includes(workspace.secondaryTabId)
      ? workspace.secondaryTabId
      : fallback
    return {
      ...workspace,
      primaryTabId: promoted,
      secondaryTabId: undefined,
      ratio: defaultSplitRatio,
      activePane: 'primary',
    }
  }
  if (workspace.secondaryTabId === tabId) {
    return {
      ...workspace,
      secondaryTabId: undefined,
      ratio: defaultSplitRatio,
      activePane: 'primary',
    }
  }
  return workspace
}

export function replaceTerminalWorkspaceTab(
  workspace: TerminalWorkspaceState,
  previousTabId: string,
  nextTabId: string,
): TerminalWorkspaceState {
  if (workspace.primaryTabId === previousTabId) return { ...workspace, primaryTabId: nextTabId }
  if (workspace.secondaryTabId === previousTabId) return { ...workspace, secondaryTabId: nextTabId }
  return workspace
}

export function terminalWorkspacePane(
  workspace: TerminalWorkspaceState,
  tabId: string,
): TerminalPaneSlot | undefined {
  if (workspace.primaryTabId === tabId) return 'primary'
  if (workspace.secondaryTabId === tabId) return 'secondary'
  return undefined
}

export function terminalWorkspaceActiveTab(workspace: TerminalWorkspaceState): string | undefined {
  return workspace.activePane === 'secondary' && workspace.secondaryTabId
    ? workspace.secondaryTabId
    : workspace.primaryTabId
}

export function terminalWorkspaceVisibleTabs(workspace: TerminalWorkspaceState): string[] {
  return [workspace.primaryTabId, workspace.secondaryTabId].filter((tabId): tabId is string => Boolean(tabId))
}

export function nextTerminalSplitCandidate(
  tabIds: string[],
  workspace: TerminalWorkspaceState,
  activeTabId?: string,
): string | undefined {
  const visible = new Set(terminalWorkspaceVisibleTabs(workspace))
  const activeIndex = Math.max(0, tabIds.indexOf(activeTabId ?? ''))
  for (let offset = 1; offset <= tabIds.length; offset += 1) {
    const candidate = tabIds[(activeIndex + offset) % tabIds.length]
    if (candidate && !visible.has(candidate)) return candidate
  }
  return undefined
}

export function terminalPaneBounds(
  workspace: TerminalWorkspaceState,
  pane: TerminalPaneSlot,
): TerminalPaneBounds {
  if (!workspace.secondaryTabId) return { left: 0, top: 0, width: 100, height: 100 }
  const primary = workspace.ratio * 100
  if (workspace.axis === 'row') {
    return pane === 'primary'
      ? { left: 0, top: 0, width: primary, height: 100 }
      : { left: primary, top: 0, width: 100 - primary, height: 100 }
  }
  return pane === 'primary'
    ? { left: 0, top: 0, width: 100, height: primary }
    : { left: 0, top: primary, width: 100, height: 100 - primary }
}

export function captureTerminalWorkspaceSplit(
  workspace: TerminalWorkspaceState,
  tabIndexes: ReadonlyMap<string, number>,
  activeTabId?: string,
): WorkspaceSplit | undefined {
  if (!workspace.primaryTabId || !workspace.secondaryTabId || !activeTabId) return undefined
  const primaryTab = tabIndexes.get(workspace.primaryTabId)
  const secondaryTab = tabIndexes.get(workspace.secondaryTabId)
  const activeTab = tabIndexes.get(activeTabId)
  if (primaryTab === undefined || secondaryTab === undefined) return undefined
  if (activeTab !== primaryTab && activeTab !== secondaryTab) return undefined
  return { axis: workspace.axis, primaryTab, secondaryTab, ratio: workspace.ratio }
}

export function restoreTerminalWorkspace(
  tabIds: string[],
  activeTab: number,
  split?: WorkspaceSplit,
): TerminalWorkspaceState {
  const activeTabId = tabIds[activeTab] ?? tabIds[0]
  if (
    split &&
    (split.axis === 'row' || split.axis === 'column') &&
    split.primaryTab !== split.secondaryTab &&
    tabIds[split.primaryTab] &&
    tabIds[split.secondaryTab] &&
    Number.isFinite(split.ratio)
  ) {
    const primaryTabId = tabIds[split.primaryTab]
    const secondaryTabId = tabIds[split.secondaryTab]
    return {
      primaryTabId,
      secondaryTabId,
      axis: split.axis,
      ratio: Math.min(maximumSplitRatio, Math.max(minimumSplitRatio, split.ratio)),
      activePane: activeTabId === secondaryTabId ? 'secondary' : 'primary',
    }
  }
  return createTerminalWorkspace(activeTabId)
}
