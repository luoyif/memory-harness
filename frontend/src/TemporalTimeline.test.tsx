import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TemporalTimeline } from './TemporalTimeline'
import { APIClient } from './api'
import type { KnowledgeUnit } from './MemorySemantics'

afterEach(cleanup)

describe('Temporal timeline', () => {
  it('keeps relative time pending until Owner submits a review-gated confirmation', async () => {
    const unit: KnowledgeUnit = {
      unit_id: 'unit-time-1', episode_id: 'episode-1', evidence_id: 'ev-time-1', unit_type: 'goal', tier_hint: 'semantic',
      statement: '下周完成 Memory Harness 时间能力。', normalized_key: '下周完成', confidence: .91, risk_tier: 'B', status: 'consolidated', scopes: ['project:test'],
      observed_at: '2026-08-22T08:00:00Z', created_at: '2026-08-22T08:00:01Z', schema_version: 'memory-harness.knowledge-unit/v2',
      structure: {
        attribution: { subject_surface: 'Memory Harness', resolution: 'resolved', owner_mapping: 'not_assumed' },
        frame: { subject: { entity_type: 'project', surface: 'Memory Harness', canonical_name: 'Memory Harness', resolution: 'resolved' }, predicate: 'targets', object: { kind: 'literal', value: '时间能力' } },
        temporal: { observed_at: '2026-08-22T08:00:00Z', event_time_text: '下周', precision: 'week', resolution: 'relative_pending', anchor_evidence_time: '2026-08-22T08:00:00Z' },
        epistemic: { polarity: 'positive', modality: 'planned', confidence: .91 },
        provenance: { evidence_id: 'ev-time-1', episode_id: 'episode-1', evidence_span: { start: 0, end: 20, quote: '下周完成 Memory Harness 时间能力。' } },
      },
    }
    const get = vi.fn(async (path: string) => {
      if (path.startsWith('/v1/timeline?')) return { project_id: 'project-test', anchor_at: '2026-08-22T12:00:00Z', events: [], relations: [], correlations: [], counts: {} }
      if (path === '/v1/knowledge-units?project_id=project-test&limit=300') return { knowledge_units: [unit] }
      if (path === '/v1/knowledge-units/unit-time-1?project_id=project-test') return { knowledge_unit: unit, governance: { current_revision: 2 } }
      throw new Error(`unexpected GET ${path}`)
    })
    const post = vi.fn(async (path: string, body: unknown) => { void path; void body; return { review: { status: 'pending', revision: 3 } } })
    render(<TemporalTimeline api={{ get, post } as unknown as APIClient} projectID="project-test" />)

    expect(await screen.findByText('待确认的相对时间')).toBeInTheDocument()
    expect(screen.getByText('下周')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '确认时间' }))
    expect(await screen.findByText(/确认“下周”对应的真实时间/)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('确认开始时间'), { target: { value: '2026-08-29T10:00' } })
    fireEvent.change(screen.getByLabelText('确认理由'), { target: { value: '根据 Evidence 时间确认下周对应日期' } })
    fireEvent.click(screen.getByRole('button', { name: '提交 Revision 等待审核' }))

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    const [path, body] = post.mock.calls[0]
    expect(path).toBe('/v1/knowledge-units/unit-time-1/revision-proposals')
    expect(body).toMatchObject({ project_id: 'project-test', expected_revision: 2, edit_reason: '根据 Evidence 时间确认下周对应日期' })
    const temporal = (body as { knowledge_unit: KnowledgeUnit }).knowledge_unit.structure?.temporal
    expect(temporal?.resolution).toBe('resolved')
    expect(temporal?.valid_from).toBe(new Date('2026-08-29T10:00').toISOString())
    expect(temporal?.occurred_from).toBe(new Date('2026-08-29T10:00').toISOString())
    expect(await screen.findByText(/已进入待审核 Revision/)).toBeInTheDocument()
  })
})
