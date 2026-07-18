import { describe, expect, it } from 'vitest'
import { terminalTabActionAvailability, type TerminalTabActionState } from './terminalTabActions'

describe('terminal tab action availability', () => {
  it.each([
    ['starting', false, false],
    ['running', false, true],
    ['closing', false, false],
    ['exited', true, false],
    ['failed', true, false],
    ['closed', true, false],
    ['disconnected', true, false],
  ] satisfies Array<[TerminalTabActionState, boolean, boolean]>) (
    'maps %s to reconnectable=%s and duplicable=%s',
    (state, reconnectable, duplicable) => {
      expect(terminalTabActionAvailability(state, true, true, false)).toEqual({
        retry: reconnectable,
        reconnectInNewTab: reconnectable,
        duplicate: duplicable,
        clearScrollback: true,
        reset: true,
      })
    },
  )

  it('requires a connection descriptor and blocks connection actions while busy', () => {
    expect(terminalTabActionAvailability('failed', true, false, false)).toMatchObject({
      retry: false, reconnectInNewTab: false, duplicate: false,
    })
    expect(terminalTabActionAvailability('running', true, true, true)).toMatchObject({
      retry: false, reconnectInNewTab: false, duplicate: false,
    })
  })

  it('allows local display actions only while a controller exists', () => {
    expect(terminalTabActionAvailability('disconnected', false, true, false)).toMatchObject({
      clearScrollback: false, reset: false,
    })
  })
})
