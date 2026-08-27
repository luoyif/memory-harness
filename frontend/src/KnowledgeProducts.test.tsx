import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { KnowledgeProducts } from './KnowledgeProducts'
import { APIClient } from './api'

afterEach(cleanup)

const productObject = {
  object_id: 'obj-brief', type_id: 'builtin.living-asset-vault.knowledge-product.v1', project_id: 'project-1', status: 'active', protection_class: 'standard', current_revision: 2,
  revision: { payload: { product_id: 'product-brief', product_type: 'project_brief', title: 'Memory Harness · 项目简报', summary: '2 个目标 · 1 项决策', body: '# 自动简报\n\n当前开发状态', format: 'markdown', source_refs: ['ev-1'], locked_fields: [], generation_status: 'auto' }, content_hash: 'hash-current', confidence: 1, importance: .85, plugin_id: 'builtin.living-asset-vault', plugin_version: '2.0.0', source_evidence_ids: ['ev-1'], source_object_ids: ['obj-memory-1'], created_at: '2026-08-22T00:00:00Z' },
  created_at: '2026-08-21T00:00:00Z', updated_at: '2026-08-22T00:00:00Z',
}

describe('Knowledge products', () => {
  it('edits a project brief through expected-revision governance and locks the body', async () => {
    const get = vi.fn(async (path:string) => {
      if (path.startsWith('/v1/harness/objects?')) return { objects: [productObject] }
      if (path === '/v1/harness/objects/obj-brief') return productObject
      if (path.startsWith('/v1/harness/objects/obj-brief/revisions')) return { revisions: [{ object_id: 'obj-brief', revision: 2, status: 'active', payload: productObject.revision.payload, content_hash: 'hash-current', created_at: '2026-08-22T00:00:00Z' }] }
      if (path.startsWith('/v1/harness/revision-reviews?object_id=obj-brief')) return { reviews: [] }
      throw new Error(`unexpected GET ${path}`)
    })
    const post = vi.fn(async () => ({}))
    render(<KnowledgeProducts api={{ get, post } as unknown as APIClient} projectID="project-1" />)
    fireEvent.click(await screen.findByRole('button', { name: /Memory Harness · 项目简报/ }))
    expect(await screen.findByText(/Object Store 是当前版本的唯一权威/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '人工修订' }))
    fireEvent.change(screen.getByLabelText('正文'), { target: { value: '# Owner narrative\n\n人工维护正文' } })
    fireEvent.click(screen.getByLabelText(/锁定 body/))
    fireEvent.change(screen.getByLabelText('修改原因'), { target: { value: 'Owner wants a stable narrative' } })
    fireEvent.click(screen.getByRole('button', { name: /创建待审核 Revision/ }))
    await waitFor(() => expect(post).toHaveBeenCalledWith('/v1/harness/objects/obj-brief/revisions', expect.objectContaining({
      expected_revision: 2, edit_reason: 'Owner wants a stable narrative', target_status: 'active',
      payload: expect.objectContaining({ body: '# Owner narrative\n\n人工维护正文', locked_fields: ['body'], generation_status: 'human_mixed' }),
    })))
  })

  it('captures owner-authored report as Evidence before materializing the product', async () => {
    const get = vi.fn(async (path:string) => {
      if (path.startsWith('/v1/harness/objects?')) return { objects: [] }
      throw new Error(`unexpected GET ${path}`)
    })
    const post = vi.fn(async (path:string) => {
      if (path === '/v1/import/text') return { captured: [{ evidence_id: 'ev-owner-report' }] }
      if (path === '/v1/harness/objects') return { object_id: 'obj-report' }
      throw new Error(`unexpected POST ${path}`)
    })
    render(<KnowledgeProducts api={{ get, post } as unknown as APIClient} projectID="project-1" />)
    fireEvent.click(await screen.findByRole('button', { name: /新建知识产品/ }))
    fireEvent.change(screen.getByLabelText('标题'), { target: { value: '阶段报告' } })
    fireEvent.change(screen.getByLabelText('摘要'), { target: { value: '本周进展' } })
    fireEvent.change(screen.getByLabelText('正文'), { target: { value: '本周完成了受治理资产和时间相关性开发。' } })
    fireEvent.click(screen.getByRole('button', { name: /保存并建立来源/ }))
    await waitFor(() => expect(post).toHaveBeenCalledWith('/v1/harness/objects', expect.objectContaining({
      type_id: 'builtin.living-asset-vault.knowledge-product.v1', project_id: 'project-1', status: 'active',
      source_evidence_ids: ['ev-owner-report'], payload: expect.objectContaining({ product_type: 'report', source_refs: ['ev-owner-report'], generation_status: 'human' }),
    })))
    expect(post.mock.calls[0][0]).toBe('/v1/import/text')
    expect(post.mock.calls[1][0]).toBe('/v1/harness/objects')
  })

  it('backfills an empty project brief without re-importing Evidence', async () => {
    let refreshed = false
    const get = vi.fn(async (path:string) => {
      if (path.startsWith('/v1/harness/objects?')) return { objects: refreshed ? [productObject] : [] }
      throw new Error(`unexpected GET ${path}`)
    })
    const post = vi.fn(async (path:string, body:unknown) => {
      void body
      if (path === '/v1/projects/project-1/knowledge-products/project-brief/refresh') {
        refreshed = true
        return { object: productObject, duplicate: false }
      }
      throw new Error(`unexpected POST ${path}`)
    })
    render(<KnowledgeProducts api={{ get, post } as unknown as APIClient} projectID="project-1" />)
    expect(await screen.findByText('还没有知识产品')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /生成 \/ 刷新项目简报/ }))
    await waitFor(() => expect(post).toHaveBeenCalledWith('/v1/projects/project-1/knowledge-products/project-brief/refresh', {}))
    expect(await screen.findByText(/项目简报已刷新到 R2/)).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /Memory Harness · 项目简报/ })).toBeInTheDocument()
    expect(post.mock.calls.some(([path]) => path === '/v1/import/text')).toBe(false)
  })

})
