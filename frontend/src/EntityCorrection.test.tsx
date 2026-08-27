import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EntityCorrectionModal } from './EntityCorrection'
import type { APIClient } from './api'

afterEach(cleanup)

describe('Entity correction', () => {
  const impact = {
    unit_ids: ['unit-a', 'unit-b'], assertion_ids: ['a1', 'a2'], temporal_fact_ids: ['f1'], evidence_ids: ['ev-1'],
    authority_object_ids: ['obj-a', 'obj-b'], current_revisions: { 'obj-a': 2, 'obj-b': 3 }, project_projection_ids: ['goal-1'],
  }
  it('submits rename as revision proposals and never approves directly', async () => {
    const post = vi.fn(async (path: string, body: unknown) => { void path; void body; return { reviews: [{ review_id: 'r1' }, { review_id: 'r2' }] } })
    const submitted = vi.fn()
    render(<EntityCorrectionModal api={{ post } as unknown as APIClient} projectID="project-1" target={{ id: 'entity-old', label: 'MemoryOS', entityType: 'system', impact }} graph={{ nodes: [], edges: [] }} onClose={vi.fn()} onSubmitted={submitted} />)

    expect(screen.getByText('Knowledge Unit').parentElement).toHaveTextContent('2 Knowledge Unit')
    fireEvent.change(screen.getByLabelText('规范名称'), { target: { value: 'Memory Harness' } })
    fireEvent.change(screen.getByLabelText('修改原因'), { target: { value: '统一规范名称并保留历史来源' } })
    fireEvent.click(screen.getByRole('button', { name: '创建待审核 Revision' }))
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    const [path, body] = post.mock.calls[0]
    expect(path).toBe('/v1/entities/entity-old/revision-proposals')
    expect(body).toMatchObject({ project_id: 'project-1', action: 'rename', canonical_name: 'Memory Harness', entity_type: 'system', edit_reason: '统一规范名称并保留历史来源' })
    expect(post.mock.calls.some(([called]) => String(called).includes('/decision'))).toBe(false)
    expect(submitted).toHaveBeenCalledWith(expect.stringContaining('2 个待审核 Revision'))
  })

  it('requires explicit unit selection for split', async () => {
    const post = vi.fn(async (path: string, body: unknown) => { void path; void body; return { reviews: [{ review_id: 'r1' }] } })
    render(<EntityCorrectionModal api={{ post } as unknown as APIClient} projectID="project-1" target={{ id: 'entity-old', label: 'MemoryOS', entityType: 'system', impact }} graph={{ nodes: [], edges: [] }} onClose={vi.fn()} onSubmitted={vi.fn()} />)
    fireEvent.click(screen.getByLabelText(/Split/))
    fireEvent.change(screen.getByLabelText('规范名称'), { target: { value: 'MemoryOS Research' } })
    fireEvent.change(screen.getByLabelText('修改原因'), { target: { value: '只拆分研究相关断言' } })
    fireEvent.click(screen.getByLabelText('unit-a'))
    fireEvent.click(screen.getByRole('button', { name: '创建待审核 Revision' }))
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post.mock.calls[0][1]).toMatchObject({ action: 'split', unit_ids: ['unit-a'] })
  })
})
