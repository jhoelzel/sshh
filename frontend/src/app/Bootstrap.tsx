import { Component, lazy, StrictMode, Suspense, type ErrorInfo, type ReactNode } from 'react'

const App = lazy(async () => {
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
  return (
    <StrictMode>
      <StartupBoundary>
        <Suspense fallback={<main className="startup-loading">Starting shh-h...</main>}>
          <App />
        </Suspense>
      </StartupBoundary>
    </StrictMode>
  )
}
