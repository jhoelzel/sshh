import { backend } from '../../lib/bridge/client'
import type {
  FrontendLease,
  Session,
  TerminalBenchmarkBackendDiagnostics,
  TerminalSettings,
} from '../../lib/bridge/types'
import { TerminalController } from './TerminalController'

export const markerReady = 'SHHH_BENCH_READY'
export const markerEcho = 'SHHH_BENCH_ECHO:'
export const markerResize = 'SHHH_BENCH_RESIZE:'
export const markerDone = 'SHHH_BENCH_DONE:'
export const markerCloseFlood = 'SHHH_BENCH_CLOSE_FLOOD'
export const markerSoakStarted = 'SHHH_BENCH_SOAK_STARTED'
export const markerSoakDone = 'SHHH_BENCH_SOAK_DONE'
export const operationTimeout = 10_000
export const smokeOperationTimeout = 45_000
export const soakOperationTimeout = 30_000

export const benchmarkSettings: TerminalSettings = {
  fontFamily: 'system-mono',
  fontSize: 13,
  lineHeight: 1.2,
  cursorStyle: 'block',
  cursorBlink: false,
  scrollback: 10_000,
  bell: false,
}

export async function waitForDrain(
  lease: FrontendLease,
  session: Session,
  controller: TerminalController,
  timeout = operationTimeout,
) {
  const deadline = performance.now() + timeout
  while (performance.now() < deadline) {
    const frontend = controller.diagnostics()
    const backendDiagnostics = await backend.getTerminalDiagnostics(lease.id, session.id, session.generation)
    if (
      frontend.pendingBytes === 0 &&
      frontend.acknowledgedSequence === frontend.acceptedSequence &&
      backendDiagnostics.unacknowledgedBytes === 0 &&
      backendDiagnostics.pendingChunks === 0
    ) {
      return { controller: frontend, backend: backendDiagnostics }
    }
    await delay(16)
  }
  throw new Error('terminal output did not drain before the benchmark timeout')
}

export function mapBackendDiagnostics(
  value: Awaited<ReturnType<typeof backend.getTerminalDiagnostics>>,
): TerminalBenchmarkBackendDiagnostics {
  return {
    nextSequence: value.nextSequence,
    emittedBytes: value.emittedBytes,
    acknowledgedSequence: value.acknowledgedSequence,
    acknowledgedBytes: value.acknowledgedBytes,
    unacknowledgedBytes: value.unacknowledgedBytes,
    pendingChunks: value.pendingChunks,
    peakUnacknowledgedBytes: value.peakUnacknowledgedBytes,
    peakPendingChunks: value.peakPendingChunks,
    maximumUnacknowledged: value.maximumUnacknowledged,
  }
}

export class TitleTracker {
  private readonly seen = new Map<string, number>()
  private readonly waiting = new Map<string, Array<{ resolve: (time: number) => void; reject: (error: Error) => void }>>()
  private failure?: Error

  record(title: string): void {
    const observed = performance.now()
    this.seen.set(title, observed)
    for (const waiter of this.waiting.get(title) ?? []) waiter.resolve(observed)
    this.waiting.delete(title)
  }

  fail(error: Error): void {
    this.failure = error
    for (const waiters of this.waiting.values()) {
      for (const waiter of waiters) waiter.reject(error)
    }
    this.waiting.clear()
  }

  wait(title: string, timeout: number): Promise<number> {
    if (this.failure) return Promise.reject(this.failure)
    const observed = this.seen.get(title)
    if (observed !== undefined) return Promise.resolve(observed)
    return new Promise((resolve, reject) => {
      const waiters = this.waiting.get(title) ?? []
      const waiter = {
        resolve: (time: number) => {
          window.clearTimeout(timer)
          resolve(time)
        },
        reject: (error: Error) => {
          window.clearTimeout(timer)
          reject(error)
        },
      }
      const timer = window.setTimeout(() => {
        this.waiting.set(title, (this.waiting.get(title) ?? []).filter((candidate) => candidate !== waiter))
        reject(new Error(`timed out waiting for benchmark marker ${title}`))
      }, timeout)
      waiters.push(waiter)
      this.waiting.set(title, waiters)
    })
  }
}

export function sanitizeFailure(cause: unknown): string {
  const value = cause instanceof Error ? cause.message : String(cause)
  return value.replace(/[\r\n]+/g, ' ').slice(0, 240) || 'unknown terminal benchmark failure'
}

export function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds))
}
