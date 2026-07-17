import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ComponentProps } from 'react'
import type { FileSession, Profile, RemotePathFavorite, Transfer, TransferResume } from '../../lib/bridge/types'
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

  it('keeps policy-skipped transfers visible', () => {
    const skipped: Transfer = {
      id: 'transfer-1', leaseId: session.leaseId, sessionId: session.id,
      direction: 'upload', source: '/tmp/report.csv', destination: '/srv/app/report.csv',
      bytes: 0, total: 120, resumeId: '', resumedFrom: 0, state: 'skipped', message: 'Destination already exists',
      startedAt: '2026-07-17T12:00:00Z', finishedAt: '2026-07-17T12:00:00Z',
    }
    render(<FileBrowser {...props({ transfers: [skipped] })} />)

    expect(screen.getByText('report.csv')).toBeTruthy()
    expect(screen.getByText('Skipped')).toBeTruthy()
  })

  it('resumes or discards a persisted interrupted transfer', async () => {
    const resumeTransfer = vi.fn(async () => undefined)
    const discardResume = vi.fn(async () => undefined)
    const resume: TransferResume = {
      id: 'resume-1', profileId: profile.id, direction: 'download',
      source: '/srv/app/archive.tar', destination: '/tmp/archive.tar',
      bytes: 50, total: 100, available: true, message: 'connection closed',
      createdAt: '2026-07-17T12:00:00Z', updatedAt: '2026-07-17T12:01:00Z',
    }
    render(<FileBrowser {...props({ resumes: [resume], onResumeTransfer: resumeTransfer, onDiscardResume: discardResume })} />)

    expect(screen.getByText('Interrupted')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Resume transfer' }))
    await waitFor(() => expect(resumeTransfer).toHaveBeenCalledWith(resume.id))

    fireEvent.click(screen.getByRole('button', { name: 'Discard partial transfer' }))
    await waitFor(() => expect(discardResume).toHaveBeenCalledWith(resume.id))
  })

  it('keeps an unavailable resume discardable', () => {
    const resume: TransferResume = {
      id: 'resume-2', profileId: profile.id, direction: 'upload',
      source: '/tmp/archive.tar', destination: '/srv/app/archive.tar',
      bytes: 50, total: 100, available: false, message: 'Local source changed since the interruption',
      createdAt: '2026-07-17T12:00:00Z', updatedAt: '2026-07-17T12:01:00Z',
    }
    render(<FileBrowser {...props({ resumes: [resume] })} />)

    expect(screen.getByText(resume.message)).toBeTruthy()
    expect((screen.getByRole('button', { name: 'Resume transfer' }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: 'Discard partial transfer' }) as HTMLButtonElement).disabled).toBe(false)
  })
})

function props(overrides: Partial<ComponentProps<typeof FileBrowser>> = {}): ComponentProps<typeof FileBrowser> {
  return {
    profile, session, path: '/srv/app', files: [], transfers: [], resumes: [], favorites: [], loading: false,
    onNavigate: vi.fn(async () => undefined), onRefresh: vi.fn(async () => undefined),
    onUpload: vi.fn(async () => undefined), onDownload: vi.fn(async () => undefined),
    onCreateDirectory: vi.fn(async () => undefined), onRename: vi.fn(async () => undefined),
    onDelete: vi.fn(async () => undefined), onChmod: vi.fn(async () => undefined),
    onCancelTransfer: vi.fn(async () => undefined), onResumeTransfer: vi.fn(async () => undefined),
    onDiscardResume: vi.fn(async () => undefined), onCreateFavorite: vi.fn(async () => undefined),
    onDeleteFavorite: vi.fn(async () => undefined), onClose: vi.fn(async () => undefined),
    ...overrides,
  }
}
