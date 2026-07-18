import { memo, useEffect, useRef } from 'react'
import type { TerminalController } from './TerminalController'
import { terminalPanelId, terminalTabId } from './terminalTabIds'

interface TerminalPaneProps {
  controller: TerminalController
  active: boolean
  tabId: string
}

export const TerminalPane = memo(function TerminalPane({ controller, active, tabId }: TerminalPaneProps) {
  const host = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (host.current) {
      controller.attach(host.current)
    }
  }, [controller])

  useEffect(() => {
    controller.setVisible(active)
  }, [active, controller])

  return (
    <div
      ref={host}
      id={terminalPanelId(tabId)}
      className={`terminal-host${active ? ' is-active' : ''}`}
      role="tabpanel"
      aria-hidden={!active}
      aria-labelledby={terminalTabId(tabId)}
    />
  )
})
