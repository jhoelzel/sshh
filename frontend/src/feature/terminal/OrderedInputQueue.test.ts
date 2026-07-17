import { describe, expect, it } from 'vitest'
import { OrderedInputQueue } from './OrderedInputQueue'

describe('OrderedInputQueue', () => {
  it('serializes and chunks input without reordering it', async () => {
    const writes: Array<{ sequence: number; value: number[] }> = []
    const queue = new OrderedInputQueue(async (sequence, data) => {
      await Promise.resolve()
      writes.push({ sequence, value: Array.from(data) })
    }, () => undefined, 3)

    queue.enqueue(Uint8Array.from([1, 2, 3, 4]))
    queue.enqueue(Uint8Array.from([5, 6]))
    await queue.settled()

    expect(writes).toEqual([
      { sequence: 1, value: [1, 2, 3] },
      { sequence: 2, value: [4] },
      { sequence: 3, value: [5, 6] },
    ])
  })

  it('halts after a failed write so later sequences cannot create a gap', async () => {
    const writes: number[] = []
    const errors: string[] = []
    const queue = new OrderedInputQueue(async (sequence) => {
      writes.push(sequence)
      throw new Error('closed')
    }, (error) => errors.push(error.message), 1)

    queue.enqueue(Uint8Array.from([1, 2, 3]))
    await expect(queue.settled()).rejects.toThrow('closed')

    expect(writes).toEqual([1])
    expect(errors).toEqual(['closed'])
  })
})
