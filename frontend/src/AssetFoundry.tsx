import { useCallback, useEffect, useMemo, useState } from 'react'
import { BookOpen, FileDiff, Filter, PackageCheck, Play, RefreshCw, ShieldCheck, X } from 'lucide-react'
import { APIClient, HarnessObject } from './api'
import { AssetGovernanceDrawer } from './AssetGovernance'

type Asset = {
  asset_id:string; asset_type:string; title:string; summary:string; status:string; version:string
  risk_level:string; source_memory_ids:string[]; validation_status:string; classification_status?:string
  classification_scores?:Record<string,number>; classification_reasons?:string[]; created_at:string; updated_at:string
}
type TemplateField={key:string;label:string;description:string;kind:'text'|'list';required:boolean;placeholder:string}
type AssetTemplate={asset_type:string;label:string;description:string;template_version:string;fields:TemplateField[]}
type TemplateResponse={template_version:string;schema_version:string;templates:AssetTemplate[]}
type RefinementResult={run_id:string;total:number;refined:number;proposed:number;skipped:number;failed:number;model_groups:number;fallback_groups:number}

const v4Type='builtin.agent-assets.governed-asset.v4'
const typeCopy:Record<string,string>={
  prompt:'Prompt · Agent 提示', skill:'Skill · 可复用技能', rule:'Rule · 判断规则', constraint:'Constraint · 强约束',
  procedure:'Procedure · 标准流程', tool_recipe:'Tool Recipe · 工具配方', mcp:'MCP · 工具合同', unclassified:'待人工分类',
}
const orderedTypes=['prompt','skill','rule','constraint','procedure','tool_recipe','mcp','unclassified']

function when(value:string){const d=new Date(value);return Number.isNaN(d.getTime())?value:new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(d)}
function payloadOf(object?:HarnessObject){return (object?.revision.payload||{}) as Record<string,unknown>}

export function AssetFoundry({api,projectID}:{api:APIClient;projectID:string}){
  const [items,setItems]=useState<Asset[]>([]); const [templates,setTemplates]=useState<AssetTemplate[]>([]); const [templateObjects,setTemplateObjects]=useState<HarnessObject[]>([])
  const [selectedType,setSelectedType]=useState(''); const [timeScope,setTimeScope]=useState<'all'|'7d'|'30d'>('all'); const [selectedID,setSelectedID]=useState(''); const [selectedAssets,setSelectedAssets]=useState<Set<string>>(new Set())
  const [loading,setLoading]=useState(false); const [error,setError]=useState(''); const [notice,setNotice]=useState(''); const [refining,setRefining]=useState(false)
  const [showTemplates,setShowTemplates]=useState(false); const [showRefine,setShowRefine]=useState(false)
  const load=useCallback(async()=>{if(!projectID)return;setLoading(true);setError('');try{const [assetsResult,templateResult,objectResult]=await Promise.all([
    api.get<{assets:Asset[]}>(`/v1/assets?project_id=${encodeURIComponent(projectID)}`),
    api.get<TemplateResponse>('/v1/asset-templates'),
    api.get<{objects:HarnessObject[]}>(`/v1/harness/objects?project_id=${encodeURIComponent(projectID)}&type_id=${encodeURIComponent(v4Type)}&limit=200`),
  ]);setItems(assetsResult.assets||[]);setTemplates(templateResult.templates||[]);setTemplateObjects(objectResult.objects||[])}catch(reason){setError(reason instanceof Error?reason.message:String(reason))}finally{setLoading(false)}},[api,projectID])
  useEffect(()=>{void load()},[load])
  useEffect(()=>setSelectedAssets(new Set()),[projectID])
  const counts=useMemo(()=>Object.fromEntries(orderedTypes.map(type=>[type,items.filter(item=>item.asset_type===type).length])),[items])
  const visible=useMemo(()=>items.filter(item=>{if(selectedType&&item.asset_type!==selectedType)return false;if(timeScope==='all')return true;const stamp=new Date(item.updated_at).getTime();const days=timeScope==='7d'?7:30;return Number.isFinite(stamp)&&stamp>=Date.now()-days*86400000}),[items,selectedType,timeScope])
  const templated=useMemo(()=>new Map(templateObjects.map(object=>[String(payloadOf(object).asset_id||''),object])),[templateObjects])
  const active=templateObjects.filter(item=>item.status==='active').length
  const ambiguous=items.filter(item=>item.asset_type==='unclassified'||item.classification_status==='ambiguous').length
  const refinementScope=selectedAssets.size?items.filter(item=>selectedAssets.has(item.asset_id)):visible.filter(item=>item.asset_type!=='unclassified')
  function toggle(assetID:string){setSelectedAssets(current=>{const next=new Set(current);if(next.has(assetID))next.delete(assetID);else next.add(assetID);return next})}
  async function refine(mode:'incremental'|'force'){
    if(mode==='force'&&!window.confirm(`将重新检查当前范围的 ${refinementScope.length} 个候选，即使之前已经模板化也会再次调用模型。原材料不会重跑或改写。继续吗？`))return
    setRefining(true);setError('');setNotice('')
    try{const result=await api.post<RefinementResult>(`/v1/projects/${encodeURIComponent(projectID)}/assets/refine`,{asset_ids:refinementScope.map(item=>item.asset_id),asset_type:selectedAssets.size?'':selectedType,idempotency_key:`asset-template-${mode}-${Date.now()}`,mode});setNotice(`二次沉淀完成：新建 ${result.refined}，待审核修订 ${result.proposed}，跳过 ${result.skipped}，失败 ${result.failed}。${result.fallback_groups?`有 ${result.fallback_groups} 个批次没有取得可验证的模型结果，只保存了待补齐骨架；这些候选下次会自动重试，不能直接激活。`:''}`);setShowRefine(false);setSelectedAssets(new Set());await load()}catch(reason){setError(reason instanceof Error?reason.message:String(reason))}finally{setRefining(false)}
  }
  return <div className="asset-foundry">
    <section className="asset-foundry-hero"><div><p>TEMPLATE-GOVERNED AGENT ASSET FOUNDRY</p><h2>先从记忆发现候选，<br/>再按标准格式二次沉淀。</h2><span>第一轮只识别“可能是什么”；第二轮才按 Skill、Prompt、规则或 MCP 的独立模板补齐触发条件、输入输出、步骤、边界和验收。来源资产与原始记忆始终保留。</span><div className="asset-foundry-actions"><button className="button light" onClick={()=>setShowTemplates(true)}><BookOpen size={15}/>查看七类标准模板</button><button className="button light" disabled={!items.length} onClick={()=>setShowRefine(true)}><Play size={15}/>按模板二次沉淀</button></div></div><div className="asset-foundry-metrics"><article><strong>{items.length}</strong><span>第一轮候选</span></article><article><strong>{templateObjects.length}</strong><span>已模板化</span></article><article><strong>{active}</strong><span>已激活</span></article><article className={ambiguous?'attention':''}><strong>{ambiguous}</strong><span>待定类</span></article></div></section>
    {notice&&<p className="surface-notice"><ShieldCheck size={14}/>{notice}</p>}
    <div className="asset-foundry-layout"><aside><header><Filter size={15}/><div><strong>资产类型</strong><small>V4 TEMPLATE CONTRACTS</small></div><button title="刷新" aria-label="刷新能力资产" onClick={load}><RefreshCw size={14}/></button></header><button className={!selectedType?'active':''} onClick={()=>setSelectedType('')}><span>全部类型</span><b>{items.length}</b></button>{orderedTypes.map(type=><button key={type} className={selectedType===type?'active':''} onClick={()=>setSelectedType(type)}><span>{typeCopy[type]}</span><b>{counts[type]||0}</b></button>)}</aside>
      <main><div className="asset-foundry-note"><ShieldCheck size={15}/><span>Agent 优先读取 <b>V4 模板化 + Active + 服务端验证通过</b> 的资产。第一轮候选、待审核、验证失败或缺字段的骨架都不会进入安全消费面。</span></div><div className="asset-selection-tools"><label>时间范围<select aria-label="能力候选时间范围" value={timeScope} onChange={event=>setTimeScope(event.target.value as 'all'|'7d'|'30d')}><option value="all">全部时间</option><option value="7d">最近 7 天</option><option value="30d">最近 30 天</option></select></label><span>{selectedAssets.size?`已选择 ${selectedAssets.size} 个候选`:`未勾选时，操作当前筛选下 ${refinementScope.length} 个已分类候选`}</span>{selectedAssets.size>0&&<button className="text-button" onClick={()=>setSelectedAssets(new Set())}>清除选择</button>}<button className="button surface" disabled={!refinementScope.length} onClick={()=>setShowRefine(true)}>沉淀当前范围</button></div>{error&&<p className="form-error inline-error">{error}</p>}{loading&&!items.length?<div className="loading-state"><span/><p>正在读取能力资产…</p></div>:visible.length?<div className="asset-foundry-grid">{visible.map(item=>{const governed=templated.get(item.asset_id);const structured=payloadOf(governed);return <article key={item.asset_id} className={selectedAssets.has(item.asset_id)?'selected':''}><label className="asset-select"><input type="checkbox" checked={selectedAssets.has(item.asset_id)} disabled={item.asset_type==='unclassified'} onChange={()=>toggle(item.asset_id)}/><span>{item.asset_type==='unclassified'?'先人工定类':'加入本次二次沉淀'}</span></label><button onClick={()=>setSelectedID(item.asset_id)}><div><span className={`asset-kind kind-${item.asset_type}`}>{typeCopy[item.asset_type]||item.asset_type}</span><em>{governed?`已模板化 R${governed.current_revision}`:item.status}</em></div><h3>{String(structured.title||item.title)}</h3><p>{String(structured.summary||item.summary)}</p><footer><span><FileDiff size={12}/>{governed?String(structured.validation_status||'not_run'):`${item.classification_status||'legacy'} · 未模板化`}</span><time>{when(governed?.updated_at||item.updated_at)}</time></footer></button></article>})}</div>:<div className="empty"><span><PackageCheck size={20}/></span><h3>这个筛选下没有资产</h3><p>程序性记忆会先经过确定性分类器；信号不足时保持未分类而不是猜测。</p></div>}</main>
    </div>
    {showTemplates&&<div className="modal-backdrop" onMouseDown={()=>setShowTemplates(false)}><section className="modal wide asset-template-modal" role="dialog" aria-modal="true" aria-labelledby="asset-template-title" onMouseDown={event=>event.stopPropagation()}><button className="drawer-close" aria-label="关闭标准模板" onClick={()=>setShowTemplates(false)}><X size={18}/></button><p className="micro">AGENT ASSET CONTRACTS</p><h2 id="asset-template-title">七类能力资产，各自有明确格式</h2><p className="modal-intro">不是把一段记忆换个标签。只有必填结构完整、来源保留、服务端验证通过并经 Owner 审核后，才可激活。</p><div className="asset-template-list">{templates.map(template=><article key={template.asset_type}><header><strong>{template.label}</strong><span>{template.fields.filter(field=>field.required).length} 个必填项</span></header><p>{template.description}</p><dl>{template.fields.map(field=><div key={field.key}><dt>{field.label}{field.required&&<b>必填</b>}</dt><dd>{field.description}</dd></div>)}</dl></article>)}</div><div className="modal-actions"><button className="button" onClick={()=>setShowTemplates(false)}>关闭</button><button className="button primary" onClick={()=>{setShowTemplates(false);setShowRefine(true)}}>开始二次沉淀</button></div></section></div>}
    {showRefine&&<div className="modal-backdrop" onMouseDown={()=>!refining&&setShowRefine(false)}><section className="modal" role="dialog" aria-modal="true" aria-labelledby="asset-refine-title" onMouseDown={event=>event.stopPropagation()}><p className="micro">SECOND DISTILLATION</p><h2 id="asset-refine-title">按标准模板整理 {refinementScope.length} 个候选</h2><div className="asset-refine-explain"><p><b>会做：</b>同类型候选放在一批比较，用模型补齐该类型的标准字段；每个候选仍独立保存。</p><p><b>不会做：</b>不会重跑原材料、不会改写 Evidence、不会把多个来源合并成一个资产、不会自动激活。</p><p><b>模型不可用：</b>只生成明确标记为“待补齐”的模板骨架，并阻止激活。</p></div>{!refinementScope.length&&<p className="form-error">当前范围没有已分类候选。请换一个筛选或先完成人工定类。</p>}<div className="modal-actions"><button className="button" disabled={refining} onClick={()=>setShowRefine(false)}>取消</button><button className="button" disabled={refining||!refinementScope.length} onClick={()=>void refine('force')}>重新检查当前范围</button><button className="button primary" disabled={refining||!refinementScope.length} onClick={()=>void refine('incremental')}>{refining?'正在二次沉淀…':'只处理新增或有变化'}</button></div></section></div>}
    {selectedID&&<AssetGovernanceDrawer api={api} assetID={selectedID} onClose={()=>setSelectedID('')} onChanged={load}/>}
  </div>
}
