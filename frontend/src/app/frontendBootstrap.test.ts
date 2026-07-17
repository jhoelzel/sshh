import { describe, expect, it, vi } from 'vitest'
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
import { createFrontendBootstrap, createNotificationStatusLoader } from './frontendBootstrap'

const lease: FrontendLease = { id: 'lease-1', expiresAt: '2026-07-17T23:00:00Z' }
const profiles: Profile[] = []
const tunnels: TunnelConfig[] = []
const snippets: Snippet[] = []
const workspaceLayouts: WorkspaceLayout[] = []
const remotePathFavorites: RemotePathFavorite[] = []
const settings: AppSettings = {
  terminal: {
    fontFamily: 'system-mono', fontSize: 13, lineHeight: 1.2,
    cursorStyle: 'block', cursorBlink: true, scrollback: 10_000, bell: true,
  },
  connection: {
    connectTimeoutSeconds: 15, keepAliveEnabled: true,
    keepAliveIntervalSeconds: 30, keepAliveMaxFailures: 3,
  },
  notifications: {
    enabled: false, transferCompleted: true,
    unexpectedDisconnect: true, longTransferSeconds: 30,
  },
  transfers: { concurrency: 2, collisionPolicy: 'ask', keepPartialFiles: false },
}
const buildInfo: BuildInfo = {
  version: '0.1.0-dev', commit: 'abcdef123456', buildDate: '2026-07-17T20:00:00Z',
  dirty: true, goVersion: 'go1.26.5', platform: 'darwin/arm64',
}
const notificationStatus: NotificationStatus = {
  available: true, authorized: false, message: 'Permission is required',
}

function backend() {
  return {
    attachFrontend: vi.fn(async () => lease),
    listProfiles: vi.fn(async () => profiles),
    listTunnels: vi.fn(async () => tunnels),
    listSnippets: vi.fn(async () => snippets),
    listWorkspaceLayouts: vi.fn(async () => workspaceLayouts),
    listRemotePathFavorites: vi.fn(async () => remotePathFavorites),
    getSettings: vi.fn(async () => settings),
    getBuildInfo: vi.fn(async () => buildInfo),
    getNotificationStatus: vi.fn(async () => notificationStatus),
  }
}

describe('frontend bootstrap', () => {
  it('coalesces concurrent StrictMode effect replay without caching settled data', async () => {
    const client = backend()
    const load = createFrontendBootstrap(client)

    const firstPromise = load('frontend-1')
    const secondPromise = load('frontend-1')
    expect(firstPromise).toBe(secondPromise)

    const [first, second] = await Promise.all([firstPromise, secondPromise])

    expect(first).toBe(second)
    expect(first).toMatchObject({ lease, profiles, settings })
    for (const command of [
      client.attachFrontend,
      client.listProfiles,
      client.listTunnels,
      client.listSnippets,
      client.listWorkspaceLayouts,
      client.listRemotePathFavorites,
      client.getSettings,
      client.getBuildInfo,
    ]) {
      expect(command).toHaveBeenCalledOnce()
    }
    expect(client.getNotificationStatus).not.toHaveBeenCalled()
    expect(client.attachFrontend).toHaveBeenCalledWith('frontend-1')

    const third = await load('frontend-1')
    expect(third).not.toBe(first)
    for (const command of [
      client.attachFrontend,
      client.listProfiles,
      client.listTunnels,
      client.listSnippets,
      client.listWorkspaceLayouts,
      client.listRemotePathFavorites,
      client.getSettings,
      client.getBuildInfo,
    ]) {
      expect(command).toHaveBeenCalledTimes(2)
    }
  })

  it('does not permanently cache a failed bootstrap attempt', async () => {
    const client = backend()
    client.attachFrontend
      .mockRejectedValueOnce(new Error('backend unavailable'))
      .mockResolvedValueOnce(lease)
    const load = createFrontendBootstrap(client)

    await expect(load('frontend-1')).rejects.toThrow('backend unavailable')
    await expect(load('frontend-1')).resolves.toMatchObject({ lease })

    expect(client.attachFrontend).toHaveBeenCalledTimes(2)
  })

  it('coalesces notification checks independently and makes failures nonfatal', async () => {
    const client = backend()
    client.getNotificationStatus.mockRejectedValueOnce(new Error('notifications unavailable'))
    const load = createNotificationStatusLoader(client)

    const firstPromise = load('frontend-1')
    const secondPromise = load('frontend-1')
    expect(firstPromise).toBe(secondPromise)

    await expect(firstPromise).resolves.toEqual({
      available: false,
      authorized: false,
      message: 'notifications unavailable',
    })
    await expect(secondPromise).resolves.toEqual({
      available: false,
      authorized: false,
      message: 'notifications unavailable',
    })
    expect(client.getNotificationStatus).toHaveBeenCalledOnce()

    await expect(load('frontend-1')).resolves.toBe(notificationStatus)
    expect(client.getNotificationStatus).toHaveBeenCalledTimes(2)
  })
})
