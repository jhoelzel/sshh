import { useRef, useState, type FormEvent } from 'react'
import { Fingerprint, KeyRound, ShieldAlert, X } from 'lucide-react'
import type { Profile, SSHAuthenticationInfo, SSHHostKey } from '../../lib/bridge/types'

interface TrustDialogProps {
  profile: Profile
  hostKey: SSHHostKey
  onCancel: () => void
  onTrust: (permanent: boolean) => Promise<void>
}

export function SSHTrustDialog({ profile, hostKey, onCancel, onTrust }: TrustDialogProps) {
  const [permanent, setPermanent] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const trust = async () => {
    setBusy(true)
    setError(undefined)
    try {
      await onTrust(permanent)
    } catch (cause) {
      setError(errorMessage(cause))
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <section className="ssh-dialog" role="dialog" aria-modal="true" aria-labelledby="host-key-title">
        <header className="ssh-dialog-header">
          <div className="ssh-dialog-title-icon"><ShieldAlert size={19} /></div>
          <div>
            <h2 id="host-key-title">Verify host identity</h2>
            <p>{profile.name} · {hostKey.address}</p>
          </div>
          <button className="icon-button" type="button" title="Cancel" aria-label="Cancel connection" disabled={busy} onClick={onCancel}>
            <X size={16} />
          </button>
        </header>
        <div className="ssh-dialog-body">
          <p>This host has not been trusted before. Compare the fingerprint with a trusted source before continuing.</p>
          <div className="fingerprint-block">
            <span><Fingerprint size={15} /> {hostKey.algorithm}</span>
            <code>{hostKey.fingerprint}</code>
          </div>
          <label className="trust-option">
            <input type="checkbox" checked={permanent} onChange={(event) => setPermanent(event.target.checked)} />
            <span>Remember in the shh-h known hosts file</span>
          </label>
          {error && <div className="form-error" role="alert">{error}</div>}
        </div>
        <footer className="ssh-dialog-actions">
          <button className="secondary-button" type="button" disabled={busy} onClick={onCancel}>Cancel</button>
          <button className="primary-button" type="button" disabled={busy} onClick={() => void trust()}>{busy ? 'Verifying' : 'Trust and connect'}</button>
        </footer>
      </section>
    </div>
  )
}

interface CredentialsDialogProps {
  profile: Profile
  authentication: SSHAuthenticationInfo
  onCancel: () => void
  onConnect: (secret: string) => Promise<void>
}

export function SSHCredentialsDialog({ profile, authentication, onCancel, onConnect }: CredentialsDialogProps) {
  const secretInput = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const input = secretInput.current
    if (!input || input.value === '') {
      input?.focus()
      return
    }
    const secret = input.value
    input.value = ''
    setBusy(true)
    setError(undefined)
    try {
      await onConnect(secret)
    } catch (cause) {
      setError(errorMessage(cause))
      setBusy(false)
      input.focus()
    }
  }

  const passphrase = authentication.secret === 'passphrase'
  return (
    <div className="modal-backdrop" role="presentation">
      <form className="ssh-dialog credentials-dialog" role="dialog" aria-modal="true" aria-labelledby="credentials-title" onSubmit={(event) => void submit(event)}>
        <header className="ssh-dialog-header">
          <div className="ssh-dialog-title-icon"><KeyRound size={19} /></div>
          <div>
            <h2 id="credentials-title">{passphrase ? 'Unlock private key' : 'SSH password'}</h2>
            <p>{profile.endpoint}</p>
          </div>
          <button className="icon-button" type="button" title="Cancel" aria-label="Cancel connection" disabled={busy} onClick={onCancel}>
            <X size={16} />
          </button>
        </header>
        <div className="ssh-dialog-body">
          {passphrase && authentication.identityFile && <div className="identity-path">{authentication.identityFile}</div>}
          <label className="field">
            <span>{passphrase ? 'Passphrase' : 'Password'}</span>
            <input ref={secretInput} autoFocus required type="password" autoComplete="off" disabled={busy} />
          </label>
          <p className="secret-note">Used for this connection only and never written to the profile.</p>
          {error && <div className="form-error" role="alert">{error}</div>}
        </div>
        <footer className="ssh-dialog-actions">
          <button className="secondary-button" type="button" disabled={busy} onClick={onCancel}>Cancel</button>
          <button className="primary-button" type="submit" disabled={busy}>{busy ? 'Connecting' : 'Connect'}</button>
        </footer>
      </form>
    </div>
  )
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}
