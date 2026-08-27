import { useEffect, useMemo, useState } from 'react'
import {
  Blocks, BookOpenCheck, Box, Check, ChevronRight, FileJson, Filter, KeyRound, Layers3,
  LockKeyhole, PackageCheck, PlugZap, RefreshCw, Save, Search, ShieldCheck, Trash2,
  SlidersHorizontal, Sparkles, ToggleLeft, ToggleRight, Upload, Workflow, XCircle,
} from 'lucide-react'

import { APIClient, PluginConformanceReport, PluginVersion, Project } from './api'
import { PluginConformancePanel, SchemaConfigEditor } from './PluginConformancePanel'

type ProjectSummary = { project: Project }
type ContributionKey = keyof PluginVersion['contributions']

type PluginImpact = {
  plugin_id:string; version:string; current_objects:number; historical_revisions:number; historical_runs:number; pipeline_versions:number; blueprint_versions:number; enabled_projects:number; active_blueprint_refs:number; package_bytes_reclaimed:number; can_retire:boolean; history_preserved:boolean; blockers:string[]
}

const contributionMeta: Array<{ key: ContributionKey; label: string; icon: typeof Blocks }> = [
  { key: 'memory_types', label: '记忆类型', icon: Layers3 },
  { key: 'pipelines', label: '流程模板', icon: Workflow },
  { key: 'stages', label: '处理阶段', icon: Blocks },
  { key: 'strategy_components', label: '策略组件', icon: SlidersHorizontal },
  { key: 'blueprints', label: '记忆方案', icon: BookOpenCheck },
  { key: 'connectors', label: '连接器', icon: PlugZap },
  { key: 'projections', label: '投影视图', icon: Filter },
  { key: 'views', label: '界面扩展', icon: Box },
]

function contributionTitle(value: unknown, fallback: string) {
  if (!value || typeof value !== 'object') return fallback
  const item = value as Record<string, unknown>
  return String(item.display_name || item.name || item.type_id || item.pipeline_id || item.component_id || item.blueprint_id || item.stage_type || item.id || fallback)
}

function contributionSubtitle(value: unknown) {
  if (!value || typeof value !== 'object') return ''
  const item = value as Record<string, unknown>
  return String(item.type_id || item.pipeline_id || item.component_id || item.blueprint_id || item.stage_type || item.id || item.version || '')
}

function projectState(plugin: PluginVersion, projectID: string) {
  return plugin.project_states.find(item => item.project_id === projectID)
}

function trustExplanation(plugin: PluginVersion) {
  if (plugin.signature_status === 'bundled') return '随 Memory Harness 发布，由本地应用维护。'
  if (plugin.signature_status === 'verified') return '包签名和发布者信任链已验证。'
  if (plugin.status === 'quarantined') return '插件已隔离，不能参与任何项目运行。'
  return '尚未建立可用于生产运行的信任链。'
}

export function PluginCenter({ api, projectID, projects }: { api: APIClient; projectID: string; projects: ProjectSummary[] }) {
  const [plugins, setPlugins] = useState<PluginVersion[]>([])
  const [loading, setLoading] = useState(true)
  const [fatal, setFatal] = useState('')
  const [selectedKey, setSelectedKey] = useState('')
  const [query, setQuery] = useState('')
  const [scope, setScope] = useState<'all' | 'builtin' | 'external'>('all')
  const [capabilities, setCapabilities] = useState<string[]>([])
  const [configText, setConfigText] = useState('{}')
  const [enabled, setEnabled] = useState(true)
  const [dirty, setDirty] = useState(false)
  const [notice, setNotice] = useState('')
  const [saving, setSaving] = useState(false)
  const [activeContribution, setActiveContribution] = useState<ContributionKey>('memory_types')
  const [impact, setImpact] = useState<PluginImpact>()
  const [impactError, setImpactError] = useState('')
  const [conformance, setConformance] = useState<PluginConformanceReport>()
  const [conformanceError, setConformanceError] = useState('')
  const [conformanceTick, setConformanceTick] = useState(0)

  async function reload(preferredKey = selectedKey) {
    setLoading(true); setFatal('')
    try {
      const response = await api.get<{ plugins: PluginVersion[] }>('/v1/plugins')
      setPlugins(response.plugins)
      const fallback = response.plugins[0] ? `${response.plugins[0].plugin_id}@${response.plugins[0].version}` : ''
      if (!response.plugins.some(item => `${item.plugin_id}@${item.version}` === preferredKey)) setSelectedKey(fallback)
    } catch (reason) { setFatal(reason instanceof Error ? reason.message : String(reason)) }
    finally { setLoading(false) }
  }

  useEffect(() => { void reload('') }, [api]) // eslint-disable-line react-hooks/exhaustive-deps

  const selected = plugins.find(item => `${item.plugin_id}@${item.version}` === selectedKey)
  const state = selected ? projectState(selected, projectID) : undefined

  useEffect(() => {
    if (!selected) return
    const explicit = projectState(selected, projectID)
    setEnabled(explicit ? explicit.status === 'enabled' : selected.plugin_id.startsWith('builtin.') && selected.status === 'enabled')
    setCapabilities(explicit?.granted_capabilities || (selected.plugin_id.startsWith('builtin.') && selected.status === 'enabled' ? selected.permissions : []))
    setConfigText(JSON.stringify(explicit?.config || {}, null, 2))
    setDirty(false); setNotice('')
    const first = contributionMeta.find(item => (selected.contributions[item.key] || []).length)
    setActiveContribution(first?.key || 'memory_types')
  }, [selectedKey, projectID]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    setImpact(undefined); setImpactError('')
    if (!selected) return
    api.get<PluginImpact>(`/v1/plugins/${encodeURIComponent(selected.plugin_id)}/${encodeURIComponent(selected.version)}/impact?project_id=${encodeURIComponent(projectID)}`)
      .then(value => {
        if (!value || typeof value !== 'object' || typeof value.current_objects !== 'number' || !Array.isArray(value.blockers)) throw new Error('插件影响接口返回格式无效')
        setImpact(value)
      }).catch(reason => setImpactError(reason instanceof Error ? reason.message : String(reason)))
  }, [api, selectedKey, projectID]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    setConformance(undefined); setConformanceError('')
    if (!selected) return
    api.get<PluginConformanceReport>(`/v1/plugins/${encodeURIComponent(selected.plugin_id)}/${encodeURIComponent(selected.version)}/conformance?project_id=${encodeURIComponent(projectID)}`)
      .then(setConformance).catch(reason => setConformanceError(reason instanceof Error ? reason.message : String(reason)))
  }, [api, selectedKey, projectID, conformanceTick]) // eslint-disable-line react-hooks/exhaustive-deps

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return plugins.filter(item => {
      if (scope === 'builtin' && !item.plugin_id.startsWith('builtin.')) return false
      if (scope === 'external' && item.plugin_id.startsWith('builtin.')) return false
      return !needle || `${item.name} ${item.plugin_id} ${item.publisher}`.toLowerCase().includes(needle)
    })
  }, [plugins, query, scope])

  function toggleCapability(value: string) {
    setCapabilities(current => current.includes(value) ? current.filter(item => item !== value) : [...current, value].sort())
    setDirty(true)
  }

  async function saveState() {
    if (!selected || !projectID) return
    let config: unknown
    try { config = JSON.parse(configText) }
    catch (reason) { setNotice(`配置不是合法 JSON：${reason instanceof Error ? reason.message : String(reason)}`); return }
    setSaving(true); setNotice('')
    try {
      const updated = await api.put<PluginVersion>(`/v1/plugins/${encodeURIComponent(selected.plugin_id)}/${encodeURIComponent(selected.version)}/projects/${encodeURIComponent(projectID)}`, {
        status: enabled ? 'enabled' : 'disabled', capabilities, config,
      })
      setPlugins(current => current.map(item => item.plugin_id === updated.plugin_id && item.version === updated.version ? updated : item))
      setDirty(false); setNotice(enabled ? '项目授权与插件配置已保存。' : '插件已对当前项目禁用。')
      setConformanceTick(value => value + 1)
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : String(reason)) }
    finally { setSaving(false) }
  }

  async function retireSelected() {
    if (!selected || !impact?.can_retire) return
    if (!window.confirm(`退役 ${selected.plugin_id}@${selected.version}？可执行包会被清除，但历史 Object、Revision、Run、Pipeline 仍保留。`)) return
    setSaving(true); setNotice('')
    try {
      const retired = await api.post<PluginVersion>(`/v1/plugins/${encodeURIComponent(selected.plugin_id)}/${encodeURIComponent(selected.version)}/retire`, {})
      setNotice(`已退役 ${retired.plugin_id}@${retired.version}；历史保持可读，如需再次运行必须重新安装。`)
      await reload(`${retired.plugin_id}@${retired.version}`)
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : String(reason)) }
    finally { setSaving(false) }
  }

  async function install() {
    try {
      const method = window.go?.main?.DesktopBridge?.InstallPluginPackage
      if (!method) throw new Error('插件安装只在桌面应用中开放')
      const item = await method('', [], false)
      if (item?.plugin_id) {
        const key = `${item.plugin_id}@${item.version}`
        setNotice(`已安装 ${key}。请检查能力后再为项目启用。`)
        await reload(key)
        setSelectedKey(key)
      }
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : String(reason)) }
  }

  if (loading && !plugins.length) return <div className="plugin-center-state"><RefreshCw className="spin" size={22} /><p>正在核对插件清单、签名与项目授权…</p></div>
  if (fatal) return <div className="plugin-center-state danger"><XCircle size={26} /><h3>插件中心读取失败</h3><p>{fatal}</p><button onClick={() => void reload()}>重试</button></div>

  const currentProject = projects.find(item => item.project.project_id === projectID)?.project
  const activeItems = selected?.contributions[activeContribution] || []

  return <div className="plugin-center-shell">
    <aside className="plugin-catalog">
      <header><div><p>PLUGIN CATALOG</p><h2>可扩展能力</h2></div><button onClick={install} title="安装签名插件"><Upload size={15} /></button></header>
      <label className="plugin-search"><Search size={14} /><input value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索插件、发布者…" /></label>
      <div className="plugin-filters">{(['all', 'builtin', 'external'] as const).map(value => <button className={scope === value ? 'active' : ''} key={value} onClick={() => setScope(value)}>{value === 'all' ? '全部' : value === 'builtin' ? '内置' : '外部'}</button>)}</div>
      <div className="plugin-catalog-list">{visible.map(item => {
        const itemState = projectState(item, projectID)
        const itemEnabled = itemState ? itemState.status === 'enabled' : item.plugin_id.startsWith('builtin.') && item.status === 'enabled'
        return <button key={`${item.plugin_id}@${item.version}`} className={selectedKey === `${item.plugin_id}@${item.version}` ? 'active' : ''} onClick={() => setSelectedKey(`${item.plugin_id}@${item.version}`)}>
          <span>{item.signature_status === 'bundled' ? <Sparkles size={15} /> : <KeyRound size={15} />}</span><div><strong>{item.name}</strong><small>{item.plugin_id} · v{item.version}</small><em>{itemEnabled ? '当前项目已启用' : '当前项目未启用'}</em></div><i className={itemEnabled ? 'enabled' : ''} /><ChevronRight size={13} />
        </button>
      })}{!visible.length && <p className="plugin-no-match">没有符合筛选条件的插件。</p>}</div>
    </aside>

    <main className="plugin-workbench">{selected ? <>
      <header className="plugin-detail-head"><div className="plugin-mark">{selected.signature_status === 'bundled' ? <PackageCheck size={25} /> : <KeyRound size={25} />}</div><div><div className="plugin-title-meta"><span>{selected.trust_class}</span><span>{selected.signature_status}</span><span>{selected.status}</span></div><h2>{selected.name}</h2><code>{selected.plugin_id}@{selected.version}</code></div><div className="plugin-detail-actions"><button className={`plugin-toggle ${enabled ? 'on' : ''}`} disabled={selected.status === 'uninstalled' || selected.status === 'quarantined'} onClick={() => { setEnabled(value => !value); setDirty(true) }}>{enabled ? <ToggleRight size={24} /> : <ToggleLeft size={24} />}<span>{selected.status === 'uninstalled' ? '已退役' : enabled ? '项目已启用' : '项目已禁用'}</span></button><button className="button primary" disabled={!dirty || saving} onClick={saveState}><Save size={14} />{saving ? '保存中' : '保存项目设置'}</button></div></header>
      {notice && <div className="plugin-notice"><ShieldCheck size={15} /><span>{notice}</span></div>}
      <div className="plugin-safety-strip"><LockKeyhole size={17} /><div><strong>插件永远不能取得 Owner 身份</strong><span>最终权限 = Agent 授权 ∩ 项目授权 ∩ 插件声明 ∩ 流程声明；此处只配置当前项目。</span></div><span>{currentProject?.name || projectID}</span></div>
      {conformanceError?<div className="plugin-notice"><XCircle size={15}/><span>Conformance 读取失败：{conformanceError}</span></div>:<PluginConformancePanel report={conformance}/>}

      <section className="plugin-contribution-section"><div className="plugin-section-title"><div><p>CONTRIBUTIONS</p><h3>这个插件给系统增加了什么</h3></div><span>{contributionMeta.reduce((sum, item) => sum + (selected.contributions[item.key]?.length || 0), 0)} 项扩展</span></div>
        <div className="contribution-tabs">{contributionMeta.map(({ key, label, icon: Icon }) => <button key={key} className={activeContribution === key ? 'active' : ''} onClick={() => setActiveContribution(key)}><Icon size={14} /><span>{label}</span><strong>{selected.contributions[key]?.length || 0}</strong></button>)}</div>
        <div className="contribution-list">{activeItems.length ? activeItems.map((item, index) => <article key={`${activeContribution}-${index}`}><span>{String(index + 1).padStart(2, '0')}</span><div><strong>{contributionTitle(item, `${activeContribution}-${index + 1}`)}</strong><code>{contributionSubtitle(item)}</code></div><details><summary>查看定义</summary><pre>{JSON.stringify(item, null, 2)}</pre></details></article>) : <div className="contribution-empty"><Blocks size={23} /><p>此插件没有贡献这一类扩展。</p></div>}</div>
      </section>

      <div className="plugin-config-grid"><section><div className="plugin-section-title"><div><p>PROJECT PERMISSIONS</p><h3>当前项目能力授权</h3></div><span>{state ? '显式配置' : '继承默认'}</span></div>{selected.permissions.length ? <div className="capability-editor">{selected.permissions.map(permission => <label key={permission}><input type="checkbox" checked={capabilities.includes(permission)} disabled={!enabled} onChange={() => toggleCapability(permission)} /><span>{capabilities.includes(permission) ? <Check size={13} /> : null}</span><div><strong>{permission}</strong><small>只能授予插件清单已经声明的能力</small></div></label>)}</div> : <div className="contribution-empty compact"><ShieldCheck size={20} /><p>此插件不请求额外能力。</p></div>}</section>
        <section><div className="plugin-section-title"><div><p>PROJECT CONFIG</p><h3>{conformance?.configuration_schema?'Schema 驱动配置':'插件配置 JSON'}</h3></div><FileJson size={17} /></div><SchemaConfigEditor schema={conformance?.configuration_schema} value={configText} disabled={!enabled} onChange={value=>{setConfigText(value);setDirty(true)}}/><small>配置只作用于 {currentProject?.name || '当前项目'}；保存时仍由服务端 Schema 再验证，不依赖前端。</small></section></div>

      <section className="plugin-impact"><div className="plugin-section-title"><div><p>LIFECYCLE IMPACT</p><h3>停用 / 退役影响</h3></div><span>{impact?.history_preserved ? '历史永久保留' : '正在核验'}</span></div>{impact ? <><div className="plugin-impact-grid"><article><small>当前对象</small><strong>{impact.current_objects}</strong></article><article><small>历史 Revision</small><strong>{impact.historical_revisions}</strong></article><article><small>历史 Run</small><strong>{impact.historical_runs}</strong></article><article><small>Pipeline / Blueprint</small><strong>{impact.pipeline_versions + impact.blueprint_versions}</strong></article><article><small>启用项目</small><strong>{impact.enabled_projects}</strong></article><article><small>活跃方案引用</small><strong>{impact.active_blueprint_refs}</strong></article></div>{impact.blockers.length > 0 && <div className="plugin-impact-blockers"><LockKeyhole size={15}/><span>{impact.blockers.join('；')}</span></div>}{!selected.plugin_id.startsWith('builtin.') && selected.status !== 'uninstalled' && <div className="plugin-retire-action"><div><strong>{impact.can_retire ? '可以安全退役' : '当前不可退役'}</strong><span>退役不会删除历史；预计释放 {Math.round(impact.package_bytes_reclaimed/1024)} KiB 包体。</span></div><button className="button" disabled={!impact.can_retire || saving} onClick={retireSelected}><Trash2 size={13}/>退役插件版本</button></div>}</> : <p className="plugin-impact-loading">{impactError || '正在计算对象、Run、项目与 Blueprint 依赖…'}</p>}</section>

      <footer className="plugin-provenance"><div><span>发布者</span><strong>{selected.publisher}</strong></div><div><span>信任说明</span><strong>{trustExplanation(selected)}</strong></div><div><span>内容哈希</span><code>{selected.content_hash || '未返回'}</code></div></footer>
    </> : <div className="plugin-center-state"><PlugZap size={27} /><h3>选择一个插件</h3><p>这里会显示扩展点、权限、项目配置与来源。</p></div>}</main>
  </div>
}
