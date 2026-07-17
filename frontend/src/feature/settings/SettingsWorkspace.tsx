import { useMemo, useState, type FormEvent } from 'react'
import { Bell, MousePointer2, RotateCcw, Save, Settings2, Type } from 'lucide-react'
import type { AppSettings, TerminalCursorStyle, TerminalFontFamily } from '../../lib/bridge/types'

interface SettingsWorkspaceProps {
  settings: AppSettings
  onSave: (settings: AppSettings) => Promise<AppSettings>
  onReset: () => Promise<AppSettings>
}

export function SettingsWorkspace({ settings, onSave, onReset }: SettingsWorkspaceProps) {
  const [draft, setDraft] = useState(settings)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const dirty = useMemo(() => JSON.stringify(draft) !== JSON.stringify(settings), [draft, settings])

  const setTerminal = <Key extends keyof AppSettings['terminal']>(key: Key, value: AppSettings['terminal'][Key]) => {
    setDraft((current) => ({ ...current, terminal: { ...current.terminal, [key]: value } }))
  }

  const save = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(undefined)
    try {
      setDraft(await onSave(draft))
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setBusy(false)
    }
  }

  const reset = async () => {
    setBusy(true)
    setError(undefined)
    try {
      setDraft(await onReset())
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="settings-workspace" aria-label="Application settings" onSubmit={(event) => void save(event)}>
      <header className="settings-header">
        <div className="settings-title"><Settings2 size={18} /><div><strong>Settings</strong><span>Terminal defaults</span></div></div>
        <button className="secondary-button" type="button" disabled={busy} onClick={() => void reset()}><RotateCcw size={15} /> Reset</button>
      </header>

      <div className="settings-content">
        <section className="settings-section" aria-labelledby="text-settings-title">
          <header><Type size={17} /><h2 id="text-settings-title">Text and spacing</h2></header>
          <div className="settings-control-list">
            <label className="settings-control">
              <span><strong>Font</strong><small>Terminal typeface</small></span>
              <select aria-label="Font" value={draft.terminal.fontFamily} onChange={(event) => setTerminal('fontFamily', event.target.value as TerminalFontFamily)}>
                <option value="system-mono">System Mono</option>
                <option value="menlo">Menlo</option>
                <option value="monaco">Monaco</option>
              </select>
            </label>
            <label className="settings-control range-control">
              <span><strong>Font size</strong><small>Points</small></span>
              <input aria-label="Font size" type="range" min={10} max={28} step={1} value={draft.terminal.fontSize} onChange={(event) => setTerminal('fontSize', Number(event.target.value))} />
              <output>{draft.terminal.fontSize}</output>
            </label>
            <label className="settings-control range-control">
              <span><strong>Line height</strong><small>Line spacing</small></span>
              <input aria-label="Line height" type="range" min={1} max={2} step={0.05} value={draft.terminal.lineHeight} onChange={(event) => setTerminal('lineHeight', Number(event.target.value))} />
              <output>{draft.terminal.lineHeight.toFixed(2)}</output>
            </label>
            <label className="settings-control range-control">
              <span><strong>Scrollback</strong><small>Stored lines</small></span>
              <input aria-label="Scrollback" type="range" min={1000} max={100000} step={1000} value={draft.terminal.scrollback} onChange={(event) => setTerminal('scrollback', Number(event.target.value))} />
              <output>{draft.terminal.scrollback.toLocaleString()}</output>
            </label>
          </div>
        </section>

        <section className="settings-section" aria-labelledby="cursor-settings-title">
          <header><MousePointer2 size={17} /><h2 id="cursor-settings-title">Cursor and feedback</h2></header>
          <div className="settings-control-list">
            <div className="settings-control">
              <span><strong>Cursor shape</strong><small>Terminal caret</small></span>
              <div className="segmented-control settings-segments" aria-label="Cursor shape">
                {(['block', 'bar', 'underline'] as TerminalCursorStyle[]).map((style) => (
                  <button className={draft.terminal.cursorStyle === style ? 'is-selected' : ''} type="button" key={style} onClick={() => setTerminal('cursorStyle', style)}>{cursorLabel(style)}</button>
                ))}
              </div>
            </div>
            <label className="settings-control toggle-control simple-toggle">
              <span><strong>Blinking cursor</strong><small>Animate the terminal caret</small></span>
              <input type="checkbox" checked={draft.terminal.cursorBlink} onChange={(event) => setTerminal('cursorBlink', event.target.checked)} />
            </label>
            <label className="settings-control toggle-control">
              <span><strong>Bell attention</strong><small>Mark inactive tabs on terminal bell</small></span>
              <Bell size={15} />
              <input type="checkbox" checked={draft.terminal.bell} onChange={(event) => setTerminal('bell', event.target.checked)} />
            </label>
          </div>
        </section>

        {error && <div className="settings-error" role="alert">{error}</div>}
      </div>

      <footer className="settings-actions">
        <span>{dirty ? 'Unsaved changes' : 'Saved'}</span>
        <button className="primary-button" type="submit" disabled={busy || !dirty}><Save size={15} /> {busy ? 'Saving' : 'Save settings'}</button>
      </footer>
    </form>
  )
}

function cursorLabel(value: TerminalCursorStyle): string {
  return value.charAt(0).toUpperCase() + value.slice(1)
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}
