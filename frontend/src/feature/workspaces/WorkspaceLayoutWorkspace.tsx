import { useState, type FormEvent } from 'react'
import {
  CircleAlert,
  FolderOpen,
  LayoutPanelTop,
  LoaderCircle,
  Pencil,
  Save,
  Trash2,
  X,
} from 'lucide-react'
import type { WorkspaceLayout } from '../../lib/bridge/types'

interface WorkspaceLayoutWorkspaceProps {
  layouts: WorkspaceLayout[]
  savableTabCount: number
  onCreate: (name: string) => Promise<void>
  onRename: (layout: WorkspaceLayout, name: string) => Promise<void>
  onReplace: (layout: WorkspaceLayout) => Promise<void>
  onRestore: (layout: WorkspaceLayout) => Promise<void>
  onDelete: (layout: WorkspaceLayout) => Promise<void>
}

type Editor = { kind: 'create' } | { kind: 'rename'; layout: WorkspaceLayout }
type Confirmation = { kind: 'replace' | 'delete'; layout: WorkspaceLayout }

export function WorkspaceLayoutWorkspace(props: WorkspaceLayoutWorkspaceProps) {
  const [editor, setEditor] = useState<Editor>()
  const [confirmation, setConfirmation] = useState<Confirmation>()
  const [busyId, setBusyId] = useState<string>()
  const [error, setError] = useState<string>()

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
    <section className="layout-workspace" aria-label="Workspace layouts">
      <header className="layout-header">
        <div className="layout-title">
          <LayoutPanelTop size={18} />
          <div><strong>Workspace layouts</strong><span>{props.layouts.length} saved</span></div>
        </div>
        <button className="primary-button" type="button" disabled={props.savableTabCount === 0} onClick={() => setEditor({ kind: 'create' })}>
          <Save size={15} /> Save current
        </button>
      </header>

      {error && (
        <div className="layout-error" role="alert">
          <CircleAlert size={16} /><span>{error}</span>
          <button className="icon-button compact" type="button" aria-label="Dismiss error" onClick={() => setError(undefined)}><X size={14} /></button>
        </div>
      )}

      <div className="layout-table" role="table" aria-label="Saved workspace layouts">
        <div className="layout-table-header" role="row">
          <span role="columnheader">Layout</span>
          <span role="columnheader">Tabs</span>
          <span role="columnheader">Updated</span>
          <span role="columnheader">Actions</span>
        </div>
        <div className="layout-table-body">
          {props.layouts.length === 0 ? (
            <div className="layout-empty"><LayoutPanelTop size={30} strokeWidth={1.4} /><h1>No layouts saved</h1></div>
          ) : props.layouts.map((layout) => {
            const busy = busyId === layout.id
            return (
              <div className="layout-row" role="row" key={layout.id}>
                <span className="layout-name-cell" role="cell">
                  <LayoutPanelTop size={16} />
                  <span>
                    <strong>{layout.name}</strong>
                    <small>{layout.tabs.length} terminal{layout.tabs.length === 1 ? '' : 's'}{layout.split ? ', split' : ''}</small>
                  </span>
                </span>
                <span className="layout-tab-summary" role="cell" title={layout.tabs.map((tab) => tab.endpoint || tab.title).join(', ')}>
                  {layout.tabs.map((tab) => tab.title).join(', ')}
                </span>
                <span className="layout-updated" role="cell">{formatUpdated(layout.updatedAt)}</span>
                <span className="layout-actions" role="cell">
                  {busy ? <LoaderCircle className="spin" size={16} /> : (
                    <>
                      <button className="icon-button compact" type="button" title="Restore layout" aria-label={`Restore ${layout.name}`} onClick={() => void run(layout.id, () => props.onRestore(layout))}><FolderOpen size={15} /></button>
                      <button className="icon-button compact" type="button" title="Replace with current workspace" aria-label={`Replace ${layout.name}`} disabled={props.savableTabCount === 0} onClick={() => setConfirmation({ kind: 'replace', layout })}><Save size={14} /></button>
                      <button className="icon-button compact" type="button" title="Rename layout" aria-label={`Rename ${layout.name}`} onClick={() => setEditor({ kind: 'rename', layout })}><Pencil size={14} /></button>
                      <button className="icon-button compact danger-quiet" type="button" title="Delete layout" aria-label={`Delete ${layout.name}`} onClick={() => setConfirmation({ kind: 'delete', layout })}><Trash2 size={14} /></button>
                    </>
                  )}
                </span>
              </div>
            )
          })}
        </div>
      </div>

      {editor && (
        <LayoutNameDialog
          title={editor.kind === 'create' ? 'Save workspace layout' : 'Rename workspace layout'}
          initialName={editor.kind === 'rename' ? editor.layout.name : ''}
          submitLabel={editor.kind === 'create' ? 'Save layout' : 'Rename'}
          onCancel={() => setEditor(undefined)}
          onSubmit={async (name) => {
            if (editor.kind === 'create') {
              await props.onCreate(name)
            } else {
              await props.onRename(editor.layout, name)
            }
            setEditor(undefined)
          }}
        />
      )}

      {confirmation && (
        <div className="modal-backdrop" role="presentation">
          <section className="dialog" role="dialog" aria-modal="true" aria-labelledby="layout-confirm-title">
            <div className="dialog-icon"><CircleAlert size={20} /></div>
            <div className="dialog-copy">
              <h2 id="layout-confirm-title">{confirmation.kind === 'delete' ? 'Delete this layout?' : 'Replace this layout?'}</h2>
              <p>{confirmation.kind === 'delete' ? `${confirmation.layout.name} will be removed.` : `${confirmation.layout.name} will use the current saved-profile tabs and split arrangement.`}</p>
            </div>
            <div className="dialog-actions">
              <button className="secondary-button" type="button" onClick={() => setConfirmation(undefined)}>Cancel</button>
              <button className={confirmation.kind === 'delete' ? 'danger-button' : 'primary-button'} type="button" onClick={() => void run(confirmation.layout.id, async () => {
                if (confirmation.kind === 'delete') await props.onDelete(confirmation.layout)
                else await props.onReplace(confirmation.layout)
                setConfirmation(undefined)
              })}>{confirmation.kind === 'delete' ? 'Delete' : 'Replace'}</button>
            </div>
          </section>
        </div>
      )}
    </section>
  )
}

interface LayoutNameDialogProps {
  title: string
  initialName: string
  submitLabel: string
  onCancel: () => void
  onSubmit: (name: string) => Promise<void>
}

function LayoutNameDialog(props: LayoutNameDialogProps) {
  const [name, setName] = useState(props.initialName)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(undefined)
    try {
      await props.onSubmit(name)
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <form className="dialog layout-name-dialog" role="dialog" aria-modal="true" aria-labelledby="layout-name-title" onSubmit={(event) => void submit(event)}>
        <div className="dialog-copy">
          <h2 id="layout-name-title">{props.title}</h2>
          <label className="field"><span>Layout name</span><input autoFocus required maxLength={120} value={name} onChange={(event) => setName(event.target.value)} /></label>
          {error && <div className="form-error" role="alert">{error}</div>}
        </div>
        <div className="dialog-actions">
          <button className="secondary-button" type="button" disabled={busy} onClick={props.onCancel}>Cancel</button>
          <button className="primary-button" type="submit" disabled={busy || !name.trim()}>{busy ? 'Saving' : props.submitLabel}</button>
        </div>
      </form>
    </div>
  )
}

function formatUpdated(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.valueOf())) return 'Unknown'
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}
