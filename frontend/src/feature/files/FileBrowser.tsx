import { useMemo, useRef, useState, type KeyboardEvent } from 'react'
import {
  ArrowDownToLine,
  ArrowUpFromLine,
  ChevronLeft,
  CircleX,
  File,
  FileSymlink,
  Folder,
  FolderPlus,
  KeyRound,
  LoaderCircle,
  Play,
  RefreshCw,
  RotateCcw,
  Star,
  Trash2,
  X,
} from 'lucide-react'
import type { FileSession, Profile, RemoteFile, RemotePathFavorite, Transfer, TransferResume } from '../../lib/bridge/types'
import { joinRemotePath, parentRemotePath } from './remotePath'

interface FileBrowserProps {
  profile: Profile
  session: FileSession
  path: string
  files: RemoteFile[]
  transfers: Transfer[]
  resumes: TransferResume[]
  favorites: RemotePathFavorite[]
  loading: boolean
  onNavigate: (path: string) => Promise<void>
  onRefresh: () => Promise<void>
  onUpload: () => Promise<void>
  onDownload: (path: string) => Promise<void>
  onCreateDirectory: (path: string) => Promise<void>
  onRename: (source: string, destination: string) => Promise<void>
  onDelete: (path: string) => Promise<void>
  onChmod: (path: string, mode: number) => Promise<void>
  onCancelTransfer: (id: string) => Promise<void>
  onResumeTransfer: (id: string) => Promise<void>
  onDiscardResume: (id: string) => Promise<void>
  onCreateFavorite: (path: string) => Promise<void>
  onDeleteFavorite: (id: string) => Promise<void>
  onClose: () => Promise<void>
}

type FileDialog =
  | { kind: 'mkdir'; value: string }
  | { kind: 'rename'; entry: RemoteFile; value: string }
  | { kind: 'chmod'; entry: RemoteFile; value: string }
  | { kind: 'delete'; entry: RemoteFile }

export function FileBrowser(props: FileBrowserProps) {
  const [selectedPath, setSelectedPath] = useState<string>()
  const [showHidden, setShowHidden] = useState(false)
  const pathInput = useRef<HTMLInputElement>(null)
  const [dialog, setDialog] = useState<FileDialog>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const visibleFiles = useMemo(
    () => (showHidden ? props.files : props.files.filter((entry) => !entry.name.startsWith('.'))),
    [props.files, showHidden],
  )
  const selected = props.files.find((entry) => entry.path === selectedPath)
  const currentFavorite = props.favorites.find((favorite) => favorite.path === props.path)
  const sessionTransfers = props.transfers.filter((transfer) => transfer.sessionId === props.session.id)
  const sessionResumeIds = new Set(sessionTransfers.map((transfer) => transfer.resumeId).filter(Boolean))
  const persistedResumes = props.resumes.filter(
    (resume) => resume.profileId === props.profile.id && !sessionResumeIds.has(resume.id),
  )

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

  const activateEntry = (entry: RemoteFile) => {
    if (entry.directory) {
      void run(() => props.onNavigate(entry.path))
    } else {
      void run(() => props.onDownload(entry.path))
    }
  }

  const entryKeyDown = (event: KeyboardEvent, entry: RemoteFile) => {
    if (event.key === 'Enter') {
      activateEntry(entry)
    }
  }

  const confirmDialog = async () => {
    if (!dialog) {
      return
    }
    await run(async () => {
      if (dialog.kind === 'mkdir') {
        await props.onCreateDirectory(joinRemotePath(props.path, dialog.value))
      } else if (dialog.kind === 'rename') {
        await props.onRename(dialog.entry.path, joinRemotePath(props.path, dialog.value))
      } else if (dialog.kind === 'chmod') {
        const mode = Number.parseInt(dialog.value, 8)
        if (!/^[0-7]{3,4}$/.test(dialog.value) || !Number.isFinite(mode)) {
          throw new Error('Mode must be three or four octal digits')
        }
        await props.onChmod(dialog.entry.path, mode)
      } else {
        await props.onDelete(dialog.entry.path)
      }
      setDialog(undefined)
      await props.onRefresh()
    })
  }

  return (
    <section className="file-workspace" aria-label="Remote files">
      <header className="file-header">
        <div className="file-title">
          <Folder size={17} />
          <div>
            <strong>{props.profile.name}</strong>
            <span>{props.profile.endpoint}</span>
          </div>
        </div>
        <div className="file-header-actions">
          <button className="icon-button" type="button" title="Refresh" aria-label="Refresh remote files" disabled={busy || props.loading} onClick={() => void run(props.onRefresh)}>
            <RefreshCw className={props.loading ? 'spin' : ''} size={16} />
          </button>
          <button className="icon-button" type="button" title="Close file session" aria-label="Close file session" disabled={busy} onClick={() => void run(props.onClose)}>
            <X size={16} />
          </button>
        </div>
      </header>

      <div className="file-toolbar">
        <button className="icon-button" type="button" title="Parent directory" aria-label="Parent directory" disabled={props.path === '/' || busy} onClick={() => void run(() => props.onNavigate(parentRemotePath(props.path)))}>
          <ChevronLeft size={16} />
        </button>
        <form className="remote-path" onSubmit={(event) => { event.preventDefault(); void run(() => props.onNavigate(pathInput.current?.value ?? props.path)) }}>
          <input ref={pathInput} key={props.path} aria-label="Remote path" defaultValue={props.path} />
        </form>
        <button
          className={`icon-button remote-favorite-toggle${currentFavorite ? ' is-favorite' : ''}`}
          type="button"
          title={currentFavorite ? 'Remove current path from favorites' : 'Add current path to favorites'}
          aria-label={currentFavorite ? 'Remove current path from favorites' : 'Add current path to favorites'}
          disabled={busy || props.loading}
          onClick={() => void run(() => currentFavorite ? props.onDeleteFavorite(currentFavorite.id) : props.onCreateFavorite(props.path))}
        >
          <Star size={15} fill={currentFavorite ? 'currentColor' : 'none'} />
        </button>
        <label className="remote-favorites">
          <Star size={13} />
          <select
            aria-label="Favorite remote paths"
            value=""
            disabled={busy || props.loading || props.favorites.length === 0}
            onChange={(event) => { if (event.target.value) void run(() => props.onNavigate(event.target.value)) }}
          >
            <option value="">Favorites</option>
            {props.favorites.map((favorite) => <option value={favorite.path} key={favorite.id}>{favorite.path}</option>)}
          </select>
        </label>
        <button className="icon-text-button" type="button" disabled={busy} onClick={() => setDialog({ kind: 'mkdir', value: '' })}>
          <FolderPlus size={15} /> New folder
        </button>
        <button className="icon-text-button" type="button" disabled={busy} onClick={() => void run(props.onUpload)}>
          <ArrowUpFromLine size={15} /> Upload
        </button>
        <label className="hidden-toggle">
          <input type="checkbox" checked={showHidden} onChange={(event) => setShowHidden(event.target.checked)} />
          Hidden
        </label>
      </div>

      <div className="file-actions">
        <button className="icon-text-button" type="button" disabled={!selected || selected.directory || busy} onClick={() => selected && void run(() => props.onDownload(selected.path))}>
          <ArrowDownToLine size={15} /> Download
        </button>
        <button className="icon-text-button" type="button" disabled={!selected || busy} onClick={() => selected && setDialog({ kind: 'rename', entry: selected, value: selected.name })}>
          <RotateCcw size={15} /> Rename
        </button>
        <button className="icon-text-button" type="button" disabled={!selected || busy} onClick={() => selected && setDialog({ kind: 'chmod', entry: selected, value: formatMode(selected.mode) })}>
          <KeyRound size={15} /> Permissions
        </button>
        <button className="icon-text-button danger-quiet" type="button" disabled={!selected || busy} onClick={() => selected && setDialog({ kind: 'delete', entry: selected })}>
          <Trash2 size={15} /> Delete
        </button>
        {error && <span className="file-operation-error" role="alert">{error}</span>}
      </div>

      <div className="file-table" role="table" aria-label={`Files in ${props.path}`}>
        <div className="file-table-header" role="row">
          <span role="columnheader">Name</span>
          <span role="columnheader">Size</span>
          <span role="columnheader">Modified</span>
          <span role="columnheader">Mode</span>
        </div>
        <div className="file-table-body">
          {props.loading ? (
            <div className="file-table-empty"><LoaderCircle className="spin" size={18} /> Loading</div>
          ) : visibleFiles.length === 0 ? (
            <div className="file-table-empty">Directory is empty</div>
          ) : (
            visibleFiles.map((entry) => (
              <div
                className={`file-row${selectedPath === entry.path ? ' is-selected' : ''}`}
                role="row"
                tabIndex={0}
                key={entry.path}
                onClick={() => setSelectedPath(entry.path)}
                onDoubleClick={() => activateEntry(entry)}
                onKeyDown={(event) => entryKeyDown(event, entry)}
              >
                <span className="file-name" role="cell">
                  {entry.symlink ? <FileSymlink size={15} /> : entry.directory ? <Folder size={15} /> : <File size={15} />}
                  <span>{entry.name}</span>
                </span>
                <span role="cell">{entry.directory ? '—' : formatBytes(entry.size)}</span>
                <span role="cell">{formatDate(entry.modifiedAt)}</span>
                <span className="file-mode" role="cell">{formatMode(entry.mode)}</span>
              </div>
            ))
          )}
        </div>
      </div>

      <section className="transfer-panel" aria-label="Transfers">
        <div className="transfer-heading">
          <span>Transfers</span>
          <span>{sessionTransfers.filter((transfer) => transfer.state === 'queued' || transfer.state === 'running').length} active</span>
        </div>
        <div className="transfer-list">
          {sessionTransfers.length === 0 && persistedResumes.length === 0 ? (
            <div className="transfer-empty">No transfers</div>
          ) : (
            <>
              {sessionTransfers.map((transfer) => {
                const resumable = transfer.resumeId !== '' && (transfer.state === 'failed' || transfer.state === 'cancelled')
                return (
                  <div className="transfer-row" key={transfer.id}>
                    {transfer.direction === 'download' ? <ArrowDownToLine size={15} /> : <ArrowUpFromLine size={15} />}
                    <div className="transfer-copy">
                      <span>{baseName(transfer.direction === 'download' ? transfer.source : transfer.destination)}</span>
                      <div className="transfer-progress"><i style={{ width: `${progressPercent(transfer)}%` }} /></div>
                    </div>
                    <span className={`transfer-state state-${transfer.state}`} title={transfer.message}>{transferLabel(transfer)}</span>
                    <div className="transfer-actions">
                      {(transfer.state === 'queued' || transfer.state === 'running') && (
                        <button className="icon-button compact" type="button" title="Cancel transfer" aria-label="Cancel transfer" disabled={busy} onClick={() => void run(() => props.onCancelTransfer(transfer.id))}>
                          <CircleX size={15} />
                        </button>
                      )}
                      {resumable && (
                        <>
                          <button className="icon-button compact" type="button" title="Resume transfer" aria-label="Resume transfer" disabled={busy} onClick={() => void run(() => props.onResumeTransfer(transfer.resumeId))}>
                            <Play size={14} />
                          </button>
                          <button className="icon-button compact" type="button" title="Discard partial transfer" aria-label="Discard partial transfer" disabled={busy} onClick={() => void run(() => props.onDiscardResume(transfer.resumeId))}>
                            <Trash2 size={14} />
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                )
              })}
              {persistedResumes.map((resume) => (
                <div className="transfer-row transfer-resume" key={`resume-${resume.id}`}>
                  {resume.direction === 'download' ? <ArrowDownToLine size={15} /> : <ArrowUpFromLine size={15} />}
                  <div className="transfer-copy">
                    <span>{baseName(resume.direction === 'download' ? resume.source : resume.destination)}</span>
                    <div className="transfer-progress"><i style={{ width: `${progressFromBytes(resume.bytes, resume.total)}%` }} /></div>
                  </div>
                  <span className={`transfer-state${resume.available ? '' : ' state-failed'}`} title={resume.message}>
                    {resume.available ? 'Interrupted' : resume.message}
                  </span>
                  <div className="transfer-actions">
                    <button className="icon-button compact" type="button" title={resume.available ? 'Resume transfer' : resume.message} aria-label="Resume transfer" disabled={busy || !resume.available} onClick={() => void run(() => props.onResumeTransfer(resume.id))}>
                      <Play size={14} />
                    </button>
                    <button className="icon-button compact" type="button" title="Discard partial transfer" aria-label="Discard partial transfer" disabled={busy} onClick={() => void run(() => props.onDiscardResume(resume.id))}>
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>
              ))}
            </>
          )}
        </div>
      </section>

      {dialog && (
        <div className="modal-backdrop" role="presentation">
          <section className="dialog file-dialog" role="dialog" aria-modal="true" aria-labelledby="file-dialog-title">
            <div className="dialog-copy file-dialog-copy">
              <h2 id="file-dialog-title">{dialog.kind === 'mkdir' ? 'New folder' : dialog.kind === 'rename' ? 'Rename item' : dialog.kind === 'chmod' ? 'File permissions' : 'Delete item?'}</h2>
              {dialog.kind === 'delete' ? (
                <p>{dialog.entry.name} will be removed. Non-empty folders are never deleted recursively.</p>
              ) : (
                <input autoFocus value={dialog.value} onChange={(event) => setDialog({ ...dialog, value: event.target.value })} onKeyDown={(event) => { if (event.key === 'Enter') void confirmDialog() }} />
              )}
            </div>
            <div className="dialog-actions">
              <button className="secondary-button" type="button" disabled={busy} onClick={() => setDialog(undefined)}>Cancel</button>
              <button className={dialog.kind === 'delete' ? 'danger-button' : 'primary-button'} type="button" disabled={busy || ('value' in dialog && dialog.value.trim() === '')} onClick={() => void confirmDialog()}>
                {dialog.kind === 'delete' ? 'Delete' : 'Save'}
              </button>
            </div>
          </section>
        </div>
      )}
    </section>
  )
}

function baseName(value: string): string {
  return value.split('/').filter(Boolean).at(-1) ?? value
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString([], { dateStyle: 'short', timeStyle: 'short' })
}

function formatMode(mode: number): string {
  return (mode & 0o7777).toString(8).padStart(4, '0')
}

function progressPercent(transfer: Transfer): number {
  if (transfer.state === 'completed') return 100
  return progressFromBytes(transfer.bytes, transfer.total)
}

function progressFromBytes(bytes: number, total: number): number {
  return total > 0 ? Math.min(100, Math.round((bytes / total) * 100)) : 0
}

function transferLabel(transfer: Transfer): string {
  if (transfer.state === 'running') return `${progressPercent(transfer)}%`
  if (transfer.state === 'failed') return transfer.message || 'Failed'
  return transfer.state.charAt(0).toUpperCase() + transfer.state.slice(1)
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}
