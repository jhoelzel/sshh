const defaultChunkSize = 48 * 1024

export type InputWriter = (sequence: number, data: Uint8Array) => Promise<void>

export class OrderedInputQueue {
  private readonly writer: InputWriter
  private readonly onError: (error: Error) => void
  private readonly chunkSize: number
  private tail = Promise.resolve()
  private nextSequence = 1
  private halted = false
  private failure?: Error

  constructor(writer: InputWriter, onError: (error: Error) => void, chunkSize = defaultChunkSize) {
    this.writer = writer
    this.onError = onError
    this.chunkSize = chunkSize
  }

  enqueue(data: Uint8Array): void {
    if (this.halted || data.byteLength === 0) {
      return
    }

    for (let offset = 0; offset < data.byteLength; offset += this.chunkSize) {
      const chunk = data.slice(offset, Math.min(offset + this.chunkSize, data.byteLength))
      const sequence = this.nextSequence++
      this.tail = this.tail.then(async () => {
        if (this.halted) {
          return
        }
        try {
          await this.writer(sequence, chunk)
        } catch (cause) {
          this.halted = true
          this.failure = asError(cause)
          this.onError(this.failure)
        }
      })
    }
  }

  async settled(): Promise<void> {
    await this.tail
    if (this.failure) {
      throw this.failure
    }
  }

  stop(): void {
    this.halted = true
  }
}

function asError(cause: unknown): Error {
  return cause instanceof Error ? cause : new Error(String(cause))
}
