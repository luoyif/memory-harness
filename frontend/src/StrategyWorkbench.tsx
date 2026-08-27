import { useEffect, useMemo, useState } from 'react'
import {
  AlertTriangle, ArrowDown, ArrowRight, ArrowUp, BadgeCheck, Blocks, BookOpenCheck,
  BrainCircuit, Check, ChevronRight, CirclePlus, Copy, FileCheck2, Fingerprint, Gauge, GitFork,
  Network, PackageOpen, RefreshCw, Save, Settings2, ShieldCheck, Trash2, XCircle,
} from 'lucide-react'

import {
  APIClient, BlueprintCurrent, BlueprintDefinition, BlueprintNode, BlueprintValidation,
  BlueprintVersion, PluginVersion, Project, StrategyComponent,
} from './api'

type ProjectSummary = { project: Project }
type NodePath = { track: number; node: number }
type CatalogComponent = StrategyComponent & {
  plugin_id: string
  plugin_version: string
  plugin_name: string
  available: boolean
  availability: string
}

const USER_BLUEPRINT_PLUGIN = 'builtin.user-workflows'

function deepClone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function parseVersion(value: string) {
  const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(value)
  return match ? match.slice(1).map(Number) : [0, 0, 0]
}

export function nextBlueprintVersion(blueprintID: string, versions: BlueprintVersion[]) {
  const matches = versions.filter(item => item.blueprint_id === blueprintID).map(item => item.version)
  if (!matches.length) return '1.0.0'
  matches.sort((left, right) => {
    const a = parseVersion(left); const b = parseVersion(right)
    return b[0] - a[0] || b[1] - a[1] || b[2] - a[2]
  })
  const [major, minor, patch] = parseVersion(matches[0])
  return `${major}.${minor}.${patch + 1}`
}

function safeSlug(project: Project) {
  const value = (project.slug || project.project_id || 'personal').toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^-+|-+$/g, '')
  return value || 'personal'
}

export function cloneBlueprint(source: BlueprintDefinition, project: Project, versions: BlueprintVersion[]) {
  const result = deepClone(source)
  const owned = result.blueprint_id.startsWith(`${USER_BLUEPRINT_PLUGIN}.`)
  result.blueprint_id = owned ? result.blueprint_id : `${USER_BLUEPRINT_PLUGIN}.${safeSlug(project)}-memory`
  result.version = nextBlueprintVersion(result.blueprint_id, versions)
  result.name = owned ? result.name : `${project.name} · 可编程记忆`
  result.description = owned ? result.description : `从“${source.name}”克隆，只作用于 ${project.name}。`
  return result
}

export function compatibleComponents(node: BlueprintNode, catalog: CatalogComponent[]) {
  return catalog.filter(item => item.role === node.role && item.kind === node.binding_kind)
}

function componentKey(item: CatalogComponent) {
  return `${item.plugin_id}@${item.plugin_version}::${item.component_id}@${item.version}`
}

function nodeKey(node: BlueprintNode) {
  return `${node.plugin_id}@${node.plugin_version}::${node.component_id}@${node.component_version}`
}

function trackIcon(role: string) {
  if (role === 'growth') return BrainCircuit
  if (role === 'organization') return Network
  if (role === 'context') return Fingerprint
  return Gauge
}

function isPluginAvailable(plugin: PluginVersion, projectID: string) {
  const explicit = plugin.project_states.find(item => item.project_id === projectID)
  if (explicit) return explicit.status === 'enabled'
  return plugin.plugin_id.startsWith('builtin.') && plugin.status === 'enabled'
}

function catalogFromPlugins(plugins: PluginVersion[], projectID: string): CatalogComponent[] {
  return plugins.flatMap(plugin => (plugin.contributions.strategy_components || []).map(component => ({
    ...component,
    plugin_id: plugin.plugin_id,
    plugin_version: plugin.version,
    plugin_name: plugin.name,
    available: isPluginAvailable(plugin, projectID),
    availability: isPluginAvailable(plugin, projectID) ? '当前项目可用' : plugin.status === 'experimental' ? '实验插件，需显式授权' : '需在插件中心启用',
  })))
}

function changedCount(source: BlueprintDefinition | undefined, draft: BlueprintDefinition | undefined) {
  if (!source || !draft) return 0
  let changed = source.policy.evidence_mode !== draft.policy.evidence_mode ||
    source.policy.model_boundary !== draft.policy.model_boundary ||
    source.policy.default_context_budget !== draft.policy.default_context_budget ||
    source.policy.cross_project_recall !== draft.policy.cross_project_recall ? 1 : 0
  const sourceNodes = new Map(source.tracks.flatMap(track => track.nodes.map(node => [node.node_id, JSON.stringify(node)])))
  for (const node of draft.tracks.flatMap(track => track.nodes)) {
    if (sourceNodes.get(node.node_id) !== JSON.stringify(node)) changed += 1
    sourceNodes.delete(node.node_id)
  }
  return changed + sourceNodes.size
}

export function StrategyWorkbench({ api, projectID, projects }: { api: APIClient; projectID: string; projects: ProjectSummary[] }) {
  const [versions, setVersions] = useState<BlueprintVersion[]>([])
  const [current, setCurrent] = useState<BlueprintCurrent>()
  const [plugins, setPlugins] = useState<PluginVersion[]>([])
  const [selectedKey, setSelectedKey] = useState('')
  const [draft, setDraft] = useState<BlueprintDefinition>()
  const [draftSource, setDraftSource] = useState<BlueprintDefinition>()
  const [selectedPath, setSelectedPath] = useState<NodePath>({ track: 0, node: 0 })
  const [configText, setConfigText] = useState('{}')
  const [validation, setValidation] = useState<BlueprintValidation>()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState('')
  const [fatal, setFatal] = useState('')

  async function reload(preferredKey = '') {
    if (!projectID) return
    setLoading(true); setFatal('')
    try {
      const [listed, active, installed] = await Promise.all([
        api.get<{ blueprints: BlueprintVersion[] }>('/v1/blueprints'),
        api.get<BlueprintCurrent>(`/v1/projects/${encodeURIComponent(projectID)}/blueprint`),
        api.get<{ plugins: PluginVersion[] }>('/v1/plugins'),
      ])
      setVersions(listed.blueprints); setCurrent(active); setPlugins(installed.plugins)
      const activeKey = `${active.blueprint.blueprint_id}@${active.blueprint.version}`
      setSelectedKey(listed.blueprints.some(item => `${item.blueprint_id}@${item.version}` === preferredKey) ? preferredKey : activeKey)
    } catch (reason) { setFatal(reason instanceof Error ? reason.message : String(reason)) }
    finally { setLoading(false) }
  }

  useEffect(() => { setDraft(undefined); setValidation(undefined); setNotice(''); void reload('') }, [api, projectID]) // eslint-disable-line react-hooks/exhaustive-deps

  const selectedVersion = versions.find(item => `${item.blueprint_id}@${item.version}` === selectedKey) || current?.blueprint
  const definition = draft || selectedVersion?.definition
  const catalog = useMemo(() => catalogFromPlugins(plugins, projectID), [plugins, projectID])
  const selectedNode = definition?.tracks[selectedPath.track]?.nodes[selectedPath.node]
  const options = selectedNode ? compatibleComponents(selectedNode, catalog) : []
  const selectedComponent = selectedNode ? catalog.find(item => componentKey(item) === nodeKey(selectedNode)) : undefined
  const currentKey = current ? `${current.blueprint.blueprint_id}@${current.blueprint.version}` : ''
  const currentProject = projects.find(item => item.project.project_id === projectID)?.project

  useEffect(() => { setConfigText(JSON.stringify(selectedNode?.config || {}, null, 2)) }, [selectedNode])

  function updateDefinition(mutator: (value: BlueprintDefinition) => void) {
    setDraft(value => {
      if (!value) return value
      const next = deepClone(value); mutator(next); return next
    })
    setValidation(undefined); setNotice('')
  }

  function beginClone() {
    if (!selectedVersion || !currentProject) return
    const next = cloneBlueprint(selectedVersion.definition, currentProject, versions)
    setDraft(next); setDraftSource(deepClone(selectedVersion.definition)); setSelectedPath({ track: 0, node: 0 }); setValidation(undefined); setNotice('已创建项目草稿。发布前可自由替换、增加、删除和排序组件。')
  }

  function replaceComponent(value: string) {
    const component = catalog.find(item => componentKey(item) === value)
    if (!component) return
    updateDefinition(next => {
      const node = next.tracks[selectedPath.track].nodes[selectedPath.node]
      node.plugin_id = component.plugin_id; node.plugin_version = component.plugin_version
      node.component_id = component.component_id; node.component_version = component.version
      node.display_name = component.display_name; node.required_capabilities = [...component.capabilities]
      if (component.configuration) {
        try { node.config = JSON.parse(component.configuration) as Record<string, unknown> } catch { node.config = {} }
      }
    })
  }

  function applyConfig() {
    try {
      const parsed = JSON.parse(configText) as Record<string, unknown>
      updateDefinition(next => { next.tracks[selectedPath.track].nodes[selectedPath.node].config = parsed })
      setNotice('节点参数已写入草稿；尚未发布。')
    } catch (reason) { setNotice(`参数不是合法 JSON：${reason instanceof Error ? reason.message : String(reason)}`) }
  }

  function addNode(trackIndex: number, value: string) {
    const component = catalog.find(item => componentKey(item) === value)
    if (!component || !draft) return
    const next = deepClone(draft)
    const existing = new Set(next.tracks.flatMap(track => track.nodes.map(node => node.node_id)))
    const base = component.component_id.split('.').at(-1)?.replace(/[^a-z0-9-]/g, '-') || 'component'
    let nodeID = base; let suffix = 2
    while (existing.has(nodeID)) nodeID = `${base}-${suffix++}`
    let config: Record<string, unknown> = {}
    if (component.configuration) { try { config = JSON.parse(component.configuration) as Record<string, unknown> } catch { config = {} } }
    next.tracks[trackIndex].nodes.push({ node_id: nodeID, role: component.role, display_name: component.display_name, binding_kind: component.kind, plugin_id: component.plugin_id, plugin_version: component.plugin_version, component_id: component.component_id, component_version: component.version, enabled: true, required_capabilities: [...component.capabilities], config })
    setDraft(next); setValidation(undefined); setNotice('')
    setSelectedPath({ track: trackIndex, node: next.tracks[trackIndex].nodes.length - 1 })
  }

  function addContextTrack() {
    if (!draft || draft.tracks.some(track => track.role === 'context')) return
    const order = ['context.profile-compiler', 'context.retrieval-policy', 'context.presentation-policy', 'context.budget-policy']
    const components = catalog.filter(item => item.role.startsWith('context.') && item.available)
      .sort((left, right) => order.indexOf(left.role) - order.indexOf(right.role))
    if (!components.length) { setNotice('当前没有可用的 Context Policy 组件；请先检查插件状态。'); return }
    const next = deepClone(draft)
    next.tracks.push({
      track_id: 'context', role: 'context', display_name: '上下文编译',
      description: '按任务决定哪些受治理画像与召回结果进入 Context Plan；不会扩大 Agent 权限。',
      nodes: components.map((component, index) => {
        let config: Record<string, unknown> = {}
        if (component.configuration) { try { config = JSON.parse(component.configuration) as Record<string, unknown> } catch { config = {} } }
        const suffix = component.role.replace(/^context\./, '').replace(/-policy$/, '').replace('-compiler', '')
        return { node_id: suffix || `context-${index + 1}`, role: component.role, display_name: component.display_name, binding_kind: component.kind, plugin_id: component.plugin_id, plugin_version: component.plugin_version, component_id: component.component_id, component_version: component.version, enabled: true, required_capabilities: [...component.capabilities], config }
      }),
    })
    setDraft(next); setValidation(undefined); setNotice('已加入可选 Context Track；旧三轨 Blueprint 仍保持兼容。')
    setSelectedPath({ track: next.tracks.length - 1, node: 0 })
  }

  function moveNode(direction: -1 | 1) {
    if (!draft) return
    const next = deepClone(draft); const nodes = next.tracks[selectedPath.track].nodes
    const target = selectedPath.node + direction
    if (target < 0 || target >= nodes.length) return
    ;[nodes[selectedPath.node], nodes[target]] = [nodes[target], nodes[selectedPath.node]]
    setDraft(next); setValidation(undefined); setNotice('')
    setSelectedPath({ track: selectedPath.track, node: target })
  }

  function removeNode() {
    updateDefinition(next => { next.tracks[selectedPath.track].nodes.splice(selectedPath.node, 1) })
    setSelectedPath({ track: selectedPath.track, node: Math.max(0, selectedPath.node - 1) })
  }

  async function validate() {
    if (!draft) return
    setSaving(true); setNotice('')
    try {
      const result = await api.post<BlueprintValidation>('/v1/blueprints/validate', draft)
      setValidation(result); setNotice(result.valid ? '结构校验通过。发布时还会检查当前项目的插件授权。' : result.errors.join('；'))
    } catch (reason) { setValidation(undefined); setNotice(reason instanceof Error ? reason.message : String(reason)) }
    finally { setSaving(false) }
  }

  async function publishAndActivate() {
    if (!draft) return
    setSaving(true); setNotice('')
    try {
      const published = await api.post<BlueprintVersion>('/v1/blueprints', { plugin_id: USER_BLUEPRINT_PLUGIN, definition: draft })
      await api.put<BlueprintCurrent>(`/v1/projects/${encodeURIComponent(projectID)}/blueprint`, { blueprint_id: published.blueprint_id, version: published.version })
      setDraft(undefined); setDraftSource(undefined); setValidation(undefined)
      setNotice(`已发布并启用 ${published.name} v${published.version}；本次运行将固定记录内容哈希。`)
      await reload(`${published.blueprint_id}@${published.version}`)
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : String(reason)) }
    finally { setSaving(false) }
  }

  async function activateSelected() {
    if (!selectedVersion) return
    setSaving(true); setNotice('')
    try {
      await api.put<BlueprintCurrent>(`/v1/projects/${encodeURIComponent(projectID)}/blueprint`, { blueprint_id: selectedVersion.blueprint_id, version: selectedVersion.version })
      setNotice(`已切换为 ${selectedVersion.name} v${selectedVersion.version}。`); await reload(selectedKey)
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : String(reason)) }
    finally { setSaving(false) }
  }

  if (loading && !current) return <div className="strategy-state"><RefreshCw className="spin" size={22} /><p>正在装配项目记忆方案…</p></div>
  if (fatal) return <div className="strategy-state danger"><XCircle size={26} /><h3>策略工坊读取失败</h3><p>{fatal}</p><button onClick={() => void reload()}>重试</button></div>
  if (!definition) return <div className="strategy-state"><PackageOpen size={25} /><p>尚未发布任何 Memory Blueprint。</p></div>

  return <div className="strategy-workbench">
    <div className="strategy-toolbar">
      <div><span className={draft ? 'draft' : 'published'}>{draft ? 'DRAFT / 项目草稿' : 'PUBLISHED / 不可变版本'}</span><strong>{draft ? draft.name : selectedVersion?.name}</strong><code>{draft ? `${draft.blueprint_id}@${draft.version}` : `${selectedVersion?.blueprint_id}@${selectedVersion?.version}`}</code></div>
      <div className="strategy-toolbar-actions">{draft ? <><button className="button" onClick={() => { setDraft(undefined); setValidation(undefined); setNotice('已放弃未发布草稿。') }}>放弃草稿</button><button className="button" disabled={saving} onClick={validate}><FileCheck2 size={14} />验证</button><button className="button primary" disabled={saving} onClick={publishAndActivate}><Save size={14} />发布并启用</button></> : <><button className="button" onClick={beginClone}><Copy size={14} />克隆为项目方案</button>{selectedKey !== currentKey && <button className="button primary" disabled={saving} onClick={activateSelected}><BadgeCheck size={14} />启用此版本</button>}</>}</div>
    </div>
    {notice && <div className={`strategy-notice ${validation?.valid ? 'success' : ''}`}>{validation?.valid ? <Check size={15} /> : <ShieldCheck size={15} />}<span>{notice}</span></div>}

    <div className="strategy-grid">
      <aside className="blueprint-catalog">
        <header><p>BLUEPRINT LIBRARY</p><h2>项目记忆方案</h2><span>发布后不可覆盖，可随时切换或克隆新版本。</span></header>
        <div className="blueprint-list">{versions.map(item => {
          const key = `${item.blueprint_id}@${item.version}`; const active = key === currentKey
          return <button key={key} className={selectedKey === key && !draft ? 'selected' : ''} disabled={Boolean(draft)} onClick={() => { setSelectedKey(key); setSelectedPath({ track: 0, node: 0 }); setValidation(undefined); setNotice('') }}><span><BookOpenCheck size={16} /></span><div><strong>{item.name}</strong><code>v{item.version} · {item.plugin_id}</code><small>{item.description}</small></div>{active ? <em>当前</em> : <ChevronRight size={14} />}</button>
        })}</div>
        <footer><GitFork size={16} /><p><strong>插件只贡献组件</strong>项目 Blueprint 决定选谁、顺序、参数与召回预算。</p></footer>
      </aside>

      <main className="blueprint-canvas">
        <header className="blueprint-summary"><div><p>MEMORY BLUEPRINT</p>{draft ? <input value={draft.name} onChange={event => updateDefinition(next => { next.name = event.target.value })} aria-label="方案名称" /> : <h2>{definition.name}</h2>}<span>{definition.intent}</span></div><dl><div><dt>轨道</dt><dd>{definition.tracks.length}</dd></div><div><dt>组件</dt><dd>{definition.tracks.reduce((sum, track) => sum + track.nodes.length, 0)}</dd></div><div><dt>变更</dt><dd>{changedCount(draftSource, draft)}</dd></div></dl></header>

        {draft && !definition.tracks.some(track => track.role === 'context') && <button className="context-track-enable" onClick={addContextTrack}><Fingerprint size={16}/><span><strong>启用可选 Context Track</strong><small>把 Profile Compiler、召回、呈现与预算策略接入 Context Plan；不会改变旧 Blueprint 的三轨兼容性。</small></span><CirclePlus size={16}/></button>}
        <div className="strategy-tracks">{definition.tracks.map((track, trackIndex) => { const Icon = trackIcon(track.role); const additions = catalog.filter(item => item.role.startsWith(`${track.role}.`))
          return <section key={track.track_id} className={`strategy-track ${track.role}`}><header><span><Icon size={17} /></span><div><strong>{track.display_name}</strong><small>{track.description}</small></div>{draft && <label><CirclePlus size={14} /><select aria-label={`向${track.display_name}添加组件`} defaultValue="" onChange={event => { addNode(trackIndex, event.target.value); event.target.value = '' }}><option value="" disabled>增加一层…</option>{additions.map(item => <option key={componentKey(item)} value={componentKey(item)}>{item.display_name}{item.available ? '' : '（需授权）'}</option>)}</select></label>}</header><div className="strategy-node-line">{track.nodes.map((node, nodeIndex) => <div className="strategy-node-wrap" key={node.node_id}><button className={`${selectedPath.track === trackIndex && selectedPath.node === nodeIndex ? 'selected' : ''} ${node.enabled ? '' : 'disabled'}`} onClick={() => setSelectedPath({ track: trackIndex, node: nodeIndex })}><span>{String(nodeIndex + 1).padStart(2, '0')}</span><div><small>{node.role}</small><strong>{node.display_name}</strong><code>{node.component_id}</code></div>{node.enabled ? <Check size={14} /> : <AlertTriangle size={14} />}</button>{nodeIndex < track.nodes.length - 1 && <ArrowRight size={16} />}</div>)}</div></section>
        })}</div>

        <section className="strategy-policy"><header><Settings2 size={16} /><div><strong>全局安全与成本边界</strong><small>这些限制属于 Blueprint，而不是某个模型或插件。</small></div></header><div className="strategy-policy-grid"><label>证据保真<select disabled={!draft} value={definition.policy.evidence_mode} onChange={event => updateDefinition(next => { next.policy.evidence_mode = event.target.value })}><option value="normalized_with_verbatim">标准化 + 保留原文</option><option value="verbatim">仅保存原文</option></select></label><label>模型边界<select disabled={!draft} value={definition.policy.model_boundary} onChange={event => updateDefinition(next => { next.policy.model_boundary = event.target.value })}><option value="rules_only">仅本地规则</option><option value="local_only">仅本地模型</option><option value="configured_provider">使用配置中心模型</option></select></label><label>默认上下文预算<input disabled={!draft} type="number" min={1000} max={200000} value={definition.policy.default_context_budget} onChange={event => updateDefinition(next => { next.policy.default_context_budget = Number(event.target.value) })} /></label><label className="policy-check"><input disabled={!draft} type="checkbox" checked={definition.policy.cross_project_recall} onChange={event => updateDefinition(next => { next.policy.cross_project_recall = event.target.checked })} /><span>允许跨项目召回</span></label></div></section>
      </main>

      <aside className="strategy-inspector">{selectedNode ? <>
        <header><p>COMPONENT INSPECTOR</p><h2>{selectedNode.display_name}</h2><code>{selectedNode.role}</code></header>
        <div className="inspector-status"><span className={selectedComponent?.available ? 'available' : 'unavailable'} /><div><strong>{selectedComponent?.plugin_name || selectedNode.plugin_id}</strong><small>{selectedComponent?.availability || '组件定义未在插件目录找到'}</small></div></div>
        <label>策略实现<select disabled={!draft} value={nodeKey(selectedNode)} onChange={event => replaceComponent(event.target.value)}>{options.map(item => <option key={componentKey(item)} value={componentKey(item)}>{item.display_name} · {item.plugin_name}{item.available ? '' : '（需授权）'}</option>)}</select></label>
        <div className="inspector-provenance"><span>插件</span><code>{selectedNode.plugin_id}@{selectedNode.plugin_version}</code><span>组件</span><code>{selectedNode.component_id}@{selectedNode.component_version}</code></div>
        <label className="inspector-enable"><input disabled={!draft} type="checkbox" checked={selectedNode.enabled} onChange={event => updateDefinition(next => { next.tracks[selectedPath.track].nodes[selectedPath.node].enabled = event.target.checked })} /><span>{selectedNode.enabled ? '此组件参与运行' : '此组件已停用'}</span></label>
        <label>组件参数 JSON<textarea disabled={!draft} value={configText} onChange={event => setConfigText(event.target.value)} rows={11} spellCheck={false} /></label>{draft && <button className="button full" onClick={applyConfig}><Settings2 size={14} />应用节点参数</button>}
        <div className="inspector-capabilities"><span>所需能力</span>{selectedNode.required_capabilities.length ? selectedNode.required_capabilities.map(value => <code key={value}>{value}</code>) : <small>纯本地，无额外能力</small>}</div>
        {draft && <div className="inspector-order"><button disabled={selectedPath.node === 0} onClick={() => moveNode(-1)}><ArrowUp size={14} />上移</button><button disabled={selectedPath.node === definition.tracks[selectedPath.track].nodes.length - 1} onClick={() => moveNode(1)}><ArrowDown size={14} />下移</button><button className="danger" onClick={removeNode}><Trash2 size={14} />删除</button></div>}
      </> : <div className="strategy-state compact"><Blocks size={24} /><h3>选择一个策略组件</h3><p>这里会显示实现、插件、参数和权限。</p></div>}</aside>
    </div>
  </div>
}
