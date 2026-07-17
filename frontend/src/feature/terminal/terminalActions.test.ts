import { describe, expect, it, vi } from 'vitest'
import { copyVisibleText, exportSelectedText } from './terminalActions'

describe('terminal text actions', () => {
  it('copies the visible viewport exactly and restores focus', async () => {
    const copy = vi.fn(async () => undefined)
    const terminal = reader({ visible: 'one\ntwo' })

    await copyVisibleText(terminal, copy)

    expect(copy).toHaveBeenCalledWith('one\ntwo')
    expect(terminal.focus).toHaveBeenCalledOnce()
  })

  it('rejects an empty viewport without touching the clipboard', async () => {
    const copy = vi.fn(async () => undefined)
    const terminal = reader()

    await expect(copyVisibleText(terminal, copy)).rejects.toThrow('no visible terminal text')
    expect(copy).not.toHaveBeenCalled()
    expect(terminal.focus).toHaveBeenCalledOnce()
  })

  it('exports the exact selection and preserves cancellation', async () => {
    const result = { cancelled: true, filename: '', bytes: 0 }
    const exportText = vi.fn(async () => result)
    const terminal = reader({ selected: 'selected\ntext' })

    await expect(exportSelectedText(terminal, 'Production', exportText)).resolves.toEqual(result)
    expect(exportText).toHaveBeenCalledWith('Production', 'selected\ntext')
    expect(terminal.focus).toHaveBeenCalledOnce()
  })
})

function reader(value: { visible?: string; selected?: string } = {}) {
  return {
    visibleText: vi.fn(() => value.visible ?? ''),
    selectedText: vi.fn(() => value.selected ?? ''),
    focus: vi.fn(),
  }
}
