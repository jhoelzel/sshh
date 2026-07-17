import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { TunnelWorkspace } from './TunnelWorkspace'
import type { Profile, TunnelConfig } from '../../lib/bridge/types'

const profile: Profile = {
  id: 'ssh', name: 'Server', protocol: 'ssh', host: 'server.test', port: 22, username: 'tester',
  authentication: 'agent', identityFile: '', shell: '', arguments: [], workingDirectory: '',
  environment: {}, tags: [], group: '', favorite: false, endpoint: 'tester@server.test:22', connectable: true,
}

const config: TunnelConfig = {
  id: 'tunnel', name: 'Database', profileId: 'ssh', kind: 'local', bindAddress: '127.0.0.1', bindPort: 15432,
  destinationHost: 'db.internal', destinationPort: 5432, autoStart: false, reconnect: false,
  createdAt: '', updatedAt: '',
}

function props() {
  return {
    configs: [] as TunnelConfig[], profiles: [profile], snapshots: [], connecting: false,
    onCreate: vi.fn(async () => undefined), onUpdate: vi.fn(async () => undefined),
    onDelete: vi.fn(async () => undefined), onStart: vi.fn(async () => undefined),
    onStop: vi.fn(async () => undefined), onRestart: vi.fn(async () => undefined),
  }
}

describe('TunnelWorkspace', () => {
  it('requires explicit confirmation before saving an all-interface bind', () => {
    render(<TunnelWorkspace {...props()} />)
    fireEvent.click(screen.getByRole('button', { name: 'New tunnel' }))
    fireEvent.change(screen.getByLabelText('Bind address'), { target: { value: '0.0.0.0' } })

    const save = screen.getByRole('button', { name: 'Save tunnel' })
    expect((save as HTMLButtonElement).disabled).toBe(true)
    fireEvent.click(screen.getByLabelText('Allow connections from every network interface'))
    expect((save as HTMLButtonElement).disabled).toBe(false)
  })

  it('starts a stopped saved tunnel', async () => {
    const callbacks = props()
    callbacks.configs = [config]
    render(<TunnelWorkspace {...callbacks} />)

    fireEvent.click(screen.getByRole('button', { name: 'Start Database' }))
    expect(callbacks.onStart).toHaveBeenCalledWith(config)
  })
})
