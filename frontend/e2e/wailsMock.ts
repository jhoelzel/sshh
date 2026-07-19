import type { Page } from '@playwright/test'

interface BrowserHarness {
  callCount(name: string): number
  emit(name: string, ...args: unknown[]): void
}

interface HarnessWindow extends Window {
  __SHHH_E2E__: BrowserHarness
  go: {
    bridge: {
      Desktop: Record<string, (...args: unknown[]) => Promise<unknown>>
    }
  }
  runtime: Record<string, (...args: unknown[]) => unknown>
}

export async function installWailsMock(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const harnessWindow = window as unknown as HarnessWindow
    const calls: Record<string, unknown[][]> = Object.create(null)
    const listeners = new Map<string, Set<(...args: unknown[]) => void>>()
    let clipboard = ''
    let sessionSequence = 0

    const lease = { id: 'lease-e2e', expiresAt: '2026-07-19T12:00:00Z' }
    const profile = {
      id: 'local-e2e',
      name: 'Local Shell',
      protocol: 'local',
      host: '',
      port: 0,
      username: '',
      authentication: 'auto',
      identityFile: '',
      shell: '',
      arguments: [],
      workingDirectory: '',
      environment: {},
      tags: [],
      group: '',
      favorite: true,
      endpoint: 'Local shell',
      connectable: true,
    }
    const settings = {
      terminal: {
        fontFamily: 'system-mono',
        fontSize: 13,
        lineHeight: 1.2,
        cursorStyle: 'block',
        cursorBlink: true,
        scrollback: 10_000,
        bell: true,
      },
      connection: {
        connectTimeoutSeconds: 15,
        keepAliveEnabled: true,
        keepAliveIntervalSeconds: 30,
        keepAliveMaxFailures: 3,
      },
      notifications: {
        enabled: false,
        transferCompleted: true,
        unexpectedDisconnect: true,
        longTransferSeconds: 30,
      },
      transfers: { concurrency: 2, collisionPolicy: 'ask', keepPartialFiles: false },
      ui: { theme: 'dark', sidebarWidth: 272, workspace: 'terminals' },
    }

    const record = (name: string, args: unknown[]) => {
      const entries = calls[name] ?? []
      entries.push(args)
      calls[name] = entries
    }
    const emit = (name: string, ...args: unknown[]) => {
      for (const callback of [...(listeners.get(name) ?? [])]) callback(...args)
    }
    const invoke = async (name: string, args: unknown[]): Promise<unknown> => {
      record(name, args)
      switch (name) {
        case 'AwaitReady':
          return undefined
        case 'AttachFrontend':
        case 'RenewFrontendLease':
          return { ...lease }
        case 'GetBuildInfo':
          return {
            version: '0.1.0-e2e',
            commit: 'playwright',
            buildDate: '2026-07-19T10:00:00Z',
            dirty: false,
            goVersion: 'go1.26.5',
            platform: 'browser/mock',
          }
        case 'GetNotificationStatus':
          return { available: false, authorized: false, message: 'Unavailable in browser tests' }
        case 'GetSettings':
          return structuredClone(settings)
        case 'ListProfiles':
          return [structuredClone(profile)]
        case 'ListRemotePathFavorites':
        case 'ListSnippets':
        case 'ListTransfers':
        case 'ListTunnels':
        case 'ListTunnelStates':
        case 'ListWorkspaceLayouts':
          return []
        case 'OpenLocalTerminal': {
          sessionSequence += 1
          return {
            id: `session-${sessionSequence}`,
            generation: 1,
            leaseId: lease.id,
            profileId: profile.id,
            title: `Local ${sessionSequence}`,
            state: 'running',
            columns: 100,
            rows: 30,
            startedAt: '2026-07-19T10:00:00Z',
          }
        }
        case 'UpdateUIPreferences': {
          const update = args[0] as Partial<typeof settings.ui>
          settings.ui = { ...settings.ui, ...update }
          return structuredClone(settings.ui)
        }
        case 'UpdateSettings':
          return structuredClone(args[0])
        case 'ResetSettings':
          return structuredClone(settings)
        default:
          return undefined
      }
    }

    const desktop = new Proxy({}, {
      get: (_target, property) => (...args: unknown[]) => invoke(String(property), args),
    }) as Record<string, (...args: unknown[]) => Promise<unknown>>

    const runtime = {
      EventsOnMultiple: (name: string, callback: (...args: unknown[]) => void) => {
        const callbacks = listeners.get(name) ?? new Set()
        callbacks.add(callback)
        listeners.set(name, callbacks)
        return () => callbacks.delete(callback)
      },
      EventsOff: (...names: string[]) => names.forEach((name) => listeners.delete(name)),
      EventsOffAll: () => listeners.clear(),
      EventsEmit: emit,
      ClipboardGetText: async () => clipboard,
      ClipboardSetText: async (value: string) => {
        clipboard = value
        return true
      },
      Environment: async () => ({ buildType: 'e2e', platform: 'browser', arch: 'mock' }),
    }
    harnessWindow.runtime = new Proxy(runtime, {
      get: (target, property) => Reflect.get(target, property) ?? (() => undefined),
    }) as Record<string, (...args: unknown[]) => unknown>
    harnessWindow.go = { bridge: { Desktop: desktop } }
    harnessWindow.__SHHH_E2E__ = {
      callCount: (name) => calls[name]?.length ?? 0,
      emit,
    }
  })
}

export async function backendCallCount(page: Page, name: string): Promise<number> {
  return page.evaluate(
    (method) => (window as unknown as HarnessWindow).__SHHH_E2E__.callCount(method),
    name,
  )
}

export async function emitBackendEvent(page: Page, name: string): Promise<void> {
  await page.evaluate(
    (eventName) => (window as unknown as HarnessWindow).__SHHH_E2E__.emit(eventName),
    name,
  )
}
