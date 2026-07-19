import { useMemo, useState, type ReactNode } from 'react'
import {
  Activity as ActivityIcon,
  ArrowDownToLine,
  ArrowRight,
  ArrowUpFromLine,
  ArrowUpRight,
  CircleAlert,
  CircleX,
  FolderOpen,
  Network,
  Play,
  RotateCw,
  Square,
  TerminalSquare,
  X,
} from 'lucide-react'
import type { Profile, Transfer, TunnelConfig, TunnelSnapshot } from '../../lib/bridge/types'
import {
  formatBytes,
  pathBaseName,
  transferProgressPercent,
} from '../files/transferPresentation'
import {
  isLiveTunnelState,
  tunnelKindLabel,
} from '../tunnels/tunnelPresentation'
import {
  buildActivityTunnels,
  filterActivitySessions,
  filterActivityTransfers,
  filterActivityTunnels,
  summarizeActivity,
  type ActivityFilter,
  type ActivitySession,
} from './activityModel'

interface ActivityWorkspaceProps {
  sessions: ActivitySession[]
  transfers: Transfer[]
  tunnelConfigs: TunnelConfig[]
  tunnelSnapshots: TunnelSnapshot[]
  profiles: Profile[]
  fileSessionId?: string
  connecting: boolean
  onOpenSession: (sessionId: string) => void
  onRetrySession: (sessionId: string) => void
  onCloseSession: (sessionId: string) => void
  onCancelTransfer: (transferId: string) => Promise<void>
  onOpenFiles: () => void
  onOpenTunnels: () => void
  onStartTunnel: (config: TunnelConfig) => Promise<void>
  onStopTunnel: (config: TunnelConfig) => Promise<void>
  onRestartTunnel: (config: TunnelConfig) => Promise<void>
}

export function ActivityWorkspace(props: ActivityWorkspaceProps) {
  const [filter, setFilter] = useState<ActivityFilter>('all')
  const [busyId, setBusyId] = useState<string>()
  const [error, setError] = useState<string>()
  const tunnels = useMemo(
    () => buildActivityTunnels(props.tunnelConfigs, props.tunnelSnapshots),
    [props.tunnelConfigs, props.tunnelSnapshots],
  )
  const summary = useMemo(
    () => summarizeActivity(props.sessions, props.transfers, tunnels),
    [props.sessions, props.transfers, tunnels],
  )
  const visibleSessions = useMemo(
    () => filterActivitySessions(props.sessions, filter),
    [filter, props.sessions],
  )
  const visibleTransfers = useMemo(
    () => filterActivityTransfers(props.transfers, filter),
    [filter, props.transfers],
  )
  const visibleTunnels = useMemo(
    () => filterActivityTunnels(tunnels, filter),
    [filter, tunnels],
  )
  const profileNames = useMemo(
    () => new Map(props.profiles.map((profile) => [profile.id, profile.name])),
    [props.profiles],
  )

  const run = async (id: string, operation: () => Promise<void>) => {
    setBusyId(id)
    setError(undefined)
    try {
      await operation()
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setBusyId(undefined)
    }
  }

  return (
    <section className="activity-workspace" aria-label="Workspace activity">
      <header className="activity-header">
        <div className="activity-title">
          <ActivityIcon size={18} />
          <div>
            <strong>Activity</strong>
            <span>{summary.active} active · {summary.issues} issue{summary.issues === 1 ? '' : 's'}</span>
          </div>
        </div>
        <div className="activity-filters" role="group" aria-label="Filter activity">
          <ActivityFilterButton filter="all" selected={filter === 'all'} count={summary.total} onSelect={setFilter} />
          <ActivityFilterButton filter="active" selected={filter === 'active'} count={summary.active} onSelect={setFilter} />
          <ActivityFilterButton filter="issues" selected={filter === 'issues'} count={summary.issues} onSelect={setFilter} />
        </div>
      </header>

      {error && (
        <div className="activity-error" role="alert">
          <CircleAlert size={16} /><span>{error}</span>
          <button className="icon-button compact" type="button" aria-label="Dismiss error" onClick={() => setError(undefined)}><X size={14} /></button>
        </div>
      )}

      <div className="activity-scroll">
        <ActivitySection title="Sessions" count={visibleSessions.length}>
          <div className="activity-table activity-session-table" role="table" aria-label="Session activity">
            <ActivityTableHeader labels={['Session', 'Endpoint', 'Status', 'Started', 'Actions']} />
            <div className="activity-table-body">
              {visibleSessions.length === 0 ? <ActivityEmpty label="No matching sessions" /> : visibleSessions.map((session) => (
                <div className="activity-row" role="row" key={session.id}>
                  <span className="activity-primary-cell" role="cell">
                    <TerminalSquare size={15} />
                    <span><strong>{session.title}</strong><small>{session.selected ? 'Focused pane' : 'Terminal session'}</small></span>
                  </span>
                  <span className="activity-value" role="cell" title={session.endpoint}>{session.endpoint || 'Local shell'}</span>
                  <ActivityStatus state={session.state} detail={session.detail} attention={session.attention} />
                  <span className="activity-time" role="cell">{formatTimestamp(session.startedAt)}</span>
                  <span className="activity-actions" role="cell">
                    <button className="icon-button compact" type="button" title="Focus terminal" aria-label={`Focus ${session.title}`} onClick={() => props.onOpenSession(session.id)}><ArrowUpRight size={14} /></button>
                    {session.canRetry && <button className="icon-button compact" type="button" title="Retry terminal" aria-label={`Retry ${session.title}`} onClick={() => props.onRetrySession(session.id)}><RotateCw size={14} /></button>}
                    <button className="icon-button compact danger-quiet" type="button" title="Close terminal" aria-label={`Close ${session.title} from activity`} onClick={() => props.onCloseSession(session.id)}><X size={14} /></button>
                  </span>
                </div>
              ))}
            </div>
          </div>
        </ActivitySection>

        <ActivitySection title="Transfers" count={visibleTransfers.length}>
          <div className="activity-table activity-transfer-table" role="table" aria-label="Transfer activity">
            <ActivityTableHeader labels={['Transfer', 'Path', 'Progress', 'Status', 'Actions']} />
            <div className="activity-table-body">
              {visibleTransfers.length === 0 ? <ActivityEmpty label="No matching transfers" /> : visibleTransfers.map((transfer) => {
                const active = transfer.state === 'queued' || transfer.state === 'running'
                const progress = transferProgressPercent(transfer)
                const displayPath = transfer.direction === 'download' ? transfer.source : transfer.destination
                return (
                  <div className="activity-row" role="row" key={transfer.id}>
                    <span className="activity-primary-cell" role="cell">
                      {transfer.direction === 'download' ? <ArrowDownToLine size={15} /> : <ArrowUpFromLine size={15} />}
                      <span><strong>{pathBaseName(displayPath)}</strong><small>{transfer.direction === 'download' ? 'Download' : 'Upload'}</small></span>
                    </span>
                    <span className="activity-route" role="cell" title={`${transfer.source} → ${transfer.destination}`}>
                      <span>{transfer.source}</span><ArrowRight size={12} /><span>{transfer.destination}</span>
                    </span>
                    <span className="activity-progress-cell" role="cell">
                      <span>{formatBytes(transfer.bytes)} / {formatBytes(transfer.total)}</span>
                      <span className="activity-progress"><i style={{ width: `${progress}%` }} /></span>
                    </span>
                    <ActivityStatus state={transfer.state} detail={transfer.message} />
                    <span className="activity-actions" role="cell">
                      {props.fileSessionId === transfer.sessionId && <button className="icon-button compact" type="button" title="Open files" aria-label={`Open files for ${pathBaseName(displayPath)}`} onClick={props.onOpenFiles}><FolderOpen size={14} /></button>}
                      {active && <button className="icon-button compact danger-quiet" type="button" title="Cancel transfer" aria-label={`Cancel ${pathBaseName(displayPath)}`} disabled={busyId === `transfer:${transfer.id}`} onClick={() => void run(`transfer:${transfer.id}`, () => props.onCancelTransfer(transfer.id))}><CircleX size={14} /></button>}
                    </span>
                  </div>
                )
              })}
            </div>
          </div>
        </ActivitySection>

        <ActivitySection title="Tunnels" count={visibleTunnels.length}>
          <div className="activity-table activity-tunnel-table" role="table" aria-label="Tunnel activity">
            <ActivityTableHeader labels={['Tunnel', 'Route', 'Status', 'Updated', 'Actions']} />
            <div className="activity-table-body">
              {visibleTunnels.length === 0 ? <ActivityEmpty label="No matching tunnels" /> : visibleTunnels.map((tunnel) => {
                const live = isLiveTunnelState(tunnel.state)
                const busy = busyId === `tunnel:${tunnel.id}`
                return (
                  <div className="activity-row" role="row" key={tunnel.id}>
                    <span className="activity-primary-cell" role="cell">
                      <Network size={15} />
                      <span><strong>{tunnel.name}</strong><small>{tunnel.kind === 'unknown' ? 'Unavailable configuration' : `${tunnelKindLabel(tunnel.kind)} · ${profileNames.get(tunnel.profileId) ?? 'Missing profile'}`}</small></span>
                    </span>
                    <span className="activity-route" role="cell">
                      <span>{tunnel.requestedEndpoint || 'Unavailable'}</span>
                      {tunnel.destinationEndpoint && <><ArrowRight size={12} /><span>{tunnel.destinationEndpoint}</span></>}
                    </span>
                    <ActivityStatus state={tunnel.state} detail={tunnel.message || tunnel.boundAddress} />
                    <span className="activity-time" role="cell">{formatTimestamp(tunnel.updatedAt)}</span>
                    <span className="activity-actions" role="cell">
                      <button className="icon-button compact" type="button" title="Manage tunnels" aria-label={`Manage ${tunnel.name}`} onClick={props.onOpenTunnels}><ArrowUpRight size={14} /></button>
                      {tunnel.config && (live ? (
                        <>
                          <button className="icon-button compact" type="button" title="Restart tunnel" aria-label={`Restart ${tunnel.name} from activity`} disabled={busy || props.connecting} onClick={() => void run(`tunnel:${tunnel.id}`, () => props.onRestartTunnel(tunnel.config!))}><RotateCw size={14} /></button>
                          <button className="icon-button compact danger-quiet" type="button" title="Stop tunnel" aria-label={`Stop ${tunnel.name} from activity`} disabled={busy} onClick={() => void run(`tunnel:${tunnel.id}`, () => props.onStopTunnel(tunnel.config!))}><Square size={13} /></button>
                        </>
                      ) : (
                        <button className="icon-button compact" type="button" title="Start tunnel" aria-label={`Start ${tunnel.name} from activity`} disabled={busy || props.connecting || !profileNames.has(tunnel.profileId)} onClick={() => void run(`tunnel:${tunnel.id}`, () => props.onStartTunnel(tunnel.config!))}><Play size={14} /></button>
                      ))}
                    </span>
                  </div>
                )
              })}
            </div>
          </div>
        </ActivitySection>
      </div>
    </section>
  )
}

interface ActivityFilterButtonProps {
  filter: ActivityFilter
  selected: boolean
  count: number
  onSelect: (filter: ActivityFilter) => void
}

function ActivityFilterButton(props: ActivityFilterButtonProps) {
  const label = props.filter === 'all' ? 'All' : props.filter === 'active' ? 'Active' : 'Issues'
  return <button className={props.selected ? 'is-selected' : ''} type="button" aria-pressed={props.selected} onClick={() => props.onSelect(props.filter)}>{label}<span>{props.count}</span></button>
}

function ActivitySection(props: { title: string; count: number; children: ReactNode }) {
  return (
    <section className="activity-section" aria-labelledby={`activity-${props.title.toLowerCase()}`}>
      <header><h2 id={`activity-${props.title.toLowerCase()}`}>{props.title}</h2><span>{props.count}</span></header>
      {props.children}
    </section>
  )
}

function ActivityTableHeader(props: { labels: string[] }) {
  return <div className="activity-table-header" role="row">{props.labels.map((label) => <span role="columnheader" key={label}>{label}</span>)}</div>
}

function ActivityEmpty(props: { label: string }) {
  return <div className="activity-empty">{props.label}</div>
}

function ActivityStatus(props: { state: string; detail: string; attention?: boolean }) {
  return (
    <span className="activity-status" role="cell">
      <span><i className={`activity-state-dot state-${props.state}${props.attention ? ' has-attention' : ''}`} />{stateLabel(props.state)}</span>
      {props.detail && <small title={props.detail}>{props.detail}</small>}
    </span>
  )
}

function stateLabel(state: string): string {
  return state.charAt(0).toUpperCase() + state.slice(1)
}

function formatTimestamp(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? '—' : new Intl.DateTimeFormat(undefined, {
    dateStyle: 'short', timeStyle: 'short',
  }).format(date)
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}
