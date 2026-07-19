import type { UISettings } from '../../lib/bridge/types'

export const defaultSidebarWidth = 272
export const minimumSidebarWidth = 220
export const maximumSidebarWidth = 420

export function clampSidebarWidth(width: number): number {
  if (!Number.isFinite(width)) return defaultSidebarWidth
  return Math.min(maximumSidebarWidth, Math.max(minimumSidebarWidth, Math.round(width)))
}

type UIPreferenceInput = Pick<UISettings, 'sidebarWidth' | 'workspace'>

export interface UIPreferenceWriter {
  enqueue: (
    preferences: UIPreferenceInput,
    onSaved: (settings: UISettings) => void,
    onError: (cause: unknown) => void,
  ) => Promise<UISettings>
  flush: () => Promise<void>
}

export function createUIPreferenceWriter(
  save: (preferences: UIPreferenceInput) => Promise<UISettings>,
): UIPreferenceWriter {
  let tail = Promise.resolve()

  return {
    enqueue(preferences, onSaved, onError) {
      const result = tail.catch(() => undefined).then(() => save(preferences))
      tail = result.then(onSaved, onError)
      return result
    },
    async flush() {
      await tail.catch(() => undefined)
    },
  }
}
