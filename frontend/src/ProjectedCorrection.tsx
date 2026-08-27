import { useCallback, useEffect, useState } from 'react'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import type { APIClient } from './api'
import { KnowledgeUnitCorrectionModal } from './MemorySemantics'
import type { CorrectionImpact, KnowledgeUnit } from './MemorySemantics'

type ProjectedKind = 'assertion' | 'temporal_fact'
type Props = {
  api: APIClient
  projectID: string
  kind: ProjectedKind
  targetID: string
  label: string
  onClose: () => void
  onSubmitted: (message: string) => void
}

type Loaded = { unit: KnowledgeUnit; expectedRevision: number; impact: CorrectionImpact }

export function ProjectedCorrectionModal({ api, projectID, kind, targetID, label, onClose, onSubmitted }: Props) {
  const [loaded, setLoaded] = useState<Loaded>()
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const impactResult = await api.get<{ impact: CorrectionImpact }>(`/v1/corrections/impact?project_id=${encodeURIComponent(projectID)}&kind=${kind}&id=${encodeURIComponent(targetID)}`)
      const impact = impactResult.impact
      if (impact.unit_ids.length !== 1) throw new Error(`投影必须唯一回溯到 1 条 Knowledge Unit；当前为 ${impact.unit_ids.length} 条。`)
      const unitID = impact.unit_ids[0]
      const detail = await api.get<{ knowledge_unit: KnowledgeUnit; governance?: { current_revision?: number } }>(`/v1/knowledge-units/${encodeURIComponent(unitID)}?project_id=${encodeURIComponent(projectID)}`)
      setLoaded({ unit: detail.knowledge_unit, expectedRevision: Number(detail.governance?.current_revision || 0), impact })
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)) }
    finally { setLoading(false) }
  }, [api, projectID, kind, targetID])

  useEffect(() => { void load() }, [load])

  async function submit(draft: KnowledgeUnit, editReason: string) {
    if (!loaded) return
    await api.post(`/v1/knowledge-units/${encodeURIComponent(draft.unit_id)}/revision-proposals`, {
      project_id: projectID,
      expected_revision: loaded.expectedRevision,
      edit_reason: editReason,
      idempotency_key: `projected-${kind}-${targetID}-${loaded.expectedRevision}-${Date.now()}`,
      knowledge_unit: draft,
    })
    onSubmitted(`${kind === 'assertion' ? '关系' : '时间事实'}“${label}”的纠正已进入待审核 KU Revision；批准前当前投影不会改变。`)
    onClose()
  }

  if (loaded) return <KnowledgeUnitCorrectionModal unit={loaded.unit} expectedRevision={loaded.expectedRevision} impact={loaded.impact} onClose={onClose} onSubmit={submit} />
  return <div className="modal-backdrop" onMouseDown={onClose}>
    <section className="modal" onMouseDown={event => event.stopPropagation()}>
      <p className="micro">PROJECTED CORRECTION → KU AUTHORITY</p>
      <h2>纠正「{label}」</h2>
      {loading ? <p className="drawer-lead"><RefreshCw size={14} /> 正在回溯唯一权威 Knowledge Unit…</p> : <><div className="governance-error"><AlertTriangle size={18}/>{error || '无法回溯纠正来源。'}</div><p className="drawer-lead">不会直接 UPDATE Assertion / Temporal Fact。请先修复来源映射，再提交纠正。</p></>}
      <div className="modal-actions"><button className="button" onClick={onClose}>关闭</button>{error && <button className="button" onClick={() => void load()}>重试</button>}</div>
    </section>
  </div>
}
