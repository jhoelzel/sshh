import { useEffect, useRef, useState } from 'react'
import { ClipboardGetText, ClipboardSetText } from '../../../wailsjs/runtime/runtime'
import { backend, onTerminalOutput } from '../../lib/bridge/client'
import type {
  FrontendLease,
  Session,
  TerminalBenchmarkConfig,
  TerminalBenchmarkReport,
} from '../../lib/bridge/types'
import { TerminalController } from './TerminalController'
import { runTerminalSoak } from './TerminalSoakBenchmark'
import {
  benchmarkSettings,
  delay,
  mapBackendDiagnostics,
  markerCloseFlood,
  markerDone,
  markerEcho,
  markerReady,
  markerResize,
  operationTimeout,
  sanitizeFailure,
  smokeOperationTimeout,
  TitleTracker,
  waitForDrain,
} from './terminalBenchmarkSupport'

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
  let config: TerminalBenchmarkConfig
  try {
    config = await backend.getTerminalBenchmarkConfig()
    if (!config.enabled) throw new Error('terminal benchmark mode is disabled')
  } catch (cause) {
    setStatus(`Benchmark configuration failed: ${sanitizeFailure(cause)}`)
    return
  }
  if (config.mode === 'soak') {
    await runTerminalSoak(host, setStatus, config)
    return
  }

  let lease: FrontendLease | undefined
  let session: Session | undefined
  let controller: TerminalController | undefined
  let unsubscribe: (() => void) | undefined
  let leaseRenewal: number | undefined
  const tracker = new TitleTracker()
  const report = emptyReport(started)
  const timeout = config.mode === 'smoke' ? smokeOperationTimeout : operationTimeout

  try {
    report.payloadBytes = config.payloadBytes
    report.controller.maximumPendingBytes = config.maximumFrontendQueueBytes
    report.backend.maximumUnacknowledged = config.maximumBackendQueueBytes

    lease = await backend.attachFrontend(`terminal-benchmark-${crypto.randomUUID()}`)
    if (config.mode === 'smoke') {
      leaseRenewal = window.setInterval(() => {
        const current = lease
        if (!current) return
        void backend.renewFrontendLease(current.id).catch((cause) => {
          tracker.fail(new Error(sanitizeFailure(cause)))
        })
      }, 5_000)
    }
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
    await tracker.wait(markerReady, timeout)

    setStatus('Checking native terminal focus and clipboard')
    report.native = await checkNativeInteractions(controller)

    setStatus('Measuring idle input and resize latency')
    report.idleEchoMilliseconds = await measureEcho(controller, tracker, 'idle', config.minimumLatencySamples, timeout)
    report.resizeMilliseconds = await measureResize(controller, tracker, config.minimumLatencySamples, timeout)

    setStatus('Streaming fixture output through PTY, Go, Wails, and xterm')
    const completed = tracker.wait(`${markerDone}${config.payloadBytes}`, timeout)
    const outputStarted = performance.now()
    await controller.sendText(`FLOOD:${config.payloadBytes}`, true)
    const floodEcho = measureEcho(controller, tracker, 'flood', config.minimumLatencySamples, timeout)
    await completed
    report.outputDurationMilliseconds = performance.now() - outputStarted
    report.floodEchoMilliseconds = await floodEcho

    const snapshots = await waitForDrain(lease, session, controller, timeout)
    report.controller = snapshots.controller
    report.backend = mapBackendDiagnostics(snapshots.backend)

    setStatus('Measuring close responsiveness under output pressure')
    const closeFloodStarted = tracker.wait(markerCloseFlood, timeout)
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
    if (leaseRenewal !== undefined) window.clearInterval(leaseRenewal)
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

async function checkNativeInteractions(controller: TerminalController): Promise<TerminalBenchmarkReport['native']> {
  return {
    terminalFocus: await checkTerminalFocus(controller),
    clipboardRoundTrip: await checkClipboardRoundTrip(),
  }
}

async function checkTerminalFocus(controller: TerminalController): Promise<boolean> {
  const probe = document.createElement('input')
  probe.type = 'text'
  probe.tabIndex = -1
  probe.style.cssText = 'position:fixed;left:-10000px;top:0;width:1px;height:1px;opacity:0'
  document.body.append(probe)
  try {
    probe.focus()
    if (document.activeElement !== probe) return false
    controller.focus()
    await nextAnimationFrame()
    return controller.hasFocus()
  } finally {
    probe.remove()
  }
}

async function checkClipboardRoundTrip(): Promise<boolean> {
  let previous: string
  try {
    previous = await ClipboardGetText()
  } catch {
    return false
  }

  let passed = false
  try {
    const marker = `shhh-native-clipboard-${crypto.randomUUID()}`
    passed = await ClipboardSetText(marker) && await ClipboardGetText() === marker
  } catch {
    passed = false
  } finally {
    try {
      passed = await ClipboardSetText(previous) && passed
    } catch {
      passed = false
    }
  }
  return passed
}

function nextAnimationFrame(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()))
}

async function measureEcho(
  controller: TerminalController,
  tracker: TitleTracker,
  phase: string,
  samples: number,
  timeout: number,
): Promise<number[]> {
  const result: number[] = []
  for (let index = 0; index < samples; index++) {
    const id = `${phase}-${index}`
    const observed = tracker.wait(`${markerEcho}${id}`, timeout)
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
  timeout: number,
): Promise<number[]> {
  const result: number[] = []
  for (let index = 0; index < samples; index++) {
    const columns = 101 + index
    const rows = 31 + index
    const observed = tracker.wait(`${markerResize}${columns}x${rows}`, timeout)
    const started = performance.now()
    controller.resize(columns, rows)
    result.push((await observed) - started)
  }
  return result
}

function emptyReport(started: Date): TerminalBenchmarkReport {
  return {
    schemaVersion: 2,
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
    native: { terminalFocus: false, clipboardRoundTrip: false },
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
