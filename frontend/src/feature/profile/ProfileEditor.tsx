import { useMemo, useRef, useState, type FormEvent } from 'react'
import { Copy, KeyRound, Laptop, Server, Star, Trash2, X } from 'lucide-react'
import type { Profile, ProfileInput, SSHAuthentication } from '../../lib/bridge/types'
import { ProfileEnvironmentEditor } from './ProfileEnvironmentEditor'
import {
  profileEnvironmentEntries,
  validateProfileEnvironment,
} from './profileEnvironment'

interface ProfileEditorProps {
  profile?: Profile
  onCancel: () => void
  onSave: (profile: ProfileInput) => Promise<void>
  onDuplicate?: () => Promise<void>
  onDelete?: () => void
}

const emptyProfile: ProfileInput = {
  id: '',
  name: '',
  protocol: 'ssh',
  host: '',
  port: 22,
  username: '',
  authentication: 'auto',
  identityFile: '',
  shell: '',
  arguments: [],
  workingDirectory: '',
  environment: {},
  tags: [],
  group: '',
  favorite: false,
}

export function ProfileEditor({ profile, onCancel, onSave, onDuplicate, onDelete }: ProfileEditorProps) {
  const initial = useMemo<ProfileInput>(() => profileToInput(profile), [profile])
  const [draft, setDraft] = useState<ProfileInput>(initial)
  const [argumentsText, setArgumentsText] = useState(initial.arguments.join('\n'))
  const [tagsText, setTagsText] = useState(initial.tags.join(', '))
  const [environmentEntries, setEnvironmentEntries] = useState(() => profileEnvironmentEntries(initial.environment))
  const nextEnvironmentEntryID = useRef(environmentEntries.length)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const setField = <Key extends keyof ProfileInput>(key: Key, value: ProfileInput[Key]) => {
    setDraft((current) => ({ ...current, [key]: value }))
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError(undefined)
    const environment = draft.protocol === 'local'
      ? validateProfileEnvironment(environmentEntries)
      : { ok: true as const, environment: {} }
    if (!environment.ok) {
      setError(environment.error)
      return
    }

    setBusy(true)
    try {
      await onSave({
        ...draft,
        arguments: splitLines(argumentsText),
        environment: environment.environment,
        tags: tagsText.split(',').map((tag) => tag.trim()).filter(Boolean),
      })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  const addEnvironmentEntry = () => {
    const id = `added:${nextEnvironmentEntryID.current}`
    nextEnvironmentEntryID.current++
    setEnvironmentEntries((current) => [...current, { id, name: '', value: '' }])
    setError(undefined)
  }

  const updateEnvironmentEntry = (id: string, field: 'name' | 'value', value: string) => {
    setEnvironmentEntries((current) => current.map((entry) => entry.id === id ? { ...entry, [field]: value } : entry))
    setError(undefined)
  }

  const removeEnvironmentEntry = (id: string) => {
    setEnvironmentEntries((current) => current.filter((entry) => entry.id !== id))
    setError(undefined)
  }

  const duplicate = async () => {
    if (!onDuplicate) {
      return
    }
    setBusy(true)
    setError(undefined)
    try {
      await onDuplicate()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <form className="profile-dialog" role="dialog" aria-modal="true" aria-labelledby="profile-title" onSubmit={(event) => void submit(event)}>
        <header className="profile-dialog-header">
          <div>
            <h2 id="profile-title">{profile ? 'Edit profile' : 'New profile'}</h2>
            <p>{draft.protocol === 'local' ? 'Local pseudoterminal' : 'Secure Shell connection'}</p>
          </div>
          <button className="icon-button" type="button" aria-label="Close profile editor" title="Close" onClick={onCancel}>
            <X size={16} />
          </button>
        </header>

        <div className="profile-form-scroll">
          <div className="segmented-control" aria-label="Profile protocol">
            <button className={draft.protocol === 'ssh' ? 'is-selected' : ''} type="button" onClick={() => setField('protocol', 'ssh')}>
              <Server size={15} /> SSH
            </button>
            <button className={draft.protocol === 'local' ? 'is-selected' : ''} type="button" onClick={() => setField('protocol', 'local')}>
              <Laptop size={15} /> Local
            </button>
          </div>

          <div className="form-grid two-columns">
            <label className="field field-wide">
              <span>Name</span>
              <input autoFocus required maxLength={120} value={draft.name} onChange={(event) => setField('name', event.target.value)} />
            </label>
            <label className="field">
              <span>Group</span>
              <input maxLength={80} placeholder="Optional" value={draft.group} onChange={(event) => setField('group', event.target.value)} />
            </label>
            <label className="field checkbox-field">
              <input type="checkbox" checked={draft.favorite} onChange={(event) => setField('favorite', event.target.checked)} />
              <Star size={15} fill={draft.favorite ? 'currentColor' : 'none'} />
              <span>Favorite</span>
            </label>
          </div>

          {draft.protocol === 'ssh' ? (
            <SSHFields draft={draft} setField={setField} />
          ) : (
            <>
              <LocalFields draft={draft} argumentsText={argumentsText} setArgumentsText={setArgumentsText} setField={setField} />
              <ProfileEnvironmentEditor
                entries={environmentEntries}
                onAdd={addEnvironmentEntry}
                onChange={updateEnvironmentEntry}
                onRemove={removeEnvironmentEntry}
              />
            </>
          )}

          <label className="field">
            <span>Tags</span>
            <input placeholder="production, database" value={tagsText} onChange={(event) => setTagsText(event.target.value)} />
          </label>

          {error && <div className="form-error" role="alert">{error}</div>}
        </div>

        <footer className="profile-dialog-actions">
          <div className="profile-destructive-actions">
            {onDelete && (
              <button className="icon-text-button danger-quiet" type="button" disabled={busy} onClick={onDelete}>
                <Trash2 size={15} /> Delete
              </button>
            )}
            {onDuplicate && (
              <button className="icon-text-button" type="button" disabled={busy} onClick={() => void duplicate()}>
                <Copy size={15} /> Duplicate
              </button>
            )}
          </div>
          <button className="secondary-button" type="button" disabled={busy} onClick={onCancel}>Cancel</button>
          <button className="primary-button" type="submit" disabled={busy}>{busy ? 'Saving' : 'Save profile'}</button>
        </footer>
      </form>
    </div>
  )
}

interface FieldProps {
  draft: ProfileInput
  setField: <Key extends keyof ProfileInput>(key: Key, value: ProfileInput[Key]) => void
}

function SSHFields({ draft, setField }: FieldProps) {
  return (
    <section className="form-section" aria-label="SSH connection">
      <div className="section-heading"><Server size={15} /> Connection</div>
      <div className="form-grid ssh-grid">
        <label className="field host-field">
          <span>Host</span>
          <input required value={draft.host} onChange={(event) => setField('host', event.target.value)} />
        </label>
        <label className="field port-field">
          <span>Port</span>
          <input required type="number" min={1} max={65535} value={draft.port} onChange={(event) => setField('port', Number(event.target.value))} />
        </label>
        <label className="field field-wide">
          <span>Username</span>
          <input placeholder="Current local user" value={draft.username} onChange={(event) => setField('username', event.target.value)} />
        </label>
      </div>

      <div className="section-heading"><KeyRound size={15} /> Authentication</div>
      <div className="form-grid two-columns">
        <label className="field">
          <span>Method</span>
          <select value={draft.authentication} onChange={(event) => setField('authentication', event.target.value as SSHAuthentication)}>
            <option value="auto">Agent, then key</option>
            <option value="agent">SSH agent</option>
            <option value="key">Private key</option>
            <option value="password">Password prompt</option>
          </select>
        </label>
        {(draft.authentication === 'auto' || draft.authentication === 'key') && (
          <label className="field">
            <span>Identity file</span>
            <input required={draft.authentication === 'key'} placeholder="~/.ssh/id_ed25519" value={draft.identityFile} onChange={(event) => setField('identityFile', event.target.value)} />
          </label>
        )}
      </div>
    </section>
  )
}

interface LocalFieldsProps extends FieldProps {
  argumentsText: string
  setArgumentsText: (value: string) => void
}

function LocalFields({ draft, argumentsText, setArgumentsText, setField }: LocalFieldsProps) {
  return (
    <section className="form-section" aria-label="Local shell">
      <div className="section-heading"><Laptop size={15} /> Shell</div>
      <div className="form-grid two-columns">
        <label className="field">
          <span>Executable</span>
          <input placeholder="Default login shell" value={draft.shell} onChange={(event) => setField('shell', event.target.value)} />
        </label>
        <label className="field">
          <span>Working directory</span>
          <input placeholder="Home directory" value={draft.workingDirectory} onChange={(event) => setField('workingDirectory', event.target.value)} />
        </label>
        <label className="field field-wide">
          <span>Arguments, one per line</span>
          <textarea rows={3} placeholder="-l" value={argumentsText} onChange={(event) => setArgumentsText(event.target.value)} />
        </label>
      </div>
    </section>
  )
}

function profileToInput(profile?: Profile): ProfileInput {
  if (!profile) {
    return { ...emptyProfile, arguments: [], environment: {}, tags: [] }
  }
  return {
    id: profile.id,
    name: profile.name,
    protocol: profile.protocol,
    host: profile.host,
    port: profile.port,
    username: profile.username,
    authentication: profile.authentication,
    identityFile: profile.identityFile,
    shell: profile.shell,
    arguments: [...profile.arguments],
    workingDirectory: profile.workingDirectory,
    environment: { ...(profile.environment ?? {}) },
    tags: [...profile.tags],
    group: profile.group,
    favorite: profile.favorite,
  }
}

function splitLines(value: string): string[] {
  return value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean)
}
