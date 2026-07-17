import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { QuickSSHInput } from '../../lib/bridge/types'
import { QuickConnectDialog } from './QuickConnectDialog'

afterEach(cleanup)

describe('QuickConnectDialog', () => {
  it('submits a trimmed one-off SSH target', async () => {
    const connect = vi.fn(async (input: QuickSSHInput) => { void input })
    render(<QuickConnectDialog onCancel={vi.fn()} onConnect={connect} />)

    fireEvent.change(screen.getByLabelText('Host'), { target: { value: '  server.example.com  ' } })
    fireEvent.change(screen.getByLabelText('Port'), { target: { value: '2222' } })
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: ' deploy ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => expect(connect).toHaveBeenCalledWith({
      host: 'server.example.com', port: 2222, username: 'deploy',
      authentication: 'auto', identityFile: '',
    }))
  })

  it('keeps the dialog active and shows backend validation errors', async () => {
    const connect = vi.fn(async () => { throw new Error('host must not include a port') })
    render(<QuickConnectDialog onCancel={vi.fn()} onConnect={connect} />)

    fireEvent.change(screen.getByLabelText('Host'), { target: { value: 'server.example.com:22' } })
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    expect(await screen.findByRole('alert')).toHaveProperty('textContent', 'host must not include a port')
    expect(screen.getByRole('button', { name: 'Connect' })).toHaveProperty('disabled', false)
  })
})
