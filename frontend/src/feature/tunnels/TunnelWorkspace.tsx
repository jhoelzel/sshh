import { useMemo, useState, type FormEvent } from 'react'
import {
  ArrowRight,
  CircleAlert,
  LoaderCircle,
  Network,
  Pencil,
  Play,
  Plus,
  RotateCw,
  ShieldAlert,
  Square,
  Trash2,
  X,
} from 'lucide-react'
import type {
  Profile,
  TunnelConfig,
  TunnelInput,
  TunnelKind,
  TunnelSnapshot,
  TunnelState,
} from '../../lib/bridge/types'

interface TunnelWorkspaceProps {
  configs: TunnelConfig[]
  profiles: Profile[]
  snapshots: TunnelSnapshot[]
  connecting: boolean
  onCreate: (input: TunnelInput) => Promise<void>
  onUpdate: (input: TunnelInput) => Promise<void>
  onDelete: (id: string) => Promise<void>
  onStart: (config: TunnelConfig) => Promise<void>
  onStop: (config: TunnelConfig) => Promise<void>
  onRestart: (config: TunnelConfig) => Promise<void>
}

export function TunnelWorkspace(props: TunnelWorkspaceProps) {
  const [editor, setEditor] = useState<TunnelConfig | null>()
  const [deleting, setDeleting] = useState<TunnelConfig>()
  const [busyId, setBusyId] = useState<string>()
  const [error, setError] = useState<string>()
  const snapshots = useMemo(
    () => new Map(props.snapshots.map((snapshot) => [snapshot.configId, snapshot])),
    [props.snapshots],
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
    <section className="tunnel-workspace" aria-label="SSH tunnels">
      <header className="tunnel-header">
        <div className="tunnel-title">
          <Network size={18} />
          <div>
            <strong>SSH tunnels</strong>
            <span>{props.configs.length} saved</span>
          </div>
        </div>
        <button className="primary-button" type="button" disabled={props.profiles.length === 0} onClick={() => setEditor(null)}>
          <Plus size={16} /> New tunnel
        </button>
      </header>

      {error && (
        <div className="tunnel-error" role="alert">
          <CircleAlert size={16} /> <span>{error}</span>
          <button className="icon-button compact" type="button" aria-label="Dismiss error" onClick={() => setError(undefined)}><X size={14} /></button>
        </div>
      )}

      <div className="tunnel-table" role="table" aria-label="Saved SSH tunnels">
        <div className="tunnel-table-header" role="row">
          <span role="columnheader">Tunnel</span>
          <span role="columnheader">Route</span>
          <span role="columnheader">Status</span>
          <span role="columnheader">Actions</span>
        </div>
        <div className="tunnel-table-body">
          {props.configs.length === 0 ? (
            <div className="tunnel-empty">
              <Network size={30} strokeWidth={1.4} />
              <h1>No tunnels saved</h1>
            </div>
          ) : props.configs.map((config) => {
            const snapshot = snapshots.get(config.id)
            const state = snapshot?.state ?? 'stopped'
            const live = isLive(state)
            const busy = busyId === config.id || props.connecting
            const profile = props.profiles.find((item) => item.id === config.profileId)
            return (
              <div className="tunnel-row" role="row" key={config.id}>
                <span className="tunnel-name-cell" role="cell">
                  <i className={`tunnel-state-dot state-${state}`} />
                  <span>
                    <strong>{config.name}</strong>
                    <small>{kindLabel(config.kind)} · {profile?.name ?? 'Missing profile'}</small>
                  </span>
                </span>
                <span className="tunnel-route" role="cell">
                  <span>{requestedEndpoint(config)}</span>
                  {config.kind !== 'dynamic' && <><ArrowRight size={13} /><span>{destinationEndpoint(config)}</span></>}
                </span>
                <span className="tunnel-status" role="cell">
                  <strong>{stateLabel(state)}</strong>
                  <small>{snapshot?.boundAddress || (config.autoStart ? 'Auto-start' : 'Manual')}</small>
                  {snapshot?.message && <span className="tunnel-message" title={snapshot.message}><CircleAlert size={12} /> {snapshot.message}</span>}
                </span>
                <span className="tunnel-actions" role="cell">
                  {busy ? <LoaderCircle className="spin" size={16} /> : live ? (
                    <>
                      <button className="icon-button compact" type="button" title="Restart tunnel" aria-label={`Restart ${config.name}`} onClick={() => void run(config.id, () => props.onRestart(config))}><RotateCw size={15} /></button>
                      <button className="icon-button compact" type="button" title="Stop tunnel" aria-label={`Stop ${config.name}`} onClick={() => void run(config.id, () => props.onStop(config))}><Square size={14} /></button>
                    </>
                  ) : (
                    <button className="icon-button compact" type="button" title="Start tunnel" aria-label={`Start ${config.name}`} disabled={!profile} onClick={() => void run(config.id, () => props.onStart(config))}><Play size={15} /></button>
                  )}
                  <button className="icon-button compact" type="button" title="Edit tunnel" aria-label={`Edit ${config.name}`} disabled={live || busy} onClick={() => setEditor(config)}><Pencil size={14} /></button>
                  <button className="icon-button compact danger-quiet" type="button" title="Delete tunnel" aria-label={`Delete ${config.name}`} disabled={live || busy} onClick={() => setDeleting(config)}><Trash2 size={14} /></button>
                </span>
              </div>
            )
          })}
        </div>
      </div>

      {editor !== undefined && (
        <TunnelEditor
          config={editor ?? undefined}
          profiles={props.profiles}
          onCancel={() => setEditor(undefined)}
          onSave={async (input) => {
            if (input.id) {
              await props.onUpdate(input)
            } else {
              await props.onCreate(input)
            }
            setEditor(undefined)
          }}
        />
      )}

      {deleting && (
        <div className="modal-backdrop" role="presentation">
          <section className="dialog" role="dialog" aria-modal="true" aria-labelledby="delete-tunnel-title">
            <div className="dialog-icon"><CircleAlert size={20} /></div>
            <div className="dialog-copy">
              <h2 id="delete-tunnel-title">Delete this tunnel?</h2>
              <p>{deleting.name} will be removed from the saved tunnel list.</p>
            </div>
            <div className="dialog-actions">
              <button className="secondary-button" type="button" onClick={() => setDeleting(undefined)}>Cancel</button>
              <button className="danger-button" type="button" onClick={() => void run(deleting.id, async () => { await props.onDelete(deleting.id); setDeleting(undefined) })}>Delete</button>
            </div>
          </section>
        </div>
      )}
    </section>
  )
}

interface TunnelEditorProps {
  config?: TunnelConfig
  profiles: Profile[]
  onCancel: () => void
  onSave: (input: TunnelInput) => Promise<void>
}

function TunnelEditor({ config, profiles, onCancel, onSave }: TunnelEditorProps) {
  const [draft, setDraft] = useState<TunnelInput>(() => config ? configToInput(config) : emptyTunnel(profiles[0]?.id ?? ''))
  const [confirmPublicBind, setConfirmPublicBind] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const publicBind = bindsAllInterfaces(draft.bindAddress)

  const setField = <Key extends keyof TunnelInput>(key: Key, value: TunnelInput[Key]) => {
    setDraft((current) => ({ ...current, [key]: value }))
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(undefined)
    try {
      await onSave(draft.kind === 'dynamic' ? { ...draft, destinationHost: '', destinationPort: 0 } : draft)
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <form className="profile-dialog tunnel-dialog" role="dialog" aria-modal="true" aria-labelledby="tunnel-editor-title" onSubmit={(event) => void submit(event)}>
        <header className="profile-dialog-header">
          <div>
            <h2 id="tunnel-editor-title">{config ? 'Edit tunnel' : 'New tunnel'}</h2>
            <p>SSH port forwarding</p>
          </div>
          <button className="icon-button" type="button" title="Close" aria-label="Close tunnel editor" onClick={onCancel}><X size={16} /></button>
        </header>
        <div className="profile-form-scroll">
          <div className="segmented-control" aria-label="Tunnel type">
            {(['local', 'remote', 'dynamic'] as TunnelKind[]).map((kind) => (
              <button className={draft.kind === kind ? 'is-selected' : ''} type="button" key={kind} onClick={() => setField('kind', kind)}>{kindLabel(kind)}</button>
            ))}
          </div>
          <div className="form-grid two-columns">
            <label className="field field-wide">
              <span>Name</span>
              <input autoFocus required maxLength={120} value={draft.name} onChange={(event) => setField('name', event.target.value)} />
            </label>
            <label className="field field-wide">
              <span>SSH profile</span>
              <select required value={draft.profileId} onChange={(event) => setField('profileId', event.target.value)}>
                {profiles.map((profile) => <option value={profile.id} key={profile.id}>{profile.name} · {profile.endpoint}</option>)}
              </select>
            </label>
            <label className="field">
              <span>Bind address</span>
              <input required value={draft.bindAddress} onChange={(event) => { setField('bindAddress', event.target.value); setConfirmPublicBind(false) }} />
            </label>
            <label className="field">
              <span>Bind port</span>
              <input required type="number" min={0} max={65535} value={draft.bindPort} onChange={(event) => setField('bindPort', Number(event.target.value))} />
            </label>
            {draft.kind !== 'dynamic' && (
              <>
                <label className="field">
                  <span>Destination host</span>
                  <input required value={draft.destinationHost} onChange={(event) => setField('destinationHost', event.target.value)} />
                </label>
                <label className="field">
                  <span>Destination port</span>
                  <input required type="number" min={1} max={65535} value={draft.destinationPort} onChange={(event) => setField('destinationPort', Number(event.target.value))} />
                </label>
              </>
            )}
          </div>
          <div className="tunnel-policy-options">
            <label><input type="checkbox" checked={draft.autoStart} onChange={(event) => setField('autoStart', event.target.checked)} /> Auto-start</label>
            <label><input type="checkbox" checked={draft.reconnect} onChange={(event) => setField('reconnect', event.target.checked)} /> Reconnect</label>
          </div>
          {publicBind && (
            <label className="public-bind-warning">
              <ShieldAlert size={17} />
              <input type="checkbox" checked={confirmPublicBind} onChange={(event) => setConfirmPublicBind(event.target.checked)} />
              <span>Allow connections from every network interface</span>
            </label>
          )}
          {error && <div className="form-error" role="alert">{error}</div>}
        </div>
        <footer className="profile-dialog-actions">
          <span />
          <button className="secondary-button" type="button" disabled={busy} onClick={onCancel}>Cancel</button>
          <button className="primary-button" type="submit" disabled={busy || (publicBind && !confirmPublicBind)}>{busy ? 'Saving' : 'Save tunnel'}</button>
        </footer>
      </form>
    </div>
  )
}

function emptyTunnel(profileId: string): TunnelInput {
  return {
    id: '', name: '', profileId, kind: 'local', bindAddress: '127.0.0.1', bindPort: 0,
    destinationHost: '127.0.0.1', destinationPort: 0, autoStart: false, reconnect: false,
  }
}

function configToInput(config: TunnelConfig): TunnelInput {
  return {
    id: config.id,
    name: config.name,
    profileId: config.profileId,
    kind: config.kind,
    bindAddress: config.bindAddress,
    bindPort: config.bindPort,
    destinationHost: config.destinationHost,
    destinationPort: config.destinationPort,
    autoStart: config.autoStart,
    reconnect: config.reconnect,
  }
}

function kindLabel(kind: TunnelKind): string {
  if (kind === 'dynamic') return 'SOCKS5'
  return kind.charAt(0).toUpperCase() + kind.slice(1)
}

function requestedEndpoint(config: TunnelConfig): string {
  return `${config.bindAddress}:${config.bindPort}`
}

function destinationEndpoint(config: TunnelConfig): string {
  return `${config.destinationHost}:${config.destinationPort}`
}

function stateLabel(state: TunnelState): string {
  return state.charAt(0).toUpperCase() + state.slice(1)
}

function isLive(state: TunnelState): boolean {
  return state === 'starting' || state === 'active' || state === 'retrying'
}

function bindsAllInterfaces(address: string): boolean {
  const normalized = address.trim().replace(/^\[|\]$/g, '')
  return normalized === '0.0.0.0' || normalized === '::'
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}
