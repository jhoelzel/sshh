import { useEffect, useRef } from 'react'
import type { TerminalController } from './TerminalController'

interface TerminalPaneProps {
  controller: TerminalController
  active: boolean
}

export function TerminalPane({ controller, active }: TerminalPaneProps) {
  const host = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (host.current) {
      controller.attach(host.current)
    }
  }, [controller])

  useEffect(() => {
    controller.setVisible(active)
  }, [active, controller])

  return <div ref={host} className={`terminal-host${active ? ' is-active' : ''}`} aria-hidden={!active} />
}
