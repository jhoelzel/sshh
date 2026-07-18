import { useEffect, useRef, useState } from 'react'
import { backend, onTerminalOutput } from '../../lib/bridge/client'
import type {
  FrontendLease,
  Session,
  TerminalBenchmarkBackendDiagnostics,
  TerminalBenchmarkConfig,
  TerminalBenchmarkReport,
  TerminalSettings,
} from '../../lib/bridge/types'
import { TerminalController } from './TerminalController'

const markerReady = 'SHHH_BENCH_READY'
const markerEcho = 'SHHH_BENCH_ECHO:'
const markerResize = 'SHHH_BENCH_RESIZE:'
const markerDone = 'SHHH_BENCH_DONE:'
const markerCloseFlood = 'SHHH_BENCH_CLOSE_FLOOD'
const operationTimeout = 10_000

const benchmarkSettings: TerminalSettings = {
  fontFamily: 'system-mono',
  fontSize: 13,
  lineHeight: 1.2,
  cursorStyle: 'block',
  cursorBlink: false,
  scrollback: 10_000,
  bell: false,
}

export function TerminalBenchmark() {
  const host = useRef<HTMLDivElement>(null)
  const [status, setStatus] = useState('Preparing native terminal benchmark')

  useEffect(() => {
    if (!host.current) return
    void runTerminalBenchmark(host.current, setStatus)
  }, [])

  return (
    <main className="terminal-benchmark">
      <div className="terminal-benchmark__status" role="status">{status}</div>
      <div className="terminal-benchmark__host" ref={host} />
    </main>
  )
}

async function runTerminalBenchmark(host: HTMLElement, setStatus: (status: string) => void): Promise<void> {
  const started = new Date()
  let config: TerminalBenchmarkConfig | undefined
  let lease: FrontendLease | undefined
  let session: Session | undefined
  let controller: TerminalController | undefined
  let unsubscribe: (() => void) | undefined
  const tracker = new TitleTracker()
  const report = emptyReport(started)

  try {
    config = await backend.getTerminalBenchmarkConfig()
    if (!config.enabled) throw new Error('terminal benchmark mode is disabled')
    report.payloadBytes = config.payloadBytes
    report.controller.maximumPendingBytes = config.maximumFrontendQueueBytes
    report.backend.maximumUnacknowledged = config.maximumBackendQueueBytes

    lease = await backend.attachFrontend(`terminal-benchmark-${crypto.randomUUID()}`)
    session = await backend.openTerminalBenchmark(lease.id, 100, 30)
    controller = new TerminalController(session, benchmarkSettings, {
      onBell: () => undefined,
      onError: (error) => tracker.fail(error),
      onSearchRequested: () => undefined,
      onSelectionChange: () => undefined,
      onTitle: (title) => tracker.record(title),
    })
    unsubscribe = onTerminalOutput((event) => controller?.acceptOutput(event))
    controller.attach(host)
    controller.setVisible(true)
    await controller.ready()
    await backend.activateTerminal(lease.id, session.id, session.generation)
    await tracker.wait(markerReady, operationTimeout)

    setStatus('Measuring idle input and resize latency')
    report.idleEchoMilliseconds = await measureEcho(controller, tracker, 'idle', config.minimumLatencySamples)
    report.resizeMilliseconds = await measureResize(controller, tracker, config.minimumLatencySamples)

    setStatus('Streaming 10 MiB through PTY, Go, Wails, and xterm')
    const completed = tracker.wait(`${markerDone}${config.payloadBytes}`, operationTimeout)
    const outputStarted = performance.now()
    await controller.sendText('FLOOD', true)
    const floodEcho = measureEcho(controller, tracker, 'flood', config.minimumLatencySamples)
    await completed
    report.outputDurationMilliseconds = performance.now() - outputStarted
    report.floodEchoMilliseconds = await floodEcho

    const snapshots = await waitForDrain(lease, session, controller)
    report.controller = snapshots.controller
    report.backend = mapBackendDiagnostics(snapshots.backend)

    setStatus('Measuring close responsiveness under output pressure')
    const closeFloodStarted = tracker.wait(markerCloseFlood, operationTimeout)
    await controller.sendText('CLOSE_FLOOD', true)
    await closeFloodStarted
    await delay(75)
    const closeStarted = performance.now()
    await backend.closeTerminal(lease.id, session.id, session.generation)
    report.closeDurationMilliseconds = performance.now() - closeStarted
    session = undefined
  } catch (cause) {
    report.failures.push(sanitizeFailure(cause))
  } finally {
    unsubscribe?.()
    controller?.dispose()
    if (lease && session) {
      try {
        await backend.closeTerminal(lease.id, session.id, session.generation)
      } catch (cause) {
        report.failures.push(sanitizeFailure(cause))
      }
    }
  }

  report.finishedAt = new Date().toISOString()
  if (!lease) {
    setStatus('Benchmark failed before frontend attachment')
    return
  }
  try {
    setStatus('Writing benchmark report')
    const completed = await backend.completeTerminalBenchmark(lease.id, report)
    setStatus(completed.passed ? 'Benchmark passed' : 'Benchmark failed')
  } catch (cause) {
    setStatus(`Benchmark report failed: ${sanitizeFailure(cause)}`)
  }
}

async function measureEcho(
  controller: TerminalController,
  tracker: TitleTracker,
  phase: string,
  samples: number,
): Promise<number[]> {
  const result: number[] = []
  for (let index = 0; index < samples; index++) {
    const id = `${phase}-${index}`
    const observed = tracker.wait(`${markerEcho}${id}`, operationTimeout)
    const started = performance.now()
    await controller.sendText(`PING:${id}`, true)
    const finished = await observed
    result.push(finished - started)
  }
  return result
}

async function measureResize(
  controller: TerminalController,
  tracker: TitleTracker,
  samples: number,
): Promise<number[]> {
  const result: number[] = []
  for (let index = 0; index < samples; index++) {
    const columns = 101 + index
    const rows = 31 + index
    const observed = tracker.wait(`${markerResize}${columns}x${rows}`, operationTimeout)
    const started = performance.now()
    controller.resize(columns, rows)
    result.push((await observed) - started)
  }
  return result
}

async function waitForDrain(
  lease: FrontendLease,
  session: Session,
  controller: TerminalController,
) {
  const deadline = performance.now() + operationTimeout
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

function mapBackendDiagnostics(value: Awaited<ReturnType<typeof backend.getTerminalDiagnostics>>): TerminalBenchmarkBackendDiagnostics {
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

function emptyReport(started: Date): TerminalBenchmarkReport {
  return {
    schemaVersion: 1,
    startedAt: started.toISOString(),
    finishedAt: started.toISOString(),
    payloadBytes: 0,
    outputDurationMilliseconds: 0,
    idleEchoMilliseconds: [],
    floodEchoMilliseconds: [],
    resizeMilliseconds: [],
    idleEchoP95Milliseconds: 0,
    floodEchoP95Milliseconds: 0,
    resizeP95Milliseconds: 0,
    closeDurationMilliseconds: 0,
    controller: {
      acceptedSequence: 0, acceptedBytes: 0, consumedSequence: 0, consumedBytes: 0,
      acknowledgedSequence: 0, pendingBytes: 0, peakPendingBytes: 0,
      maximumPendingBytes: 0, outputFailed: false,
    },
    backend: {
      nextSequence: 0, emittedBytes: 0, acknowledgedSequence: 0, acknowledgedBytes: 0,
      unacknowledgedBytes: 0, pendingChunks: 0, peakUnacknowledgedBytes: 0,
      peakPendingChunks: 0, maximumUnacknowledged: 0,
    },
    runtime: { operatingSystem: '', architecture: '', goVersion: '', processId: 0 },
    host: {
      model: '', processor: '', operatingSystemVersion: '', memoryBytes: 0,
      processTreePeakRssBytes: 0, processTreePeakProcesses: 0,
      webKitPeakProcesses: 0, rssSamples: 0,
    },
    passed: false,
    failures: [],
  }
}

class TitleTracker {
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

function sanitizeFailure(cause: unknown): string {
  const value = cause instanceof Error ? cause.message : String(cause)
  return value.replace(/[\r\n]+/g, ' ').slice(0, 240) || 'unknown terminal benchmark failure'
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds))
}
