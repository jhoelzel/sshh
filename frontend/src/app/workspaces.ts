import type { WorkspaceLayout } from '../lib/bridge/types'

export interface DisconnectedTab {
  id: string
  profileId: string
  endpoint: string
  title: string
  state: 'disconnected'
  attention: false
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
  }))
}
