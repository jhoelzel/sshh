import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Session, TerminalOutput, TerminalSettings } from '../../lib/bridge/types'

interface TerminalDouble {
  emitBinary: (value: string) => void
  emitData: (value: string) => void
  emitResize: (columns: number, rows: number) => void
  rejectWrites: boolean
  resize: (columns: number, rows: number) => void
  writes: Uint8Array[]
}

const harness = vi.hoisted(() => ({
  backend: {
    acknowledgeTerminalOutput: vi.fn(),
    resizeTerminal: vi.fn(),
    writeTerminal: vi.fn(),
  },
  terminals: [] as TerminalDouble[],
}))

vi.mock('../../lib/bridge/client', () => ({ backend: harness.backend }))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit = vi.fn()
  },
}))

vi.mock('@xterm/addon-search', () => ({
  SearchAddon: class {
    findNext = vi.fn(() => false)
    findPrevious = vi.fn(() => false)
  },
}))

vi.mock('@xterm/xterm', () => ({
  Terminal: class implements TerminalDouble {
    private binaryHandler?: (value: string) => void
    private dataHandler?: (value: string) => void
    private resizeHandler?: (size: { cols: number; rows: number }) => void
    readonly buffer = { active: { baseY: 0, cursorY: 0, getLine: () => undefined, viewportY: 0 } }
    readonly rows = 24
    readonly writes: Uint8Array[] = []
    options: Record<string, unknown>
    rejectWrites = false

    constructor(options: Record<string, unknown>) {
      this.options = { ...options }
      harness.terminals.push(this)
    }

    attachCustomKeyEventHandler = vi.fn()
    dispose = vi.fn()
    focus = vi.fn()
    getSelection = vi.fn(() => '')
    hasSelection = vi.fn(() => false)
    loadAddon = vi.fn()
    open = vi.fn()
    resize = vi.fn((columns: number, rows: number) => {
      this.emitResize(columns, rows)
    })

    onBell = vi.fn(() => disposable())
    onSelectionChange = vi.fn(() => disposable())
    onTitleChange = vi.fn(() => disposable())
    onBinary = vi.fn((handler: (value: string) => void) => {
      this.binaryHandler = handler
      return disposable()
    })
    onData = vi.fn((handler: (value: string) => void) => {
      this.dataHandler = handler
      return disposable()
    })
    onResize = vi.fn((handler: (size: { cols: number; rows: number }) => void) => {
      this.resizeHandler = handler
      return disposable()
    })

    write = vi.fn((data: Uint8Array, callback?: () => void) => {
      if (this.rejectWrites) throw new Error('parser failed')
      this.writes.push(Uint8Array.from(data))
      callback?.()
    })

    emitBinary(value: string): void {
      this.binaryHandler?.(value)
    }

    emitData(value: string): void {
      this.dataHandler?.(value)
    }

    emitResize(columns: number, rows: number): void {
      this.resizeHandler?.({ cols: columns, rows })
    }
  },
}))

import { TerminalController } from './TerminalController'

const session: Session = {
  id: 'session-1', generation: 3, leaseId: 'lease-1', profileId: 'local-1',
  title: 'Local', state: 'running', columns: 80, rows: 24,
  startedAt: '2026-07-18T12:00:00Z',
}

const settings: TerminalSettings = {
  fontFamily: 'system-mono', fontSize: 13, lineHeight: 1.2,
  cursorStyle: 'block', cursorBlink: true, scrollback: 10_000, bell: true,
}

beforeEach(() => {
  harness.terminals.length = 0
  harness.backend.acknowledgeTerminalOutput.mockReset().mockResolvedValue(undefined)
  harness.backend.resizeTerminal.mockReset().mockResolvedValue(undefined)
  harness.backend.writeTerminal.mockReset().mockResolvedValue(undefined)
  vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1))
  vi.stubGlobal('cancelAnimationFrame', vi.fn())
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('TerminalController', () => {
  it('preserves callback order across text, binary mouse, and paste input', async () => {
    let releaseFirst!: () => void
    const firstWrite = new Promise<void>((resolve) => { releaseFirst = resolve })
    harness.backend.writeTerminal.mockImplementation(async (_lease, _session, _generation, sequence) => {
      if (sequence === 1) await firstWrite
    })
    const { controller } = createController()
    const terminal = harness.terminals[0]

    const mouseReport = String.fromCharCode(0x1b, 0x5b, 0x4d, 0, 0xff)
    terminal.emitData('lambda: \u03bb')
    terminal.emitBinary(mouseReport)
    terminal.emitData('pasted line\r')

    await vi.waitFor(() => expect(harness.backend.writeTerminal).toHaveBeenCalledTimes(1))
    releaseFirst()
    await vi.waitFor(() => expect(harness.backend.writeTerminal).toHaveBeenCalledTimes(3))

    expect(harness.backend.writeTerminal.mock.calls.map((call) => ({
      identity: call.slice(0, 3),
      sequence: call[3],
      bytes: decodeBase64(call[4] as string),
    }))).toEqual([
      { identity: ['lease-1', 'session-1', 3], sequence: 1, bytes: Array.from(new TextEncoder().encode('lambda: \u03bb')) },
      { identity: ['lease-1', 'session-1', 3], sequence: 2, bytes: [0x1b, 0x5b, 0x4d, 0, 0xff] },
      { identity: ['lease-1', 'session-1', 3], sequence: 3, bytes: Array.from(new TextEncoder().encode('pasted line\r')) },
    ])
    controller.dispose()
  })

  it('coalesces resize callbacks and delivers the final dimensions', async () => {
    vi.useFakeTimers()
    const { controller } = createController()
    const terminal = harness.terminals[0]

    controller.resize(81, 25)
    await vi.advanceTimersByTimeAsync(40)
    controller.resize(120, 40)
    await vi.advanceTimersByTimeAsync(79)
    expect(harness.backend.resizeTerminal).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(harness.backend.resizeTerminal).toHaveBeenCalledOnce()
    expect(harness.backend.resizeTerminal).toHaveBeenCalledWith('lease-1', 'session-1', 3, 120, 40)
    expect(terminal.resize).toHaveBeenCalledTimes(2)
    controller.dispose()
  })

  it('contains malformed frames and forwards arbitrary valid bytes unchanged', async () => {
    const { callbacks, controller } = createController()
    const terminal = harness.terminals[0]

    expect(() => controller.acceptOutput(output({ payload: '!!!!', byteCount: 1, endOffset: 1 }))).not.toThrow()
    expect(terminal.writes).toHaveLength(0)

    const malformedStream = Uint8Array.from([0xff, 0xfe, 0xe2, 0x28, 0xa1, 0x1b, 0x5b, 0x33, 0x38, 0x3b])
    expect(() => controller.acceptOutput(output({
      payload: encodeBase64(malformedStream), byteCount: malformedStream.byteLength,
      endOffset: malformedStream.byteLength,
    }))).not.toThrow()
    expect(Array.from(terminal.writes[0])).toEqual(Array.from(malformedStream))

    expect(() => controller.acceptOutput(output({
      sequence: 2, payload: 'AQ==', byteCount: 1, endOffset: 99,
    }))).not.toThrow()
    expect(terminal.writes).toHaveLength(1)

    expect(() => controller.acceptOutput(output({
      sequence: 2, payload: 'AQ==', byteCount: 1, endOffset: malformedStream.byteLength + 1,
    }))).not.toThrow()
    expect(terminal.writes).toHaveLength(2)

    expect(() => controller.acceptOutput(output({
      sequence: 3, byteCount: 64 * 1024 + 1, payload: '', endOffset: malformedStream.byteLength + 1,
    }))).not.toThrow()
    expect(() => controller.acceptOutput(null as unknown as TerminalOutput)).not.toThrow()

    expect(callbacks.onError.mock.calls.map(([error]) => error.message)).toEqual([
      'terminal output payload is not valid base64',
      'terminal output byte offset is invalid',
      'terminal output metadata is invalid',
      'terminal output event is malformed',
    ])
    const acknowledgementFrame = vi.mocked(requestAnimationFrame).mock.calls[0][0]
    acknowledgementFrame(0)
    await vi.waitFor(() => expect(harness.backend.acknowledgeTerminalOutput).toHaveBeenCalledOnce())
    expect(harness.backend.acknowledgeTerminalOutput).toHaveBeenCalledWith(
      'lease-1', 'session-1', 3, 2, malformedStream.byteLength + 1,
    )
    expect(controller.diagnostics()).toEqual({
      acceptedSequence: 2,
      acceptedBytes: malformedStream.byteLength + 1,
      consumedSequence: 2,
      consumedBytes: malformedStream.byteLength + 1,
      acknowledgedSequence: 2,
      pendingBytes: 0,
      peakPendingBytes: malformedStream.byteLength,
      maximumPendingBytes: 1024 * 1024,
      outputFailed: false,
    })
    controller.dispose()
  })

  it('turns a synchronous xterm parser failure into a controlled error', () => {
    const { callbacks, controller } = createController()
    harness.terminals[0].rejectWrites = true

    expect(() => controller.acceptOutput(output({ payload: 'AQ==', byteCount: 1, endOffset: 1 }))).not.toThrow()
    expect(callbacks.onError).toHaveBeenCalledOnce()
    expect(callbacks.onError.mock.calls[0][0].message).toBe('terminal output parser rejected a chunk')

    harness.terminals[0].rejectWrites = false
    controller.acceptOutput(output({ sequence: 2, payload: 'Ag==', byteCount: 1, endOffset: 2 }))
    expect(harness.terminals[0].writes).toHaveLength(0)
    expect(requestAnimationFrame).not.toHaveBeenCalled()
    expect(harness.backend.acknowledgeTerminalOutput).not.toHaveBeenCalled()
    controller.dispose()
  })
})

function createController() {
  const callbacks = {
    onBell: vi.fn(),
    onError: vi.fn(),
    onSearchRequested: vi.fn(),
    onSelectionChange: vi.fn(),
    onTitle: vi.fn(),
  }
  return { callbacks, controller: new TerminalController(session, settings, callbacks) }
}

function disposable() {
  return { dispose: vi.fn() }
}

function output(overrides: Partial<TerminalOutput>): TerminalOutput {
  return {
    leaseId: session.leaseId,
    sessionId: session.id,
    generation: session.generation,
    sequence: 1,
    endOffset: 0,
    byteCount: 0,
    payload: '',
    final: false,
    ...overrides,
  }
}

function encodeBase64(data: Uint8Array): string {
  return btoa(String.fromCharCode(...data))
}

function decodeBase64(value: string): number[] {
  return Array.from(atob(value), (character) => character.charCodeAt(0))
}
