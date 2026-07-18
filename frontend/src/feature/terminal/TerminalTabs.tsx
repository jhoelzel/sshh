import { useRef, useState, type DragEvent, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'
import type { TabDropPosition } from '../../app/workspaces'
import { terminalPanelId, terminalTabId } from './terminalTabIds'

export interface TerminalTabItem {
  id: string
  title: string
  state: string
  attention: boolean
}

interface TerminalTabsProps {
  tabs: TerminalTabItem[]
  activeId?: string
  onSelect: (tabId: string) => void
  onClose: (tabId: string) => void
  onReorder: (sourceId: string, targetId: string, position: TabDropPosition) => void
}

interface DropTarget {
  id: string
  position: TabDropPosition
}

const tabDragType = 'application/x-shhh-terminal-tab'

export function TerminalTabs({ tabs, activeId, onSelect, onClose, onReorder }: TerminalTabsProps) {
  const tabButtons = useRef(new Map<string, HTMLButtonElement>())
  const dragSourceId = useRef<string | undefined>(undefined)
  const [draggedId, setDraggedId] = useState<string>()
  const [dropTarget, setDropTarget] = useState<DropTarget>()

  const focusTab = (index: number) => {
    const tab = tabs[index]
    if (tab) tabButtons.current.get(tab.id)?.focus()
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let target: number | undefined
    if (event.key === 'ArrowLeft') target = (index - 1 + tabs.length) % tabs.length
    if (event.key === 'ArrowRight') target = (index + 1) % tabs.length
    if (event.key === 'Home') target = 0
    if (event.key === 'End') target = tabs.length - 1
    if (target === undefined) return
    event.preventDefault()
    focusTab(target)
  }

  const beginDrag = (event: DragEvent<HTMLButtonElement>, tabId: string) => {
    dragSourceId.current = tabId
    setDraggedId(tabId)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData(tabDragType, tabId)
  }

  const updateDropTarget = (event: DragEvent<HTMLDivElement>, targetId: string) => {
    if (!dragSourceId.current || dragSourceId.current === targetId) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    const bounds = event.currentTarget.getBoundingClientRect()
    const position = event.clientX < bounds.left + bounds.width / 2 ? 'before' : 'after'
    setDropTarget({ id: targetId, position })
  }

  const finishDrag = () => {
    dragSourceId.current = undefined
    setDraggedId(undefined)
    setDropTarget(undefined)
  }

  const drop = (event: DragEvent<HTMLDivElement>, targetId: string) => {
    event.preventDefault()
    const sourceId = event.dataTransfer.getData(tabDragType) || dragSourceId.current
    const position = dropTarget?.id === targetId ? dropTarget.position : 'after'
    if (sourceId && sourceId !== targetId) onReorder(sourceId, targetId, position)
    finishDrag()
  }

  return (
    <div className="tabs" role="tablist" aria-label="Terminal sessions" aria-orientation="horizontal">
      {tabs.map((tab, index) => (
        <div
          className={`tab${tab.id === activeId ? ' is-active' : ''}${tab.id === draggedId ? ' is-dragging' : ''}${dropTarget?.id === tab.id ? ` is-drop-${dropTarget.position}` : ''}`}
          key={tab.id}
          onDragOver={(event) => updateDropTarget(event, tab.id)}
          onDrop={(event) => drop(event, tab.id)}
        >
          <button
            ref={(element) => {
              if (element) tabButtons.current.set(tab.id, element)
              else tabButtons.current.delete(tab.id)
            }}
            id={terminalTabId(tab.id)}
            className="tab-select"
            type="button"
            role="tab"
            aria-controls={terminalPanelId(tab.id)}
            aria-selected={tab.id === activeId}
            tabIndex={tab.id === activeId ? 0 : -1}
            title={`${tab.title}. Drag to reorder`}
            draggable
            onClick={() => onSelect(tab.id)}
            onDragEnd={finishDrag}
            onDragStart={(event) => beginDrag(event, tab.id)}
            onKeyDown={(event) => handleKeyDown(event, index)}
          >
            <span className={`state-dot state-${tab.state}${tab.attention ? ' has-attention' : ''}`} />
            <span className="tab-title">{tab.title}</span>
          </button>
          <button
            className="tab-close"
            type="button"
            title="Close terminal"
            aria-label={`Close ${tab.title}`}
            tabIndex={tab.id === activeId ? 0 : -1}
            onClick={() => onClose(tab.id)}
          >
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  )
}
