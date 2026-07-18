import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Profile, ProfileInput } from '../../lib/bridge/types'
import { ProfileEditor } from './ProfileEditor'

const localProfile: Profile = {
  id: 'local-1', name: 'Development shell', protocol: 'local', host: '', port: 0,
  username: '', authentication: 'auto', identityFile: '', shell: '/bin/zsh', arguments: ['-l'],
  workingDirectory: '/tmp', environment: { ZED: 'last', LANG: 'C' }, tags: ['local'], group: '', favorite: false,
  endpoint: '/bin/zsh', connectable: true,
}

afterEach(cleanup)

describe('ProfileEditor environment overrides', () => {
  it('adds, edits, removes, and saves exact local environment values', async () => {
    const save = vi.fn(async (input: ProfileInput) => { void input })
    render(<ProfileEditor profile={localProfile} onCancel={vi.fn()} onSave={save} />)

    expect((screen.getByLabelText('Environment variable 1') as HTMLInputElement).value).toBe('LANG')
    expect((screen.getByLabelText('Environment variable 2') as HTMLInputElement).value).toBe('ZED')
    fireEvent.change(screen.getByLabelText('Environment value 1'), { target: { value: 'C.UTF-8' } })
    fireEvent.click(screen.getByRole('button', { name: 'Remove ZED' }))
    fireEvent.click(screen.getByRole('button', { name: 'Add variable' }))
    fireEvent.change(screen.getByLabelText('Environment variable 2'), { target: { value: 'EMPTY' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save profile' }))

    await waitFor(() => expect(save).toHaveBeenCalledOnce())
    expect(save.mock.calls[0][0].environment).toEqual({ LANG: 'C.UTF-8', EMPTY: '' })
  })

  it('preserves an existing multiline value when saving', async () => {
    const save = vi.fn(async (input: ProfileInput) => { void input })
    const multiline = 'first line\nsecond line'
    render(
      <ProfileEditor
        profile={{ ...localProfile, environment: { MULTILINE: multiline } }}
        onCancel={vi.fn()}
        onSave={save}
      />,
    )

    expect((screen.getByLabelText('Environment value 1') as HTMLTextAreaElement).value).toBe(multiline)
    fireEvent.click(screen.getByRole('button', { name: 'Save profile' }))

    await waitFor(() => expect(save).toHaveBeenCalledOnce())
    expect(save.mock.calls[0][0].environment).toEqual({ MULTILINE: multiline })
  })

  it('blocks duplicate and app-managed variable names before saving', async () => {
    const save = vi.fn(async (input: ProfileInput) => { void input })
    render(<ProfileEditor profile={{ ...localProfile, environment: { PATH: '/usr/bin' } }} onCancel={vi.fn()} onSave={save} />)

    fireEvent.click(screen.getByRole('button', { name: 'Add variable' }))
    fireEvent.change(screen.getByLabelText('Environment variable 2'), { target: { value: 'Path' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save profile' }))
    expect((await screen.findByRole('alert')).textContent).toContain('differ only by case')
    expect(save).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Environment variable 2'), { target: { value: 'TERM' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save profile' }))
    expect((await screen.findByRole('alert')).textContent).toContain('managed by shh-h')
    expect(save).not.toHaveBeenCalled()
  })

  it('shows overrides only for local profiles and clears them when converted to SSH', async () => {
    const save = vi.fn(async (input: ProfileInput) => { void input })
    const sshProfile: Profile = {
      ...localProfile,
      id: 'ssh-1', name: 'Remote', protocol: 'ssh', host: 'server.test', port: 22,
      environment: { LOCAL_ONLY: 'value' }, endpoint: 'server.test:22',
    }
    render(<ProfileEditor profile={sshProfile} onCancel={vi.fn()} onSave={save} />)

    expect(screen.queryByRole('button', { name: 'Add variable' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Local' }))
    expect((screen.getByLabelText('Environment variable 1') as HTMLInputElement).value).toBe('LOCAL_ONLY')
    fireEvent.click(screen.getByRole('button', { name: 'SSH' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save profile' }))

    await waitFor(() => expect(save).toHaveBeenCalledOnce())
    expect(save.mock.calls[0][0].environment).toEqual({})
  })
})
