import { describe, expect, it } from 'vitest'
import type { Transfer, TunnelConfig, TunnelSnapshot } from '../../lib/bridge/types'
import {
  buildActivityTunnels,
  filterActivitySessions,
  filterActivityTransfers,
  filterActivityTunnels,
  sessionActivityGroup,
  summarizeActivity,
  transferActivityGroup,
  tunnelActivityGroup,
  type ActivitySession,
} from './activityModel'

describe('activity model', () => {
  it('classifies every resource state without treating normal completion as a problem', () => {
    expect((['starting', 'running', 'closing'] as const).map((state) => sessionActivityGroup(state))).toEqual(['active', 'active', 'active'])
    expect((['exited', 'closed', 'disconnected'] as const).map((state) => sessionActivityGroup(state))).toEqual(['idle', 'idle', 'idle'])
    expect(sessionActivityGroup('failed')).toBe('issue')

    expect((['queued', 'running'] as const).map((state) => transferActivityGroup(state))).toEqual(['active', 'active'])
    expect((['completed', 'cancelled', 'skipped'] as const).map((state) => transferActivityGroup(state))).toEqual(['idle', 'idle', 'idle'])
    expect(transferActivityGroup('failed')).toBe('issue')

    expect((['starting', 'active', 'retrying'] as const).map((state) => tunnelActivityGroup(state))).toEqual(['active', 'active', 'active'])
    expect(tunnelActivityGroup('stopped')).toBe('idle')
    expect(tunnelActivityGroup('failed')).toBe('issue')
  })

  it('filters and summarizes sessions, transfers, and tunnels consistently', () => {
    const sessions = [session('running', 'live'), session('failed', 'broken'), session('exited', 'done')]
    const transfers = [transfer('running', 'copying'), transfer('failed', 'copy-failed')]
    const tunnels = buildActivityTunnels(
      [tunnelConfig('live-tunnel', 'Live'), tunnelConfig('idle-tunnel', 'Idle')],
      [tunnelSnapshot('live-tunnel', 'active')],
    )

    expect(summarizeActivity(sessions, transfers, tunnels)).toEqual({ total: 7, active: 3, issues: 2 })
    expect(filterActivitySessions(sessions, 'issues').map((item) => item.id)).toEqual(['broken'])
    expect(filterActivityTransfers(transfers, 'active').map((item) => item.id)).toEqual(['copying'])
    expect(filterActivityTunnels(tunnels, 'active').map((item) => item.id)).toEqual(['live-tunnel'])
  })

  it('joins configured and orphaned tunnel snapshots and orders issues before live and idle rows', () => {
    const tunnels = buildActivityTunnels(
      [tunnelConfig('idle', 'Idle'), tunnelConfig('failed', 'Failed'), tunnelConfig('live', 'Live')],
      [
        tunnelSnapshot('live', 'active', '127.0.0.1:4400'),
        tunnelSnapshot('failed', 'failed'),
        tunnelSnapshot('orphaned', 'retrying'),
      ],
    )

    expect(tunnels.map((tunnel) => tunnel.id)).toEqual(['failed', 'live', 'orphaned', 'idle'])
    expect(tunnels.find((tunnel) => tunnel.id === 'live')).toMatchObject({
      requestedEndpoint: '127.0.0.1:4400', destinationEndpoint: 'db.internal:5432',
    })
    expect(tunnels.find((tunnel) => tunnel.id === 'orphaned')).toMatchObject({
      name: 'Unavailable tunnel', config: undefined,
    })
  })
})

function session(state: ActivitySession['state'], id: string): ActivitySession {
  return {
    id, title: id, endpoint: `${id}.example`, state, startedAt: '', detail: '',
    selected: false, attention: false, canRetry: state === 'failed',
  }
}

function transfer(state: Transfer['state'], id: string): Transfer {
  return {
    id, leaseId: 'lease-1', sessionId: 'files-1', direction: 'download',
    source: `/remote/${id}`, destination: `/local/${id}`, bytes: 25, total: 100,
    state, message: '', resumeId: '', resumedFrom: 0,
    startedAt: '2026-07-19T08:00:00Z', finishedAt: '',
  }
}

function tunnelConfig(id: string, name: string): TunnelConfig {
  return {
    id, name, profileId: 'profile-1', kind: 'local', bindAddress: '127.0.0.1', bindPort: 0,
    destinationHost: 'db.internal', destinationPort: 5432, autoStart: false, reconnect: false,
    createdAt: '2026-07-19T08:00:00Z', updatedAt: '2026-07-19T08:00:00Z',
  }
}

function tunnelSnapshot(
  configId: string,
  state: TunnelSnapshot['state'],
  boundAddress = '',
): TunnelSnapshot {
  return {
    configId, leaseId: 'lease-1', state, boundAddress, message: '',
    startedAt: '2026-07-19T08:00:00Z', updatedAt: '2026-07-19T08:01:00Z',
  }
}
