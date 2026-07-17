import { useState } from 'react'
import { FileText, LoaderCircle, X } from 'lucide-react'

interface LoggingDialogProps {
  title: string
  onCancel: () => void
  onStart: (timestampLines: boolean) => Promise<void>
}

export function LoggingDialog({ title, onCancel, onStart }: LoggingDialogProps) {
  const [timestampLines, setTimestampLines] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const start = async () => {
    setBusy(true)
    setError(undefined)
    try {
      await onStart(timestampLines)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <section className="logging-dialog" role="dialog" aria-modal="true" aria-labelledby="logging-title">
        <header className="profile-dialog-header">
          <div><h2 id="logging-title">Start session log</h2><p>{title}</p></div>
          <button className="icon-button" type="button" aria-label="Close session log dialog" onClick={onCancel}><X size={16} /></button>
        </header>
        <div className="logging-options">
          <FileText size={20} />
          <label><input type="checkbox" checked={timestampLines} onChange={(event) => setTimestampLines(event.target.checked)} /> Prefix lines with timestamps</label>
          {error && <div className="form-error" role="alert">{error}</div>}
        </div>
        <footer className="profile-dialog-actions">
          <span />
          <button className="secondary-button" type="button" disabled={busy} onClick={onCancel}>Cancel</button>
          <button className="primary-button" type="button" disabled={busy} onClick={() => void start()}>{busy ? <LoaderCircle className="spin" size={14} /> : <FileText size={14} />} Start logging</button>
        </footer>
      </section>
    </div>
  )
}
