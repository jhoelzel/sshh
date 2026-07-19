import type {
  SessionState,
  Transfer,
  TransferState,
  TunnelConfig,
  TunnelSnapshot,
  TunnelState,
} from '../../lib/bridge/types'
import {
  tunnelDestinationEndpoint,
  tunnelRequestedEndpoint,
} from '../tunnels/tunnelPresentation'

export type ActivityFilter = 'all' | 'active' | 'issues'
export type ActivityStateGroup = 'active' | 'issue' | 'idle'
export type ActivitySessionState = SessionState | 'disconnected'

export interface ActivitySession {
  id: string
  title: string
  endpoint: string
  state: ActivitySessionState
  startedAt: string
  detail: string
  selected: boolean
  attention: boolean
  canRetry: boolean
}

export interface ActivityTunnel {
  id: string
  name: string
  profileId: string
  kind: TunnelConfig['kind'] | 'unknown'
  requestedEndpoint: string
  destinationEndpoint: string
  state: TunnelState
  boundAddress: string
  message: string
  startedAt: string
  updatedAt: string
  config?: TunnelConfig
}

export interface ActivitySummary {
  total: number
  active: number
  issues: number
}

export function sessionActivityGroup(state: ActivitySessionState): ActivityStateGroup {
  if (state === 'starting' || state === 'running' || state === 'closing') return 'active'
  return state === 'failed' ? 'issue' : 'idle'
}

export function transferActivityGroup(state: TransferState): ActivityStateGroup {
  if (state === 'queued' || state === 'running') return 'active'
  return state === 'failed' ? 'issue' : 'idle'
}

export function tunnelActivityGroup(state: TunnelState): ActivityStateGroup {
  if (state === 'starting' || state === 'active' || state === 'retrying') return 'active'
  return state === 'failed' ? 'issue' : 'idle'
}

export function matchesActivityFilter(filter: ActivityFilter, group: ActivityStateGroup): boolean {
  return filter === 'all' || filter === group || (filter === 'issues' && group === 'issue')
}

export function filterActivitySessions(
  sessions: ActivitySession[],
  filter: ActivityFilter,
): ActivitySession[] {
  return sortActivitySessions(sessions.filter((session) =>
    matchesActivityFilter(filter, sessionActivityGroup(session.state)),
  ))
}

export function filterActivityTransfers(transfers: Transfer[], filter: ActivityFilter): Transfer[] {
  return sortActivityTransfers(transfers.filter((transfer) =>
    matchesActivityFilter(filter, transferActivityGroup(transfer.state)),
  ))
}

export function filterActivityTunnels(tunnels: ActivityTunnel[], filter: ActivityFilter): ActivityTunnel[] {
  return sortActivityTunnels(tunnels.filter((tunnel) =>
    matchesActivityFilter(filter, tunnelActivityGroup(tunnel.state)),
  ))
}

export function summarizeActivity(
  sessions: ActivitySession[],
  transfers: Transfer[],
  tunnels: ActivityTunnel[],
): ActivitySummary {
  const groups = [
    ...sessions.map((session) => sessionActivityGroup(session.state)),
    ...transfers.map((transfer) => transferActivityGroup(transfer.state)),
    ...tunnels.map((tunnel) => tunnelActivityGroup(tunnel.state)),
  ]
  return {
    total: groups.length,
    active: groups.filter((group) => group === 'active').length,
    issues: groups.filter((group) => group === 'issue').length,
  }
}

export function buildActivityTunnels(
  configs: TunnelConfig[],
  snapshots: TunnelSnapshot[],
): ActivityTunnel[] {
  const snapshotsByConfig = new Map(snapshots.map((snapshot) => [snapshot.configId, snapshot]))
  const knownConfigIds = new Set(configs.map((config) => config.id))
  const configured = configs.map((config) => activityTunnel(config, snapshotsByConfig.get(config.id)))
  const orphaned = snapshots
    .filter((snapshot) => !knownConfigIds.has(snapshot.configId))
    .map((snapshot) => activityTunnel(undefined, snapshot))
  return sortActivityTunnels([...configured, ...orphaned])
}

function activityTunnel(config: TunnelConfig | undefined, snapshot: TunnelSnapshot | undefined): ActivityTunnel {
  const id = config?.id ?? snapshot?.configId ?? ''
  return {
    id,
    name: config?.name ?? 'Unavailable tunnel',
    profileId: config?.profileId ?? '',
    kind: config?.kind ?? 'unknown',
    requestedEndpoint: snapshot?.boundAddress || (config ? tunnelRequestedEndpoint(config) : ''),
    destinationEndpoint: config
      ? config.kind === 'dynamic' ? 'SOCKS5 proxy' : tunnelDestinationEndpoint(config)
      : '',
    state: snapshot?.state ?? 'stopped',
    boundAddress: snapshot?.boundAddress ?? '',
    message: snapshot?.message ?? '',
    startedAt: snapshot?.startedAt ?? '',
    updatedAt: snapshot?.updatedAt ?? config?.updatedAt ?? '',
    config,
  }
}

function sortActivitySessions(sessions: ActivitySession[]): ActivitySession[] {
  return [...sessions].sort((left, right) => {
    const group = activityGroupRank(sessionActivityGroup(left.state)) -
      activityGroupRank(sessionActivityGroup(right.state))
    if (group !== 0) return group
    if (left.selected !== right.selected) return left.selected ? -1 : 1
    const started = timestamp(right.startedAt) - timestamp(left.startedAt)
    return started || left.title.localeCompare(right.title, undefined, { sensitivity: 'base' })
  })
}

function sortActivityTransfers(transfers: Transfer[]): Transfer[] {
  return [...transfers].sort((left, right) => {
    const group = activityGroupRank(transferActivityGroup(left.state)) -
      activityGroupRank(transferActivityGroup(right.state))
    if (group !== 0) return group
    const updated = timestamp(right.finishedAt || right.startedAt) - timestamp(left.finishedAt || left.startedAt)
    return updated || left.id.localeCompare(right.id)
  })
}

function sortActivityTunnels(tunnels: ActivityTunnel[]): ActivityTunnel[] {
  return [...tunnels].sort((left, right) => {
    const group = activityGroupRank(tunnelActivityGroup(left.state)) -
      activityGroupRank(tunnelActivityGroup(right.state))
    if (group !== 0) return group
    const updated = timestamp(right.updatedAt) - timestamp(left.updatedAt)
    return updated || left.name.localeCompare(right.name, undefined, { sensitivity: 'base' })
  })
}

function activityGroupRank(group: ActivityStateGroup): number {
  if (group === 'issue') return 0
  return group === 'active' ? 1 : 2
}

function timestamp(value: string): number {
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : 0
}
