import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SnippetWorkspace } from './SnippetWorkspace'

const snippet = {
  id: 'deploy', name: 'Deploy', folder: 'Ops', tags: ['release'], body: 'deploy {{target}}',
  variables: ['target'], createdAt: '', updatedAt: '',
}

afterEach(cleanup)

describe('SnippetWorkspace', () => {
  it('requires a backend-rendered preview before execution', async () => {
    const renderSnippet = vi.fn(async (_id: string, values: Record<string, string>) => ({
      text: `deploy ${values.target}`, variables: ['target'],
    }))
    const execute = vi.fn(async () => undefined)
    render(<SnippetWorkspace snippets={[snippet]} targets={[{ id: 'one', title: 'Local', active: true }]}
      onCreate={vi.fn()} onUpdate={vi.fn()} onDelete={vi.fn()} onRender={renderSnippet} onExecute={execute} />)

    fireEvent.click(screen.getByRole('button', { name: 'Run Deploy' }))
    const runButton = screen.getByRole('button', { name: 'Run' })
    expect((runButton as HTMLButtonElement).disabled).toBe(true)
    fireEvent.change(screen.getByLabelText('target'), { target: { value: 'prod' } })
    fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
    await screen.findByText('deploy prod')
    expect((runButton as HTMLButtonElement).disabled).toBe(false)
    fireEvent.click(runButton)
    await waitFor(() => expect(execute).toHaveBeenCalledWith('deploy prod', ['one'], true))
  })

  it('requires confirmation for multiple terminal targets', async () => {
    render(<SnippetWorkspace snippets={[snippet]} targets={[
      { id: 'one', title: 'One', active: true }, { id: 'two', title: 'Two', active: false },
    ]} onCreate={vi.fn()} onUpdate={vi.fn()} onDelete={vi.fn()}
      onRender={vi.fn(async () => ({ text: 'deploy prod', variables: ['target'] }))} onExecute={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Run Deploy' }))
    fireEvent.click(screen.getByText('Two'))
    fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
    await screen.findByText('deploy prod')
    expect((screen.getByRole('button', { name: 'Run' }) as HTMLButtonElement).disabled).toBe(true)
    fireEvent.click(screen.getByText('Confirm execution in 2 terminals'))
    expect((screen.getByRole('button', { name: 'Run' }) as HTMLButtonElement).disabled).toBe(false)
  })
})
