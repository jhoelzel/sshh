import { useState, type FormEvent } from 'react'
import { KeyRound, LoaderCircle, Server, X, Zap } from 'lucide-react'
import type { QuickSSHInput, SSHAuthentication } from '../../lib/bridge/types'

interface QuickConnectDialogProps {
  onCancel: () => void
  onConnect: (input: QuickSSHInput) => Promise<void>
}

const initialInput: QuickSSHInput = {
  host: '',
  port: 22,
  username: '',
  authentication: 'auto',
  identityFile: '',
}

export function QuickConnectDialog({ onCancel, onConnect }: QuickConnectDialogProps) {
  const [draft, setDraft] = useState(initialInput)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const setField = <Key extends keyof QuickSSHInput>(key: Key, value: QuickSSHInput[Key]) => {
    setDraft((current) => ({ ...current, [key]: value }))
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(undefined)
    try {
      await onConnect({
        ...draft,
        host: draft.host.trim(),
        username: draft.username.trim(),
        identityFile: draft.identityFile.trim(),
      })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <form className="ssh-dialog quick-connect-dialog" role="dialog" aria-modal="true" aria-labelledby="quick-connect-title" onSubmit={(event) => void submit(event)}>
        <header className="ssh-dialog-header">
          <div className="ssh-dialog-title-icon"><Zap size={19} /></div>
          <div><h2 id="quick-connect-title">Quick connect</h2><p>SSH</p></div>
          <button className="icon-button" type="button" title="Cancel" aria-label="Cancel quick connect" disabled={busy} onClick={onCancel}>
            <X size={16} />
          </button>
        </header>

        <div className="ssh-dialog-body quick-connect-body">
          <div className="section-heading"><Server size={15} /> Connection</div>
          <div className="form-grid ssh-grid">
            <label className="field host-field">
              <span>Host</span>
              <input autoFocus required maxLength={255} value={draft.host} onChange={(event) => setField('host', event.target.value)} />
            </label>
            <label className="field port-field">
              <span>Port</span>
              <input required type="number" min={1} max={65535} value={draft.port} onChange={(event) => setField('port', Number(event.target.value))} />
            </label>
            <label className="field field-wide">
              <span>Username</span>
              <input maxLength={255} placeholder="Current local user" value={draft.username} onChange={(event) => setField('username', event.target.value)} />
            </label>
          </div>

          <div className="section-heading quick-auth-heading"><KeyRound size={15} /> Authentication</div>
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
                <input required={draft.authentication === 'key'} maxLength={4096} placeholder="~/.ssh/id_ed25519" value={draft.identityFile} onChange={(event) => setField('identityFile', event.target.value)} />
              </label>
            )}
          </div>
          {error && <div className="form-error" role="alert">{error}</div>}
        </div>

        <footer className="ssh-dialog-actions">
          <button className="secondary-button" type="button" disabled={busy} onClick={onCancel}>Cancel</button>
          <button className="primary-button" type="submit" disabled={busy}>
            {busy ? <LoaderCircle className="spin" size={14} /> : <Zap size={14} />} Connect
          </button>
        </footer>
      </form>
    </div>
  )
}
