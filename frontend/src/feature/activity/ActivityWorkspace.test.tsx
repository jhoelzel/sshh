import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Profile, Transfer, TunnelConfig, TunnelSnapshot } from '../../lib/bridge/types'
import type { ActivitySession } from './activityModel'
import { ActivityWorkspace } from './ActivityWorkspace'

afterEach(cleanup)

describe('ActivityWorkspace', () => {
  it('filters global resources and routes actions through the owning workflows', async () => {
    const actions = actionDoubles()
    render(<ActivityWorkspace {...fixtures()} {...actions} />)

    expect(screen.getByText('3 active · 2 issues')).toBeTruthy()
    expect(screen.getByRole('table', { name: 'Session activity' })).toBeTruthy()
    expect(screen.getByRole('table', { name: 'Transfer activity' })).toBeTruthy()
    expect(screen.getByRole('table', { name: 'Tunnel activity' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /Issues/ }))
    expect(screen.getByText('Broken session')).toBeTruthy()
    expect(screen.getByText('transfer failed')).toBeTruthy()
    expect(screen.queryByText('Running session')).toBeNull()
    expect(screen.queryByText('Active tunnel')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /All/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Focus Broken session' }))
    fireEvent.click(screen.getByRole('button', { name: 'Retry Broken session' }))
    fireEvent.click(screen.getByRole('button', { name: 'Close Broken session from activity' }))
    fireEvent.click(screen.getByRole('button', { name: 'Open files for archive.tar' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel archive.tar' }))
    fireEvent.click(screen.getByRole('button', { name: 'Manage Active tunnel' }))
    fireEvent.click(screen.getByRole('button', { name: 'Restart Active tunnel from activity' }))
    await waitFor(() => expect(actions.onRestartTunnel).toHaveBeenCalledOnce())
    fireEvent.click(screen.getByRole('button', { name: 'Stop Active tunnel from activity' }))
    fireEvent.click(screen.getByRole('button', { name: 'Start Idle tunnel from activity' }))

    expect(actions.onOpenSession).toHaveBeenCalledWith('failed-session')
    expect(actions.onRetrySession).toHaveBeenCalledWith('failed-session')
    expect(actions.onCloseSession).toHaveBeenCalledWith('failed-session')
    expect(actions.onOpenFiles).toHaveBeenCalledOnce()
    expect(actions.onCancelTransfer).toHaveBeenCalledWith('active-transfer')
    expect(actions.onOpenTunnels).toHaveBeenCalledOnce()
    await waitFor(() => {
      expect(actions.onStopTunnel).toHaveBeenCalledWith(expect.objectContaining({ id: 'active-tunnel' }))
      expect(actions.onStartTunnel).toHaveBeenCalledWith(expect.objectContaining({ id: 'idle-tunnel' }))
    })
  })

  it('keeps backend action failures visible and dismissible', async () => {
    const actions = actionDoubles()
    actions.onCancelTransfer.mockRejectedValueOnce(new Error('Transfer cancellation failed'))
    render(<ActivityWorkspace {...fixtures()} {...actions} />)

    fireEvent.click(screen.getByRole('button', { name: 'Cancel archive.tar' }))
    expect((await screen.findByRole('alert')).textContent).toContain('Transfer cancellation failed')
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss error' }))
    expect(screen.queryByRole('alert')).toBeNull()
  })
})

function fixtures() {
  const sessions: ActivitySession[] = [
    {
      id: 'running-session', title: 'Running session', endpoint: 'local shell', state: 'running',
      startedAt: '2026-07-19T08:00:00Z', detail: '', selected: true, attention: false, canRetry: false,
    },
    {
      id: 'failed-session', title: 'Broken session', endpoint: 'prod.example:22', state: 'failed',
      startedAt: '2026-07-19T07:00:00Z', detail: 'Connection reset', selected: false,
      attention: true, canRetry: true,
    },
  ]
  const transfers: Transfer[] = [
    {
      id: 'active-transfer', leaseId: 'lease-1', sessionId: 'files-1', direction: 'download',
      source: '/remote/archive.tar', destination: '/local/archive.tar', bytes: 50, total: 100,
      state: 'running', message: '', resumeId: '', resumedFrom: 0,
      startedAt: '2026-07-19T08:00:00Z', finishedAt: '',
    },
    {
      id: 'failed-transfer', leaseId: 'lease-1', sessionId: 'files-1', direction: 'upload',
      source: '/local/report.txt', destination: '/remote/report.txt', bytes: 10, total: 100,
      state: 'failed', message: 'transfer failed', resumeId: '', resumedFrom: 0,
      startedAt: '2026-07-19T07:00:00Z', finishedAt: '2026-07-19T07:01:00Z',
    },
  ]
  const tunnelConfigs: TunnelConfig[] = [
    tunnelConfig('active-tunnel', 'Active tunnel'),
    tunnelConfig('idle-tunnel', 'Idle tunnel'),
  ]
  const tunnelSnapshots: TunnelSnapshot[] = [{
    configId: 'active-tunnel', leaseId: 'lease-1', state: 'active',
    boundAddress: '127.0.0.1:4400', message: '',
    startedAt: '2026-07-19T08:00:00Z', updatedAt: '2026-07-19T08:01:00Z',
  }]
  const profiles = [{ id: 'profile-1', name: 'Production' } as Profile]
  return { sessions, transfers, tunnelConfigs, tunnelSnapshots, profiles, fileSessionId: 'files-1', connecting: false }
}

function tunnelConfig(id: string, name: string): TunnelConfig {
  return {
    id, name, profileId: 'profile-1', kind: 'local', bindAddress: '127.0.0.1', bindPort: 0,
    destinationHost: 'db.internal', destinationPort: 5432, autoStart: false, reconnect: false,
    createdAt: '2026-07-19T08:00:00Z', updatedAt: '2026-07-19T08:00:00Z',
  }
}

function actionDoubles() {
  return {
    onOpenSession: vi.fn(),
    onRetrySession: vi.fn(),
    onCloseSession: vi.fn(),
    onCancelTransfer: vi.fn(async () => undefined),
    onOpenFiles: vi.fn(),
    onOpenTunnels: vi.fn(),
    onStartTunnel: vi.fn(async () => undefined),
    onStopTunnel: vi.fn(async () => undefined),
    onRestartTunnel: vi.fn(async () => undefined),
  }
}
