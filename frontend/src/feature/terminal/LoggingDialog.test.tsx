import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LoggingDialog } from './LoggingDialog'

afterEach(cleanup)

describe('LoggingDialog', () => {
  it('starts with the selected timestamp policy', async () => {
    const start = vi.fn(async () => undefined)
    render(<LoggingDialog title="Local" onCancel={vi.fn()} onStart={start} />)

    fireEvent.click(screen.getByLabelText('Prefix lines with timestamps'))
    fireEvent.click(screen.getByRole('button', { name: 'Start logging' }))
    await waitFor(() => expect(start).toHaveBeenCalledWith(true))
  })
})
