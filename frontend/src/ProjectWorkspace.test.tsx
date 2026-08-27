import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { APIClient, Project } from './api'
import { ProjectWorkspace } from './ProjectWorkspace'

const project: Project = { project_id: 'project-1', slug: 'memory', name: '记忆产品', description: '统一的长期记忆应用', status: 'active', color: '#52715f', default_currency: 'CNY', budget_minor: 0 }
const summary = { project, metrics: { evidence: 3, episodes: 1, memories: 2, facts: 1, open_goals: 0, open_risks: 0, pending_review: 0 }, finance: { currencies: [] } }

describe('ProjectWorkspace', () => {
  it('shows the operating memory and creates a project goal', async () => {
    const get = vi.fn(async () => ({ summary, goals: [], milestones: [], decisions: [], risks: [], context_blocks: [], finance_accounts: [] }))
    const post = vi.fn(async () => ({}))
    const api = { get, post } as unknown as APIClient
    render(<ProjectWorkspace api={api} projectID={project.project_id} projects={[summary]} select={vi.fn()} onNavigate={vi.fn()} />)
    await screen.findByRole('heading', { name: '记忆产品' })
    fireEvent.click(screen.getAllByRole('button', { name: /^新增$/ })[0])
    expect(screen.getByRole('heading', { name: '新增核心上下文' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    fireEvent.click(screen.getAllByRole('button', { name: '新增' })[1])
    fireEvent.change(screen.getByLabelText('标题'), { target: { value: '完成首次使用流程' } })
    fireEvent.click(screen.getByRole('button', { name: '保存记录' }))
    await waitFor(() => expect(post).toHaveBeenCalledWith('/v1/goals', expect.objectContaining({ project_id: project.project_id, title: '完成首次使用流程' })))
  })
})
