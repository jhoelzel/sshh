import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Profile } from '../../lib/bridge/types'
import { ProfileExchangeDialog } from './ProfileExchangeDialog'

const profile: Profile = {
  id: 'imported', name: 'Production', protocol: 'ssh', host: 'prod.example.com', port: 22,
  username: 'deploy', authentication: 'auto', identityFile: '', shell: '', arguments: [],
  workingDirectory: '', environment: {}, tags: [], group: '', favorite: false,
  endpoint: 'deploy@prod.example.com:22', connectable: true,
}

afterEach(cleanup)

describe('ProfileExchangeDialog', () => {
  it('reports imported profiles and OpenSSH diagnostics', () => {
    const close = vi.fn()
    render(<ProfileExchangeDialog exchange={{
      kind: 'import',
      result: {
        cancelled: false, format: 'OpenSSH config', filename: 'config', imported: [profile],
        warnings: ['line 8: unsupported directive "ForwardAgent" was ignored'],
      },
    }} onClose={close} />)

    expect(screen.getByText('1 profile imported from OpenSSH config.')).toBeTruthy()
    expect(screen.getByText('Production')).toBeTruthy()
    expect(screen.getByText(/ForwardAgent/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Done' }))
    expect(close).toHaveBeenCalledOnce()
  })

  it('reports an export without rendering import sections', () => {
    render(<ProfileExchangeDialog exchange={{
      kind: 'export',
      result: { cancelled: false, filename: 'shh-h-profiles.json', exported: 3 },
    }} onClose={vi.fn()} />)

    expect(screen.getByText('3 profiles exported.')).toBeTruthy()
    expect(screen.queryByText('Imported profiles')).toBeNull()
  })
})
