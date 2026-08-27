import { ShieldCheck, ShieldAlert, Wrench } from 'lucide-react'
import { PluginConformanceReport } from './api'

type SchemaProperty={type?:string;title?:string;description?:string;enum?:unknown[];minimum?:number;maximum?:number;default?:unknown}
type ConfigSchema={type?:string;required?:string[];properties?:Record<string,SchemaProperty>}

function statusCopy(value:string){return value==='passed'?'通过':value==='failed'?'失败':value==='warning'?'警告':value==='not_executed'?'未执行':value}
function parsedObject(value:string){try{const item=JSON.parse(value);return item&&typeof item==='object'&&!Array.isArray(item)?item as Record<string,unknown>:undefined}catch{return undefined}}

function schemaFields(schema?:Record<string,unknown>){
  const typed=schema as ConfigSchema|undefined
  if(!typed||typed.type!=='object'||!typed.properties)return undefined
  for(const property of Object.values(typed.properties)){
    if(!property||(!property.enum&&!['string','integer','number','boolean'].includes(String(property.type))))return undefined
  }
  return typed
}

export function SchemaConfigEditor({schema,value,disabled,onChange}:{schema?:Record<string,unknown>;value:string;disabled?:boolean;onChange:(value:string)=>void}){
  const typed=schemaFields(schema);const current=parsedObject(value)
  if(!typed||!current)return <textarea value={value} onChange={event=>onChange(event.target.value)} spellCheck={false} rows={12} disabled={disabled}/>
  const required=new Set(typed.required||[])
  function update(key:string,next:unknown){const copy={...current,[key]:next};onChange(JSON.stringify(copy,null,2))}
  return <div className="plugin-schema-config">{Object.entries(typed.properties||{}).map(([key,property])=>{
    const label=property.title||key;const raw=current[key]??(property.type==='boolean'?false:'')
    const field=property.enum?<select value={String(raw)} disabled={disabled} onChange={event=>update(key,event.target.value)}>{property.enum.map(item=><option value={String(item)} key={String(item)}>{String(item)}</option>)}</select>:property.type==='boolean'?<input type="checkbox" checked={Boolean(raw)} disabled={disabled} onChange={event=>update(key,event.target.checked)}/>:property.type==='string'?<input type="text" value={String(raw)} disabled={disabled} onChange={event=>update(key,event.target.value)}/>:<input type="number" value={String(raw)} min={property.minimum} max={property.maximum} step={property.type==='integer'?1:'any'} disabled={disabled} onChange={event=>update(key,property.type==='integer'?Number.parseInt(event.target.value||'0',10):Number(event.target.value||0))}/>
    return <label key={key}><span>{label}{required.has(key)?' *':''}</span>{field}{property.description&&<small>{property.description}</small>}</label>
  })}</div>
}
export function PluginConformancePanel({report}:{report?:PluginConformanceReport}){
  if(!report)return <section className="plugin-conformance"><p>正在读取 Conformance、兼容性与权限差异…</p></section>
  return <section className={`plugin-conformance state-${report.overall_status}`}>
    <header><div>{report.overall_status==='failed'?<ShieldAlert size={17}/>:<ShieldCheck size={17}/>}<div><p>CONFORMANCE REPORT</p><h3>可运行性、权限与兼容性</h3></div></div><strong>{statusCopy(report.overall_status)}</strong></header>
    <div className="plugin-conformance-summary">
      <article><span>Memory Harness</span><b>{report.memory_harness_version}</b><small>{report.compatibility_requirement} · {report.compatibility_status}</small></article>
      <article><span>配置 Schema</span><b>{report.configuration_status}</b><small>{report.configuration_schema?'schema-driven':'无声明 Schema'}</small></article>
      <article><span>Required 缺失</span><b>{report.missing_required.length}</b><small>{report.missing_required.join(' · ')||'none'}</small></article>
      <article><span>Optional 未授予</span><b>{report.optional_not_granted.length}</b><small>{report.optional_not_granted.join(' · ')||'none'}</small></article>
    </div>
    <div className="plugin-conformance-checks">{report.checks.map(check=><article key={check.name} className={`state-${check.status}`}><div><Wrench size={13}/><strong>{check.name}</strong><span>{statusCopy(check.status)}</span></div><p>{check.detail}</p>{check.data&&Object.keys(check.data).length>0&&<pre>{JSON.stringify(check.data,null,2)}</pre>}</article>)}</div>
    <small>Conformance 是 server-owned 静态/重放检查；`not_executed` 表示没有执行网络、工具或插件副作用。</small>
  </section>
}
