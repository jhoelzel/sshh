import { useMemo, useState, type FormEvent } from 'react'
import {
  Braces,
  CircleAlert,
  LoaderCircle,
  Pencil,
  Play,
  Plus,
  Search,
  TerminalSquare,
  Trash2,
  X,
} from 'lucide-react'
import type { Snippet, SnippetInput, SnippetPreview } from '../../lib/bridge/types'

export interface SnippetTarget {
  id: string
  title: string
  active: boolean
}

interface SnippetWorkspaceProps {
  snippets: Snippet[]
  targets: SnippetTarget[]
  onCreate: (input: SnippetInput) => Promise<void>
  onUpdate: (input: SnippetInput) => Promise<void>
  onDelete: (id: string) => Promise<void>
  onRender: (id: string, values: Record<string, string>) => Promise<SnippetPreview>
  onExecute: (text: string, targetIds: string[], submit: boolean) => Promise<void>
}

interface ExecutionState {
  snippet: Snippet
  values: Record<string, string>
  targets: string[]
  submit: boolean
  confirmed: boolean
  preview?: SnippetPreview
}

export function SnippetWorkspace(props: SnippetWorkspaceProps) {
  const [filter, setFilter] = useState('')
  const [editor, setEditor] = useState<Snippet | null>()
  const [deleting, setDeleting] = useState<Snippet>()
  const [execution, setExecution] = useState<ExecutionState>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const visible = useMemo(() => {
    const query = filter.trim().toLowerCase()
    if (!query) return props.snippets
    return props.snippets.filter((snippet) =>
      [snippet.name, snippet.folder, snippet.body, ...snippet.tags].some((value) => value.toLowerCase().includes(query)),
    )
  }, [filter, props.snippets])

  const run = async (operation: () => Promise<void>) => {
    setBusy(true)
    setError(undefined)
    try {
      await operation()
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setBusy(false)
    }
  }

  const beginExecution = (snippet: Snippet) => {
    setError(undefined)
    const active = props.targets.find((target) => target.active) ?? props.targets[0]
    setExecution({
      snippet,
      values: Object.fromEntries(snippet.variables.map((variable) => [variable, ''])),
      targets: active ? [active.id] : [],
      submit: true,
      confirmed: false,
    })
  }

  return (
    <section className="snippet-workspace" aria-label="Command snippets">
      <header className="snippet-header">
        <div className="snippet-title">
          <Braces size={18} />
          <div><strong>Command snippets</strong><span>{props.snippets.length} saved</span></div>
        </div>
        <button className="primary-button" type="button" onClick={() => setEditor(null)}>
          <Plus size={16} /> New snippet
        </button>
      </header>

      <div className="snippet-toolbar">
        <Search size={14} />
        <input
          aria-label="Filter snippets"
          placeholder="Filter snippets"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
        {filter && <button className="icon-button compact" type="button" aria-label="Clear snippet filter" onClick={() => setFilter('')}><X size={14} /></button>}
      </div>

      {error && (
        <div className="snippet-error" role="alert">
          <CircleAlert size={15} /><span>{error}</span>
          <button className="icon-button compact" type="button" aria-label="Dismiss error" onClick={() => setError(undefined)}><X size={14} /></button>
        </div>
      )}

      <div className="snippet-table" role="table" aria-label="Saved command snippets">
        <div className="snippet-table-header" role="row">
          <span role="columnheader">Snippet</span>
          <span role="columnheader">Command</span>
          <span role="columnheader">Tags</span>
          <span role="columnheader">Actions</span>
        </div>
        <div className="snippet-table-body">
          {visible.length === 0 ? (
            <div className="snippet-empty"><Braces size={30} strokeWidth={1.4} /><h1>No snippets found</h1></div>
          ) : visible.map((snippet) => (
            <div className="snippet-row" role="row" key={snippet.id}>
              <span className="snippet-name-cell" role="cell">
                <Braces size={15} />
                <span><strong>{snippet.name}</strong><small>{snippet.folder || 'Unfiled'}</small></span>
              </span>
              <code role="cell" title={snippet.body}>{singleLine(snippet.body)}</code>
              <span className="snippet-tags" role="cell">
                {snippet.tags.length === 0 ? <small>None</small> : snippet.tags.map((tag) => <i key={tag}>{tag}</i>)}
              </span>
              <span className="snippet-actions" role="cell">
                <button className="icon-button compact" type="button" title="Run snippet" aria-label={`Run ${snippet.name}`} disabled={props.targets.length === 0 || busy} onClick={() => beginExecution(snippet)}><Play size={15} /></button>
                <button className="icon-button compact" type="button" title="Edit snippet" aria-label={`Edit ${snippet.name}`} disabled={busy} onClick={() => setEditor(snippet)}><Pencil size={14} /></button>
                <button className="icon-button compact danger-quiet" type="button" title="Delete snippet" aria-label={`Delete ${snippet.name}`} disabled={busy} onClick={() => setDeleting(snippet)}><Trash2 size={14} /></button>
              </span>
            </div>
          ))}
        </div>
      </div>

      {editor !== undefined && (
        <SnippetEditor
          snippet={editor ?? undefined}
          onCancel={() => setEditor(undefined)}
          onSave={async (input) => {
            if (input.id) await props.onUpdate(input)
            else await props.onCreate(input)
            setEditor(undefined)
          }}
        />
      )}

      {deleting && (
        <div className="modal-backdrop" role="presentation">
          <section className="dialog" role="dialog" aria-modal="true" aria-labelledby="delete-snippet-title">
            <div className="dialog-icon"><CircleAlert size={20} /></div>
            <div className="dialog-copy"><h2 id="delete-snippet-title">Delete this snippet?</h2><p>{deleting.name} will be removed.</p></div>
            <div className="dialog-actions">
              <button className="secondary-button" type="button" onClick={() => setDeleting(undefined)}>Cancel</button>
              <button className="danger-button" type="button" disabled={busy} onClick={() => void run(async () => { await props.onDelete(deleting.id); setDeleting(undefined) })}>Delete</button>
            </div>
          </section>
        </div>
      )}

      {execution && (
        <ExecutionDialog
          state={execution}
          targets={props.targets}
          busy={busy}
          error={error}
          onChange={setExecution}
          onCancel={() => { setExecution(undefined); setError(undefined) }}
          onPreview={() => void run(async () => {
            const preview = await props.onRender(execution.snippet.id, execution.values)
            setExecution((current) => current ? { ...current, preview, confirmed: false } : current)
          })}
          onExecute={() => void run(async () => {
            if (!execution.preview) throw new Error('Preview the snippet before execution')
            await props.onExecute(execution.preview.text, execution.targets, execution.submit)
            setExecution(undefined)
          })}
        />
      )}
    </section>
  )
}

interface SnippetEditorProps {
  snippet?: Snippet
  onCancel: () => void
  onSave: (input: SnippetInput) => Promise<void>
}

function SnippetEditor({ snippet, onCancel, onSave }: SnippetEditorProps) {
  const [draft, setDraft] = useState<SnippetInput>(() => snippet ? {
    id: snippet.id, name: snippet.name, folder: snippet.folder, tags: [...snippet.tags], body: snippet.body,
  } : { id: '', name: '', folder: '', tags: [], body: '' })
  const [tags, setTags] = useState(snippet?.tags.join(', ') ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(undefined)
    try {
      await onSave({ ...draft, tags: tags.split(',').map((tag) => tag.trim()).filter(Boolean) })
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <form className="profile-dialog snippet-dialog" role="dialog" aria-modal="true" aria-labelledby="snippet-editor-title" onSubmit={(event) => void submit(event)}>
        <header className="profile-dialog-header">
          <div><h2 id="snippet-editor-title">{snippet ? 'Edit snippet' : 'New snippet'}</h2><p>Saved command</p></div>
          <button className="icon-button" type="button" aria-label="Close snippet editor" onClick={onCancel}><X size={16} /></button>
        </header>
        <div className="profile-form-scroll">
          <div className="form-grid two-columns">
            <label className="field"><span>Name</span><input autoFocus required maxLength={120} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label>
            <label className="field"><span>Folder</span><input maxLength={120} value={draft.folder} onChange={(event) => setDraft({ ...draft, folder: event.target.value })} /></label>
            <label className="field field-wide"><span>Tags</span><input value={tags} onChange={(event) => setTags(event.target.value)} /></label>
            <label className="field field-wide"><span>Command</span><textarea className="snippet-body-input" required value={draft.body} onChange={(event) => setDraft({ ...draft, body: event.target.value })} /></label>
          </div>
          {error && <div className="form-error" role="alert">{error}</div>}
        </div>
        <footer className="profile-dialog-actions">
          <span />
          <button className="secondary-button" type="button" disabled={busy} onClick={onCancel}>Cancel</button>
          <button className="primary-button" type="submit" disabled={busy}>{busy ? 'Saving' : 'Save snippet'}</button>
        </footer>
      </form>
    </div>
  )
}

interface ExecutionDialogProps {
  state: ExecutionState
  targets: SnippetTarget[]
  busy: boolean
  error?: string
  onChange: (state: ExecutionState) => void
  onCancel: () => void
  onPreview: () => void
  onExecute: () => void
}

function ExecutionDialog({ state, targets, busy, error, onChange, onCancel, onPreview, onExecute }: ExecutionDialogProps) {
  const multi = state.targets.length > 1
  const setValue = (name: string, value: string) => onChange({
    ...state, values: { ...state.values, [name]: value }, preview: undefined, confirmed: false,
  })
  const toggleTarget = (id: string, checked: boolean) => onChange({
    ...state,
    targets: checked ? [...state.targets, id] : state.targets.filter((target) => target !== id),
    confirmed: false,
  })

  return (
    <div className="modal-backdrop" role="presentation">
      <section className="snippet-run-dialog" role="dialog" aria-modal="true" aria-labelledby="snippet-run-title">
        <header className="profile-dialog-header">
          <div><h2 id="snippet-run-title">{state.snippet.name}</h2><p>Command preview</p></div>
          <button className="icon-button" type="button" aria-label="Close snippet preview" onClick={onCancel}><X size={16} /></button>
        </header>
        <div className="snippet-run-content">
          {state.snippet.variables.length > 0 && (
            <div className="snippet-variable-grid">
              {state.snippet.variables.map((variable) => (
                <label className="field" key={variable}><span>{variable}</span><input value={state.values[variable] ?? ''} onChange={(event) => setValue(variable, event.target.value)} /></label>
              ))}
            </div>
          )}
          <pre className={`snippet-preview${state.preview ? '' : ' is-pending'}`}>{state.preview?.text ?? 'Preview required'}</pre>
          <fieldset className="snippet-targets">
            <legend>Live terminals</legend>
            {targets.map((target) => (
              <label key={target.id}>
                <input type="checkbox" checked={state.targets.includes(target.id)} onChange={(event) => toggleTarget(target.id, event.target.checked)} />
                <TerminalSquare size={14} /><span>{target.title}</span>{target.active && <small>Active</small>}
              </label>
            ))}
          </fieldset>
          <label className="snippet-submit-option"><input type="checkbox" checked={state.submit} onChange={(event) => onChange({ ...state, submit: event.target.checked })} /> Send Enter</label>
          {multi && (
            <label className="multi-execution-warning">
              <CircleAlert size={17} />
              <input type="checkbox" checked={state.confirmed} onChange={(event) => onChange({ ...state, confirmed: event.target.checked })} />
              <span>Confirm execution in {state.targets.length} terminals</span>
            </label>
          )}
          {error && <div className="form-error" role="alert">{error}</div>}
        </div>
        <footer className="profile-dialog-actions snippet-run-actions">
          <button className="secondary-button" type="button" disabled={busy} onClick={onPreview}>{busy ? <LoaderCircle className="spin" size={14} /> : null} Preview</button>
          <span />
          <button className="secondary-button" type="button" disabled={busy} onClick={onCancel}>Cancel</button>
          <button className="primary-button" type="button" disabled={busy || !state.preview || state.targets.length === 0 || (multi && !state.confirmed)} onClick={onExecute}><Play size={14} /> Run</button>
        </footer>
      </section>
    </div>
  )
}

function singleLine(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}
