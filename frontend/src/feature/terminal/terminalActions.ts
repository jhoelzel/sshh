import type { TerminalTextExportResult } from '../../lib/bridge/types'

interface TerminalTextReader {
  visibleText: () => string
  selectedText: () => string
  focus: () => void
}

export async function copyVisibleText(
  terminal: TerminalTextReader,
  copy: (text: string) => Promise<void>,
): Promise<void> {
  try {
    const text = terminal.visibleText()
    if (!text) throw new Error('There is no visible terminal text to copy')
    await copy(text)
  } finally {
    terminal.focus()
  }
}

export async function exportSelectedText(
  terminal: TerminalTextReader,
  title: string,
  exportText: (title: string, text: string) => Promise<TerminalTextExportResult>,
): Promise<TerminalTextExportResult> {
  try {
    const text = terminal.selectedText()
    if (!text) throw new Error('There is no terminal selection to export')
    return await exportText(title, text)
  } finally {
    terminal.focus()
  }
}
