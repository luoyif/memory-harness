import { ArrowRight, CheckCircle2, Network, ShieldCheck } from 'lucide-react'

export type ContextBridgeManifest = {
  status: string
  protocol: string
  mcp_baseline_unchanged: boolean
  contracts: Record<string,string>
  endpoints: Array<{ method:string; path:string; permission:string }>
  evidence_states: string[]
  invariants: string[]
}

const stateCopy:Record<string,string>={
  retrieved:'召回候选', planned:'计划注入', delivery_unverified:'送达未验证', delivered:'已送达', used_unknown:'是否使用未知', outcome_observed:'结果已观测',
}

export function ContextBridgeCapabilities({ manifest }:{ manifest?: ContextBridgeManifest }) {
  if (!manifest) return null
  return <section className="context-bridge-capabilities">
    <header><div><Network size={19}/><div><p className="micro">RICH ADAPTER · OPTIONAL</p><h3>Context Bridge v1</h3><span>{manifest.protocol}</span></div></div><b>{manifest.status}</b></header>
    <div className="context-state-flow">{manifest.evidence_states.map((state,index)=><span key={state}><strong>{stateCopy[state]||state}</strong><code>{state}</code>{index<manifest.evidence_states.length-1&&<ArrowRight size={13}/>}</span>)}</div>
    <div className="context-bridge-grid"><article><ShieldCheck size={16}/><div><strong>MCP 基线保持兼容</strong><p>{manifest.mcp_baseline_unchanged?'旧 MCP 客户端继续使用现有 L1/L2 工具，不需要实现 Receipt。':'需要检查兼容性'}</p></div></article><article><CheckCircle2 size={16}/><div><strong>Rich Adapter 需要显式权限</strong><p>context.plan / context.receipt / outcome.report 不会从 memory.read 自动继承。</p></div></article></div>
    <div className="context-endpoint-list">{manifest.endpoints.map(item=><div key={`${item.method}:${item.path}`}><code>{item.method}</code><strong>{item.path}</strong><span>{item.permission}</span></div>)}</div>
    <footer>{manifest.invariants.map(item=><span key={item}>{item}</span>)}</footer>
  </section>
}
