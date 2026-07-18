import { describe, expect, it, vi } from 'vitest'

describe('xterm stream handling', () => {
  it('accepts invalid UTF-8 and truncated control sequences without crashing', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(() => null)
    const { Terminal } = await import('@xterm/xterm')
    const terminal = new Terminal({ cols: 80, rows: 24 })

    const chunks = [
      Uint8Array.from([0x66, 0x6f, 0x80, 0xbf]),
      Uint8Array.from([0xe2]),
      Uint8Array.from([0x28, 0xa1, 0x1b, 0x5b, 0x33, 0x38, 0x3b, 0x32, 0x3b]),
      Uint8Array.from([0x1b, 0x5d, 0x35, 0x32, 0x3b, 0x63, 0x3b, 0xff, 0x07]),
    ]

    for (const chunk of chunks) {
      await expect(write(terminal, chunk)).resolves.toBeUndefined()
    }
    terminal.dispose()
  })
})

async function write(
  terminal: { write: (data: Uint8Array, callback: () => void) => void },
  data: Uint8Array,
): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    try {
      terminal.write(data, resolve)
    } catch (error) {
      reject(error)
    }
  })
}
