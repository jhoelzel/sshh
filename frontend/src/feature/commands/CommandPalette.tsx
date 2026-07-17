import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { Search, X, type LucideIcon } from 'lucide-react'

export interface PaletteCommand {
  id: string
  label: string
  group: string
  icon: LucideIcon
  run: () => void
  keywords?: string[]
  disabled?: boolean
}

interface CommandPaletteProps {
  commands: PaletteCommand[]
  onClose: () => void
}

export function CommandPalette({ commands, onClose }: CommandPaletteProps) {
  const [query, setQuery] = useState('')
  const [preferredActiveId, setPreferredActiveId] = useState<string>()
  const resultsRef = useRef<HTMLDivElement>(null)
  const filtered = useMemo(() => filterCommands(commands, query), [commands, query])
  const selectableIds = filtered.filter((item) => !item.disabled).map((item) => item.id)
  const activeId = preferredActiveId && selectableIds.includes(preferredActiveId)
    ? preferredActiveId
    : selectableIds[0]

  useEffect(() => {
    if (!activeId) return
    resultsRef.current
      ?.querySelector<HTMLElement>(`#command-${activeId}`)
      ?.scrollIntoView?.({ block: 'nearest' })
  }, [activeId])

  const execute = (command: PaletteCommand | undefined) => {
    if (!command || command.disabled) return
    onClose()
    command.run()
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (event.key === 'Enter') {
      event.preventDefault()
      execute(filtered.find((item) => item.id === activeId))
      return
    }
    if ((event.key !== 'ArrowDown' && event.key !== 'ArrowUp') || selectableIds.length === 0) return
    event.preventDefault()
    const currentIndex = Math.max(0, selectableIds.indexOf(activeId ?? ''))
    const direction = event.key === 'ArrowDown' ? 1 : -1
    const nextIndex = (currentIndex + direction + selectableIds.length) % selectableIds.length
    setPreferredActiveId(selectableIds[nextIndex])
  }

  const groups = groupCommands(filtered)

  return (
    <div className="modal-backdrop command-palette-backdrop" role="presentation">
      <section
        className="command-palette"
        role="dialog"
        aria-modal="true"
        aria-labelledby="command-palette-title"
        onKeyDown={handleKeyDown}
      >
        <header className="command-palette-search">
          <Search size={17} aria-hidden="true" />
          <input
            autoFocus
            aria-label="Search commands"
            aria-controls="command-palette-results"
            aria-activedescendant={activeId ? `command-${activeId}` : undefined}
            placeholder="Search commands"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <button className="icon-button compact" type="button" title="Close" aria-label="Close command palette" onClick={onClose}>
            <X size={15} />
          </button>
        </header>

        <h2 id="command-palette-title" className="visually-hidden">Command palette</h2>
        <div ref={resultsRef} id="command-palette-results" className="command-palette-results" role="listbox" aria-label="Commands">
          {groups.map(([group, items]) => (
            <section className="command-group" role="group" aria-label={group} key={group}>
              <div className="command-group-label">{group}</div>
              {items.map((item) => {
                const Icon = item.icon
                return (
                  <button
                    id={`command-${item.id}`}
                    className={`command-item${item.id === activeId ? ' is-active' : ''}`}
                    type="button"
                    role="option"
                    aria-selected={item.id === activeId}
                    disabled={item.disabled}
                    key={item.id}
                    onMouseEnter={() => {
                      if (!item.disabled) setPreferredActiveId(item.id)
                    }}
                    onClick={() => execute(item)}
                  >
                    <Icon size={16} aria-hidden="true" />
                    <span>{item.label}</span>
                  </button>
                )
              })}
            </section>
          ))}
          {filtered.length === 0 && <div className="command-palette-empty">No matching commands</div>}
        </div>
      </section>
    </div>
  )
}

function filterCommands(commands: PaletteCommand[], query: string): PaletteCommand[] {
  const normalized = query.trim().toLocaleLowerCase()
  if (!normalized) return commands
  return commands.filter((command) =>
    [command.label, command.group, ...(command.keywords ?? [])]
      .some((value) => value.toLocaleLowerCase().includes(normalized)),
  )
}

function groupCommands(commands: PaletteCommand[]): Array<[string, PaletteCommand[]]> {
  const groups = new Map<string, PaletteCommand[]>()
  for (const command of commands) {
    groups.set(command.group, [...(groups.get(command.group) ?? []), command])
  }
  return [...groups.entries()]
}
