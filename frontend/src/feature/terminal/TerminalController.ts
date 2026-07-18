import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal, type IDisposable } from '@xterm/xterm'
import { backend } from '../../lib/bridge/client'
import type { Session, TerminalOutput, TerminalSettings } from '../../lib/bridge/types'
import { OrderedInputQueue } from './OrderedInputQueue'
import { sanitizeTerminalLink } from './terminalLinks'
import { visibleBufferText } from './terminalText'

const maxPendingOutput = 1024 * 1024
const maxOutputChunk = 64 * 1024
const resizeDelay = 80

interface ControllerCallbacks {
  onTitle: (title: string) => void
  onBell: () => void
  onError: (error: Error) => void
  onLinkRequested: (url: string) => void
  onSearchRequested: () => void
  onSelectionChange: (selected: boolean) => void
}

export interface TerminalControllerDiagnostics {
  acceptedSequence: number
  acceptedBytes: number
  consumedSequence: number
  consumedBytes: number
  acknowledgedSequence: number
  pendingBytes: number
  peakPendingBytes: number
  maximumPendingBytes: number
  outputFailed: boolean
}

export class TerminalController {
  readonly session: Session

  private readonly terminal: Terminal
  private readonly fitAddon = new FitAddon()
  private readonly searchAddon = new SearchAddon()
  private readonly callbacks: ControllerCallbacks
  private readonly disposables: IDisposable[] = []
  private readonly inputQueue: OrderedInputQueue
  private host?: HTMLElement
  private resizeObserver?: ResizeObserver
  private resizeTimer?: number
  private visible = false
  private disposed = false
  private outputFailed = false
  private readyResolve!: () => void
  private readonly readyPromise: Promise<void>
  private expectedOutputSequence = 1
  private expectedOutputOffset = 0
  private pendingOutputBytes = 0
  private peakPendingOutputBytes = 0
  private consumedSequence = 0
  private consumedOffset = 0
  private acknowledgedSequence = 0
  private acknowledgementFrame?: number
  private acknowledgementRunning = false
  private bellEnabled: boolean

  constructor(session: Session, settings: TerminalSettings, callbacks: ControllerCallbacks) {
    this.session = session
    this.callbacks = callbacks
    this.bellEnabled = settings.bell
    this.readyPromise = new Promise((resolve) => {
      this.readyResolve = resolve
    })
    this.terminal = new Terminal({
      allowProposedApi: false,
      cursorBlink: settings.cursorBlink,
      cursorStyle: settings.cursorStyle,
      fontFamily: fontFamily(settings.fontFamily),
      fontSize: settings.fontSize,
      lineHeight: settings.lineHeight,
      linkHandler: {
        activate: (event, url) => this.requestLink(event, url),
        allowNonHttpProtocols: false,
      },
      macOptionIsMeta: true,
      rightClickSelectsWord: true,
      scrollback: settings.scrollback,
      theme: {
        background: '#101214',
        foreground: '#d9ddd8',
        cursor: '#a8e6a3',
        cursorAccent: '#101214',
        selectionBackground: '#356c5a99',
        black: '#1b1e20',
        red: '#e06c75',
        green: '#98c379',
        yellow: '#e5c07b',
        blue: '#61afef',
        magenta: '#c678dd',
        cyan: '#56b6c2',
        white: '#d7dae0',
        brightBlack: '#5c6370',
        brightRed: '#f07a83',
        brightGreen: '#b2df8a',
        brightYellow: '#f3d38b',
        brightBlue: '#79c0ff',
        brightMagenta: '#d895ee',
        brightCyan: '#71d1dd',
        brightWhite: '#ffffff',
      },
    })
    this.terminal.loadAddon(this.fitAddon)
    this.terminal.loadAddon(this.searchAddon)
    this.terminal.loadAddon(new WebLinksAddon((event, url) => this.requestLink(event, url)))

    this.inputQueue = new OrderedInputQueue(
      async (sequence, data) => {
        await backend.writeTerminal(
          session.leaseId,
          session.id,
          session.generation,
          sequence,
          encodeBase64(data),
        )
      },
      callbacks.onError,
    )

    this.disposables.push(
      this.terminal.onData((data) => this.inputQueue.enqueue(new TextEncoder().encode(data))),
      this.terminal.onBinary((data) => this.inputQueue.enqueue(binaryStringToBytes(data))),
      this.terminal.onResize(({ cols, rows }) => this.scheduleResize(cols, rows)),
      this.terminal.onTitleChange(callbacks.onTitle),
      this.terminal.onSelectionChange(() => callbacks.onSelectionChange(this.terminal.hasSelection())),
      this.terminal.onBell(() => {
        if (this.bellEnabled) callbacks.onBell()
      }),
    )
    this.terminal.attachCustomKeyEventHandler((event) => {
      if (event.type === 'keydown' && event.metaKey && event.key.toLowerCase() === 'f') {
        callbacks.onSearchRequested()
        return false
      }
      return true
    })
  }

  attach(host: HTMLElement): void {
    if (this.disposed || this.host === host) {
      return
    }
    if (this.host) {
      throw new Error('terminal controller cannot move between host elements')
    }

    this.host = host
    this.terminal.open(host)
    this.resizeObserver = new ResizeObserver(() => {
      if (this.visible) {
        this.fit()
      }
    })
    this.resizeObserver.observe(host)
    this.readyResolve()
  }

  ready(): Promise<void> {
    return this.readyPromise
  }

  setVisible(visible: boolean): void {
    this.visible = visible
    if (visible) {
      requestAnimationFrame(() => {
        this.fit()
        this.terminal.focus()
      })
    }
  }

  acceptOutput(event: TerminalOutput): void {
    if (this.disposed || this.outputFailed) {
      return
    }
    if (!event || typeof event !== 'object') {
      this.callbacks.onError(new Error('terminal output event is malformed'))
      return
    }
    if (
      event.leaseId !== this.session.leaseId ||
      event.sessionId !== this.session.id ||
      event.generation !== this.session.generation
    ) {
      return
    }
    if (!Number.isSafeInteger(event.sequence) || event.sequence < 1) {
      this.callbacks.onError(new Error('terminal output sequence is invalid'))
      return
    }
    if (event.sequence < this.expectedOutputSequence) {
      this.scheduleAcknowledgement()
      return
    }
    if (event.sequence !== this.expectedOutputSequence) {
      this.callbacks.onError(new Error(`terminal output gap: expected ${this.expectedOutputSequence}, got ${event.sequence}`))
      return
    }

    if (
      !Number.isSafeInteger(event.byteCount) ||
      event.byteCount < 0 ||
      event.byteCount > maxOutputChunk ||
      !Number.isSafeInteger(event.endOffset) ||
      event.endOffset < 0 ||
      typeof event.payload !== 'string' ||
      event.payload.length !== Math.ceil(event.byteCount / 3) * 4
    ) {
      this.callbacks.onError(new Error('terminal output metadata is invalid'))
      return
    }

    let data: Uint8Array
    try {
      data = decodeBase64(event.payload)
    } catch {
      this.callbacks.onError(new Error('terminal output payload is not valid base64'))
      return
    }
    if (data.byteLength !== event.byteCount) {
      this.callbacks.onError(new Error('terminal output byte count does not match its payload'))
      return
    }
    const expectedEndOffset = this.expectedOutputOffset + data.byteLength
    if (!Number.isSafeInteger(expectedEndOffset) || event.endOffset !== expectedEndOffset) {
      this.callbacks.onError(new Error('terminal output byte offset is invalid'))
      return
    }
    if (this.pendingOutputBytes + data.byteLength > maxPendingOutput) {
      this.callbacks.onError(new Error('terminal output exceeded the frontend flow-control window'))
      return
    }

    this.expectedOutputSequence++
    this.expectedOutputOffset = event.endOffset
    this.pendingOutputBytes += data.byteLength
    this.peakPendingOutputBytes = Math.max(this.peakPendingOutputBytes, this.pendingOutputBytes)
    let consumedOnce = false
    const consumed = () => {
      if (consumedOnce) return
      consumedOnce = true
      this.pendingOutputBytes -= data.byteLength
      this.consumedSequence = event.sequence
      this.consumedOffset = event.endOffset
      this.scheduleAcknowledgement()
    }
    if (data.byteLength === 0) {
      consumed()
    } else {
      try {
        this.terminal.write(data, consumed)
      } catch {
        this.outputFailed = true
        if (!consumedOnce) {
          this.pendingOutputBytes -= data.byteLength
        }
        this.callbacks.onError(new Error('terminal output parser rejected a chunk'))
      }
    }
  }

  findNext(query: string): boolean {
    return query !== '' && this.searchAddon.findNext(query, { incremental: true })
  }

  findPrevious(query: string): boolean {
    return query !== '' && this.searchAddon.findPrevious(query)
  }

  focus(): void {
    this.terminal.focus()
  }

  hasFocus(): boolean {
    return this.terminal.element?.contains(document.activeElement) ?? false
  }

  resize(columns: number, rows: number): void {
    if (this.disposed) {
      throw new Error('terminal is closed')
    }
    this.terminal.resize(columns, rows)
  }

  diagnostics(): TerminalControllerDiagnostics {
    return {
      acceptedSequence: this.expectedOutputSequence - 1,
      acceptedBytes: this.expectedOutputOffset,
      consumedSequence: this.consumedSequence,
      consumedBytes: this.consumedOffset,
      acknowledgedSequence: this.acknowledgedSequence,
      pendingBytes: this.pendingOutputBytes,
      peakPendingBytes: this.peakPendingOutputBytes,
      maximumPendingBytes: maxPendingOutput,
      outputFailed: this.outputFailed,
    }
  }

  visibleText(): string {
    if (this.disposed) return ''
    return visibleBufferText(this.terminal.buffer.active, this.terminal.rows)
  }

  selectedText(): string {
    if (this.disposed) return ''
    return this.terminal.getSelection()
  }

  applySettings(settings: TerminalSettings): void {
    if (this.disposed) return
    this.bellEnabled = settings.bell
    this.terminal.options.fontFamily = fontFamily(settings.fontFamily)
    this.terminal.options.fontSize = settings.fontSize
    this.terminal.options.lineHeight = settings.lineHeight
    this.terminal.options.cursorStyle = settings.cursorStyle
    this.terminal.options.cursorBlink = settings.cursorBlink
    this.terminal.options.scrollback = settings.scrollback
    if (this.visible) requestAnimationFrame(() => this.fit())
  }

  async sendText(text: string, submit: boolean): Promise<void> {
    if (this.disposed) {
      throw new Error('terminal is closed')
    }
    const payload = submit ? `${text}\r` : text
    this.inputQueue.enqueue(new TextEncoder().encode(payload))
    await this.inputQueue.settled()
  }

  dispose(): void {
    if (this.disposed) {
      return
    }
    this.disposed = true
    this.inputQueue.stop()
    if (this.resizeTimer !== undefined) {
      window.clearTimeout(this.resizeTimer)
    }
    if (this.acknowledgementFrame !== undefined) {
      cancelAnimationFrame(this.acknowledgementFrame)
    }
    this.resizeObserver?.disconnect()
    for (const disposable of this.disposables) {
      disposable.dispose()
    }
    this.terminal.dispose()
  }

  private fit(): void {
    if (!this.host || this.host.clientWidth === 0 || this.host.clientHeight === 0) {
      return
    }
    this.fitAddon.fit()
  }

  private requestLink(event: MouseEvent, value: string): void {
    event.preventDefault()
    event.stopPropagation()
    const url = sanitizeTerminalLink(value)
    if (url) this.callbacks.onLinkRequested(url)
  }

  private scheduleResize(columns: number, rows: number): void {
    if (this.resizeTimer !== undefined) {
      window.clearTimeout(this.resizeTimer)
    }
    this.resizeTimer = window.setTimeout(() => {
      void backend
        .resizeTerminal(this.session.leaseId, this.session.id, this.session.generation, columns, rows)
        .catch((cause) => this.callbacks.onError(asError(cause)))
    }, resizeDelay)
  }

  private scheduleAcknowledgement(): void {
    if (this.acknowledgementFrame !== undefined || this.consumedSequence <= this.acknowledgedSequence) {
      return
    }
    this.acknowledgementFrame = requestAnimationFrame(() => {
      this.acknowledgementFrame = undefined
      void this.flushAcknowledgement()
    })
  }

  private async flushAcknowledgement(): Promise<void> {
    if (this.acknowledgementRunning || this.consumedSequence <= this.acknowledgedSequence) {
      return
    }
    this.acknowledgementRunning = true
    try {
      while (!this.disposed && this.consumedSequence > this.acknowledgedSequence) {
        const sequence = this.consumedSequence
        const offset = this.consumedOffset
        await backend.acknowledgeTerminalOutput(
          this.session.leaseId,
          this.session.id,
          this.session.generation,
          sequence,
          offset,
        )
        this.acknowledgedSequence = sequence
      }
    } catch (cause) {
      this.callbacks.onError(asError(cause))
    } finally {
      this.acknowledgementRunning = false
      if (this.consumedSequence > this.acknowledgedSequence) {
        this.scheduleAcknowledgement()
      }
    }
  }
}

function encodeBase64(data: Uint8Array): string {
  let binary = ''
  for (const value of data) {
    binary += String.fromCharCode(value)
  }
  return btoa(binary)
}

function decodeBase64(value: string): Uint8Array {
  const binary = atob(value)
  const result = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index++) {
    result[index] = binary.charCodeAt(index)
  }
  return result
}

function binaryStringToBytes(value: string): Uint8Array {
  const result = new Uint8Array(value.length)
  for (let index = 0; index < value.length; index++) {
    result[index] = value.charCodeAt(index) & 0xff
  }
  return result
}

function asError(cause: unknown): Error {
  return cause instanceof Error ? cause : new Error(String(cause))
}

function fontFamily(value: TerminalSettings['fontFamily']): string {
  switch (value) {
    case 'menlo': return 'Menlo, Monaco, Consolas, monospace'
    case 'monaco': return 'Monaco, Menlo, Consolas, monospace'
    default: return 'SFMono-Regular, Menlo, Monaco, Consolas, monospace'
  }
}
