import type { WorkspaceLayout } from '../lib/bridge/types'

export interface DisconnectedTab {
  id: string
  profileId: string
  endpoint: string
  title: string
  state: 'disconnected'
  attention: false
  hasSelection: false
}

interface OrderedTab {
  id: string
}

export type TabDropPosition = 'before' | 'after'

interface TabCycleKey {
  altKey: boolean
  ctrlKey: boolean
  key: string
  metaKey: boolean
  repeat: boolean
  shiftKey: boolean
}

export function createDisconnectedTabs(
  layout: WorkspaceLayout,
  createID: () => string = () => crypto.randomUUID(),
): DisconnectedTab[] {
  return layout.tabs.map((tab) => ({
    id: createID(),
    profileId: tab.profileId,
    endpoint: tab.endpoint,
    title: tab.title,
    state: 'disconnected',
    attention: false,
    hasSelection: false,
  }))
}

export function reorderTabs<T extends OrderedTab>(
  tabs: T[],
  sourceId: string,
  targetId: string,
  position: TabDropPosition,
): T[] {
  if (sourceId === targetId || !tabs.some((tab) => tab.id === sourceId)) return tabs
  const remaining = tabs.filter((tab) => tab.id !== sourceId)
  const targetIndex = remaining.findIndex((tab) => tab.id === targetId)
  if (targetIndex < 0) return tabs
  const source = tabs.find((tab) => tab.id === sourceId)!
  const insertionIndex = targetIndex + (position === 'after' ? 1 : 0)
  const reordered = [...remaining]
  reordered.splice(insertionIndex, 0, source)
  return hasSameOrder(tabs, reordered) ? tabs : reordered
}

export function moveTabByOffset<T extends OrderedTab>(tabs: T[], tabId: string, offset: number): T[] {
  const sourceIndex = tabs.findIndex((tab) => tab.id === tabId)
  const targetIndex = sourceIndex + offset
  if (sourceIndex < 0 || offset === 0 || targetIndex < 0 || targetIndex >= tabs.length) return tabs
  const reordered = [...tabs]
  const [tab] = reordered.splice(sourceIndex, 1)
  reordered.splice(targetIndex, 0, tab)
  return reordered
}

export function adjacentTabId<T extends OrderedTab>(tabs: T[], activeId: string | undefined, offset: number): string | undefined {
  if (tabs.length === 0) return undefined
  const activeIndex = tabs.findIndex((tab) => tab.id === activeId)
  const start = activeIndex >= 0 ? activeIndex : 0
  const index = (start + offset % tabs.length + tabs.length) % tabs.length
  return tabs[index].id
}

export function tabCycleOffset(event: TabCycleKey): -1 | 1 | undefined {
  if (event.key !== 'Tab' || !event.ctrlKey || event.altKey || event.metaKey || event.repeat) return undefined
  return event.shiftKey ? -1 : 1
}

function hasSameOrder<T extends OrderedTab>(left: T[], right: T[]): boolean {
  return left.length === right.length && left.every((tab, index) => tab.id === right[index].id)
}
