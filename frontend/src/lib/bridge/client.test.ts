import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BackendError } from './errors'
import { backend, events, onTerminalOutput } from './client'

interface WailsTestWindow extends Window {
  runtime: {
    EventsOnMultiple: (eventName: string, callback: (...data: unknown[]) => void, maxCallbacks: number) => () => void
  }
  go: {
    bridge: {
      Desktop: {
        ListWorkspaceLayouts: () => Promise<unknown>
        RenewFrontendLease: (leaseId: string) => Promise<unknown>
      }
    }
  }
}

const wailsWindow = window as unknown as WailsTestWindow

beforeEach(() => {
  wailsWindow.runtime = {
    EventsOnMultiple: vi.fn(() => vi.fn()),
  }
  wailsWindow.go = {
    bridge: {
      Desktop: {
        ListWorkspaceLayouts: vi.fn(),
        RenewFrontendLease: vi.fn(),
      },
    },
  }
})

describe('bridge client', () => {
  it('uses the listener-specific disposer returned by Wails', () => {
    const disposeFirst = vi.fn()
    const disposeSecond = vi.fn()
    const subscribe = vi.mocked(wailsWindow.runtime.EventsOnMultiple)
      .mockReturnValueOnce(disposeFirst)
      .mockReturnValueOnce(disposeSecond)

    const first = onTerminalOutput(() => undefined)
    const second = onTerminalOutput(() => undefined)
    first()

    expect(subscribe).toHaveBeenNthCalledWith(1, events.terminalOutput, expect.any(Function), -1)
    expect(disposeFirst).toHaveBeenCalledOnce()
    expect(disposeSecond).not.toHaveBeenCalled()

    second()
    expect(disposeSecond).toHaveBeenCalledOnce()
  })

  it('normalizes structured backend rejections for every client method', async () => {
    vi.mocked(wailsWindow.go.bridge.Desktop.RenewFrontendLease).mockRejectedValue(new Error(JSON.stringify({
      code: 'stale', message: 'Frontend lease is missing or stale.', retryable: true,
    })))

    const promise = backend.renewFrontendLease('stale-lease')

    await expect(promise).rejects.toMatchObject({
      name: 'BackendError',
      code: 'stale',
      message: 'Frontend lease is missing or stale.',
      retryable: true,
    } satisfies Partial<BackendError>)
  })

  it('keeps valid workspace splits and rejects malformed split metadata', async () => {
    const base = {
      name: 'Operations',
      tabs: [
        { profileId: 'one', title: 'One', endpoint: 'one.example' },
        { profileId: 'two', title: 'Two', endpoint: 'two.example' },
      ],
      activeTab: 1,
      createdAt: '2026-07-18T20:00:00Z',
      updatedAt: '2026-07-18T20:00:00Z',
    }
    vi.mocked(wailsWindow.go.bridge.Desktop.ListWorkspaceLayouts).mockResolvedValue([
      { ...base, id: 'valid', split: { axis: 'row', primaryTab: 0, secondaryTab: 1, ratio: 0.6 } },
      { ...base, id: 'invalid', split: { axis: 'row', primaryTab: 0, secondaryTab: 1, ratio: 0.9 } },
    ])

    const layouts = await backend.listWorkspaceLayouts()

    expect(layouts[0].split).toEqual({ axis: 'row', primaryTab: 0, secondaryTab: 1, ratio: 0.6 })
    expect(layouts[1].split).toBeUndefined()
  })
})
