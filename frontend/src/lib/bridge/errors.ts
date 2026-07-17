const backendErrorCodes = [
  'invalid_argument',
  'not_found',
  'conflict',
  'stale',
  'authentication_required',
  'permission_denied',
  'unavailable',
  'canceled',
  'deadline_exceeded',
  'internal',
] as const

export type BackendErrorCode = typeof backendErrorCodes[number]

interface BackendErrorPayload {
  code: BackendErrorCode
  message: string
  operation?: string
  retryable: boolean
}

export class BackendError extends Error {
  readonly code: BackendErrorCode
  readonly operation?: string
  readonly retryable: boolean

  constructor(payload: BackendErrorPayload, cause?: Error) {
    super(payload.message, cause ? { cause } : undefined)
    this.name = 'BackendError'
    this.code = payload.code
    this.operation = payload.operation
    this.retryable = payload.retryable
  }
}

export function asBackendError(cause: unknown): BackendError {
  if (cause instanceof BackendError) {
    return cause
  }
  const source = cause instanceof Error ? cause : new Error(String(cause))
  const payload = parsePayload(source.message) ?? {
    code: 'internal' as const,
    message: source.message || 'The operation could not be completed.',
    retryable: false,
  }
  return new BackendError(payload, source)
}

function parsePayload(value: string): BackendErrorPayload | undefined {
  try {
    const candidate: unknown = JSON.parse(value)
    if (!isRecord(candidate) || !isBackendErrorCode(candidate.code)) {
      return undefined
    }
    if (typeof candidate.message !== 'string' || candidate.message.trim() === '') {
      return undefined
    }
    if (typeof candidate.retryable !== 'boolean') {
      return undefined
    }
    if (candidate.operation !== undefined && typeof candidate.operation !== 'string') {
      return undefined
    }
    return {
      code: candidate.code,
      message: candidate.message,
      operation: candidate.operation,
      retryable: candidate.retryable,
    }
  } catch {
    return undefined
  }
}

function isBackendErrorCode(value: unknown): value is BackendErrorCode {
  return typeof value === 'string' && (backendErrorCodes as readonly string[]).includes(value)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}
