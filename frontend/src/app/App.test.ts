import { describe, expect, it } from 'vitest'
import type { WorkspaceLayout } from '../lib/bridge/types'
import { createDisconnectedTabs } from './workspaces'

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
      },
      {
        id: 'tab-2', profileId: 'profile-2', title: 'Database', endpoint: 'db.example:22',
        state: 'disconnected', attention: false,
      },
    ])
    expect(tabs.every((tab) => !('session' in tab) && !('controller' in tab))).toBe(true)
  })
})
