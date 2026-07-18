import { describe, expect, it } from 'vitest'
import {
  MAX_PROFILE_ENVIRONMENT_OVERRIDES,
  profileEnvironmentEntries,
  validateProfileEnvironment,
  type ProfileEnvironmentEntry,
} from './profileEnvironment'

function entry(name: string, value = 'value', id = name): ProfileEnvironmentEntry {
  return { id, name, value }
}

describe('profile environment', () => {
  it('sorts saved variables and preserves exact values', () => {
    expect(profileEnvironmentEntries({ ZED: 'last', EMPTY: '', ALPHA: 'line one\nline two' })).toEqual([
      { id: 'saved:0', name: 'ALPHA', value: 'line one\nline two' },
      { id: 'saved:1', name: 'EMPTY', value: '' },
      { id: 'saved:2', name: 'ZED', value: 'last' },
    ])
  })

  it('builds an own-property record without treating names as object metadata', () => {
    const result = validateProfileEnvironment([entry('__proto__', 'literal')])
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(Object.prototype.hasOwnProperty.call(result.environment, '__proto__')).toBe(true)
      expect(Object.getOwnPropertyDescriptor(result.environment, '__proto__')?.value).toBe('literal')
    }
  })

  it.each([
    [[entry('', '', 'empty')], 'needs a name'],
    [[entry('BAD-NAME')], 'must start with a letter or underscore'],
    [[entry('PATH'), entry('Path', 'other')], 'differ only by case'],
    [[entry('term')], 'managed by shh-h'],
    [[entry('VALID', 'before\0after')], 'contains a null byte'],
  ] as const)('rejects invalid entries', (entries, message) => {
    const result = validateProfileEnvironment([...entries])
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error).toContain(message)
    }
  })

  it('limits the number of profile overrides', () => {
    const entries = Array.from(
      { length: MAX_PROFILE_ENVIRONMENT_OVERRIDES + 1 },
      (_, index) => entry(`SHHH_TEST_${index}`, 'value', String(index)),
    )
    const result = validateProfileEnvironment(entries)
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error).toContain('at most 128')
    }
  })
})
