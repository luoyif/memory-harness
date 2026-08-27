import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { CalendarClock, CheckCircle2, ChevronRight, Clock3, RefreshCw, TimerReset } from 'lucide-react'
import { APIClient } from './api'
import type { KnowledgeUnit } from './MemorySemantics'

type TemporalEvent = {
  event_id: string; project_id: string; kind: string; title: string; detail?: string; status: string
  anchor_at: string; start_at?: string; end_at?: string; observed_at?: string; recorded_at?: string
  temporal_relation: string; temporal_relevance: number; source_id: string; source_evidence_ids: string[]
}
type TemporalCorrelation = { left_id:string; right_id:string; kind:string; strength:number; delta_seconds?:number; reasons:string[] }
type Timeline = { project_id: string; anchor_at: string; from?: string; until?: string; events: TemporalEvent[]; relations: Array<{ from_id: string; to_id: string; kind: string; delta_seconds?: number }>; correlations: TemporalCorrelation[]; counts: Record<string, number> }
type TemporalCorrection = { unit: KnowledgeUnit; expectedRevision: number }

const kinds = [
  ['fact', '事实'], ['goal', '目标'], ['milestone', '里程碑'], ['decision', '决策'], ['episode', '会话'],
  ['memory', '记忆'], ['run', '运行'], ['evidence', '证据'], ['finance', '财务'],
] as const
const relationCopy: Record<string, string> = { active: '当前有效', upcoming: '即将发生', overdue: '已经逾期', completed: '已经完成', historical: '历史区间', past: '过去记录', future: '未来记录', unknown: '时间待确认' }
const kindCopy = Object.fromEntries(kinds) as Record<string, string>
const correlationCopy:Record<string,string>={overlaps:'时间区间重叠',contains:'左侧时间包含右侧',during:'左侧处于右侧期间',near_in_time:'时间临近'}

function localInput(value: Date) {
  const shifted = new Date(value.getTime() - value.getTimezoneOffset() * 60000)
  return shifted.toISOString().slice(0, 16)
}
function displayTime(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}
function dayKey(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value.slice(0, 10) : new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', weekday: 'short' }).format(date)
}
function unresolvedTemporal(unit: KnowledgeUnit) {
  const temporal = unit.structure?.temporal
  return Boolean(temporal?.event_time_text?.trim()) && !['resolved', 'not_applicable'].includes(temporal?.resolution || '')
}

export function TemporalTimeline({ api, projectID }: { api: APIClient; projectID: string }) {
  const [anchorInput, setAnchorInput] = useState(() => localInput(new Date()))
  const [selectedKinds, setSelectedKinds] = useState<string[]>([])
  const [mode, setMode] = useState<'relevance' | 'chronology'>('relevance')
  const [data, setData] = useState<Timeline>()
  const [pendingUnits, setPendingUnits] = useState<KnowledgeUnit[]>([])
  const [correction, setCorrection] = useState<TemporalCorrection>()
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const anchorISO = useMemo(() => {
    const date = new Date(anchorInput)
    return Number.isNaN(date.getTime()) ? '' : date.toISOString()
  }, [anchorInput])
  const kindKey = selectedKinds.join(',')
  const load = useCallback(async () => {
    if (!projectID) return
    setLoading(true); setError('')
    const params = new URLSearchParams({ project_id: projectID, limit: '300' })
    if (anchorISO) params.set('anchor_at', anchorISO)
    if (kindKey) params.set('kinds', kindKey)
    try { setData(await api.get<Timeline>(`/v1/timeline?${params.toString()}`)) }
    catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)) }
    finally { setLoading(false) }
  }, [api, projectID, anchorISO, kindKey])
  const loadPending = useCallback(async () => {
    if (!projectID) return
    try {
      const result = await api.get<{ knowledge_units: KnowledgeUnit[] }>(`/v1/knowledge-units?project_id=${encodeURIComponent(projectID)}&limit=300`)
      setPendingUnits((result.knowledge_units || []).filter(unresolvedTemporal))
    } catch { setPendingUnits([]) }
  }, [api, projectID])
  useEffect(() => { void load() }, [load])
  useEffect(() => { void loadPending() }, [loadPending])

  const events = useMemo(() => {
    const list = [...(data?.events || [])]
    if (mode === 'chronology') list.sort((a, b) => b.anchor_at.localeCompare(a.anchor_at))
    return list
  }, [data, mode])
  const groups = useMemo(() => {
    const grouped = new Map<string, TemporalEvent[]>()
    for (const event of events) { const key = dayKey(event.anchor_at); grouped.set(key, [...(grouped.get(key) || []), event]) }
    return [...grouped.entries()]
  }, [events])
  const eventByID = useMemo(() => new Map(events.map(item => [item.event_id, item])), [events])
  const topCorrelations = useMemo(() => [...(data?.correlations || [])].sort((a,b) => b.strength-a.strength).slice(0, 10), [data])
  const metrics = useMemo(() => ({
    active: events.filter(item => item.temporal_relation === 'active').length,
    upcoming: events.filter(item => item.temporal_relation === 'upcoming').length,
    overdue: events.filter(item => item.temporal_relation === 'overdue').length,
  }), [events])
  function toggleKind(kind: string) { setSelectedKinds(current => current.includes(kind) ? current.filter(item => item !== kind) : [...current, kind]) }
  async function openCorrection(unit: KnowledgeUnit) {
    setError('')
    try {
      const detail = await api.get<{ knowledge_unit: KnowledgeUnit; governance?: { current_revision?: number } }>(`/v1/knowledge-units/${encodeURIComponent(unit.unit_id)}?project_id=${encodeURIComponent(projectID)}`)
      setCorrection({ unit: detail.knowledge_unit, expectedRevision: Number(detail.governance?.current_revision || 0) })
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)) }
  }
  async function confirmTime(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!correction) return
    const form = new FormData(event.currentTarget)
    const start = new Date(String(form.get('start_at') || ''))
    const endRaw = String(form.get('end_at') || '')
    const end = endRaw ? new Date(endRaw) : undefined
    if (Number.isNaN(start.getTime()) || (end && Number.isNaN(end.getTime()))) { setError('请选择有效的确认时间。'); return }
    if (end && end <= start) { setError('结束时间必须晚于开始时间。'); return }
    const draft = JSON.parse(JSON.stringify(correction.unit)) as KnowledgeUnit
    draft.structure ||= {}; draft.structure.temporal ||= {}
    const startISO = start.toISOString(); const endISO = end?.toISOString() || ''
    draft.structure.temporal.valid_from = startISO
    draft.structure.temporal.valid_until = endISO
    draft.structure.temporal.occurred_from = startISO
    draft.structure.temporal.occurred_until = endISO
    draft.structure.temporal.precision = String(form.get('precision') || 'exact')
    draft.structure.temporal.resolution = 'resolved'
    try {
      await api.post(`/v1/knowledge-units/${encodeURIComponent(draft.unit_id)}/revision-proposals`, {
        project_id: projectID, expected_revision: correction.expectedRevision,
        edit_reason: String(form.get('edit_reason') || '').trim(),
        idempotency_key: `temporal-confirm-${draft.unit_id}-${Date.now()}`, knowledge_unit: draft,
      })
      setNotice(`“${draft.structure.temporal.event_time_text || draft.statement}”的时间确认已进入待审核 Revision；批准前当前时间事实不会改变。`)
      setCorrection(undefined)
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)) }
  }

  return <div className="temporal-page">
    <section className="temporal-hero"><div><p>PROJECT TEMPORAL CONTEXT</p><h2>时间不是排序字段，而是记忆的第四条坐标轴。</h2><span>同一条事实在不同时间可能同时“正确”；任务、里程碑和运行记录也要相对于一个时间锚点解释。</span></div><label><Clock3 size={16} /><span>时间锚点</span><input type="datetime-local" value={anchorInput} onChange={event => setAnchorInput(event.target.value)} /></label></section>
    <div className="temporal-metrics"><article><strong>{events.length}</strong><span>时间事件</span></article><article><strong>{metrics.active}</strong><span>当前有效</span></article><article className={metrics.upcoming ? 'attention' : ''}><strong>{metrics.upcoming}</strong><span>即将发生</span></article><article className={metrics.overdue ? 'danger' : ''}><strong>{metrics.overdue}</strong><span>已经逾期</span></article><article><strong>{data?.correlations?.length || 0}</strong><span>时间关联</span></article></div>
    {notice && <p className="surface-notice"><CheckCircle2 size={14}/>{notice}</p>}
    {pendingUnits.length > 0 && <section className="temporal-pending"><header><div><Clock3 size={16}/><strong>待确认的相对时间</strong></div><span>{pendingUnits.length} 条知识包含“下周 / 明年 / 之后”等时间表达。系统不会猜日期；确认后再进入 Temporal Fact 与项目时间投影。</span></header><div>{pendingUnits.slice(0,12).map(unit => <article key={unit.unit_id}><div><b>{unit.structure?.temporal?.event_time_text}</b><span>{unit.structure?.temporal?.resolution || 'pending'}</span></div><p>{unit.statement}</p><footer><small>Evidence · {unit.evidence_id}</small><button className="button" onClick={() => void openCorrection(unit)}>确认时间</button></footer></article>)}</div></section>}
    <section className="temporal-controls"><div className="temporal-kind-filter">{kinds.map(([kind, label]) => <button key={kind} className={!selectedKinds.length || selectedKinds.includes(kind) ? 'active' : ''} onClick={() => toggleKind(kind)}>{label}<em>{data?.counts?.[kind] || 0}</em></button>)}</div><div className="temporal-mode"><button className={mode === 'relevance' ? 'active' : ''} onClick={() => setMode('relevance')}>按时间相关性</button><button className={mode === 'chronology' ? 'active' : ''} onClick={() => setMode('chronology')}>按时间顺序</button><button title="刷新" onClick={load}><RefreshCw size={14} /></button></div></section>
    {topCorrelations.length > 0 && <section className="temporal-correlations"><header><div><TimerReset size={16}/><strong>时间相关性</strong></div><span>不是语义因果；仅表示时间区间、时间距离、同类任务或共同 Evidence 的可解释关联。</span></header><div>{topCorrelations.map((correlation,index)=>{const left=eventByID.get(correlation.left_id);const right=eventByID.get(correlation.right_id);return <article key={`${correlation.left_id}:${correlation.right_id}:${index}`}><div><b>{Math.round(correlation.strength*100)}%</b><span>{correlationCopy[correlation.kind]||correlation.kind}</span></div><p><strong>{left?.title||correlation.left_id}</strong><ChevronRight size={13}/><strong>{right?.title||correlation.right_id}</strong></p><small>{correlation.reasons.join(' · ')}{correlation.delta_seconds!==undefined?` · 间隔 ${Math.round(correlation.delta_seconds/3600)}h`:''}</small></article>})}</div></section>}
    {error && <p className="temporal-error">{error}</p>}
    {loading && !data ? <div className="temporal-loading"><RefreshCw size={18} />正在重建当前项目的时间视图…</div> : events.length ? <div className="temporal-layout"><main className="temporal-stream">{groups.map(([day, items]) => <section key={day}><header><CalendarClock size={15} /><strong>{day}</strong><span>{items.length} 项</span></header>{items.map(event => <article key={event.event_id} className={`temporal-event relation-${event.temporal_relation}`}><i /><div className="temporal-event-main"><div><span className="temporal-kind">{kindCopy[event.kind] || event.kind}</span><span className="temporal-relation">{relationCopy[event.temporal_relation] || event.temporal_relation}</span><time>{displayTime(event.anchor_at)}</time></div><h3>{event.title}</h3>{event.detail && <p>{event.detail}</p>}<footer><span>{event.status}</span><span>{event.source_evidence_ids?.length || 0} 条 Evidence</span><code>{event.source_id}</code></footer></div><aside><strong>{Math.round(event.temporal_relevance * 100)}</strong><small>时间相关</small><span><b style={{ width: `${Math.max(2, event.temporal_relevance * 100)}%` }} /></span><ChevronRight size={15} /></aside></article>)}</section>)}</main><aside className="temporal-explain"><TimerReset size={20} /><h3>这条时间轴怎样判断</h3><p><b>事件时间</b>回答事情什么时候发生。</p><p><b>有效时间</b>回答一条事实从什么时候开始/停止成立。</p><p><b>记录时间</b>回答系统什么时候知道这件事。</p><p><b>相关性</b>分两层：单条记录相对时间锚点的相关度，以及两条记录之间的重叠/包含/临近关系；共同 Evidence 会提高关联置信度。</p><small>因此“现在最相关”和“2025 年 12 月当时最相关”会得到不同结果。</small></aside></div> : <div className="temporal-empty"><CalendarClock size={32} /><h3>当前筛选范围还没有时间事件</h3><p>有时间戳的记忆、事实、目标、里程碑、Run 和 Evidence 会自动进入这里。</p></div>}
    {correction && <div className="modal-backdrop" onMouseDown={() => setCorrection(undefined)}><form className="modal" onMouseDown={event => event.stopPropagation()} onSubmit={confirmTime}><p className="micro">TEMPORAL CONFIRMATION · R{correction.expectedRevision || 'legacy'}</p><h2>确认“{correction.unit.structure?.temporal?.event_time_text || '相对时间'}”对应的真实时间</h2><p className="drawer-lead">Evidence 原文不会改变。提交后只创建待审核 KU Revision；Owner 批准后才刷新 Temporal Fact、Goal/Decision 时间与检索。</p><label>确认开始时间<input name="start_at" type="datetime-local" required /></label><label>确认结束时间（可选）<input name="end_at" type="datetime-local" /></label><label>精度<select name="precision" defaultValue="exact"><option value="exact">精确时间</option><option value="day">天</option><option value="week">周</option><option value="month">月</option><option value="year">年</option></select></label><label>确认理由<textarea name="edit_reason" rows={2} required minLength={4} placeholder="例如：根据会话发生时间，“下周”对应 2026-08-29。" /></label><div className="modal-actions"><button type="button" className="button" onClick={() => setCorrection(undefined)}>取消</button><button className="button primary">提交 Revision 等待审核</button></div></form></div>}
  </div>
}
