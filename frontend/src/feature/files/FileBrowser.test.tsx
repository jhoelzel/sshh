import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ComponentProps } from 'react'
import type { FileSession, Profile, RemotePathFavorite } from '../../lib/bridge/types'
import { FileBrowser } from './FileBrowser'

const profile: Profile = {
  id: 'profile-1', name: 'Production', protocol: 'ssh', host: 'server.test', port: 22,
  username: 'tester', authentication: 'agent', identityFile: '', shell: '', arguments: [],
  workingDirectory: '', environment: {}, tags: [], group: '', favorite: false,
  endpoint: 'tester@server.test:22', connectable: true,
}

const session: FileSession = {
  id: 'files-1', leaseId: 'lease-1', profileId: profile.id, root: '/srv/app', openedAt: '2026-07-17T12:00:00Z',
}

const favorite: RemotePathFavorite = {
  id: 'favorite-1', profileId: profile.id, path: '/srv/app', createdAt: '2026-07-17T12:00:00Z',
}

afterEach(cleanup)

describe('FileBrowser remote path favorites', () => {
  it('adds the current path and navigates from the favorites menu', async () => {
    const createFavorite = vi.fn(async () => undefined)
    const navigate = vi.fn(async () => undefined)
    render(<FileBrowser {...props({ favorites: [{ ...favorite, path: '/var/log' }], onCreateFavorite: createFavorite, onNavigate: navigate })} />)

    fireEvent.click(screen.getByRole('button', { name: 'Add current path to favorites' }))
    await waitFor(() => expect(createFavorite).toHaveBeenCalledWith('/srv/app'))

    fireEvent.change(screen.getByLabelText('Favorite remote paths'), { target: { value: '/var/log' } })
    await waitFor(() => expect(navigate).toHaveBeenCalledWith('/var/log'))
  })

  it('removes the current favorite by its stable id', async () => {
    const deleteFavorite = vi.fn(async () => undefined)
    render(<FileBrowser {...props({ favorites: [favorite], onDeleteFavorite: deleteFavorite })} />)

    fireEvent.click(screen.getByRole('button', { name: 'Remove current path from favorites' }))
    await waitFor(() => expect(deleteFavorite).toHaveBeenCalledWith(favorite.id))
  })
})

function props(overrides: Partial<ComponentProps<typeof FileBrowser>> = {}): ComponentProps<typeof FileBrowser> {
  return {
    profile, session, path: '/srv/app', files: [], transfers: [], favorites: [], loading: false,
    onNavigate: vi.fn(async () => undefined), onRefresh: vi.fn(async () => undefined),
    onUpload: vi.fn(async () => undefined), onDownload: vi.fn(async () => undefined),
    onCreateDirectory: vi.fn(async () => undefined), onRename: vi.fn(async () => undefined),
    onDelete: vi.fn(async () => undefined), onChmod: vi.fn(async () => undefined),
    onCancelTransfer: vi.fn(async () => undefined), onCreateFavorite: vi.fn(async () => undefined),
    onDeleteFavorite: vi.fn(async () => undefined), onClose: vi.fn(async () => undefined),
    ...overrides,
  }
}
