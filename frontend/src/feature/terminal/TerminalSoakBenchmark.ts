import { backend, onTerminalOutput } from '../../lib/bridge/client'
import type {
  FrontendLease,
  Session,
  TerminalBenchmarkConfig,
  TerminalSoakReport,
} from '../../lib/bridge/types'
import { TerminalController } from './TerminalController'
import {
  benchmarkSettings,
  delay,
  mapBackendDiagnostics,
  markerEcho,
  markerReady,
  markerSoakDone,
  markerSoakStarted,
  operationTimeout,
  sanitizeFailure,
  soakOperationTimeout,
  TitleTracker,
  waitForDrain,
} from './terminalBenchmarkSupport'

interface SoakRuntime {
  index: number
  session: Session
  controller: TerminalController
  tracker: TitleTracker
  host: HTMLDivElement
  closed: boolean
}

export async function runTerminalSoak(
  host: HTMLElement,
  setStatus: (status: string) => void,
  config: TerminalBenchmarkConfig,
): Promise<void> {
  const report = emptySoakReport(new Date())
  const runtimes: SoakRuntime[] = []
  const controllers = new Map<string, TerminalController>()
  let lease: FrontendLease | undefined
  let unsubscribe: (() => void) | undefined

  try {
    if (config.soakSessionCount < 1 || config.soakDurationMilliseconds < 1) {
      throw new Error('terminal soak configuration is invalid')
    }
    host.replaceChildren()
    unsubscribe = onTerminalOutput((event) => controllers.get(event.sessionId)?.acceptOutput(event))
    lease = await backend.attachFrontend(`terminal-soak-${crypto.randomUUID()}`)

    setStatus(`Opening ${config.soakSessionCount} native terminal sessions`)
    for (let index = 0; index < config.soakSessionCount; index++) {
      const sessionHost = document.createElement('div')
      sessionHost.className = `terminal-benchmark__session${index === 0 ? ' is-active' : ''}`
      sessionHost.setAttribute('aria-hidden', index === 0 ? 'false' : 'true')
      host.append(sessionHost)

      const session = await backend.openTerminalBenchmark(lease.id, 100, 30)
      const tracker = new TitleTracker()
      const controller = new TerminalController(session, benchmarkSettings, {
        onBell: () => undefined,
        onError: (error) => tracker.fail(error),
        onLinkRequested: () => undefined,
        onSearchRequested: () => undefined,
        onSelectionChange: () => undefined,
        onTitle: (title) => tracker.record(title),
      })
      const runtime = { index, session, controller, tracker, host: sessionHost, closed: false }
      runtimes.push(runtime)
      controllers.set(session.id, controller)
      controller.attach(sessionHost)
      controller.setVisible(index === 0)
      await controller.ready()
      await backend.activateTerminal(lease.id, session.id, session.generation)
      await tracker.wait(markerReady, operationTimeout)
      await backend.recordTerminalBenchmarkProgress(lease.id, 'opening', index + 1)
    }

    const startedMarkers = runtimes.map((runtime) => runtime.tracker.wait(markerSoakStarted, soakOperationTimeout))
    await Promise.all(runtimes.map((runtime) => runtime.controller.sendText('SOAK', true)))
    await Promise.all(startedMarkers)

    const soakStarted = performance.now()
    const deadline = soakStarted + config.soakDurationMilliseconds
    let cycle = 0
    while (performance.now() < deadline) {
      setActiveRuntime(runtimes, cycle % runtimes.length)
      report.visibilitySwitches++
      report.echoMilliseconds.push(...await measureEchoCycle(runtimes, cycle))
      cycle++

      if (cycle === 1 || cycle % 12 === 0) {
        const elapsedMinutes = Math.min(performance.now() - soakStarted, config.soakDurationMilliseconds) / 60_000
        setStatus(`Soaking ${runtimes.length} terminals: ${elapsedMinutes.toFixed(1)} minutes elapsed`)
        await backend.recordTerminalBenchmarkProgress(lease.id, 'running', cycle)
      }
      const nextCycle = Math.min(soakStarted + cycle * config.soakHeartbeatMilliseconds, deadline)
      await delay(Math.max(0, nextCycle - performance.now()))
    }
    report.durationMilliseconds = performance.now() - soakStarted

    setStatus('Stopping output and draining every terminal queue')
    await backend.recordTerminalBenchmarkProgress(lease.id, 'stopping', cycle)
    const doneMarkers = runtimes.map((runtime) => runtime.tracker.wait(markerSoakDone, soakOperationTimeout))
    await Promise.all(runtimes.map((runtime) => runtime.controller.sendText('STOP_SOAK', true)))
    await Promise.all(doneMarkers)
    await backend.recordTerminalBenchmarkProgress(lease.id, 'draining', runtimes.length)
    const snapshots = await Promise.all(runtimes.map((runtime) =>
      waitForDrain(lease!, runtime.session, runtime.controller, soakOperationTimeout),
    ))

    report.sessionCount = runtimes.length
    report.sessions = snapshots.map((snapshot, index) => ({
      index,
      closeDurationMilliseconds: 0,
      controller: snapshot.controller,
      backend: mapBackendDiagnostics(snapshot.backend),
    }))
    report.totalBytes = report.sessions.reduce((total, session) => total + session.controller.acceptedBytes, 0)

    setStatus('Closing all soak terminals')
    for (const runtime of runtimes) {
      const closeStarted = performance.now()
      await backend.closeTerminal(lease.id, runtime.session.id, runtime.session.generation)
      report.sessions[runtime.index].closeDurationMilliseconds = performance.now() - closeStarted
      runtime.closed = true
      controllers.delete(runtime.session.id)
      await backend.recordTerminalBenchmarkProgress(lease.id, 'closing', runtime.index + 1)
    }
  } catch (cause) {
    report.failures.push(sanitizeFailure(cause))
    if (lease) {
      try {
        await backend.recordTerminalBenchmarkProgress(lease.id, 'failed', report.failures.length)
      } catch {
        // Preserve the original benchmark failure.
      }
    }
  } finally {
    unsubscribe?.()
    for (const runtime of runtimes) {
      runtime.controller.dispose()
      if (lease && !runtime.closed) {
        try {
          await backend.closeTerminal(lease.id, runtime.session.id, runtime.session.generation)
          runtime.closed = true
        } catch (cause) {
          report.failures.push(sanitizeFailure(cause))
        }
      }
    }
  }

  report.finishedAt = new Date().toISOString()
  if (!lease) {
    setStatus('Terminal soak failed before frontend attachment')
    return
  }
  try {
    setStatus('Writing terminal soak report')
    await backend.recordTerminalBenchmarkProgress(lease.id, 'completing', report.sessions.length)
    const completed = await backend.completeTerminalSoak(lease.id, report)
    setStatus(completed.passed ? 'Terminal soak passed' : 'Terminal soak failed')
  } catch (cause) {
    setStatus(`Terminal soak report failed: ${sanitizeFailure(cause)}`)
  }
}

async function measureEchoCycle(runtimes: SoakRuntime[], cycle: number): Promise<number[]> {
  return Promise.all(runtimes.map(async (runtime) => {
    const id = `soak-${cycle}-${runtime.index}`
    const observed = runtime.tracker.wait(`${markerEcho}${id}`, soakOperationTimeout)
    const started = performance.now()
    await runtime.controller.sendText(`PING:${id}`, true)
    return (await observed) - started
  }))
}

function setActiveRuntime(runtimes: SoakRuntime[], activeIndex: number): void {
  for (const runtime of runtimes) {
    const active = runtime.index === activeIndex
    runtime.host.classList.toggle('is-active', active)
    runtime.host.setAttribute('aria-hidden', active ? 'false' : 'true')
    runtime.controller.setVisible(active)
  }
}

function emptySoakReport(started: Date): TerminalSoakReport {
  return {
    schemaVersion: 1,
    startedAt: started.toISOString(),
    finishedAt: started.toISOString(),
    durationMilliseconds: 0,
    sessionCount: 0,
    visibilitySwitches: 0,
    totalBytes: 0,
    echoMilliseconds: [],
    echoP95Milliseconds: 0,
    closeP95Milliseconds: 0,
    sessions: [],
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
