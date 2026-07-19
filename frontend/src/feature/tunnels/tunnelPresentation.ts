import type { TunnelConfig, TunnelKind, TunnelState } from '../../lib/bridge/types'

export function tunnelKindLabel(kind: TunnelKind): string {
  if (kind === 'dynamic') return 'SOCKS5'
  return kind.charAt(0).toUpperCase() + kind.slice(1)
}

export function tunnelRequestedEndpoint(config: TunnelConfig): string {
  return `${config.bindAddress}:${config.bindPort}`
}

export function tunnelDestinationEndpoint(config: TunnelConfig): string {
  return `${config.destinationHost}:${config.destinationPort}`
}

export function tunnelStateLabel(state: TunnelState): string {
  return state.charAt(0).toUpperCase() + state.slice(1)
}

export function isLiveTunnelState(state: TunnelState): boolean {
  return state === 'starting' || state === 'active' || state === 'retrying'
}
