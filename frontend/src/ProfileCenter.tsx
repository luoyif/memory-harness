import { useCallback, useEffect, useMemo, useState } from 'react'
import { Fingerprint, LockKeyhole, RefreshCw, ShieldAlert, ShieldCheck, UnlockKeyhole } from 'lucide-react'
import { APIClient, HarnessObject } from './api'

type ProfileBlock = {
  block_id:string; label:string; content:string; source_refs:string[]; source_object_ids:string[]
  source_hash:string; candidate_source_hash?:string; valid_from?:string; valid_until?:string
  last_verified_at:string; confidence:number; locked:boolean; review_status:string
}
type ProfilePayload = {
  profile_id:string; view_kind:string; profile_class:string; title:string; summary:string
  blocks:ProfileBlock[]; locked_block_ids:string[]; generation_status:string
  generated_from_revision:number; generated_at:string
}
const copy:Record<string,{label:string;detail:string}> = {
  owner_identity:{label:'稳定身份',detail:'只供 Owner 查看；默认不会进入 Agent Profile View。'},
  dynamic_project:{label:'项目动态',detail:'目标、决策、风险和当前有效时间事实。'},
  session_resume:{label:'会话恢复',detail:'未完成目标、里程碑和最近可恢复记忆。'},
  stable_preference:{label:'稳定偏好',detail:'跨会话保持、可人工锁定的稳定偏好。'},
  relationship:{label:'关系视图',detail:'人物关系与互动背景，不等于事实真值。'},
  agent_identity:{label:'Agent 身份',detail:'面向特定 Agent 的受限身份视图。'},
}
function payloadOf(object:HarnessObject){return object.revision.payload as unknown as ProfilePayload}
function stamp(value?:string){if(!value)return '—';const d=new Date(value);return Number.isNaN(d.valueOf())?value:d.toLocaleString('zh-CN',{hour12:false})}
function short(value?:string){return value?value.slice(0,12):'—'}
export function ProfileCenter({api,projectID}:{api:APIClient;projectID:string}){
  const [items,setItems]=useState<HarnessObject[]>([])
  const [selected,setSelected]=useState<HarnessObject>()
  const [loading,setLoading]=useState(false)
  const [refreshing,setRefreshing]=useState(false)
  const [error,setError]=useState('')
  const [notice,setNotice]=useState('')
  const load=useCallback(async()=>{if(!projectID)return;setLoading(true);setError('');try{
    const result=await api.get<{profiles:HarnessObject[]}>(`/v1/profiles?project_id=${encodeURIComponent(projectID)}`)
    setItems(result.profiles||[])
  }catch(reason){setError(reason instanceof Error?reason.message:String(reason))}finally{setLoading(false)}},[api,projectID])
  useEffect(()=>{void load()},[load])
  const metrics=useMemo(()=>({blocks:items.reduce((sum,item)=>sum+(payloadOf(item).blocks?.length||0),0),locked:items.reduce((sum,item)=>sum+(payloadOf(item).locked_block_ids?.length||0),0),stale:items.reduce((sum,item)=>sum+(payloadOf(item).blocks||[]).filter(block=>block.review_status==='stale_locked').length,0)}),[items])
  async function refresh(){setRefreshing(true);setError('');setNotice('');try{
    await api.post(`/v1/projects/${encodeURIComponent(projectID)}/profiles/refresh`,{})
    setNotice('画像已从当前受治理权威重新编译；锁定 Block 未被自动覆盖。')
    await load()
  }catch(reason){setError(reason instanceof Error?reason.message:String(reason))}finally{setRefreshing(false)}}
  return <div className="profile-center"><section className="profile-center-hero"><div><p>PROFILE COMPILER · GOVERNED PROJECTION</p><h2>不是一个越来越长的 MD，而是按用途编译的上下文画像。</h2><span>画像只从当前受治理 Memory / Project authority 生成。人工锁定的 Block 会保留；来源变化时只标记 stale，不静默覆盖。</span></div><button className="button light" disabled={refreshing} onClick={()=>void refresh()}><RefreshCw size={14}/>{refreshing?'正在编译…':'重新编译画像'}</button></section>
    {notice&&<p className="surface-notice"><ShieldCheck size={14}/>{notice}</p>}
    {error&&<p className="form-error inline-error">{error}</p>}
    <div className="profile-metrics"><article><span>画像</span><strong>{items.length}</strong></article><article><span>Block</span><strong>{metrics.blocks}</strong></article><article><span>人工锁定</span><strong>{metrics.locked}</strong></article><article className={metrics.stale?'attention':''}><span>待复核锁定项</span><strong>{metrics.stale}</strong></article></div>
    {loading&&!items.length?<div className="loading-state"><span/><p>正在读取上下文画像…</p></div>:items.length?<div className="profile-grid">{items.map(item=>{const payload=payloadOf(item);const label=copy[payload.view_kind]?.label||payload.title;const stale=(payload.blocks||[]).filter(block=>block.review_status==='stale_locked').length;return <button key={item.object_id} onClick={()=>setSelected(item)}><header><div><Fingerprint size={16}/><span>{label}</span></div><em>{payload.profile_class}</em></header><h3>{payload.title}</h3><p>{payload.summary}</p><footer><span>{payload.blocks?.length||0} blocks · {payload.locked_block_ids?.length||0} locked</span><span className={stale?'warn-text':''}>{stale?`${stale} stale_locked`:`R${item.current_revision}`}</span></footer></button>})}</div>:<div className="empty"><span><Fingerprint size={20}/></span><h3>当前项目还没有画像</h3><p>点击“重新编译画像”，系统会从当前受治理事实、目标、决策、风险和长期记忆生成用途分离的 Projection。</p></div>}
    {selected&&<ProfileDrawer api={api} object={selected} projectID={projectID} onClose={()=>setSelected(undefined)} onChanged={load}/>}
  </div>
}

function ProfileDrawer({api,object,projectID,onClose,onChanged}:{api:APIClient;object:HarnessObject;projectID:string;onClose:()=>void;onChanged:()=>void}){
  const [current,setCurrent]=useState(object)
  const [saving,setSaving]=useState(false)
  const [error,setError]=useState('')
  const payload=payloadOf(current)
  useEffect(()=>setCurrent(object),[object])
  async function setLocks(blockID:string,locked:boolean){setSaving(true);setError('');try{
    const next=new Set(payload.locked_block_ids||[]);if(locked)next.add(blockID);else next.delete(blockID)
    const result=await api.put<{object:HarnessObject}>(`/v1/projects/${encodeURIComponent(projectID)}/profiles/${encodeURIComponent(payload.view_kind)}/locks`,{block_ids:[...next]})
    setCurrent(result.object);await onChanged()
  }catch(reason){setError(reason instanceof Error?reason.message:String(reason))}finally{setSaving(false)}}
  return <div className="drawer-backdrop" onMouseDown={onClose}><aside className="drawer profile-drawer" onMouseDown={event=>event.stopPropagation()}><button className="drawer-close" aria-label="关闭" onClick={onClose}>×</button>
    <p className="micro">PROFILE · {payload.view_kind.toUpperCase()}</p><h2>{payload.title}</h2>
    <div className="drawer-status"><span>R{current.current_revision}</span><span>{payload.profile_class}</span><span>{payload.generation_status}</span><span>{payload.locked_block_ids?.length||0} locked</span></div>
    <p className="drawer-lead">{copy[payload.view_kind]?.detail||payload.summary}</p>{error&&<p className="form-error">{error}</p>}
    <div className="profile-block-list">{(payload.blocks||[]).map(block=><article key={block.block_id} className={block.review_status==='stale_locked'?'stale':''}><header><div><strong>{block.label}</strong><code>{block.block_id}</code></div><button className="text-button" disabled={saving} onClick={()=>void setLocks(block.block_id,!block.locked)}>{block.locked?<><UnlockKeyhole size={13}/>解除锁定</>:<><LockKeyhole size={13}/>锁定此 Block</>}</button></header>
      <pre>{block.content}</pre><footer><span>{block.review_status==='stale_locked'?<><ShieldAlert size={12}/>来源已变化，等待人工复核</>:<><ShieldCheck size={12}/>当前来源一致</>}</span><span>confidence {Math.round((block.confidence||0)*100)}%</span><span>{stamp(block.last_verified_at)}</span></footer>
      <div className="profile-block-proof"><span>source {short(block.source_hash)}</span>{block.candidate_source_hash&&<span>candidate {short(block.candidate_source_hash)}</span>}<span>{block.source_refs?.length||0} Evidence · {block.source_object_ids?.length||0} Objects</span></div>
    </article>)}</div>
    <footer className="profile-drawer-foot"><span>Generated {stamp(payload.generated_at)}</span><span>source revision ≤ R{payload.generated_from_revision||0}</span><span>Object Store current R{current.current_revision}</span></footer>
  </aside></div>
}
