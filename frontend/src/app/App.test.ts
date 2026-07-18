import { describe, expect, it } from 'vitest'
import type { WorkspaceLayout } from '../lib/bridge/types'
import { adjacentTabId, createDisconnectedTabs, moveTabByOffset, reorderTabs, tabCycleOffset } from './workspaces'

describe('workspace restoration', () => {
  it('creates disconnected metadata-only tabs without runtime resources', () => {
    const layout: WorkspaceLayout = {
      id: 'layout-1',
      name: 'Operations',
      tabs: [
        { profileId: 'profile-1', title: 'Production', endpoint: 'prod.example:22' },
        { profileId: 'profile-2', title: 'Database', endpoint: 'db.example:22' },
      ],
      activeTab: 1,
      createdAt: '2026-07-17T08:00:00Z',
      updatedAt: '2026-07-17T09:00:00Z',
    }
    const ids = ['tab-1', 'tab-2']

    const tabs = createDisconnectedTabs(layout, () => ids.shift()!)

    expect(tabs).toEqual([
      {
        id: 'tab-1', profileId: 'profile-1', title: 'Production', endpoint: 'prod.example:22',
        state: 'disconnected', attention: false,
        hasSelection: false,
      },
      {
        id: 'tab-2', profileId: 'profile-2', title: 'Database', endpoint: 'db.example:22',
        state: 'disconnected', attention: false,
        hasSelection: false,
      },
    ])
    expect(tabs.every((tab) => !('session' in tab) && !('controller' in tab))).toBe(true)
  })
})

describe('terminal tab ordering', () => {
  const tabs = [{ id: 'one' }, { id: 'two' }, { id: 'three' }]

  it('reorders a dropped tab before or after its target without mutating input', () => {
    expect(reorderTabs(tabs, 'three', 'one', 'before').map((tab) => tab.id)).toEqual(['three', 'one', 'two'])
    expect(reorderTabs(tabs, 'one', 'two', 'after').map((tab) => tab.id)).toEqual(['two', 'one', 'three'])
    expect(tabs.map((tab) => tab.id)).toEqual(['one', 'two', 'three'])
  })

  it('moves by bounded offsets and preserves identity for no-op moves', () => {
    expect(moveTabByOffset(tabs, 'two', -1).map((tab) => tab.id)).toEqual(['two', 'one', 'three'])
    expect(moveTabByOffset(tabs, 'two', 1).map((tab) => tab.id)).toEqual(['one', 'three', 'two'])
    expect(moveTabByOffset(tabs, 'one', -1)).toBe(tabs)
    expect(reorderTabs(tabs, 'one', 'two', 'before')).toBe(tabs)
    expect(reorderTabs(tabs, 'missing', 'two', 'after')).toBe(tabs)
  })

  it('cycles adjacent tabs in both directions', () => {
    expect(adjacentTabId(tabs, 'one', -1)).toBe('three')
    expect(adjacentTabId(tabs, 'three', 1)).toBe('one')
    expect(adjacentTabId(tabs, undefined, 1)).toBe('two')
    expect(adjacentTabId([], undefined, 1)).toBeUndefined()
  })

  it('recognizes Ctrl+Tab without stealing modified or repeated keys', () => {
    const event = { altKey: false, ctrlKey: true, key: 'Tab', metaKey: false, repeat: false, shiftKey: false }
    expect(tabCycleOffset(event)).toBe(1)
    expect(tabCycleOffset({ ...event, shiftKey: true })).toBe(-1)
    expect(tabCycleOffset({ ...event, altKey: true })).toBeUndefined()
    expect(tabCycleOffset({ ...event, metaKey: true })).toBeUndefined()
    expect(tabCycleOffset({ ...event, repeat: true })).toBeUndefined()
    expect(tabCycleOffset({ ...event, key: 't' })).toBeUndefined()
  })
})
