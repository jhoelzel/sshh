import { CheckCircle2, FileDown, FileUp, TriangleAlert, X } from 'lucide-react'
import type { ProfileExchangeResult } from '../../lib/bridge/types'

interface ProfileExchangeDialogProps {
  exchange: ProfileExchangeResult
  onClose: () => void
}

const visibleImportLimit = 20

export function ProfileExchangeDialog({ exchange, onClose }: ProfileExchangeDialogProps) {
  const imported = exchange.kind === 'import' ? exchange.result.imported : []
  const warnings = exchange.kind === 'import' ? exchange.result.warnings : []
  const title = exchange.kind === 'import' ? 'Profile import complete' : 'Profile export complete'

  return (
    <div className="modal-backdrop" role="presentation">
      <section className="profile-exchange-dialog" role="dialog" aria-modal="true" aria-labelledby="profile-exchange-title">
        <header className="profile-exchange-header">
          <span className="profile-exchange-icon" aria-hidden="true">
            {exchange.kind === 'import' ? <FileUp size={19} /> : <FileDown size={19} />}
          </span>
          <div>
            <h2 id="profile-exchange-title">{title}</h2>
            <p>{exchange.result.filename}</p>
          </div>
          <button className="icon-button" type="button" aria-label="Close profile result" onClick={onClose}>
            <X size={16} />
          </button>
        </header>

        <div className="profile-exchange-body">
          <div className="profile-exchange-summary">
            <CheckCircle2 size={17} />
            {exchange.kind === 'import'
              ? `${imported.length} profile${imported.length === 1 ? '' : 's'} imported from ${exchange.result.format}.`
              : `${exchange.result.exported} profile${exchange.result.exported === 1 ? '' : 's'} exported.`}
          </div>

          {imported.length > 0 && (
            <section className="profile-exchange-section" aria-labelledby="imported-profiles-title">
              <h3 id="imported-profiles-title">Imported profiles</h3>
              <ul className="profile-exchange-list">
                {imported.slice(0, visibleImportLimit).map((profile) => (
                  <li key={profile.id}><span>{profile.name}</span><code>{profile.endpoint}</code></li>
                ))}
              </ul>
              {imported.length > visibleImportLimit && (
                <p className="profile-exchange-more">{imported.length - visibleImportLimit} more profiles imported</p>
              )}
            </section>
          )}

          {warnings.length > 0 && (
            <section className="profile-exchange-section profile-exchange-warnings" aria-labelledby="import-warnings-title">
              <h3 id="import-warnings-title"><TriangleAlert size={14} /> Import diagnostics</h3>
              <ol className="profile-exchange-warning-list">
                {warnings.map((warning, index) => <li key={`${index}-${warning}`}>{warning}</li>)}
              </ol>
            </section>
          )}
        </div>

        <footer className="profile-exchange-actions">
          <button className="primary-button" type="button" autoFocus onClick={onClose}>Done</button>
        </footer>
      </section>
    </div>
  )
}
