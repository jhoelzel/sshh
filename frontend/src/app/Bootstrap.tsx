import { Component, lazy, StrictMode, Suspense, type ErrorInfo, type ReactNode } from 'react'
import { AwaitReady } from '../../wailsjs/go/bridge/Desktop'
import { asBackendError } from '../lib/bridge/errors'

const benchmarkBuild = import.meta.env.VITE_TERMINAL_BENCHMARK === '1'

const Root = lazy(async () => {
  try {
    await AwaitReady()
  } catch (cause) {
    throw asBackendError(cause)
  }
  if (benchmarkBuild) {
    const module = await import('../feature/terminal/TerminalBenchmark')
    return { default: module.TerminalBenchmark }
  }
  const module = await import('./App')
  return { default: module.App }
})

interface StartupBoundaryState {
  error?: Error
}

class StartupBoundary extends Component<{ children: ReactNode }, StartupBoundaryState> {
  state: StartupBoundaryState = {}

  static getDerivedStateFromError(error: Error): StartupBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Application startup failed', error, info.componentStack)
  }

  render() {
    if (this.state.error) {
      return (
        <main className="startup-failure" role="alert">
          <strong>shh-h could not start</strong>
          <span>{this.state.error.message || 'An unexpected frontend error occurred.'}</span>
        </main>
      )
    }
    return this.props.children
  }
}

export function Bootstrap() {
  const content = (
    <StartupBoundary>
      <Suspense fallback={<main className="startup-loading">Starting shh-h...</main>}>
        <Root />
      </Suspense>
    </StartupBoundary>
  )
  if (benchmarkBuild) return content
  return (
    <StrictMode>
      {content}
    </StrictMode>
  )
}
