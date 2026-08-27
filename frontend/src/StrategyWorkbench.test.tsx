import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { APIClient, BlueprintCurrent, BlueprintVersion, PluginVersion, Project } from './api'
import { cloneBlueprint, nextBlueprintVersion, StrategyWorkbench } from './StrategyWorkbench'

const project: Project = { project_id: 'project-1', slug: 'personal', name: '个人', description: '', status: 'active', color: '#52715f', default_currency: 'CNY', budget_minor: 0 }
const blueprint: BlueprintVersion = {
  blueprint_id: 'builtin.memory-harness-core.default', version: '1.0.0', plugin_id: 'builtin.memory-harness-core', name: '默认可编程记忆', description: 'default', content_hash: 'sha256:default', status: 'published', created_at: '2026-08-21T00:00:00Z',
  definition: {
    api_version: 'memory-harness.blueprint/v1alpha1', blueprint_id: 'builtin.memory-harness-core.default', version: '1.0.0', name: '默认可编程记忆', description: 'default', intent: 'keep evidence and grow memory',
    policy: { evidence_mode: 'normalized_with_verbatim', model_boundary: 'configured_provider', default_context_budget: 12000, cross_project_recall: false },
    tracks: [
      { track_id: 'growth', role: 'growth', display_name: '语义生长', description: 'growth', nodes: [{ node_id: 'knowledge', role: 'growth.knowledge', display_name: '知识点', binding_kind: 'memory_type', plugin_id: 'builtin.core-memory-growth', plugin_version: '2.0.0', component_id: 'builtin.core-memory-growth.knowledge-point', component_version: '1.0.0', enabled: true, required_capabilities: ['memory.materialize'], config: {} }] },
      { track_id: 'organization', role: 'organization', display_name: '空间组织', description: 'organization', nodes: [{ node_id: 'scope', role: 'organization.scope', display_name: 'Project / Wing', binding_kind: 'provider', plugin_id: 'builtin.palace-organization', plugin_version: '1.0.0', component_id: 'builtin.palace-organization.project-scope', component_version: '1.0.0', enabled: true, required_capabilities: ['evidence.read'], config: {} }] },
      { track_id: 'recall', role: 'recall', display_name: '召回成本', description: 'recall', nodes: [{ node_id: 'l3', role: 'recall.deep', display_name: 'L3 混合深搜', binding_kind: 'stage', plugin_id: 'builtin.hybrid-retrieval', plugin_version: '1.0.0', component_id: 'builtin.hybrid-retrieval.deep', component_version: '1.0.0', enabled: true, required_capabilities: ['memory.read'], config: {} }] },
    ],
  },
}
const current: BlueprintCurrent = { assignment: { project_id: 'project-1', blueprint_id: blueprint.blueprint_id, blueprint_version: blueprint.version, blueprint_hash: blueprint.content_hash, status: 'inherited', activated_by: 'system', activated_at: '', updated_at: '' }, blueprint, inherited: true, validation: { valid: true, errors: [], warnings: [], track_count: 3, enabled_component_count: 3, required_capabilities: ['evidence.read', 'memory.materialize', 'memory.read'] } }
const plugin: PluginVersion = { plugin_id: 'builtin.hybrid-retrieval', version: '1.0.0', name: '混合深度检索', publisher: 'memory-harness', trust_class: 'declarative', signature_status: 'bundled', status: 'enabled', permissions: ['memory.read'], contributions: { strategy_components: [{ component_id: 'builtin.hybrid-retrieval.deep', version: '1.0.0', display_name: 'L3 混合深搜', description: 'hybrid', role: 'recall.deep', kind: 'stage', capabilities: ['memory.read'] }] }, project_states: [] }
const contextPlugin: PluginVersion = { plugin_id: 'builtin.context-policy', version: '1.0.0', name: '上下文编译策略', publisher: 'memory-harness', trust_class: 'declarative', signature_status: 'bundled', status: 'enabled', permissions: ['memory.read','project.read'], contributions: { strategy_components: [
  { component_id: 'builtin.context-policy.profile-compiler', version: '1.0.0', display_name: '用途画像编译', description: 'profile', role: 'context.profile-compiler', kind: 'policy', configuration: '{"views":["dynamic_project","session_resume"]}', capabilities: ['memory.read','project.read'] },
  { component_id: 'builtin.context-policy.retrieval', version: '1.0.0', display_name: '上下文召回策略', description: 'retrieval', role: 'context.retrieval-policy', kind: 'policy', configuration: '{"kinds":["object","memory","evidence"]}', capabilities: ['memory.read'] },
  { component_id: 'builtin.context-policy.presentation', version: '1.0.0', display_name: '上下文呈现策略', description: 'presentation', role: 'context.presentation-policy', kind: 'policy', configuration: '{"profile":"profile"}', capabilities: ['memory.read'] },
  { component_id: 'builtin.context-policy.budget', version: '1.0.0', display_name: '上下文预算策略', description: 'budget', role: 'context.budget-policy', kind: 'policy', configuration: '{"profile_max_chars":7000}', capabilities: ['project.read'] },
] }, project_states: [] }

afterEach(cleanup)

describe('strategy workbench', () => {
  it('creates a project-owned immutable version number', () => {
    const first = cloneBlueprint(blueprint.definition, project, [blueprint])
    expect(first.blueprint_id).toBe('builtin.user-workflows.personal-memory')
    expect(first.version).toBe('1.0.0')
    expect(nextBlueprintVersion(first.blueprint_id, [{ ...blueprint, blueprint_id: first.blueprint_id, version: '1.2.9' }])).toBe('1.2.10')
  })

  it('clones, publishes and activates the full project Blueprint', async () => {
    const api = new APIClient({ endpoint: 'http://local', sessionID: 's', token: 't', csrf: 'c', expiresAt: '', version: '2.0.0' })
    vi.spyOn(api, 'get').mockImplementation(async path => {
      if (path === '/v1/blueprints') return { blueprints: [blueprint] }
      if (path.includes('/blueprint')) return current
      return { plugins: [plugin] }
    })
    const post = vi.spyOn(api, 'post').mockImplementation(async (path, body) => {
      if (path === '/v1/blueprints') {
        const definition = (body as { definition: BlueprintVersion['definition'] }).definition
        return { ...blueprint, blueprint_id: definition.blueprint_id, version: definition.version, name: definition.name, definition, content_hash: 'sha256:custom' }
      }
      return current.validation
    })
    const put = vi.spyOn(api, 'put').mockResolvedValue(current)
    render(<StrategyWorkbench api={api} projectID="project-1" projects={[{ project }]} />)

    expect(await screen.findByRole('heading', { name: '默认可编程记忆' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /克隆为项目方案/ }))
    expect(screen.getByDisplayValue('个人 · 可编程记忆')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /发布并启用/ }))

    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    const publishCall = post.mock.calls.find(call => call[0] === '/v1/blueprints')
    expect(publishCall?.[1]).toMatchObject({ plugin_id: 'builtin.user-workflows', definition: { blueprint_id: 'builtin.user-workflows.personal-memory', version: '1.0.0' } })
    expect(put.mock.calls[0][0]).toContain('/v1/projects/project-1/blueprint')
    expect(put.mock.calls[0][1]).toEqual({ blueprint_id: 'builtin.user-workflows.personal-memory', version: '1.0.0' })
  })

  it('adds the optional Context Track to a legacy three-track draft', async () => {
    const api = new APIClient({ endpoint: 'http://local', sessionID: 's', token: 't', csrf: 'c', expiresAt: '', version: '2.0.0' })
    vi.spyOn(api, 'get').mockImplementation(async path => {
      if (path === '/v1/blueprints') return { blueprints: [blueprint] }
      if (path.includes('/blueprint')) return current
      return { plugins: [plugin, contextPlugin] }
    })
    const post = vi.spyOn(api, 'post').mockImplementation(async (path, body) => {
      if (path === '/v1/blueprints') {
        const definition = (body as { definition: BlueprintVersion['definition'] }).definition
        return { ...blueprint, blueprint_id: definition.blueprint_id, version: definition.version, definition, content_hash: 'sha256:context' }
      }
      return current.validation
    })
    vi.spyOn(api, 'put').mockResolvedValue(current)
    render(<StrategyWorkbench api={api} projectID="project-1" projects={[{ project }]} />)
    await screen.findByRole('heading', { name: '默认可编程记忆' })
    fireEvent.click(screen.getByRole('button', { name: /克隆为项目方案/ }))
    fireEvent.click(screen.getByRole('button', { name: /启用可选 Context Track/ }))
    expect(screen.getByText('上下文编译')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '用途画像编译' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /发布并启用/ }))
    await waitFor(() => expect(post.mock.calls.some(call => call[0] === '/v1/blueprints')).toBe(true))
    const publishCall = post.mock.calls.find(call => call[0] === '/v1/blueprints')
    const definition = (publishCall?.[1] as { definition: BlueprintVersion['definition'] }).definition
    expect(definition.tracks.map(track => track.role)).toEqual(['growth', 'organization', 'recall', 'context'])
    expect(definition.tracks.at(-1)?.nodes).toHaveLength(4)
  })
})
