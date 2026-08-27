import { useEffect, useMemo, useState } from 'react'
import { CheckCircle2, Clock3, FileCheck2, ShieldAlert } from 'lucide-react'
import { APIClient, HarnessRun } from './api'

type PlanItem = {
  item_id: string; source_kind: string; source_id: string; revision?: number; content_hash: string
  project_id: string; reason_codes: string[]; priority: number; token_estimate?: number
  presentation: string; valid_from?: string; valid_until?: string
}
type ContextPlan = {
  plan_id: string; plan_hash: string; project_id: string; agent_id: string; request_fingerprint: string
  blueprint_id?: string; blueprint_version?: string; blueprint_hash?: string
  budget: { max_tokens?: number; max_chars?: number; max_latency_ms?: number; max_cost_minor?: number }
  items: PlanItem[]; created_at: string; expires_at?: string
}
type ReceiptItem = {
  item_id: string; status: string; revision?: number; content_hash?: string; presentation?: string
  actual_tokens?: number; actual_chars?: number; compaction?: string; detail?: string
}
type ContextReceipt = {
  receipt_id: string; receipt_hash: string; evidence_level: string; completeness: string
  items: ReceiptItem[]; latency_ms?: number; received_at: string
  retention: { mode: string; ttl_seconds?: number; redaction: string }
}
type Exchange = { run: HarnessRun; plan?: ContextPlan; receipt?: ContextReceipt; delivery_status: Record<string,string> }

const statusCopy: Record<string,string> = {
  delivered: '已送达', trimmed: '被裁剪', denied: '被拒绝', failed: '送达失败', delivery_unverified: '送达未验证',
}
function shortHash(value?: string) { return value ? value.slice(0, 12) : '—' }
function time(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

export function ContextExchangeInspector({ api, runID }: { api: APIClient; runID: string }) {
  const [data, setData] = useState<Exchange>()
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    setError('')
    void api.get<Exchange>(`/v1/context/exchanges/${encodeURIComponent(runID)}`)
      .then(value => { if (active) setData(value) })
      .catch(reason => { if (active) setError(reason instanceof Error ? reason.message : String(reason)) })
    return () => { active = false }
  }, [api, runID])
  const receiptByItem = useMemo(() => new Map((data?.receipt?.items || []).map(item => [item.item_id, item])), [data])
  const counts = useMemo(() => {
    const values = Object.values(data?.delivery_status || {})
    return {
      delivered: values.filter(value => value === 'delivered').length,
      trimmed: values.filter(value => value === 'trimmed').length,
      denied: values.filter(value => value === 'denied').length,
      failed: values.filter(value => value === 'failed').length,
      unverified: values.filter(value => value === 'delivery_unverified').length,
    }
  }, [data])
  if (error) return <div className="context-exchange-error"><ShieldAlert size={18}/><div><strong>上下文交换记录读取失败</strong><p>{error}</p></div></div>
  if (!data?.plan) return <div className="context-exchange-loading"><Clock3 size={17}/>正在读取 Context Plan…</div>
  const plan = data.plan
  const receipt = data.receipt
  return <section className="context-exchange-inspector">
    <header><div><p className="micro">CONTEXT PLAN → RECEIPT</p><h3>计划给了什么，外部 Harness 实际收到什么</h3></div><span className={receipt ? 'verified' : 'unverified'}>{receipt ? `${receipt.evidence_level} · ${receipt.completeness}` : 'delivery_unverified'}</span></header>
    <div className="context-exchange-metrics"><article><strong>{plan.items.length}</strong><span>计划项</span></article><article><strong>{counts.delivered}</strong><span>已送达</span></article><article><strong>{counts.trimmed}</strong><span>被裁剪</span></article><article><strong>{counts.denied + counts.failed}</strong><span>拒绝 / 失败</span></article><article><strong>{counts.unverified}</strong><span>未验证</span></article></div>
    <div className="context-plan-meta"><div><span>Plan</span><code>{plan.plan_id}</code><small>{shortHash(plan.plan_hash)}</small></div><div><span>Blueprint</span><strong>{plan.blueprint_id || 'default'} · {plan.blueprint_version || '—'}</strong><small>{shortHash(plan.blueprint_hash)}</small></div><div><span>预算</span><strong>{plan.budget.max_tokens || 0} tokens · {plan.budget.max_chars || 0} chars</strong><small>expires {time(plan.expires_at)}</small></div></div>
    <div className="context-item-list">{plan.items.map(item => {
      const delivered = receiptByItem.get(item.item_id)
      const status = data.delivery_status[item.item_id] || 'delivery_unverified'
      return <article key={item.item_id} className={`context-item status-${status}`}>
        <div className="context-item-state">{status === 'delivered' ? <CheckCircle2 size={17}/> : <ShieldAlert size={17}/>}<strong>{statusCopy[status] || status}</strong><span>{item.priority}/100</span></div>
        <div className="context-item-main"><h4>{item.source_kind} · {item.source_id}</h4><p>{item.reason_codes.join(' · ')}</p><div><code>R{item.revision || '—'} · {shortHash(item.content_hash)}</code><span>{item.presentation}</span><span>≈ {item.token_estimate || 0} tokens</span></div></div>
        <aside><small>Receipt</small><strong>{delivered ? `${delivered.actual_tokens || 0} tokens` : '—'}</strong><span>{delivered?.compaction || 'no receipt'}</span></aside>
      </article>
    })}</div>
    <footer className="context-exchange-proof"><FileCheck2 size={16}/><p><strong>证据边界：</strong>Recall 命中 ≠ Plan；Plan ≠ 已送达；已送达 ≠ 模型实际使用；Outcome 也只是观测，不会自动修改长期记忆或 Blueprint。</p></footer>
    {receipt && <div className="context-receipt-meta"><span>Receipt <code>{receipt.receipt_id}</code> · {shortHash(receipt.receipt_hash)}</span><span>Latency {receipt.latency_ms || 0} ms</span><span>Retention {receipt.retention.mode} · Redaction {receipt.retention.redaction}</span><time>{time(receipt.received_at)}</time></div>}
  </section>
}
