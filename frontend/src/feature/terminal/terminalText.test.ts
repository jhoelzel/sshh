import { describe, expect, it } from 'vitest'
import { visibleBufferText } from './terminalText'

describe('visibleBufferText', () => {
  it('reads only viewport rows and joins soft-wrapped lines', () => {
    const source = [
      { text: 'outside', wrapped: false },
      { text: 'first ', wrapped: false },
      { text: 'continued ', wrapped: true },
      { text: 'second', wrapped: false },
      { text: 'outside below', wrapped: false },
    ]
    const text = visibleBufferText({
      viewportY: 1,
      length: source.length,
      getLine: (index) => {
        const line = source[index]
        return line ? {
          isWrapped: line.wrapped,
          translateToString: (trimRight) => trimRight ? line.text.trimEnd() : line.text,
        } : undefined
      },
    }, 3)

    expect(text).toBe('first continued\nsecond')
  })

  it('removes trailing blank viewport rows without removing internal blanks', () => {
    const source = ['one', '', 'two', '', '']
    const text = visibleBufferText({
      viewportY: 0,
      length: source.length,
      getLine: (index) => source[index] === undefined ? undefined : {
        isWrapped: false,
        translateToString: () => source[index],
      },
    }, 5)

    expect(text).toBe('one\n\ntwo')
  })
})
