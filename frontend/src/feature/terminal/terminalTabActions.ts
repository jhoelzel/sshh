import type { SessionState } from '../../lib/bridge/types'

export type TerminalTabActionState = SessionState | 'disconnected'

export interface TerminalTabActionAvailability {
  retry: boolean
  reconnectInNewTab: boolean
  duplicate: boolean
  clearScrollback: boolean
  reset: boolean
}

export function terminalTabActionAvailability(
  state: TerminalTabActionState | undefined,
  hasController: boolean,
  hasConnectionDescriptor: boolean,
  connectionBusy: boolean,
): TerminalTabActionAvailability {
  const reconnectable = state === 'disconnected' || state === 'exited' || state === 'failed' || state === 'closed'
  const canConnect = hasConnectionDescriptor && !connectionBusy
  return {
    retry: reconnectable && canConnect,
    reconnectInNewTab: reconnectable && canConnect,
    duplicate: state === 'running' && canConnect,
    clearScrollback: hasController,
    reset: hasController,
  }
}
