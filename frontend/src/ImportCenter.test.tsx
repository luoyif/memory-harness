import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { APIClient, BlueprintCurrent, Project } from './api'
import { ImportCenter } from './ImportCenter'

afterEach(cleanup)

const project: Project = { project_id: 'project-personal', slug: 'personal', name: '个人', description: '', status: 'active', color: '#52715f', default_currency: 'CNY', budget_minor: 0 }
const blueprint = {
  assignment: { project_id: project.project_id, blueprint_id: 'builtin.default', blueprint_version: '1.0.0', blueprint_hash: 'sha256:x', status: 'inherited', activated_by: 'system', activated_at: '', updated_at: '' },
  blueprint: { blueprint_id: 'builtin.default', version: '1.0.0', plugin_id: 'builtin.core', name: '默认策略', description: '', content_hash: 'sha256:x', status: 'published', created_at: '', definition: { api_version: 'memory-harness.blueprint/v1alpha1', blueprint_id: 'builtin.default', version: '1.0.0', name: '默认策略', description: '', intent: '', tracks: [], policy: { evidence_mode: 'normalized_with_verbatim', model_boundary: 'configured_provider', default_context_budget: 12000, cross_project_recall: false } } },
  inherited: true,
  validation: { valid: true, errors: [], warnings: [], track_count: 0, enabled_component_count: 0, required_capabilities: [] },
} satisfies BlueprintCurrent

function mockAPI() {
  const get = vi.fn(async (path: string) => {
    if (path === '/v1/sources') return { sources: [] }
    if (path === '/v1/model/config') return { runtime: { mode: 'rules' }, providers: [] }
    if (path.endsWith('/blueprint')) return blueprint
    throw new Error(`unexpected GET ${path}`)
  })
  const post = vi.fn<(path: string, body: unknown) => Promise<{ pipeline: { knowledge_units: number; status: string } }>>()
    .mockResolvedValue({ pipeline: { knowledge_units: 2, status: 'completed' } })
  return { api: { get, post } as unknown as APIClient, get, post }
}

describe('ImportCenter', () => {
  it('imports pasted content into the selected project with a stable idempotency key', async () => {
    const { api, post } = mockAPI()
    render(<ImportCenter api={api} project={project} onNavigate={vi.fn()} />)
    await screen.findByText('默认策略')
    fireEvent.click(screen.getByRole('button', { name: /粘贴内容/ }))
    fireEvent.change(screen.getByLabelText('标题'), { target: { value: '产品决定' } })
    fireEvent.change(screen.getByLabelText('内容'), { target: { value: '我们决定把普通用户的导入和记忆浏览放在第一层。' } })
    fireEvent.click(screen.getByRole('button', { name: '导入并生成记忆' }))
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post.mock.calls[0][0]).toBe('/v1/import/text')
    expect(post.mock.calls[0][1]).toMatchObject({ project_id: project.project_id, source_system: 'manual-note', documents: [{ title: '产品决定' }] })
    expect(String((post.mock.calls[0][1] as Record<string, unknown>).idempotency_key)).toMatch(/^text:[0-9a-f]{8}$/)
    expect(await screen.findByText(/已生成 2 条有来源的关键信息/)).toBeInTheDocument()
  })

  it('keeps model configuration optional and directly reachable', async () => {
    const { api } = mockAPI()
    const navigate = vi.fn()
    render(<ImportCenter api={api} project={project} onNavigate={navigate} />)
    await screen.findByText('本地规则沉淀')
    fireEvent.click(screen.getByRole('button', { name: /打开模型与 Agent 配置/ }))
    expect(navigate).toHaveBeenCalledWith('connections')
  })
})
