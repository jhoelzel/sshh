import type { Transfer } from '../../lib/bridge/types'

export function pathBaseName(value: string): string {
  return value.split('/').filter(Boolean).at(-1) ?? value
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

export function transferProgressPercent(transfer: Transfer): number {
  if (transfer.state === 'completed') return 100
  return progressFromBytes(transfer.bytes, transfer.total)
}

export function progressFromBytes(bytes: number, total: number): number {
  return total > 0 ? Math.min(100, Math.round((bytes / total) * 100)) : 0
}

export function transferLabel(transfer: Transfer): string {
  if (transfer.state === 'running') return `${transferProgressPercent(transfer)}%`
  if (transfer.state === 'failed') return transfer.message || 'Failed'
  return transfer.state.charAt(0).toUpperCase() + transfer.state.slice(1)
}
