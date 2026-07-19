import { Quit } from '../../../wailsjs/runtime/runtime'
import { backend, onCloseRequested, onTerminalOutput } from '../../lib/bridge/client'
import type { FrontendLease, Session, TerminalBenchmarkConfig } from '../../lib/bridge/types'
import { TerminalController } from './TerminalController'
import {
  benchmarkSettings,
  delay,
  markerReady,
  sanitizeFailure,
  smokeOperationTimeout,
  TitleTracker,
} from './terminalBenchmarkSupport'

const processObservationDelay = 250
const closeDecisionDelay = 250

export async function runTerminalLifecycleSmoke(
  host: HTMLElement,
  setStatus: (status: string) => void,
  config: TerminalBenchmarkConfig,
): Promise<void> {
  if (config.mode !== 'lifecycle') {
    setStatus('Native lifecycle smoke mode is not configured')
    return
  }

  let lease: FrontendLease | undefined
  let session: Session | undefined
  let controller: TerminalController | undefined
  let unsubscribeOutput: (() => void) | undefined
  let closeWaiter: CloseWaiter | undefined
  const tracker = new TitleTracker()

  try {
    setStatus('Attaching native lifecycle smoke frontend')
    lease = await backend.attachFrontend(`terminal-lifecycle-${crypto.randomUUID()}`)
    await backend.recordTerminalBenchmarkProgress(lease.id, 'frontend-attached', 1)

    setStatus('Opening lifecycle PTY fixture')
    session = await backend.openTerminalBenchmark(lease.id, 100, 30)
    controller = new TerminalController(session, benchmarkSettings, {
      onBell: () => undefined,
      onError: (error) => tracker.fail(error),
      onLinkRequested: () => undefined,
      onSearchRequested: () => undefined,
      onSelectionChange: () => undefined,
      onTitle: (title) => tracker.record(title),
    })
    unsubscribeOutput = onTerminalOutput((event) => controller?.acceptOutput(event))
    controller.attach(host)
    controller.setVisible(true)
    await controller.ready()
    await backend.activateTerminal(lease.id, session.id, session.generation)
    await tracker.wait(markerReady, smokeOperationTimeout)
    await backend.recordTerminalBenchmarkProgress(lease.id, 'terminal-opened', 1)

    // Give the external process sampler several opportunities to observe both
    // the native WebView helper and the live PTY fixture child.
    await delay(processObservationDelay)

    setStatus('Requesting native window close with a live terminal')
    closeWaiter = waitForCloseRequest(smokeOperationTimeout)
    const closeStarted = performance.now()
    Quit()
    await closeWaiter.promise
    await backend.recordTerminalBenchmarkProgress(lease.id, 'close-requested', 1)

    // Continuing to execute after this delay proves that OnBeforeClose kept the
    // native window and frontend alive while the decision was pending.
    await delay(closeDecisionDelay)
    const decisionDelay = Math.max(1, Math.round(performance.now() - closeStarted))
    await backend.recordTerminalBenchmarkProgress(lease.id, 'confirming', decisionDelay)

    setStatus('Confirming coordinated native shutdown')
    await backend.confirmApplicationClose(lease.id)
  } catch (cause) {
    setStatus(`Native lifecycle smoke failed: ${sanitizeFailure(cause)}`)
    if (lease) {
      await backend.recordTerminalBenchmarkProgress(lease.id, 'failed', 1).catch(() => undefined)
      await backend.confirmApplicationClose(lease.id).catch(() => Quit())
    } else {
      Quit()
    }
  } finally {
    closeWaiter?.cancel()
    unsubscribeOutput?.()
    controller?.dispose()
  }
}

interface CloseWaiter {
  promise: Promise<void>
  cancel(): void
}

function waitForCloseRequest(timeout: number): CloseWaiter {
  let dispose: () => void = () => undefined
  let timer = 0
  const promise = new Promise<void>((resolve, reject) => {
    timer = window.setTimeout(() => {
      dispose()
      reject(new Error('timed out waiting for native close interception'))
    }, timeout)
    dispose = onCloseRequested(() => {
      window.clearTimeout(timer)
      dispose()
      resolve()
    })
  })
  return {
    promise,
    cancel: () => {
      window.clearTimeout(timer)
      dispose()
    },
  }
}
