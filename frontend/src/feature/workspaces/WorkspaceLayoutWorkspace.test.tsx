import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceLayout } from '../../lib/bridge/types'
import { WorkspaceLayoutWorkspace } from './WorkspaceLayoutWorkspace'

const layout: WorkspaceLayout = {
  id: 'layout-1',
  name: 'Operations',
  tabs: [
    { profileId: 'profile-1', title: 'Production', endpoint: 'prod.example:22' },
    { profileId: 'profile-2', title: 'Logs', endpoint: 'logs.example:22' },
  ],
  activeTab: 1,
  split: { axis: 'row', primaryTab: 0, secondaryTab: 1, ratio: 0.5 },
  createdAt: '2026-07-17T08:00:00Z',
  updatedAt: '2026-07-17T09:00:00Z',
}

afterEach(cleanup)

describe('WorkspaceLayoutWorkspace', () => {
  it('creates a named snapshot of the current tabs', async () => {
    const create = vi.fn(async () => undefined)
    render(<WorkspaceLayoutWorkspace layouts={[]} savableTabCount={2} onCreate={create} onRename={vi.fn()} onReplace={vi.fn()} onRestore={vi.fn()} onDelete={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Save current' }))
    fireEvent.change(screen.getByLabelText('Layout name'), { target: { value: 'Daily work' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save layout' }))

    await waitFor(() => expect(create).toHaveBeenCalledWith('Daily work'))
  })

  it('restores a layout through the explicit callback', async () => {
    const restore = vi.fn(async () => undefined)
    render(<WorkspaceLayoutWorkspace layouts={[layout]} savableTabCount={1} onCreate={vi.fn()} onRename={vi.fn()} onReplace={vi.fn()} onRestore={restore} onDelete={vi.fn()} />)

    expect(screen.getByText('2 terminals, split')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Restore Operations' }))
    await waitFor(() => expect(restore).toHaveBeenCalledWith(layout))
  })

  it('confirms before replacing a saved layout', async () => {
    const replace = vi.fn(async () => undefined)
    render(<WorkspaceLayoutWorkspace layouts={[layout]} savableTabCount={1} onCreate={vi.fn()} onRename={vi.fn()} onReplace={replace} onRestore={vi.fn()} onDelete={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Replace Operations' }))
    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    await waitFor(() => expect(replace).toHaveBeenCalledWith(layout))
  })
})
