import { describe, expect, it } from 'vitest'
import type { Transfer } from '../../lib/bridge/types'
import {
  formatBytes,
  pathBaseName,
  progressFromBytes,
  transferLabel,
  transferProgressPercent,
} from './transferPresentation'

describe('transfer presentation', () => {
  it('formats names, byte counts, and bounded progress consistently', () => {
    expect(pathBaseName('/remote/archive.tar')).toBe('archive.tar')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(progressFromBytes(150, 100)).toBe(100)
    expect(progressFromBytes(50, 0)).toBe(0)
  })

  it('uses progress for running transfers and preserves useful failure messages', () => {
    const running = transfer({ state: 'running', bytes: 50, total: 100 })
    const failed = transfer({ state: 'failed', message: 'Remote disk is full' })
    const completed = transfer({ state: 'completed', bytes: 0, total: 100 })

    expect(transferProgressPercent(running)).toBe(50)
    expect(transferProgressPercent(completed)).toBe(100)
    expect(transferLabel(running)).toBe('50%')
    expect(transferLabel(failed)).toBe('Remote disk is full')
  })
})

function transfer(changes: Partial<Transfer>): Transfer {
  return {
    id: 'transfer-1', leaseId: 'lease-1', sessionId: 'files-1', direction: 'download',
    source: '/remote/archive.tar', destination: '/local/archive.tar', bytes: 0, total: 0,
    state: 'queued', message: '', resumeId: '', resumedFrom: 0, startedAt: '', finishedAt: '',
    ...changes,
  }
}
