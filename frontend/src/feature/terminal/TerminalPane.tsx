import { memo, useEffect, useRef, type CSSProperties } from 'react'
import type { TerminalController } from './TerminalController'
import { terminalPanelId, terminalTabId } from './terminalTabIds'

interface TerminalPaneProps {
  controller: TerminalController
  visible: boolean
  selected: boolean
  tabId: string
  style?: CSSProperties
}

export const TerminalPane = memo(function TerminalPane({ controller, visible, selected, tabId, style }: TerminalPaneProps) {
  const host = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (host.current) {
      controller.attach(host.current)
    }
  }, [controller])

  useEffect(() => {
    controller.setVisible(visible)
  }, [controller, visible])

  useEffect(() => {
    if (visible && selected) controller.focus()
  }, [controller, selected, visible])

  return (
    <div
      ref={host}
      id={terminalPanelId(tabId)}
      className={`terminal-host${visible ? ' is-visible' : ''}${selected ? ' is-selected' : ''}`}
      role="tabpanel"
      aria-hidden={!visible}
      aria-labelledby={terminalTabId(tabId)}
      style={style}
    />
  )
})
