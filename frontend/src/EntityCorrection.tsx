import { FormEvent, useMemo, useState } from 'react'
import { GitMerge, Scissors, ShieldAlert } from 'lucide-react'
import type { APIClient } from './api'
import type { CorrectionImpact, SemanticGraphData } from './MemorySemantics'

type EntityTarget = { id: string; label: string; entityType: string; impact: CorrectionImpact }

type Props = {
  api: APIClient
  projectID: string
  target: EntityTarget
  graph?: SemanticGraphData
  onClose: () => void
  onSubmitted: (message: string) => void
}

export function EntityCorrectionModal({ api, projectID, target, graph, onClose, onSubmitted }: Props) {
  const [action, setAction] = useState<'rename' | 'merge' | 'split'>('rename')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const candidates = useMemo(() => (graph?.nodes || []).filter(node => node.layer !== 'literal' && node.id !== target.id), [graph, target.id])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setSaving(true); setError('')
    const form = new FormData(event.currentTarget)
    const unitIDs = form.getAll('unit_ids').map(String)
    const request = {
      project_id: projectID,
      action,
      target_entity_id: action === 'merge' ? String(form.get('target_entity_id') || '') : '',
      canonical_name: action === 'merge' ? '' : String(form.get('canonical_name') || '').trim(),
      entity_type: action === 'merge' ? '' : String(form.get('entity_type') || target.entityType).trim(),
      unit_ids: action === 'split' ? unitIDs : [],
      edit_reason: String(form.get('edit_reason') || '').trim(),
      idempotency_key: `entity-correction-${target.id}-${action}-${Date.now()}`,
    }
    try {
      const result = await api.post<{ reviews?: Array<{ review_id: string }> }>(`/v1/entities/${encodeURIComponent(target.id)}/revision-proposals`, request)
      onSubmitted(`${action === 'rename' ? '实体重命名' : action === 'merge' ? '实体合并' : '实体拆分'}已生成 ${result.reviews?.length || 0} 个待审核 Revision；当前实体和关系尚未改变。`)
      onClose()
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)) }
    finally { setSaving(false) }
  }

  return <div className="modal-backdrop" onMouseDown={() => !saving && onClose()}>
    <form className="modal wide entity-correction-modal" onMouseDown={event => event.stopPropagation()} onSubmit={submit}>
      <p className="micro">OWNER ENTITY CORRECTION</p>
      <h2>纠正实体「{target.label}」</h2>
      <p className="drawer-lead">实体本身不会被直接 UPDATE。系统会把受影响的 Knowledge Unit 分别变成待审核 Revision；批准后才重建 Assertion、Temporal Fact 和项目投影。</p>
      <div className="correction-impact-strip">
        <span><b>{target.impact.unit_ids.length}</b> Knowledge Unit</span>
        <span><b>{target.impact.assertion_ids.length}</b> Assertion</span>
        <span><b>{target.impact.temporal_fact_ids.length}</b> 时间事实</span>
        <span><b>{target.impact.evidence_ids.length}</b> Evidence</span>
      </div>
      <fieldset><legend>纠正方式</legend><div className="entity-action-grid">
        <label className={action === 'rename' ? 'active' : ''}><input type="radio" name="action" value="rename" checked={action === 'rename'} onChange={() => setAction('rename')} /><ShieldAlert size={15}/><span><strong>Rename</strong><small>同一个实体，纠正规范名称或类型</small></span></label>
        <label className={action === 'merge' ? 'active' : ''}><input type="radio" name="action" value="merge" checked={action === 'merge'} onChange={() => setAction('merge')} /><GitMerge size={15}/><span><strong>Merge</strong><small>把受影响 KU 指向已经存在的实体</small></span></label>
        <label className={action === 'split' ? 'active' : ''}><input type="radio" name="action" value="split" checked={action === 'split'} onChange={() => setAction('split')} /><Scissors size={15}/><span><strong>Split</strong><small>只把明确选中的 KU 分到新实体</small></span></label>
      </div></fieldset>
      {action === 'merge' ? <label>合并到<select name="target_entity_id" required defaultValue=""><option value="" disabled>选择现有实体</option>{candidates.map(node => <option key={node.id} value={node.id}>{node.label} · {node.entity_type || node.layer}</option>)}</select></label> : <div className="form-grid two"><label>规范名称<input name="canonical_name" required defaultValue={action === 'rename' ? target.label : ''} /></label><label>实体类型<input name="entity_type" required defaultValue={target.entityType} /></label></div>}
      {action === 'split' && <fieldset><legend>只拆分这些 Knowledge Unit</legend><div className="check-grid entity-unit-list">{target.impact.unit_ids.map(id => <label key={id}><input type="checkbox" name="unit_ids" value={id} />{id}</label>)}</div><small className="token-warning">Split 必须明确选择至少一条 KU；未选中的关系继续留在原实体。</small></fieldset>}
      <label>修改原因<textarea name="edit_reason" required minLength={4} rows={3} placeholder="说明为什么这些关系应该重命名、合并或拆分；这条理由会进入 Revision 审计。" /></label>
      <p className="token-warning">Evidence、历史 Revision 和其他未受影响对象不会被改写。若某条 KU 已在别处被修改，stale proposal 会 fail closed。</p>
      {error && <p className="form-error">{error}</p>}
      <div className="modal-actions"><button type="button" className="button" disabled={saving} onClick={onClose}>取消</button><button className="button primary" disabled={saving}>{saving ? '正在创建 Revision…' : '创建待审核 Revision'}</button></div>
    </form>
  </div>
}
