import { useEffect, useMemo, useRef, useState } from 'react'
import type { Core, ElementDefinition, StylesheetJson } from 'cytoscape'
import {
  ArrowLeftRight, CalendarClock, Eye, EyeOff, Fingerprint, Focus,
  Link2Off, MapPin, Maximize2, RotateCcw, Search, ShieldAlert, Users, ZoomIn, ZoomOut,
} from 'lucide-react'

export type KnowledgeEntity = {
  entity_id?: string
  entity_type?: string
  surface?: string
  canonical_name?: string
  resolution?: string
}

export type KnowledgeUnit = {
  unit_id: string
  episode_id: string
  evidence_id: string
  unit_type: string
  tier_hint: string
  statement: string
  normalized_key?: string
  confidence: number
  risk_tier: string
  status: string
  scopes?: string[]
  observed_at: string
  created_at?: string
  processed_at?: string
  schema_version?: string
  structure?: {
    attribution?: {
      source_speaker_ref?: string
      asserted_by_ref?: string
      subject_ref?: string
      subject_surface?: string
      resolution?: string
      candidate_refs?: string[]
      reason_codes?: string[]
      owner_mapping?: string
    }
    frame?: {
      subject?: KnowledgeEntity
      predicate?: string
      inverse_label?: string
      object?: { kind?: string; entity?: KnowledgeEntity; value?: string }
      action?: string
      participants?: Array<{ role: string; entity: KnowledgeEntity }>
      locations?: Array<{ role: string; entity: KnowledgeEntity }>
      context?: string
    }
    temporal?: {
      observed_at?: string
      recorded_at?: string
      event_time_text?: string
      valid_from?: string
      valid_until?: string
      occurred_from?: string
      occurred_until?: string
      precision?: string
      resolution?: string
      anchor_evidence_time?: string
    }
    epistemic?: {
      polarity?: string
      modality?: string
      confidence?: number
      importance?: number
      novelty?: number
      quality_flags?: string[]
      review_reasons?: string[]
    }
    provenance?: {
      evidence_id?: string
      episode_id?: string
      run_id?: string
      span_id?: string
      extractor_plugin?: string
      extractor_version?: string
      model_profile?: string
      prompt_hash?: string
      evidence_span?: { start?: number; end?: number; quote?: string; quote_hash?: string }
    }
  }
}

export type CorrectionImpact = {
  unit_ids: string[]
  assertion_ids: string[]
  temporal_fact_ids: string[]
  evidence_ids: string[]
  authority_object_ids: string[]
  current_revisions: Record<string, number>
  project_projection_ids: string[]
}

export type SemanticGraphData = {
  nodes: Array<{ id: string; layer: string; label: string; status: string; entity_type?: string; confidence?: number }>
  edges: Array<{ id?: string; from: string; to: string; kind: string; label?: string; inverse_label?: string; confidence?: number; evidence_id?: string }>
}

const shown = (value: unknown, fallback = '未明确') => String(value || fallback)

function entityName(entity?: KnowledgeEntity, fallback?: string) {
  return entity?.canonical_name || entity?.surface || fallback || '未解析主体'
}

function resolutionLabel(value?: string) {
  return ({ resolved: '已解析', unresolved: '待确认', ambiguous: '有歧义', not_applicable: '不适用' } as Record<string, string>)[value || ''] || shown(value)
}

function timeText(value?: string) {
  if (!value) return '未明确'
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

export function KnowledgeUnitDrawer({ unit, onClose, onCorrect }: { unit: KnowledgeUnit; onClose: () => void; onCorrect?: () => void }) {
  const attribution = unit.structure?.attribution
  const frame = unit.structure?.frame
  const temporal = unit.structure?.temporal
  const epistemic = unit.structure?.epistemic
  const provenance = unit.structure?.provenance
  const object = frame?.object?.entity ? entityName(frame.object.entity) : shown(frame?.object?.value, '未明确对象')
  const reasons = [...(attribution?.reason_codes || []), ...(epistemic?.review_reasons || [])]
  return <div className="drawer-backdrop" onMouseDown={onClose}>
    <aside className="drawer knowledge-drawer" onMouseDown={event => event.stopPropagation()}>
      <button className="drawer-close" onClick={onClose} aria-label="关闭">×</button>
      <p className="micro">STRUCTURED KNOWLEDGE UNIT · V2</p>
      <h2>{unit.statement}</h2>
      <div className="drawer-status"><span>{unit.unit_type}</span><span>{unit.risk_tier} 级风险</span><span>置信度 {Math.round(unit.confidence * 100)}%</span></div>

      <section className="semantic-frame-card">
        <small>语义关系</small>
        <div className="semantic-triple">
          <strong>{entityName(frame?.subject, attribution?.subject_surface)}</strong>
          <span><b>{shown(frame?.predicate, '未识别关系')}</b><em>{frame?.inverse_label ? `反向：${frame.inverse_label}` : '反向关系待定义'}</em></span>
          <strong>{object}</strong>
        </div>
        {frame?.action && <p>动作：{frame.action}</p>}
        {frame?.context && <p>上下文：{frame.context}</p>}
      </section>

      <div className="knowledge-fact-grid">
        <section><Fingerprint size={17} /><div><small>主体归属</small><strong>{resolutionLabel(attribution?.resolution)}</strong><p>来源说话者：{shown(attribution?.source_speaker_ref)}</p><p>陈述者：{shown(attribution?.asserted_by_ref)}</p><p>实际主体：{entityName(frame?.subject, attribution?.subject_surface)}</p><p>Owner 映射：{shown(attribution?.owner_mapping, '从不默认')}</p></div></section>
        <section><CalendarClock size={17} /><div><small>时间语义</small><strong>{shown(temporal?.event_time_text, '未发现事件时间')}</strong><p>Evidence 时间：{timeText(temporal?.observed_at || unit.observed_at)}</p><p>有效期：{shown(temporal?.valid_from)} → {shown(temporal?.valid_until)}</p><p>解析精度：{shown(temporal?.precision)} · {resolutionLabel(temporal?.resolution)}</p></div></section>
        <section><Users size={17} /><div><small>人物与参与者</small><strong>{frame?.participants?.length || 0} 个参与角色</strong>{frame?.participants?.map((item, index) => <p key={`${item.role}:${index}`}>{shown(item.role)}：{entityName(item.entity)}</p>)}{!frame?.participants?.length && <p>当前语料没有可靠识别出额外参与者。</p>}</div></section>
        <section><MapPin size={17} /><div><small>地点</small><strong>{frame?.locations?.length || 0} 个地点角色</strong>{frame?.locations?.map((item, index) => <p key={`${item.role}:${index}`}>{shown(item.role)}：{entityName(item.entity)}</p>)}{!frame?.locations?.length && <p>当前语料没有可靠识别出地点。</p>}</div></section>
      </div>

      <section className="knowledge-evidence-card">
        <small>原始 Evidence 中的直接依据</small>
        <blockquote>{provenance?.evidence_span?.quote || unit.statement}</blockquote>
        <dl><div><dt>Evidence</dt><dd><code>{provenance?.evidence_id || unit.evidence_id}</code></dd></div><div><dt>字符区间</dt><dd>{provenance?.evidence_span?.start ?? '—'} → {provenance?.evidence_span?.end ?? '—'}</dd></div><div><dt>抽取器</dt><dd>{shown(provenance?.extractor_plugin)}@{shown(provenance?.extractor_version)}</dd></div><div><dt>模型配置</dt><dd>{shown(provenance?.model_profile, '本地规则')}</dd></div></dl>
      </section>

      <section className="epistemic-card"><small>事实状态</small><div><span>极性 {shown(epistemic?.polarity)}</span><span>语气 {shown(epistemic?.modality)}</span><span>质量标记 {(epistemic?.quality_flags || []).join(' · ') || '无'}</span></div></section>
      {reasons.length > 0 && <section className="knowledge-review-card"><ShieldAlert size={18} /><div><strong>这条知识仍需确认</strong><p>{Array.from(new Set(reasons)).join(' · ')}</p></div></section>}
      {onCorrect && <div className="modal-actions"><button className="button primary" onClick={onCorrect}>提出人工修正</button></div>}
    </aside>
  </div>
}


export function KnowledgeUnitCorrectionModal({ unit, expectedRevision, impact, onClose, onSubmit }: {
  unit: KnowledgeUnit
  expectedRevision: number
  impact?: CorrectionImpact
  onClose: () => void
  onSubmit: (draft: KnowledgeUnit, editReason: string) => Promise<void>
}) {
  const [statement, setStatement] = useState(unit.statement)
  const [unitType, setUnitType] = useState(unit.unit_type || 'fact')
  const [confidence, setConfidence] = useState(String(Math.round((unit.confidence || 0) * 100)))
  const [subject, setSubject] = useState(unit.structure?.frame?.subject?.canonical_name || unit.structure?.frame?.subject?.surface || unit.structure?.attribution?.subject_surface || '')
  const [subjectType, setSubjectType] = useState(unit.structure?.frame?.subject?.entity_type || 'concept')
  const [resolution, setResolution] = useState(unit.structure?.attribution?.resolution || 'unresolved')
  const [predicate, setPredicate] = useState(unit.structure?.frame?.predicate || '')
  const [objectValue, setObjectValue] = useState(unit.structure?.frame?.object?.value || unit.structure?.frame?.object?.entity?.canonical_name || unit.structure?.frame?.object?.entity?.surface || '')
  const [validFrom, setValidFrom] = useState(unit.structure?.temporal?.valid_from || '')
  const [validUntil, setValidUntil] = useState(unit.structure?.temporal?.valid_until || '')
  const [temporalResolution, setTemporalResolution] = useState(unit.structure?.temporal?.resolution || 'not_applicable')
  const [editReason, setEditReason] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!editReason.trim()) { setError('请说明为什么要修正；理由会进入 Revision 审计。'); return }
    const numericConfidence = Number(confidence) / 100
    if (!Number.isFinite(numericConfidence) || numericConfidence < 0 || numericConfidence > 1) { setError('置信度请输入 0–100。'); return }
    const draft = JSON.parse(JSON.stringify(unit)) as KnowledgeUnit
    draft.statement = statement.trim()
    draft.unit_type = unitType
    draft.confidence = numericConfidence
    draft.structure ||= {}
    draft.structure.attribution ||= {}
    draft.structure.frame ||= {}
    draft.structure.temporal ||= {}
    draft.structure.epistemic ||= {}
    draft.structure.attribution.subject_surface = subject.trim()
    draft.structure.attribution.resolution = resolution
    draft.structure.attribution.owner_mapping = 'not_assumed'
    draft.structure.frame.subject = {
      ...(draft.structure.frame.subject || {}), entity_type: subjectType.trim() || 'concept', surface: subject.trim(), canonical_name: subject.trim(), resolution,
    }
    draft.structure.frame.predicate = predicate.trim()
    draft.structure.frame.object = { kind: 'literal', value: objectValue.trim() }
    draft.structure.temporal.valid_from = validFrom.trim()
    draft.structure.temporal.valid_until = validUntil.trim()
    draft.structure.temporal.resolution = temporalResolution
    draft.structure.epistemic.confidence = numericConfidence
    setSaving(true); setError('')
    try { await onSubmit(draft, editReason.trim()) }
    catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)) }
    finally { setSaving(false) }
  }

  return <div className="modal-backdrop" onMouseDown={() => !saving && onClose()}>
    <form className="modal knowledge-correction-modal" onMouseDown={event => event.stopPropagation()} onSubmit={submit}>
      <p className="micro">OWNER KNOWLEDGE REVISION · v{expectedRevision || 'legacy'}</p>
      <h2>修正这条知识的解释</h2>
      <p className="drawer-lead">原始 Evidence、Unit ID 和 Evidence 时间不会被改写。提交只创建待审核 Revision；批准前当前知识、图谱与项目投影都不改变。</p>
      {impact && <div className="correction-impact-strip"><span><b>{impact.assertion_ids.length}</b> Assertion</span><span><b>{impact.temporal_fact_ids.length}</b> 时间事实</span><span><b>{impact.project_projection_ids.length}</b> 项目投影</span><span><b>{impact.evidence_ids.length}</b> Evidence</span></div>}
      <label>知识表述<textarea rows={3} value={statement} onChange={event => setStatement(event.target.value)} required /></label>
      <div className="form-grid two"><label>类型<select value={unitType} onChange={event => setUnitType(event.target.value)}>{['fact','state','event','decision','goal','risk','outcome','procedure','identity','correction'].map(value => <option key={value} value={value}>{value}</option>)}</select></label><label>置信度（0–100）<input type="number" min="0" max="100" value={confidence} onChange={event => setConfidence(event.target.value)} /></label></div>
      <div className="form-grid two"><label>实际主体<input value={subject} onChange={event => setSubject(event.target.value)} placeholder="例如 MemoryOS / 李明" /></label><label>主体类型<input value={subjectType} onChange={event => setSubjectType(event.target.value)} placeholder="person / system / project" /></label></div>
      <label>主体解析<select value={resolution} onChange={event => setResolution(event.target.value)}><option value="resolved">已确认 resolved</option><option value="unresolved">待确认 unresolved</option><option value="ambiguous">有歧义 ambiguous</option></select></label>
      <div className="form-grid two"><label>关系 / Predicate<input value={predicate} onChange={event => setPredicate(event.target.value)} placeholder="例如 targets_safe_release" /></label><label>对象 / Object<input value={objectValue} onChange={event => setObjectValue(event.target.value)} /></label></div>
      <div className="form-grid two"><label>有效开始（RFC3339，可空）<input value={validFrom} onChange={event => setValidFrom(event.target.value)} placeholder="2026-09-01T00:00:00Z" /></label><label>有效结束（RFC3339，可空）<input value={validUntil} onChange={event => setValidUntil(event.target.value)} placeholder="2026-12-31T23:59:59Z" /></label></div>
      <label>时间解析<select value={temporalResolution} onChange={event => setTemporalResolution(event.target.value)}><option value="resolved">已确认 resolved</option><option value="not_applicable">不适用 not_applicable</option><option value="unresolved">待确认 unresolved</option><option value="ambiguous">有歧义 ambiguous</option></select></label>
      <label>修正理由<textarea rows={2} value={editReason} onChange={event => setEditReason(event.target.value)} placeholder="例如：原抽取把风险误判为目标；已对照原文确认主体和时间。" required /></label>
      <small className="token-warning">Evidence 来源不可修改。若原文有误，请追加新的更正 Evidence，而不是重写旧 Evidence。</small>
      {error && <p className="form-error">{error}</p>}
      <div className="modal-actions"><button type="button" className="button" disabled={saving} onClick={onClose}>取消</button><button type="submit" className="button primary" disabled={saving}>{saving ? '正在提交…' : '提交 Revision 等待审核'}</button></div>
    </form>
  </div>
}

const semanticColors: Record<string, string> = {
  person: '#52715f', organization: '#607f8a', project: '#b95e46', system: '#17201c',
  concept: '#9b6e2d', procedure: '#6f665d', asset: '#99594f', market: '#7c6a45',
  industry: '#6b7b68', event: '#826d8a', risk: '#a34435', decision: '#406b78', literal: '#c6bfb3',
}

const humanRelation = (value?: string) => shown(value, 'related_to').replaceAll('_', ' ')
const compactLabel = (value: string, limit = 26) => value.length > limit ? `${value.slice(0, limit - 1)}…` : value

function componentCount(nodeIDs: string[], edges: SemanticGraphData['edges']) {
  const neighbors = new Map(nodeIDs.map(id => [id, new Set<string>()]))
  edges.forEach(edge => {
    if (!neighbors.has(edge.from) || !neighbors.has(edge.to)) return
    neighbors.get(edge.from)?.add(edge.to)
    neighbors.get(edge.to)?.add(edge.from)
  })
  const visited = new Set<string>()
  let count = 0
  nodeIDs.forEach(id => {
    if (visited.has(id)) return
    count += 1
    const queue = [id]
    visited.add(id)
    while (queue.length) {
      neighbors.get(queue.shift()!)?.forEach(next => {
        if (!visited.has(next)) { visited.add(next); queue.push(next) }
      })
    }
  })
  return count
}

const semanticStyles: StylesheetJson = [
  { selector: 'node', style: {
    'background-color': 'data(color)', 'border-color': '#f5f1e8', 'border-width': 5,
    width: 'data(size)', height: 'data(size)', label: 'data(displayLabel)', color: '#17201c',
    'font-family': 'Avenir Next, PingFang SC, sans-serif', 'font-size': 11, 'font-weight': 700,
    'text-valign': 'bottom', 'text-halign': 'center', 'text-margin-y': 9,
    'text-wrap': 'ellipsis', 'text-max-width': '150px', 'min-zoomed-font-size': 8,
    'overlay-opacity': 0,
  } },
  { selector: 'node.property', style: {
    shape: 'round-rectangle', width: 24, height: 14, 'border-width': 2,
    'background-color': '#d8d1c4', 'font-size': 8, 'font-weight': 500,
  } },
  { selector: 'node.secondary-label', style: { label: '' } },
  { selector: 'node.secondary-label.context, node.secondary-label.selected, node.secondary-label.peek', style: { label: 'data(displayLabel)' } },
  { selector: 'edge', style: {
    width: 'data(width)', 'line-color': '#91a398', 'target-arrow-color': '#6e8776',
    'target-arrow-shape': 'triangle', 'curve-style': 'bezier', opacity: .56,
    label: '', 'font-family': 'SFMono-Regular, monospace', 'font-size': 8,
    color: '#43554b', 'text-background-color': '#fbf8f1', 'text-background-opacity': .95,
    'text-background-padding': '4px', 'text-border-color': '#d8d1c4', 'text-border-width': 1,
    'text-border-opacity': .9, 'text-rotation': 'autorotate', 'text-margin-y': -8,
  } },
  { selector: 'edge.property', style: { 'line-style': 'dashed', opacity: .34, 'target-arrow-shape': 'none' } },
  { selector: 'edge.context', style: { label: 'data(displayLabel)', width: 2.4, opacity: .95, 'z-index': 8 } },
  { selector: 'node.selected', style: { 'border-color': '#b95e46', 'border-width': 7, 'z-index': 10 } },
  { selector: '.dim', style: { opacity: .1, label: '' } },
]

export function SemanticGraph({ graph, onCorrectEntity, onCorrectAssertion }: { graph: SemanticGraphData; onCorrectEntity?: (entity: { id: string; label: string; entityType: string }) => void; onCorrectAssertion?: (assertion: { id: string; label: string }) => void }) {
  const canvasRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<Core>()
  const [query, setQuery] = useState('')
  const [entityType, setEntityType] = useState('all')
  const [showProperties, setShowProperties] = useState(false)
  const [showIsolates, setShowIsolates] = useState(false)
  const [selectedID, setSelectedID] = useState('')

  const nodeByID = useMemo(() => new Map(graph.nodes.map(node => [node.id, node])), [graph.nodes])
  const entityNodes = useMemo(() => graph.nodes.filter(node => node.layer !== 'literal'), [graph.nodes])
  const relationEdges = useMemo(() => graph.edges.filter(edge => nodeByID.get(edge.from)?.layer !== 'literal' && nodeByID.get(edge.to)?.layer !== 'literal'), [graph.edges, nodeByID])
  const attributeEdges = useMemo(() => graph.edges.filter(edge => nodeByID.get(edge.to)?.layer === 'literal'), [graph.edges, nodeByID])
  const relatedEntityIDs = useMemo(() => new Set(relationEdges.flatMap(edge => [edge.from, edge.to])), [relationEdges])
  const types = useMemo(() => Array.from(new Set(entityNodes.map(node => node.entity_type || 'other'))).sort(), [entityNodes])
  const degrees = useMemo(() => {
    const value = new Map<string, number>()
    graph.edges.forEach(edge => { value.set(edge.from, (value.get(edge.from) || 0) + 1); value.set(edge.to, (value.get(edge.to) || 0) + 1) })
    return value
  }, [graph.edges])
  const labelledEntityIDs = useMemo(() => {
    const labelled = new Set(entityNodes.filter(node => !relatedEntityIDs.has(node.id) || node.entity_type !== 'concept').map(node => node.id))
    const neighbors = new Map(Array.from(relatedEntityIDs).map(id => [id, new Set<string>()]))
    relationEdges.forEach(edge => { neighbors.get(edge.from)?.add(edge.to); neighbors.get(edge.to)?.add(edge.from) })
    const visited = new Set<string>()
    relatedEntityIDs.forEach(id => {
      if (visited.has(id)) return
      const component: string[] = []
      const queue = [id]
      visited.add(id)
      while (queue.length) {
        const current = queue.shift()!
        component.push(current)
        neighbors.get(current)?.forEach(next => { if (!visited.has(next)) { visited.add(next); queue.push(next) } })
      }
      if (!component.some(member => labelled.has(member))) {
        component.sort((left, right) => (degrees.get(right) || 0) - (degrees.get(left) || 0) || (nodeByID.get(left)?.label.length || 0) - (nodeByID.get(right)?.label.length || 0))
        if (component[0]) labelled.add(component[0])
      }
    })
    return labelled
  }, [degrees, entityNodes, nodeByID, relatedEntityIDs, relationEdges])
  const visibleEntityIDs = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase()
    const matches = new Set(entityNodes.filter(node =>
      (entityType === 'all' || node.entity_type === entityType)
      && (!normalized || node.label.toLocaleLowerCase().includes(normalized))
      && (normalized || showIsolates || relatedEntityIDs.size === 0 || relatedEntityIDs.has(node.id)),
    ).map(node => node.id))
    if (!normalized) return matches
    relationEdges.forEach(edge => {
      if (matches.has(edge.from)) matches.add(edge.to)
      if (matches.has(edge.to)) matches.add(edge.from)
    })
    return matches
  }, [entityNodes, entityType, query, relatedEntityIDs, relationEdges, showIsolates])
  const visibleNodes = useMemo(() => graph.nodes.filter(node => visibleEntityIDs.has(node.id) || (showProperties && node.layer === 'literal' && graph.edges.some(edge => edge.to === node.id && visibleEntityIDs.has(edge.from)))), [graph.edges, graph.nodes, showProperties, visibleEntityIDs])
  const visibleIDs = useMemo(() => new Set(visibleNodes.map(node => node.id)), [visibleNodes])
  const visibleEdges = useMemo(() => graph.edges.filter(edge => visibleIDs.has(edge.from) && visibleIDs.has(edge.to)), [graph.edges, visibleIDs])
  const selectedNode = nodeByID.get(selectedID)
  const selectedRelations = useMemo(() => selectedID ? graph.edges.filter(edge => edge.from === selectedID || edge.to === selectedID) : [], [graph.edges, selectedID])
  const isolates = entityNodes.length - relatedEntityIDs.size
  const themes = componentCount(Array.from(relatedEntityIDs), relationEdges)

  const elements = useMemo<ElementDefinition[]>(() => [
    ...visibleNodes.map(node => ({
      group: 'nodes' as const,
      data: {
        id: node.id, label: node.label, displayLabel: compactLabel(node.label),
        color: semanticColors[node.entity_type || node.layer] || '#7d857f',
        size: node.layer === 'literal' ? 18 : 34 + Math.min(30, Math.sqrt(degrees.get(node.id) || 1) * 7),
        kind: node.layer === 'literal' ? 'property' : 'entity',
      },
      classes: node.layer === 'literal' ? 'property secondary-label' : (labelledEntityIDs.has(node.id) ? '' : 'secondary-label'),
    })),
    ...visibleEdges.map((edge, index) => ({
      group: 'edges' as const,
      data: {
        id: edge.id || `semantic-edge-${index}`, source: edge.from, target: edge.to,
        displayLabel: nodeByID.get(edge.to)?.layer === 'literal'
          ? humanRelation(edge.label)
          : `${humanRelation(edge.label)} ↔ ${humanRelation(edge.inverse_label)}`,
        width: 1 + Math.max(0, Number(edge.confidence || 0)) * 1.2,
      },
      classes: nodeByID.get(edge.to)?.layer === 'literal' ? 'property' : '',
    })),
  ], [degrees, labelledEntityIDs, nodeByID, visibleEdges, visibleNodes])

  useEffect(() => {
    if (!canvasRef.current || typeof ResizeObserver === 'undefined') return
    let disposed = false
    let cy: Core | undefined
    let observer: ResizeObserver | undefined
    const container = canvasRef.current
    void import('cytoscape').then(({ default: createCytoscape }) => {
      if (disposed || !container) return
      cy = createCytoscape({
        container, elements, style: semanticStyles,
        minZoom: .28, maxZoom: 2.4,
        layout: { name: 'cose', animate: false, randomize: true, nodeRepulsion: () => 9500, idealEdgeLength: () => showProperties ? 105 : 145, edgeElasticity: () => .12, gravity: .32, numIter: 700, fit: true, padding: 48 },
      })
      cy.on('tap', 'node', event => setSelectedID(event.target.id()))
      cy.on('tap', event => { if (event.target === cy) setSelectedID('') })
      cy.on('mouseover', 'node', event => event.target.addClass('peek'))
      cy.on('mouseout', 'node', event => event.target.removeClass('peek'))
      cyRef.current = cy
      observer = new ResizeObserver(() => { cy?.resize(); cy?.fit(undefined, 48) })
      observer.observe(container)
    })
    return () => { disposed = true; observer?.disconnect(); cy?.destroy(); cyRef.current = undefined }
  }, [elements, showProperties])

  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    cy.elements().removeClass('dim context selected')
    const selected = cy.getElementById(selectedID)
    if (!selected.length) return
    const neighborhood = selected.closedNeighborhood()
    cy.elements().not(neighborhood).addClass('dim')
    neighborhood.nodes().addClass('context')
    selected.addClass('selected')
    selected.connectedEdges().addClass('context')
  }, [selectedID, elements])

  const zoom = (factor: number) => {
    const cy = cyRef.current
    if (!cy) return
    cy.animate({ zoom: Math.min(cy.maxZoom(), Math.max(cy.minZoom(), cy.zoom() * factor)), center: { eles: cy.$(':selected').length ? cy.$(':selected') : cy.nodes() }, duration: 180 })
  }
  const fit = () => cyRef.current?.animate({ fit: { eles: cyRef.current.elements(), padding: 48 }, duration: 220 })
  const focus = () => {
    const cy = cyRef.current
    if (!cy || !selectedID) return
    const node = cy.getElementById(selectedID)
    cy.animate({ fit: { eles: node.closedNeighborhood(), padding: 90 }, duration: 240 })
  }

  return <div className="semantic-graph-shell graph-workbench">
    <header className="graph-workbench-head">
      <div><ArrowLeftRight size={18} /><div><strong>双向语义关系</strong><span>{entityNodes.length} 个实体 · {relationEdges.length} 条实体关系 · {attributeEdges.length} 条属性断言</span></div></div>
      <div className="graph-insight-strip"><span>{themes} 个连通主题</span><span>{isolates} 个待补关系实体</span></div>
    </header>
    <div className="graph-toolbar">
      <label><Search size={15} /><span className="sr-only">搜索图谱</span><input value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索人物、项目、概念…" /></label>
      <label><span>类型</span><select value={entityType} onChange={event => setEntityType(event.target.value)}><option value="all">全部实体</option>{types.map(type => <option key={type} value={type}>{type}</option>)}</select></label>
      <button className={showIsolates ? 'active' : ''} onClick={() => setShowIsolates(value => !value)} title={showIsolates ? '隐藏尚无关系的实体' : '显示尚无关系的实体'}><Link2Off size={15} /> 未连接</button>
      <button className={showProperties ? 'active' : ''} onClick={() => setShowProperties(value => !value)} title={showProperties ? '隐藏属性节点' : '显示属性节点'}>{showProperties ? <EyeOff size={15} /> : <Eye size={15} />} 属性</button>
      <span className="graph-toolbar-spacer" />
      <button onClick={() => zoom(1.25)} aria-label="放大图谱"><ZoomIn size={16} /></button>
      <button onClick={() => zoom(.8)} aria-label="缩小图谱"><ZoomOut size={16} /></button>
      <button onClick={fit} aria-label="适应画布"><Maximize2 size={16} /></button>
      <button onClick={() => { setQuery(''); setEntityType('all'); setShowIsolates(false); setShowProperties(false); setSelectedID(''); fit() }} aria-label="重置图谱"><RotateCcw size={16} /></button>
    </div>
    <div className="graph-workbench-body">
      <div ref={canvasRef} className="semantic-graph-canvas cytoscape-canvas" role="img" aria-label={`语义关系图，${visibleNodes.length} 个可见节点，${visibleEdges.length} 条可见关系`} />
      <aside className="graph-inspector">
        {selectedNode ? <>
          <p className="micro">SELECTED KNOWLEDGE ENTITY</p>
          <h3>{selectedNode.label}</h3>
          <div className="graph-node-meta"><span>{selectedNode.entity_type || selectedNode.layer}</span><span>置信度 {Math.round(Number(selectedNode.confidence || 0) * 100)}%</span></div>
          <button className="graph-focus-button" onClick={focus}><Focus size={15} />只看直接关系</button>
          {onCorrectEntity && selectedNode.layer !== 'literal' && <button className="graph-focus-button entity-correct-button" onClick={() => onCorrectEntity({ id: selectedNode.id, label: selectedNode.label, entityType: selectedNode.entity_type || 'concept' })}><ShieldAlert size={15} />纠正 / 合并实体</button>}
          <h4>可追溯关系</h4>
          <div className="graph-relation-list">{selectedRelations.length ? selectedRelations.map((edge, index) => {
            const outgoing = edge.from === selectedID
            const other = nodeByID.get(outgoing ? edge.to : edge.from)
            const relationLabel = outgoing ? humanRelation(edge.label) : humanRelation(edge.inverse_label)
            return <article key={edge.id || index} className="graph-relation-item">
              <button className="graph-relation-navigate" onClick={() => other?.layer !== 'literal' && setSelectedID(other?.id || '')} disabled={other?.layer === 'literal'}>
                <span>{relationLabel}</span><strong>{other?.label || '未命名对象'}</strong><small>Evidence {edge.evidence_id || '未记录'}</small>
              </button>
              {onCorrectAssertion && edge.id && <button className="graph-relation-correct" onClick={() => onCorrectAssertion({ id: edge.id!, label: `${selectedNode.label} · ${relationLabel} · ${other?.label || '未命名对象'}` })}><ShieldAlert size={12}/>纠正关系</button>}
            </article>
          }) : <p>当前实体还没有可靠关系。</p>}</div>
        </> : <>
          <p className="micro">GRAPH READING GUIDE</p>
          <h3>先看主题，再检查来源</h3>
          <p>节点大小代表可追溯断言数量。默认只画已有可靠关系的实体；点击节点会淡化无关内容，只显示它的一跳邻域与关系标签。</p>
          <dl className="graph-type-legend">{types.map(type => <div key={type}><dt><i style={{ background: semanticColors[type] || '#7d857f' }} />{type}</dt><dd>{entityNodes.filter(node => (node.entity_type || 'other') === type).length}</dd></div>)}</dl>
          <p className="graph-structure-note">{isolates} 个待补关系实体与属性文本默认收起；需要核查时可点击上方“未连接”或“属性”。</p>
        </>}
      </aside>
    </div>
    <details className="graph-adjacency"><summary>关系清单（无障碍与逐条核查）</summary><div>
      {relationEdges.map((edge, index) => <button key={edge.id || index} onClick={() => setSelectedID(edge.from)}><strong>{nodeByID.get(edge.from)?.label}</strong><span><b>{humanRelation(edge.label)}</b><em>↔ {humanRelation(edge.inverse_label)}</em></span><strong>{nodeByID.get(edge.to)?.label}</strong><small>{edge.evidence_id || '无 Evidence 编号'}</small></button>)}
      {attributeEdges.map((edge, index) => <button key={edge.id || `attribute-${index}`} onClick={() => setSelectedID(edge.from)}><strong>{nodeByID.get(edge.from)?.label}</strong><span>{humanRelation(edge.label)}</span><strong>{nodeByID.get(edge.to)?.label}</strong><small>{edge.evidence_id || '无 Evidence 编号'}</small></button>)}
    </div></details>
  </div>
}
