import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ContextExchangeInspector } from './ContextExchangeInspector'
import type { APIClient } from './api'

afterEach(cleanup)

const run = {
  run_id:'run-context-1', project_id:'project-1', caller_type:'agent', caller_id:'agent-1', channel:'context-bridge',
  pipeline_id:'builtin.context-bridge.exchange', pipeline_version:'1.0.0', pipeline_hash:'pipeline-hash', status:'running', created_at:'2026-08-22T12:00:00Z',
}
const plan = {
  plan_id:'plan-1', plan_hash:'plan-hash-123456789', project_id:'project-1', agent_id:'agent-1', request_fingerprint:'req-hash',
  blueprint_id:'builtin.default', blueprint_version:'1.0.0', blueprint_hash:'bp-hash', budget:{max_tokens:2048,max_chars:8000},
  items:[{item_id:'item-1',source_kind:'object',source_id:'obj-1',revision:3,content_hash:'content-hash-123456',project_id:'project-1',reason_codes:['unified_recall','temporal:active'],priority:92,token_estimate:120,presentation:'summary'}],
  created_at:'2026-08-22T12:00:00Z', expires_at:'2026-08-22T12:15:00Z',
}

describe('Context exchange inspector', () => {
  it('never claims delivery before a Receipt exists', async () => {
    const get = vi.fn(async (path:string) => { void path; return { run, plan, delivery_status:{'item-1':'delivery_unverified'} } })
    render(<ContextExchangeInspector api={{get} as unknown as APIClient} runID="run-context-1" />)
    expect(await screen.findByText('送达未验证')).toBeInTheDocument()
    expect(screen.getByText(/Plan ≠ 已送达/)).toBeInTheDocument()
    expect(screen.getByText('已送达').parentElement).toHaveTextContent('0已送达')
  })
  it('shows verified delivery, receipt evidence and actual usage separately', async () => {
    const receipt = {
      receipt_id:'receipt-1',receipt_hash:'receipt-hash-123456',evidence_level:'harness_observed',completeness:'complete',latency_ms:43,received_at:'2026-08-22T12:00:02Z',
      retention:{mode:'session',redaction:'supported'},items:[{item_id:'item-1',status:'delivered',revision:3,content_hash:'content-hash-123456',presentation:'summary',actual_tokens:98,compaction:'none'}],
    }
    const get = vi.fn(async (path:string) => { void path; return { run:{...run,status:'completed'}, plan, receipt, delivery_status:{'item-1':'delivered'} } })
    render(<ContextExchangeInspector api={{get} as unknown as APIClient} runID="run-context-1" />)
    expect((await screen.findAllByText('已送达')).length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText(/harness_observed · complete/)).toBeInTheDocument()
    expect(screen.getByText('98 tokens')).toBeInTheDocument()
    expect(screen.getByText(/Outcome 也只是观测/)).toBeInTheDocument()
  })
})
