import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { MemoryPage, ReviewPage } from './App'
import { APIClient } from './api'

afterEach(cleanup)

describe('Memory library', () => {
  it('loads a small overview first and fetches paginated knowledge only when opened', async () => {
    const get = vi.fn(async (path: string) => {
      if (path.startsWith('/v1/layers')) return { layers: [
        { count: 1 }, { count: 1182 }, { count: 1 }, { count: 1172 }, { count: 3 }, { count: 5 },
      ], needs_review: 9 }
      if (path.startsWith('/v1/memories')) return { memories: [], total: 1172 }
      if (path.startsWith('/v1/episodes')) return { episodes: [], total: 1 }
      if (path.startsWith('/v1/knowledge-units')) return { knowledge_units: [{ unit_id: 'unit-1', statement: '真实知识点', evidence_id: 'ev-1', confidence: 0.9, observed_at: '2026-08-21T00:00:00Z' }], total: 1182 }
      throw new Error(`unexpected GET ${path}`)
    })
    const api = { get } as unknown as APIClient
    render(<MemoryPage api={api} projectID="project-inbox" openRun={vi.fn()} />)

    await screen.findByText('最近长期记忆')
    expect(get.mock.calls.some(([path]) => String(path).includes('/v1/knowledge-units'))).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: '知识点 1182' }))
    expect(await screen.findByText('真实知识点')).toBeInTheDocument()
    expect(screen.getByText('第 1 / 30 页 · 共 1182 条')).toBeInTheDocument()
    expect(get).toHaveBeenCalledWith('/v1/knowledge-units?project_id=project-inbox&limit=40&offset=0')
  })

  it('opens living knowledge, governed assets and the real relationship graph', async () => {
    const get = vi.fn(async (path: string) => {
      if (path.startsWith('/v1/layers')) return { layers: [
        { count: 1 }, { count: 2 }, { count: 1 }, { count: 2 }, { count: 1 }, { count: 1 },
      ], needs_review: 0 }
      if (path.startsWith('/v1/memories')) return { memories: [], total: 2 }
      if (path.startsWith('/v1/episodes')) return { episodes: [], total: 1 }
      if (path === '/v1/living?project_id=project-inbox') return { views: [{ view_id: 'living-1', title: '活跃上下文', summary: '可持续更新的项目知识', source_memory_ids: ['memory-1'], status: 'active' }] }
      if (path === '/v1/living/living-1') return { view: { title: '活跃上下文', summary: '可持续更新的项目知识' }, content: '# 项目上下文\n\n- 已确认模型分段提炼。', memories: [{ memory_id: 'memory-1' }] }
      if (path === '/v1/assets?project_id=project-inbox') return { assets: [{ asset_id: 'asset-1', title: '录音提炼流程', summary: '第一步去除口语噪声，然后保留来源并检查输出。', asset_type: 'procedure', status: 'candidate', validation_status: 'not_run', classification_status: 'classified', source_memory_ids: ['memory-1'] }] }
      if (path === '/v1/assets/asset-1') return { asset: { asset_id: 'asset-1', title: '录音提炼流程', summary: '第一步去除口语噪声，然后保留来源并检查输出。', asset_type: 'procedure', status: 'candidate', validation_status: 'not_run', classification_status: 'classified', source_memory_ids: ['memory-1'] }, governance: { object: { object_id: 'obj-asset-1', type_id: 'builtin.agent-assets.governed-asset.v3', project_id: 'project-inbox', status: 'candidate', current_revision: 1, revision: { object_id: 'obj-asset-1', revision: 1, status: 'candidate', payload: { asset_id: 'asset-1', asset_type: 'procedure', title: '录音提炼流程', body: '第一步去除口语噪声，然后保留来源并检查输出。', source_memory_ids: ['memory-1'], validation_status: 'not_run' }, content_hash: 'hash-asset-1', confidence: 1, importance: .8, plugin_id: 'builtin.agent-assets', plugin_version: '2.0.0', created_at: '2026-08-22T00:00:00Z' } }, revisions: [{ object_id: 'obj-asset-1', revision: 1, status: 'candidate', payload: { asset_id: 'asset-1', asset_type: 'procedure', title: '录音提炼流程', body: '第一步去除口语噪声，然后保留来源并检查输出。', source_memory_ids: ['memory-1'], validation_status: 'not_run' }, content_hash: 'hash-asset-1', confidence: 1, importance: .8, plugin_id: 'builtin.agent-assets', plugin_version: '2.0.0', created_at: '2026-08-22T00:00:00Z' }], reviews: [] } }
      if (path.startsWith('/v1/graph?project_id=project-inbox')) return { nodes: [{ id: 'ev-1', layer: 'evidence', label: '原始录音', status: 'canonical' }, { id: 'memory-1', layer: 'memory', label: '分段提炼策略', status: 'active' }], edges: [{ from: 'ev-1', to: 'memory-1', kind: 'supports' }] }
      if (path.startsWith('/v1/graph/semantic?project_id=project-inbox')) return { nodes: [
        { id: 'person-li', layer: 'entity', label: '李明', status: 'active', entity_type: 'person', confidence: .95 },
        { id: 'product-memory', layer: 'entity', label: '记忆产品', status: 'active', entity_type: 'product', confidence: .9 },
        { id: 'concept-isolated', layer: 'entity', label: '待补关系概念', status: 'active', entity_type: 'concept', confidence: .8 },
        { id: 'literal-shanghai', layer: 'literal', label: '上海', status: 'active', entity_type: 'literal', confidence: .9 },
      ], edges: [
        { id: 'assertion-1', from: 'person-li', to: 'product-memory', kind: 'semantic', label: 'launches', inverse_label: 'launched_by', evidence_id: 'ev-1' },
        { id: 'assertion-2', from: 'person-li', to: 'literal-shanghai', kind: 'attribute', label: 'launch_location', inverse_label: '', evidence_id: 'ev-1' },
      ] }
      throw new Error(`unexpected GET ${path}`)
    })
    const api = { get } as unknown as APIClient
    render(<MemoryPage api={api} projectID="project-inbox" openRun={vi.fn()} />)

    await screen.findByText('最近长期记忆')
    fireEvent.click(screen.getByRole('button', { name: '生长知识 1' }))
    fireEvent.click(await screen.findByRole('button', { name: /活跃上下文/ }))
    expect(await screen.findByText(/已确认模型分段提炼/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))

    fireEvent.click(screen.getByRole('button', { name: '能力资产 1' }))
    fireEvent.click(await screen.findByRole('button', { name: /录音提炼流程/ }))
    expect(await screen.findByText('当前生效内容')).toBeInTheDocument()
    expect(screen.getByText(/编辑不会覆盖 R1/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))

    fireEvent.click(screen.getByRole('button', { name: '知识图谱' }))
    expect((await screen.findAllByText('李明')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('记忆产品').length).toBeGreaterThan(0)
    expect(screen.getByText('launches')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: '语义关系图，2 个可见节点，1 条可见关系' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '未连接' }))
    expect(screen.getByRole('img', { name: '语义关系图，3 个可见节点，1 条可见关系' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '属性' }))
    expect(screen.getByRole('img', { name: '语义关系图，4 个可见节点，2 条可见关系' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '六层来源链' }))
    expect(await screen.findByText('原始录音')).toBeInTheDocument()
    expect(screen.getByText('分段提炼策略')).toBeInTheDocument()
  })

  it('opens a knowledge unit with subject, temporal context and exact Evidence quote', async () => {
    const unit = {
      unit_id: 'unit-structured', episode_id: 'episode-1', evidence_id: 'ev-structured', unit_type: 'decision', tier_hint: 'semantic', statement: '李明决定下周在上海发布记忆产品。', confidence: .94, risk_tier: 'B', status: 'candidate', observed_at: '2026-08-21T08:00:00Z',
      structure: {
        attribution: { source_speaker_ref: 'participant:wang', asserted_by_ref: 'participant:wang', subject_ref: 'person:li', subject_surface: '李明', resolution: 'resolved', owner_mapping: 'not_assumed' },
        frame: { subject: { entity_id: 'person:li', entity_type: 'person', canonical_name: '李明', resolution: 'resolved' }, predicate: 'launches', inverse_label: 'launched_by', object: { kind: 'entity', entity: { entity_id: 'product:memory', entity_type: 'product', canonical_name: '记忆产品', resolution: 'resolved' } }, action: '发布', locations: [{ role: 'destination', entity: { entity_id: 'location:shanghai', entity_type: 'location', canonical_name: '上海', resolution: 'resolved' } }] },
        temporal: { observed_at: '2026-08-21T08:00:00Z', event_time_text: '下周', precision: 'week', resolution: 'resolved' },
        epistemic: { polarity: 'positive', modality: 'asserted', confidence: .94, review_reasons: [] },
        provenance: { evidence_id: 'ev-structured', extractor_plugin: 'builtin.semantic-frame', extractor_version: '1.0.0', evidence_span: { start: 0, end: 17, quote: '李明决定下周在上海发布记忆产品。' } },
      },
    }
    const get = vi.fn(async (path: string) => {
      if (path.startsWith('/v1/layers')) return { layers: [{ count: 1 }, { count: 1 }, { count: 1 }, { count: 1 }, { count: 0 }, { count: 0 }], needs_review: 0 }
      if (path.startsWith('/v1/memories')) return { memories: [], total: 1 }
      if (path.startsWith('/v1/episodes')) return { episodes: [], total: 1 }
      if (path.startsWith('/v1/knowledge-units')) return { knowledge_units: [unit], total: 1 }
      throw new Error(`unexpected GET ${path}`)
    })
    render(<MemoryPage api={{ get } as unknown as APIClient} projectID="project-1" openRun={vi.fn()} />)
    await screen.findByText('最近长期记忆')
    fireEvent.click(screen.getByRole('button', { name: '知识点 1' }))
    fireEvent.click(await screen.findByRole('button', { name: /李明决定下周/ }))
    expect(await screen.findByText('来源说话者：participant:wang')).toBeInTheDocument()
    expect(screen.getByText('下周')).toBeInTheDocument()
    expect(screen.getByText('destination：上海')).toBeInTheDocument()
    expect(screen.getByText('反向：launched_by')).toBeInTheDocument()
    expect(screen.getByText('ev-structured')).toBeInTheDocument()
  })

  it('submits knowledge correction as a review-gated revision proposal', async () => {
    const unit = {
      unit_id: 'unit-correct', episode_id: 'episode-correct', evidence_id: 'ev-correct', unit_type: 'risk', tier_hint: 'semantic',
      statement: '旧风险描述。', normalized_key: '旧风险描述', confidence: .81, risk_tier: 'B', status: 'consolidated', scopes: ['project:1'],
      observed_at: '2026-08-21T08:00:00Z', created_at: '2026-08-21T08:01:00Z', schema_version: 'memory-harness.knowledge-unit/v2',
      structure: {
        attribution: { subject_surface: 'MemoryOS', resolution: 'resolved', owner_mapping: 'not_assumed' },
        frame: { subject: { entity_type: 'system', surface: 'MemoryOS', canonical_name: 'MemoryOS', resolution: 'resolved' }, predicate: 'has_risk', object: { kind: 'literal', value: '旧路径' } },
        temporal: { observed_at: '2026-08-21T08:00:00Z', valid_from: '2026-09-01T00:00:00Z', precision: 'day', resolution: 'resolved' },
        epistemic: { polarity: 'positive', modality: 'asserted', confidence: .81 },
        provenance: { evidence_id: 'ev-correct', episode_id: 'episode-correct', evidence_span: { start: 0, end: 6, quote: '旧风险描述。' } },
      },
    }
    const get = vi.fn(async (path: string) => {
      if (path === '/v1/knowledge-units/unit-correct?project_id=project-1') return { knowledge_unit: unit, governance: { current_revision: 4 } }
      if (path === '/v1/corrections/impact?project_id=project-1&kind=knowledge_unit&id=unit-correct') return { impact: { unit_ids: ['unit-correct'], assertion_ids: ['assertion-1'], temporal_fact_ids: ['fact-1'], evidence_ids: ['ev-correct'], authority_object_ids: ['obj-ku'], current_revisions: { 'obj-ku': 4 }, project_projection_ids: ['auto-risk-1'] } }
      if (path.startsWith('/v1/layers')) return { layers: [{ count: 1 }, { count: 1 }, { count: 1 }, { count: 1 }, { count: 0 }, { count: 0 }], needs_review: 0 }
      if (path.startsWith('/v1/memories')) return { memories: [], total: 1 }
      if (path.startsWith('/v1/episodes')) return { episodes: [], total: 1 }
      if (path.startsWith('/v1/knowledge-units')) return { knowledge_units: [unit], total: 1 }
      throw new Error(`unexpected GET ${path}`)
    })
    const post = vi.fn(async (path: string, body: unknown) => { void path; void body; return { review: { status: 'pending', revision: 5 } } })
    render(<MemoryPage api={{ get, post } as unknown as APIClient} projectID="project-1" openRun={vi.fn()} />)
    await screen.findByText('最近长期记忆')
    fireEvent.click(screen.getByRole('button', { name: '知识点 1' }))
    fireEvent.click(await screen.findByRole('button', { name: /旧风险描述/ }))
    fireEvent.click(screen.getByRole('button', { name: '提出人工修正' }))
    expect(await screen.findByText(/OWNER KNOWLEDGE REVISION · v4/)).toBeInTheDocument()
    expect(screen.getByText('Assertion').parentElement).toHaveTextContent('1 Assertion')
    expect(screen.getByText('时间事实').parentElement).toHaveTextContent('1 时间事实')
    fireEvent.change(screen.getByLabelText('知识表述'), { target: { value: '发布目标已改为安全上线。' } })
    fireEvent.change(screen.getByLabelText('类型'), { target: { value: 'goal' } })
    fireEvent.change(screen.getByLabelText('修正理由'), { target: { value: '对照原始 Evidence 后确认原分类错误' } })
    fireEvent.click(screen.getByRole('button', { name: '提交 Revision 等待审核' }))
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    const [path, body] = post.mock.calls[0]
    expect(path).toBe('/v1/knowledge-units/unit-correct/revision-proposals')
    expect(body).toMatchObject({ project_id: 'project-1', expected_revision: 4, edit_reason: '对照原始 Evidence 后确认原分类错误' })
    expect((body as { knowledge_unit: { statement: string; unit_type: string; evidence_id: string } }).knowledge_unit).toMatchObject({ statement: '发布目标已改为安全上线。', unit_type: 'goal', evidence_id: 'ev-correct' })
    expect(await screen.findByText(/当前版本尚未改变/)).toBeInTheDocument()
  })

  it('submits long-term memory correction as a governed revision without rewriting provenance', async () => {
    const memory = {
      memory_id: 'memory-correct', tier: 'semantic', asset_form: 'fact', domain: 'MemoryOS', status: 'active',
      summary: '旧的长期记忆摘要', body: '旧的长期记忆正文。', canonical_key: '旧的长期记忆正文',
      confidence: .82, importance: .72, strength: 2, source_evidence_ids: ['ev-memory'], source_episode_ids: ['episode-memory'],
      scopes: ['project:1'], visibility: 'private', observed_at: '2026-08-21T08:00:00Z', created_at: '2026-08-21T08:01:00Z', updated_at: '2026-08-21T08:02:00Z',
    }
    const get = vi.fn(async (path: string) => {
      if (path === '/v1/memories/memory-correct/governance?project_id=project-1') return { memory, governance: { current_revision: 3 } }
      if (path.startsWith('/v1/layers')) return { layers: [{ count: 1 }, { count: 1 }, { count: 1 }, { count: 1 }, { count: 0 }, { count: 0 }], needs_review: 0 }
      if (path.startsWith('/v1/memories')) return { memories: [memory], total: 1 }
      if (path.startsWith('/v1/episodes')) return { episodes: [], total: 1 }
      throw new Error(`unexpected GET ${path}`)
    })
    const post = vi.fn(async (path: string, body: unknown) => { void path; void body; return { review: { status: 'pending', revision: 4 } } })
    render(<MemoryPage api={{ get, post } as unknown as APIClient} projectID="project-1" openRun={vi.fn()} />)

    await screen.findByText('最近长期记忆')
    fireEvent.click(screen.getByRole('button', { name: '长期记忆 1' }))
    await screen.findByText('旧的长期记忆摘要')
    fireEvent.click(screen.getByRole('button', { name: '人工修正' }))
    expect(await screen.findByText(/OWNER MEMORY REVISION · v3/)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('摘要'), { target: { value: '人工确认后的长期记忆摘要' } })
    fireEvent.change(screen.getByLabelText('正文'), { target: { value: '人工确认后的长期记忆正文，只能经审核 Revision 生效。' } })
    fireEvent.change(screen.getByLabelText('修正理由'), { target: { value: '对照原始 Evidence 后修正表述' } })
    fireEvent.click(screen.getByRole('button', { name: '提交 Revision 等待审核' }))

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    const [path, body] = post.mock.calls[0]
    expect(path).toBe('/v1/memories/memory-correct/revision-proposals')
    expect(body).toMatchObject({ project_id: 'project-1', expected_revision: 3, edit_reason: '对照原始 Evidence 后修正表述' })
    expect((body as { memory: Record<string, unknown> }).memory).toMatchObject({
      memory_id: 'memory-correct', summary: '人工确认后的长期记忆摘要',
      body: '人工确认后的长期记忆正文，只能经审核 Revision 生效。', source_evidence_ids: ['ev-memory'],
    })
    expect(await screen.findByText(/当前版本尚未改变/)).toBeInTheDocument()
  })

})

describe('Protected memory review', () => {
  it('shows exact content and sends the backend approve decision', async () => {
    const operation = {
      operation_id: 'op-1', type: 'CREATE', status: 'review_required', risk_tier: 'C', confidence: 0.88,
      patch_json: JSON.stringify({ summary: '格式偏好' }), reason_codes: ['protected_tier'], created_at: '2026-08-21T00:00:00Z',
    }
    const get = vi.fn(async (path: string) => {
      if (path.startsWith('/v1/operations?')) return { operations: [operation] }
      if (path.startsWith('/v1/pipelines/reviews')) return { reviews: [] }
      if (path.startsWith('/v1/harness/revision-reviews')) return { reviews: [] }
      if (path === '/v1/operations/op-1') return {
        operation,
        proposed_memory: { summary: '格式偏好', body: '我每次都要求换一个格式。', tier: 'procedural', status: 'candidate' },
        knowledge_unit: { unit_id: 'unit-1', statement: '用户偏好调整输出格式。' },
        episode: { title: '录音导入', summary: '一次录音内容沉淀。' },
        evidence: [{ evidence_id: 'ev-1', preview: '我每次都跟他说，你给我换一个格式。', source_system: 'audio', observed_at: '2026-08-21T00:00:00Z' }],
      }
      throw new Error(`unexpected GET ${path}`)
    })
    const post = vi.fn(async () => ({}))
    const api = { get, post } as unknown as APIClient
    render(<ReviewPage api={api} openRun={vi.fn()} />)

    await screen.findByText('格式偏好')
    expect(screen.queryByText('undefined')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '查看来源与全文' }))
    expect(await screen.findByText('我每次都要求换一个格式。')).toBeInTheDocument()
    expect(screen.getByText('我每次都跟他说，你给我换一个格式。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '确认接受' }))
    await waitFor(() => expect(post).toHaveBeenCalledWith('/v1/operations/op-1/review', { decision: 'approve' }))
  })
})
