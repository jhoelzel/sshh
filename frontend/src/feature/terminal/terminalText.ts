interface ReadableBufferLine {
  isWrapped: boolean
  translateToString: (trimRight?: boolean, startColumn?: number, endColumn?: number) => string
}

interface ReadableBuffer {
  viewportY: number
  length: number
  getLine: (line: number) => ReadableBufferLine | undefined
}

export function visibleBufferText(buffer: ReadableBuffer, rows: number): string {
  const lines: string[] = []
  const start = Math.max(0, buffer.viewportY)
  const end = Math.min(buffer.length, start + Math.max(0, rows))
  for (let index = start; index < end; index++) {
    const line = buffer.getLine(index)
    if (!line) continue
    const nextLine = buffer.getLine(index + 1)
    const text = line.translateToString(!nextLine?.isWrapped)
    if (line.isWrapped && lines.length > 0) {
      lines[lines.length - 1] += text
    } else {
      lines.push(text)
    }
  }
  while (lines.at(-1) === '') lines.pop()
  return lines.join('\n')
}
