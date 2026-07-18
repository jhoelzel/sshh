import { Braces, Plus, Trash2 } from 'lucide-react'
import { MAX_PROFILE_ENVIRONMENT_OVERRIDES, type ProfileEnvironmentEntry } from './profileEnvironment'

interface ProfileEnvironmentEditorProps {
  entries: ProfileEnvironmentEntry[]
  onAdd: () => void
  onChange: (id: string, field: 'name' | 'value', value: string) => void
  onRemove: (id: string) => void
}

export function ProfileEnvironmentEditor({ entries, onAdd, onChange, onRemove }: ProfileEnvironmentEditorProps) {
  return (
    <section className="form-section" aria-labelledby="profile-environment-heading">
      <div className="environment-toolbar">
        <div className="section-heading" id="profile-environment-heading"><Braces size={15} /> Environment</div>
        <button
          className="icon-text-button"
          type="button"
          disabled={entries.length >= MAX_PROFILE_ENVIRONMENT_OVERRIDES}
          title={entries.length >= MAX_PROFILE_ENVIRONMENT_OVERRIDES ? 'Environment variable limit reached' : 'Add environment variable'}
          onClick={onAdd}
        >
          <Plus size={14} /> Add variable
        </button>
      </div>

      {entries.length > 0 && (
        <div className="environment-table">
          <div className="environment-columns" aria-hidden="true">
            <span>Variable</span>
            <span>Value</span>
            <span />
          </div>
          {entries.map((entry, index) => {
            const nameID = `profile-environment-name-${entry.id}`
            const valueID = `profile-environment-value-${entry.id}`
            const removeLabel = entry.name ? `Remove ${entry.name}` : `Remove environment variable ${index + 1}`
            return (
              <div className="environment-row" key={entry.id}>
                <label className="visually-hidden" htmlFor={nameID}>Environment variable {index + 1}</label>
                <input
                  className="environment-input"
                  id={nameID}
                  maxLength={128}
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  placeholder="LANG"
                  value={entry.name}
                  onChange={(event) => onChange(entry.id, 'name', event.target.value)}
                />
                <label className="visually-hidden" htmlFor={valueID}>Environment value {index + 1}</label>
                <textarea
                  className="environment-input"
                  id={valueID}
                  rows={1}
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  placeholder="en_US.UTF-8"
                  value={entry.value}
                  onChange={(event) => onChange(entry.id, 'value', event.target.value)}
                />
                <button className="icon-button compact" type="button" title="Remove variable" aria-label={removeLabel} onClick={() => onRemove(entry.id)}>
                  <Trash2 size={14} />
                </button>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}
