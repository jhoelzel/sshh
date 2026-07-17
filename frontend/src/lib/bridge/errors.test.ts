import { describe, expect, it } from 'vitest'
import { asBackendError, BackendError } from './errors'

describe('backend errors', () => {
  it('decodes the structured Wails error envelope', () => {
    const source = new Error(JSON.stringify({
      code: 'stale',
      message: 'Frontend lease is missing or stale.',
      operation: 'renew frontend lease',
      retryable: true,
    }))

    const error = asBackendError(source)

    expect(error).toBeInstanceOf(BackendError)
    expect(error).toMatchObject({
      code: 'stale',
      message: 'Frontend lease is missing or stale.',
      operation: 'renew frontend lease',
      retryable: true,
    })
    expect(error.cause).toBe(source)
  })

  it('normalizes malformed and legacy rejections as internal errors', () => {
    const error = asBackendError(new Error('[object Object]'))

    expect(error).toMatchObject({
      code: 'internal',
      message: '[object Object]',
      retryable: false,
    })
  })

  it('does not wrap an error twice', () => {
    const error = asBackendError(new Error(JSON.stringify({
      code: 'conflict', message: 'Reload before saving.', retryable: false,
    })))

    expect(asBackendError(error)).toBe(error)
  })
})
