import { describe, expect, it, vi } from 'vitest'
import type { UISettings } from '../../lib/bridge/types'
import {
  clampSidebarWidth,
  createUIPreferenceWriter,
  defaultSidebarWidth,
  maximumSidebarWidth,
  minimumSidebarWidth,
} from './uiPreferences'

describe('UI preferences', () => {
  it('clamps persisted sidebar dimensions to the desktop contract', () => {
    expect(clampSidebarWidth(180)).toBe(minimumSidebarWidth)
    expect(clampSidebarWidth(318.6)).toBe(319)
    expect(clampSidebarWidth(800)).toBe(maximumSidebarWidth)
    expect(clampSidebarWidth(Number.NaN)).toBe(defaultSidebarWidth)
  })

  it('serializes preference writes and applies responses in order', async () => {
    const resolvers: Array<(value: UISettings) => void> = []
    const save = vi.fn(() => new Promise<UISettings>((resolve) => resolvers.push(resolve)))
    const applied: UISettings[] = []
    const onError = vi.fn()
    const writer = createUIPreferenceWriter(save)
    const onSaved = (value: UISettings) => applied.push(value)

    const firstInput = { sidebarWidth: 300, workspace: 'activity' as const }
    const secondInput = { sidebarWidth: 336, workspace: 'layouts' as const }
    const first = writer.enqueue(firstInput, onSaved, onError)
    const second = writer.enqueue(secondInput, onSaved, onError)

    await Promise.resolve()
    await Promise.resolve()
    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenNthCalledWith(1, firstInput)

    const firstResult: UISettings = { theme: 'dark', ...firstInput }
    resolvers[0](firstResult)
    await first
    await Promise.resolve()
    await Promise.resolve()
    expect(save).toHaveBeenCalledTimes(2)
    expect(save).toHaveBeenNthCalledWith(2, secondInput)

    const secondResult: UISettings = { theme: 'light', ...secondInput }
    resolvers[1](secondResult)
    await second
    await writer.flush()
    expect(applied).toEqual([firstResult, secondResult])
    expect(onError).not.toHaveBeenCalled()
  })

  it('continues after a failed write', async () => {
    const onSaved = vi.fn()
    const onError = vi.fn()
    const save = vi.fn()
      .mockRejectedValueOnce(new Error('settings unavailable'))
      .mockResolvedValueOnce({ theme: 'dark', sidebarWidth: 310, workspace: 'tunnels' })
    const writer = createUIPreferenceWriter(save)

    await expect(writer.enqueue({ sidebarWidth: 300, workspace: 'activity' }, onSaved, onError)).rejects.toThrow('settings unavailable')
    await expect(writer.enqueue({ sidebarWidth: 310, workspace: 'tunnels' }, onSaved, onError)).resolves.toMatchObject({ workspace: 'tunnels' })
    await writer.flush()
    expect(onError).toHaveBeenCalledOnce()
    expect(onSaved).toHaveBeenCalledOnce()
  })
})
