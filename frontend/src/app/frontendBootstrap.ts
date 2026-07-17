import { backend } from '../lib/bridge/client'
import type {
  AppSettings,
  BuildInfo,
  FrontendLease,
  NotificationStatus,
  Profile,
  RemotePathFavorite,
  Snippet,
  TunnelConfig,
  WorkspaceLayout,
} from '../lib/bridge/types'

type FrontendBootstrapBackend = Pick<
  typeof backend,
  | 'attachFrontend'
  | 'listProfiles'
  | 'listTunnels'
  | 'listSnippets'
  | 'listWorkspaceLayouts'
  | 'listRemotePathFavorites'
  | 'getSettings'
  | 'getBuildInfo'
>
type NotificationStatusBackend = Pick<typeof backend, 'getNotificationStatus'>

export interface FrontendBootstrapSnapshot {
  lease: FrontendLease
  profiles: Profile[]
  tunnels: TunnelConfig[]
  snippets: Snippet[]
  workspaceLayouts: WorkspaceLayout[]
  remotePathFavorites: RemotePathFavorite[]
  settings: AppSettings
  buildInfo: BuildInfo
}

interface InFlightAttempt<T> {
  promise: Promise<T>
  token: object
}

function createNonceScopedLoader<T>(load: (nonce: string) => Promise<T>) {
  const inFlight = new Map<string, InFlightAttempt<T>>()

  return (nonce: string): Promise<T> => {
    const current = inFlight.get(nonce)
    if (current) {
      return current.promise
    }

    const token = {}
    const execute = Promise.resolve().then(() => load(nonce))
    const tracked = execute.finally(() => {
      if (inFlight.get(nonce)?.token === token) {
        inFlight.delete(nonce)
      }
    })
    inFlight.set(nonce, { promise: tracked, token })
    return tracked
  }
}

export function createFrontendBootstrap(client: FrontendBootstrapBackend = backend) {
  return createNonceScopedLoader(async (nonce): Promise<FrontendBootstrapSnapshot> => {
    const [
      lease,
      profiles,
      tunnels,
      snippets,
      workspaceLayouts,
      remotePathFavorites,
      settings,
      buildInfo,
    ] = await Promise.all([
      client.attachFrontend(nonce),
      client.listProfiles(),
      client.listTunnels(),
      client.listSnippets(),
      client.listWorkspaceLayouts(),
      client.listRemotePathFavorites(),
      client.getSettings(),
      client.getBuildInfo(),
    ])
    return {
      lease,
      profiles,
      tunnels,
      snippets,
      workspaceLayouts,
      remotePathFavorites,
      settings,
      buildInfo,
    }
  })
}

export function createNotificationStatusLoader(client: NotificationStatusBackend = backend) {
  return createNonceScopedLoader(async (): Promise<NotificationStatus> => {
    try {
      return await client.getNotificationStatus()
    } catch (cause) {
      return {
        available: false,
        authorized: false,
        message: cause instanceof Error ? cause.message : String(cause),
      }
    }
  })
}

export const loadFrontendBootstrap = createFrontendBootstrap()
export const loadFrontendNotificationStatus = createNotificationStatusLoader()
