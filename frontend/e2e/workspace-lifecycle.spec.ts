import { expect, test, type Locator, type Page } from '@playwright/test'
import { backendCallCount, emitBackendEvent, installWailsMock } from './wailsMock'

const closeRequestedEvent = 'shhh:close-requested'

test.beforeEach(async ({ page }) => {
  await installWailsMock(page)
})

test('focus, shortcuts, split layout, tab close, and shutdown decisions stay coherent', async ({ page }) => {
  const browserErrors: string[] = []
  page.on('pageerror', (error) => browserErrors.push(error.message))
  page.on('console', (message) => {
    if (message.type() === 'error') browserErrors.push(message.text())
  })

  await page.goto('/')
  await expect(page.locator('.app-shell')).toHaveAttribute('data-theme', 'dark')
  await expect(page.getByRole('button', { name: 'New local terminal' })).toBeEnabled()
  const applicationModifier = await page.evaluate(() =>
    navigator.userAgent.includes('Macintosh') ? 'Meta' : 'Control',
  )

  await page.keyboard.press(`${applicationModifier}+Shift+T`)
  const firstTab = page.getByRole('tab', { name: 'Local 1' })
  await expect(firstTab).toHaveAttribute('aria-selected', 'true')
  await expectTerminalFocus(page, firstTab)

  await page.keyboard.press(`${applicationModifier}+Shift+T`)
  const secondTab = page.getByRole('tab', { name: 'Local 2' })
  await expect(secondTab).toHaveAttribute('aria-selected', 'true')
  await expectTerminalFocus(page, secondTab)

  await page.keyboard.press('Control+Tab')
  await expect(firstTab).toHaveAttribute('aria-selected', 'true')
  await expect(secondTab).toHaveAttribute('aria-selected', 'false')
  await expectTerminalFocus(page, firstTab)

  await page.keyboard.press('Control+Shift+Tab')
  await expect(secondTab).toHaveAttribute('aria-selected', 'true')
  await expectTerminalFocus(page, secondTab)

  await page.getByRole('button', { name: 'Split terminal right' }).click()
  const split = page.getByRole('separator', { name: 'Resize terminal split' })
  await expect(split).toHaveAttribute('aria-orientation', 'vertical')
  await expect(split).toHaveAttribute('aria-valuenow', '50')
  await expect(page.getByRole('tablist', { name: 'Terminal sessions' })).toHaveAttribute('aria-multiselectable', 'true')
  await expect(firstTab).toHaveAttribute('aria-selected', 'true')
  await expect(secondTab).toHaveAttribute('aria-selected', 'true')

  await split.focus()
  await split.press('ArrowRight')
  await expect(split).toHaveAttribute('aria-valuenow', '55')

  await page.getByRole('button', { name: 'Focus Local 1 pane' }).click()
  await expect(page.getByRole('button', { name: 'Focus Local 1 pane' })).toHaveAttribute('aria-pressed', 'true')
  await expectTerminalFocus(page, firstTab)

  await page.getByRole('button', { name: 'Close Local 1' }).click()
  let dialog = page.getByRole('dialog', { name: 'Close this terminal?' })
  await expect(dialog).toContainText('The shell process and its child processes will be terminated.')
  await dialog.getByRole('button', { name: 'Cancel' }).click()
  await expect(firstTab).toBeVisible()
  expect(await backendCallCount(page, 'CloseTerminal')).toBe(0)

  await page.getByRole('button', { name: 'Close Local 1' }).click()
  dialog = page.getByRole('dialog', { name: 'Close this terminal?' })
  await dialog.getByRole('button', { name: 'Close' }).click()
  await expect(firstTab).toHaveCount(0)
  await expect(split).toHaveCount(0)
  await expect.poll(() => backendCallCount(page, 'CloseTerminal')).toBe(1)

  await emitBackendEvent(page, closeRequestedEvent)
  dialog = page.getByRole('dialog', { name: 'Close running sessions?' })
  await expect(dialog).toContainText('1 active resource will be closed.')
  await dialog.getByRole('button', { name: 'Cancel' }).click()
  expect(await backendCallCount(page, 'ConfirmApplicationClose')).toBe(0)

  await emitBackendEvent(page, closeRequestedEvent)
  dialog = page.getByRole('dialog', { name: 'Close running sessions?' })
  await dialog.getByRole('button', { name: 'Close' }).click()
  await expect.poll(() => backendCallCount(page, 'ConfirmApplicationClose')).toBe(1)

  expect(browserErrors).toEqual([])
})

async function expectTerminalFocus(page: Page, tab: Locator): Promise<void> {
  const panelId = await tab.getAttribute('aria-controls')
  expect(panelId).toBeTruthy()
  await expect(page.locator(`#${panelId} .xterm-helper-textarea`)).toBeFocused()
}
