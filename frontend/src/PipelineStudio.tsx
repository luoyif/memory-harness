import { ChangeEvent, useCallback, useEffect, useRef, useState } from 'react'
import {
  addEdge, Background, Connection, Controls, Edge, Handle, MarkerType, MiniMap,
  Node, NodeProps, Panel, Position, ReactFlow, ReactFlowInstance, useEdgesState, useNodesState,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  AlertTriangle, Braces, CheckCircle2, ChevronRight, CirclePlus, Copy, Download,
  GitBranch, Grip, Play, Plus, Save, ShieldCheck, Trash2, Upload, WandSparkles,
} from 'lucide-react'

import { APIClient, PipelineDefinition, PipelineDraft, PipelineVersion } from './api'

type StageDescriptor = { stage_type: string; version: string; class: string; capabilities: string[]; description: string }
type PipelineNode = PipelineDefinition['nodes'][number]
type StageData = { node: PipelineNode; descriptor?: StageDescriptor; label: string }
type StageGraphNode = Node<StageData, 'stage'>
type Validation = { valid: boolean; execution_order: string[]; required_capabilities: string[]; model_calls: number; node_count: number }
type DryRunResult = { project_id:string; pipeline_id:string; pipeline_version:string; pipeline_hash:string; pure_previewed:number; planned_writes:number; planned_model_calls:number; review_gates:number; no_writes_performed:boolean; warnings:string[]; nodes:Array<{node_id:string;stage_type:string;class:string;status:string;would_write:boolean;would_invoke_model:boolean;review_gate:boolean;detail:string;preview?:unknown}> }
type Catalog = { pipelines: PipelineVersion[]; drafts: PipelineDraft[]; stages: StageDescriptor[] }

const classLabel: Record<string, string> = { deterministic: '本地', human: '审核', model: '模型', memory: '记忆', project: '项目' }

function StageCard({ data, selected }: NodeProps<StageGraphNode>) {
  const tone = data.descriptor?.class || 'deterministic'
  return <div className={`flow-node-card ${tone} ${selected ? 'selected' : ''}`}>
    <Handle type="target" position={Position.Left} />
    <div><span>{classLabel[tone] || tone}</span><Grip size={13} /></div>
    <strong>{data.node.id}</strong>
    <small>{data.node.stage_type}</small>
    <Handle type="source" position={Position.Right} />
  </div>
}

const nodeTypes = { stage: StageCard }

function defaultConfig(stageType: string): Record<string, unknown> {
  if (stageType === 'transform.map') return { merge: {} }
  if (stageType === 'transform.filter') return { field: 'status', equals: 'active' }
  if (stageType === 'policy.require_review') return { reason: 'owner_review_required' }
  if (stageType === 'llm.extract') return { prompt: 'Extract only durable, evidence-supported information.', output_schema: { type: 'object', additionalProperties: true }, max_tokens: 1200 }
  if (stageType === 'object.materialize') return { type_id: 'builtin.core-memory-growth.semantic', status: 'candidate', plugin_id: 'builtin.core-memory-growth', plugin_version: '2.0.0', confidence: 0.8, importance: 0.6 }
  if (stageType === 'memory.compile') return { extraction_policy: 'structured-attribution-v2', unresolved_subject: 'review' }
  if (stageType === 'project.derive') return { projections: ['context', 'goals', 'decisions', 'risks', 'opportunities'] }
  if (stageType === 'memory.semantic_graph') return { resolved_subjects_only: true, inverse_relations: true, temporal_facts: true }
  if (stageType === 'memory.materialize') return { layers: ['knowledge', 'episode', 'memory', 'living_asset', 'agent_asset'] }
  if (stageType === 'search.refresh') return { scope: 'current_project' }
  return {}
}

export function nextPatchVersion(value: string) {
  const match = /^(\d+)\.(\d+)\.(\d+)/.exec(value)
  return match ? `${match[1]}.${match[2]}.${Number(match[3]) + 1}` : '1.0.0'
}

function cloneDefinition(value: PipelineDefinition): PipelineDefinition {
  return JSON.parse(JSON.stringify(value)) as PipelineDefinition
}

function blankDefinition(): PipelineDefinition {
  const suffix = new Date().toISOString().replace(/[-:TZ.]/g, '').slice(0, 12)
  return {
    api_version: 'memory-harness.pipeline/v1alpha1', pipeline_id: `builtin.user-workflows.flow-${suffix}`,
    version: '1.0.0', name: '未命名记忆流程', intent: '说明这条流程把什么输入变成什么可检查结果。',
    required_capabilities: [],
    nodes: [{ id: 'input', stage_type: 'trigger.manual', stage_version: '1.0.0', plugin_id: 'builtin.memory-harness-core', depends_on: [], config: {} }],
    outputs: [{ name: 'result', node_id: 'input' }], policy: { max_stages: 8, timeout_seconds: 120, max_model_calls: 0 },
    editor: { positions: { input: { x: 220, y: 220 } } },
  }
}

export function graphFromDefinition(definition: PipelineDefinition, stages: StageDescriptor[]) {
  const descriptors = new Map(stages.map(item => [item.stage_type, item]))
  const positions = definition.editor?.positions || {}
  const nodes: StageGraphNode[] = definition.nodes.map((node, index) => ({
    id: node.id, type: 'stage', position: positions[node.id] || { x: 130 + index * 245, y: 120 + (index % 2) * 170 },
    data: { node: { ...node, depends_on: [...node.depends_on], config: structuredClone(node.config || {}) }, descriptor: descriptors.get(node.stage_type), label: node.id },
  }))
  const edges: Edge[] = definition.nodes.flatMap(node => node.depends_on.map(source => ({
    id: `${source}->${node.id}`, source, target: node.id, markerEnd: { type: MarkerType.ArrowClosed }, animated: false,
  })))
  return { nodes, edges }
}

export function isCycle(connection: Connection | Edge, edges: Edge[]) {
  if (!connection.source || !connection.target || connection.source === connection.target) return true
  const adjacency = new Map<string, string[]>()
  for (const edge of edges) adjacency.set(edge.source, [...(adjacency.get(edge.source) || []), edge.target])
  const pending = [connection.target]
  const seen = new Set<string>()
  while (pending.length) {
    const current = pending.pop()!
    if (current === connection.source) return true
    if (seen.has(current)) continue
    seen.add(current)
    pending.push(...(adjacency.get(current) || []))
  }
  return false
}

function Badge({ value }: { value: string }) {
  return <span className={`studio-badge ${['valid', 'published', 'saved'].includes(value) ? 'good' : value === 'invalid' ? 'bad' : ''}`}>{value}</span>
}

export function PipelineStudio({ api, projectID, openRun }: { api: APIClient; projectID: string; openRun: (id: string) => void }) {
  const [catalog, setCatalog] = useState<Catalog>()
  const [loading, setLoading] = useState(true)
  const [fatal, setFatal] = useState('')
  const [definition, setDefinition] = useState<PipelineDefinition>()
  const [pluginID, setPluginID] = useState('builtin.user-workflows')
  const [draftRevision, setDraftRevision] = useState(0)
  const [baseVersion, setBaseVersion] = useState('')
  const [mode, setMode] = useState<'published' | 'draft'>('published')
  const [selectedNodeID, setSelectedNodeID] = useState('')
  const [nodes, setNodes, onNodesChange] = useNodesState<StageGraphNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
  const [notice, setNotice] = useState('')
  const [validation, setValidation] = useState<Validation>()
  const [dryRunResult, setDryRunResult] = useState<DryRunResult>()
  const [dirty, setDirty] = useState(false)
  const [input, setInput] = useState('{\n  "statement": "一条等待流程处理的真实输入"\n}')
  const [paletteOpen, setPaletteOpen] = useState(true)
  const [configText, setConfigText] = useState('{}')
  const [nodeIDText, setNodeIDText] = useState('')
  const reactFlow = useRef<ReactFlowInstance<StageGraphNode, Edge>>()
  const graphLoaded = useRef(false)

  const reload = useCallback(async () => {
    setLoading(true); setFatal('')
    try {
      const [pipelines, drafts, stages] = await Promise.all([
        api.get<{ pipelines: PipelineVersion[] }>('/v1/pipelines'),
        api.get<{ drafts: PipelineDraft[] }>('/v1/pipelines/drafts'),
        api.get<{ stages: StageDescriptor[] }>('/v1/pipelines/stages'),
      ])
      const next = { pipelines: pipelines.pipelines, drafts: drafts.drafts, stages: stages.stages }
      setCatalog(next)
      if (!graphLoaded.current) {
        if (next.drafts[0]) openDraft(next.drafts[0], next.stages)
        else if (next.pipelines[0]) openPublished(next.pipelines[0], next.stages)
      }
    } catch (reason) { setFatal(reason instanceof Error ? reason.message : String(reason)) }
    finally { setLoading(false) }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api])

  useEffect(() => { void reload() }, [reload])

  function loadGraph(next: PipelineDefinition, stages = catalog?.stages || []) {
    graphLoaded.current = true
    const graph = graphFromDefinition(next, stages)
    setDefinition(cloneDefinition(next)); setNodes(graph.nodes); setEdges(graph.edges); setSelectedNodeID(''); setValidation(undefined); setDryRunResult(undefined); setDirty(false)
    requestAnimationFrame(() => reactFlow.current?.fitView({ padding: 0.22, duration: 280 }))
  }

  function openPublished(item: PipelineVersion, stages = catalog?.stages || []) {
    if (item.definition.nodes.some(node => node.stage_type === 'memory.compile')) {
      setInput(`{\n  "project_id": "${projectID}",\n  "session_id": "",\n  "evidence_ids": [],\n  "force": false\n}`)
    }
    setMode('published'); setPluginID(item.plugin_id); setDraftRevision(0); setBaseVersion(item.version); setNotice('已发布版本为只读。点击“编辑为新版本”开始 DIY。'); loadGraph(item.definition, stages)
  }

  function openDraft(item: PipelineDraft, stages = catalog?.stages || []) {
    setMode('draft'); setPluginID(item.plugin_id); setDraftRevision(item.revision); setBaseVersion(item.base_version || ''); setNotice('草稿可以自由修改；发布前会执行完整 DAG 与权限校验。'); loadGraph(item.definition, stages)
  }

  function startBlank() {
    setMode('draft'); setPluginID('builtin.user-workflows'); setDraftRevision(0); setBaseVersion(''); setNotice('新流程尚未保存。'); loadGraph(blankDefinition())
  }

  function editPublished() {
    if (!definition) return
    const next = cloneDefinition(definition)
    next.version = nextPatchVersion(next.version)
    next.name = `${next.name} · 自定义`
    setMode('draft'); setDraftRevision(0); setBaseVersion(definition.version); setNotice(`已克隆为 v${next.version} 草稿；原版本保持不变。`); loadGraph(next)
  }

  function markChanged() { setDirty(true); setValidation(undefined); setDryRunResult(undefined); setNotice('有尚未保存的修改。') }

  function updateDefinition(patch: Partial<PipelineDefinition>) {
    setDefinition(current => current ? { ...current, ...patch } : current); markChanged()
  }

  function addStage(stage: StageDescriptor, droppedAt?: { x: number; y: number }) {
    if (!definition || mode !== 'draft') return
    const base = stage.stage_type.split('.').pop()!.replace(/[^a-z0-9]+/g, '-')
    let id = base
    let index = 2
    while (nodes.some(node => node.id === id)) id = `${base}-${index++}`
    const node: PipelineNode = { id, stage_type: stage.stage_type, stage_version: stage.version, plugin_id: 'builtin.memory-harness-core', depends_on: [], config: defaultConfig(stage.stage_type) }
    const position = droppedAt || reactFlow.current?.screenToFlowPosition({ x: window.innerWidth * .53, y: window.innerHeight * .5 }) || { x: 320, y: 220 }
    setNodes(current => [...current, { id, type: 'stage', position, data: { node, descriptor: stage, label: id } }])
    setSelectedNodeID(id); markChanged()
  }

  const onConnect = useCallback((connection: Connection) => {
    if (mode !== 'draft' || isCycle(connection, edges) || edges.some(edge => edge.source === connection.source && edge.target === connection.target)) return
    setEdges(current => addEdge({ ...connection, id: `${connection.source}->${connection.target}`, markerEnd: { type: MarkerType.ArrowClosed } }, current)); markChanged()
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, edges])

  function updateSelectedNode(patch: Partial<PipelineNode>) {
    setNodes(current => current.map(item => {
      if (item.id !== selectedNodeID) return item
      const node = { ...item.data.node, ...patch }
      const descriptor = catalog?.stages.find(stage => stage.stage_type === node.stage_type)
      return { ...item, data: { node, descriptor, label: node.id } }
    })); markChanged()
  }

  function renameSelected(nextID: string) {
    const value = nextID.trim().toLowerCase()
    if (!/^[a-z0-9][a-z0-9._-]{1,159}$/.test(value) || nodes.some(node => node.id === value && node.id !== selectedNodeID)) return
    const old = selectedNodeID
    setNodes(current => current.map(item => item.id === old ? { ...item, id: value, data: { ...item.data, node: { ...item.data.node, id: value }, label: value } } : item))
    setEdges(current => current.map(edge => ({ ...edge, id: `${edge.source === old ? value : edge.source}->${edge.target === old ? value : edge.target}`, source: edge.source === old ? value : edge.source, target: edge.target === old ? value : edge.target })))
    setSelectedNodeID(value); markChanged()
  }

  function deleteSelected() {
    if (!selectedNodeID || mode !== 'draft') return
    setNodes(current => current.filter(node => node.id !== selectedNodeID))
    setEdges(current => current.filter(edge => edge.source !== selectedNodeID && edge.target !== selectedNodeID))
    setSelectedNodeID(''); markChanged()
  }

  function currentDefinition(): PipelineDefinition | undefined {
    if (!definition) return undefined
    const positionMap = Object.fromEntries(nodes.map(node => [node.id, { x: Math.round(node.position.x), y: Math.round(node.position.y) }]))
    const graphNodes = nodes.map(item => ({ ...item.data.node, depends_on: edges.filter(edge => edge.target === item.id).map(edge => edge.source).sort() }))
    const outgoing = new Set(edges.map(edge => edge.source))
    const terminals = graphNodes.filter(node => !outgoing.has(node.id))
    const outputNode = definition.outputs[0]?.node_id
    const chosenOutput = graphNodes.some(node => node.id === outputNode) ? outputNode : terminals[0]?.id || graphNodes[0]?.id || ''
    const required = [...new Set(graphNodes.flatMap(node => catalog?.stages.find(stage => stage.stage_type === node.stage_type)?.capabilities || []))].sort()
    return { ...definition, required_capabilities: required, nodes: graphNodes, outputs: chosenOutput ? [{ name: definition.outputs[0]?.name || 'result', node_id: chosenOutput }] : [], editor: { positions: positionMap } }
  }

  async function validate() {
    const current = currentDefinition()
    if (!current) return
    try {
      const result = await api.post<Validation>('/v1/pipelines/validate', { plugin_id: pluginID, definition: current })
      setValidation(result); setNotice(`校验通过：执行顺序 ${result.execution_order.join(' → ')}`)
    } catch (reason) { setValidation({ valid: false, execution_order: [], required_capabilities: [], model_calls: 0, node_count: nodes.length }); setNotice(reason instanceof Error ? reason.message : String(reason)) }
  }

  async function saveDraft() {
    const current = currentDefinition()
    if (!current) return
    try {
      const saved = await api.put<PipelineDraft>(`/v1/pipelines/drafts/${encodeURIComponent(current.pipeline_id)}`, { plugin_id: pluginID, base_version: baseVersion, expected_revision: draftRevision, definition: current })
      setDefinition(saved.definition); setDraftRevision(saved.revision); setDirty(false); setNotice(`草稿修订 r${saved.revision} 已保存。`); await reload()
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : String(reason)) }
  }

  async function publish() {
    const current = currentDefinition()
    if (!current) return
    try {
      await api.post('/v1/pipelines/validate', { plugin_id: pluginID, definition: current })
      const item = await api.post<PipelineVersion>(`/v1/pipelines?plugin_id=${encodeURIComponent(pluginID)}`, current)
      if (draftRevision) await api.delete(`/v1/pipelines/drafts/${encodeURIComponent(current.pipeline_id)}`)
      setNotice(`v${item.version} 已发布为不可变版本。`); await reload(); openPublished(item)
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : String(reason)) }
  }

  async function dryRun() {
    if (!definition || mode !== 'published') return
    try {
      const parsed = JSON.parse(input) as Record<string, unknown>
      const result = await api.post<DryRunResult>('/v1/pipelines/dry-run', { project_id: projectID, pipeline_id: definition.pipeline_id, pipeline_version: definition.version, input: parsed })
      setDryRunResult(result)
      setNotice(result.no_writes_performed ? `Dry Run 完成：${result.pure_previewed} 个纯阶段已在内存中预演，${result.planned_writes} 个写入阶段被跳过。` : 'Dry Run 未满足零写入保证。')
    } catch (reason) { setDryRunResult(undefined); setNotice(reason instanceof Error ? reason.message : String(reason)) }
  }

  async function run() {
    if (!definition || mode !== 'published') return
    try {
      const parsed = JSON.parse(input) as Record<string, unknown>
      if (definition.nodes.some(node => node.stage_type === 'memory.compile') && !String(parsed.session_id || '').trim()) {
        setNotice('这条流程处理真实原材料，请先填写一个会话 ID；日常使用可直接在记忆库点击“处理新增或失败的原材料”，也可以手动选择范围。')
        return
      }
      const response = await api.post<{ run_id: string }>('/v1/pipelines/execute', { project_id: projectID, pipeline_id: definition.pipeline_id, pipeline_version: definition.version, idempotency_key: `studio-${Date.now()}`, input: parsed })
      openRun(response.run_id)
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : String(reason)) }
  }

  function exportJSON() {
    const current = currentDefinition()
    if (!current) return
    const blob = new Blob([`${JSON.stringify(current, null, 2)}\n`], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a'); link.href = url; link.download = `${current.pipeline_id}-${current.version}.json`; link.click(); URL.revokeObjectURL(url)
  }

  async function importJSON(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    try {
      const imported = JSON.parse(await file.text()) as PipelineDefinition
      const prefix = imported.pipeline_id.split('.').slice(0, -1).join('.')
      setPluginID(prefix.startsWith('builtin.') ? prefix : 'builtin.user-workflows'); setMode('draft'); setDraftRevision(0); setBaseVersion(''); loadGraph(imported); setNotice('已载入本地 JSON；保存前请检查插件命名空间。')
    } catch (reason) { setNotice(`导入失败：${reason instanceof Error ? reason.message : String(reason)}`) }
  }

  const selectedNode = nodes.find(node => node.id === selectedNodeID)
  const editable = mode === 'draft'

  useEffect(() => {
    setConfigText(selectedNode ? JSON.stringify(selectedNode.data.node.config, null, 2) : '{}')
    setNodeIDText(selectedNode?.data.node.id || '')
  }, [selectedNodeID]) // eslint-disable-line react-hooks/exhaustive-deps

  function applyConfig() {
    if (!selectedNode || !editable) return
    try { updateSelectedNode({ config: JSON.parse(configText) }); setNotice('节点配置已应用到当前草稿。') }
    catch (reason) { setNotice(`配置不是合法 JSON：${reason instanceof Error ? reason.message : String(reason)}`) }
  }

  if (loading && !catalog) return <div className="studio-loading">正在装载流程组件与草稿…</div>
  if (fatal) return <div className="studio-fatal"><AlertTriangle size={22} /><h3>流程工坊无法读取</h3><p>{fatal}</p><button onClick={() => void reload()}>重试</button></div>

  return <div className="flow-studio-shell">
    <aside className="flow-library">
      <div className="flow-library-head"><p>WORKFLOWS</p><button onClick={startBlank}><Plus size={14} />新建</button></div>
      {!!catalog?.drafts.length && <section><h3>我的草稿 <span>{catalog.drafts.length}</span></h3>{catalog.drafts.map(item => <button key={item.draft_id} className={mode === 'draft' && definition?.pipeline_id === item.pipeline_id ? 'active' : ''} onClick={() => openDraft(item)}><Save size={15} /><div><strong>{item.definition.name}</strong><small>r{item.revision} · v{item.definition.version}</small></div><ChevronRight size={14} /></button>)}</section>}
      <section><h3>已发布 <span>{catalog?.pipelines.length || 0}</span></h3>{catalog?.pipelines.map(item => <button key={`${item.pipeline_id}:${item.version}`} className={mode === 'published' && definition?.pipeline_id === item.pipeline_id && definition?.version === item.version ? 'active' : ''} onClick={() => openPublished(item)}><GitBranch size={15} /><div><strong>{item.name}</strong><small>{item.plugin_id} · v{item.version}</small></div><ChevronRight size={14} /></button>)}</section>
    </aside>

    <main className="flow-workbench">
      {definition ? <>
        <header className="flow-toolbar">
          <div><div className="flow-title-line"><Badge value={mode} />{dirty && <Badge value="unsaved" />}{validation && <Badge value={validation.valid ? 'valid' : 'invalid'} />}</div><input className="flow-name" value={definition.name} readOnly={!editable} onChange={event => updateDefinition({ name: event.target.value })} /><span>{definition.pipeline_id} · v{definition.version}</span></div>
          <div className="flow-toolbar-actions"><button onClick={() => setPaletteOpen(value => !value)}><CirclePlus size={14} />组件</button><label><Upload size={14} />导入<input type="file" accept="application/json,.json" onChange={importJSON} /></label><button onClick={exportJSON}><Download size={14} />导出</button>{mode === 'published' ? <><button onClick={editPublished}><Copy size={14} />编辑为新版本</button><button onClick={dryRun}><ShieldCheck size={14} />Dry Run</button><button className="primary" onClick={run}><Play size={14} />运行</button></> : <><button onClick={validate}><ShieldCheck size={14} />校验</button><button onClick={saveDraft}><Save size={14} />保存草稿</button><button className="primary" onClick={publish}><WandSparkles size={14} />发布版本</button></>}</div>
        </header>
        {notice && <div className={`flow-notice ${validation && !validation.valid ? 'bad' : ''}`}>{validation?.valid ? <CheckCircle2 size={14} /> : <Braces size={14} />}<span>{notice}</span></div>}
        {dryRunResult && <section className="flow-dry-run"><header><div><ShieldCheck size={16}/><strong>Dry Run · 零写入预演</strong></div><span>{dryRunResult.no_writes_performed ? 'NO WRITES PERFORMED' : 'CHECK REQUIRED'}</span></header><div className="flow-dry-run-metrics"><article><small>纯阶段</small><strong>{dryRunResult.pure_previewed}</strong></article><article><small>预计写入阶段</small><strong>{dryRunResult.planned_writes}</strong></article><article><small>模型调用</small><strong>{dryRunResult.planned_model_calls}</strong></article><article><small>审核门</small><strong>{dryRunResult.review_gates}</strong></article></div><div className="flow-dry-run-nodes">{dryRunResult.nodes.map(node=><article key={node.node_id}><span>{node.node_id}</span><b>{node.status.replaceAll('_',' ')}</b><small>{node.detail}</small></article>)}</div>{dryRunResult.warnings.length>0&&<p>{dryRunResult.warnings.join('；')}</p>}</section>}
        <div className="flow-editor-grid">
          {paletteOpen && <aside className="stage-palette"><p>COMPONENTS</p><h3>点击或拖入处理步骤</h3>{['memory', 'project', 'deterministic', 'model', 'human'].map(group => <section key={group}><h4>{classLabel[group] || group}</h4>{catalog?.stages.filter(stage => stage.class === group).map(stage => <button key={stage.stage_type} draggable={editable} disabled={!editable} onDragStart={event => { event.dataTransfer.setData('application/memory-harness-stage', stage.stage_type); event.dataTransfer.effectAllowed = 'copy' }} onClick={() => addStage(stage)}><span>{stage.stage_type.split('.').pop()?.slice(0, 2).toUpperCase()}</span><div><strong>{stage.stage_type}</strong><small>{stage.description}</small></div><Plus size={13} /></button>)}</section>)}</aside>}
          <div className="flow-canvas"><ReactFlow<StageGraphNode, Edge>
            nodes={nodes} edges={edges} nodeTypes={nodeTypes} onNodesChange={editable ? changes => { onNodesChange(changes); if (changes.some(change => change.type === 'position' && change.dragging === false)) markChanged() } : undefined}
            onEdgesChange={editable ? changes => { onEdgesChange(changes); markChanged() } : undefined} onConnect={onConnect} isValidConnection={connection => editable && !isCycle(connection, edges)}
            onNodeClick={(_, node) => setSelectedNodeID(node.id)} nodesDraggable={editable} nodesConnectable={editable} elementsSelectable fitView onInit={instance => { reactFlow.current = instance }}
            onDragOver={event => { if (editable) { event.preventDefault(); event.dataTransfer.dropEffect = 'copy' } }} onDrop={event => { event.preventDefault(); const stageType = event.dataTransfer.getData('application/memory-harness-stage'); const stage = catalog?.stages.find(item => item.stage_type === stageType); if (stage && reactFlow.current) addStage(stage, reactFlow.current.screenToFlowPosition({ x: event.clientX, y: event.clientY })) }}
            deleteKeyCode={editable ? ['Backspace', 'Delete'] : null} minZoom={.25} maxZoom={1.7}>
            <Background gap={22} size={1} /><Controls showInteractive={false} /><MiniMap pannable zoomable nodeColor={node => (node.data as StageData).descriptor?.class === 'model' ? '#b35f49' : (node.data as StageData).descriptor?.class === 'human' ? '#b58c46' : '#587664'} />
            <Panel position="bottom-center" className="flow-canvas-help">拖动节点 · 从端口连线 · 点击节点编辑参数 · 滚轮缩放</Panel>
          </ReactFlow></div>
          <aside className="flow-inspector">
            {selectedNode ? <><div className="inspector-head"><div><p>NODE INSPECTOR</p><h3>{selectedNode.data.node.id}</h3></div>{editable && <button onClick={deleteSelected}><Trash2 size={15} /></button>}</div>
              <label>节点 ID<input value={nodeIDText} readOnly={!editable} onChange={event => setNodeIDText(event.target.value)} onBlur={() => renameSelected(nodeIDText)} /></label>
              <label>阶段类型<select value={selectedNode.data.node.stage_type} disabled={!editable} onChange={event => { const stage = catalog?.stages.find(item => item.stage_type === event.target.value); updateSelectedNode({ stage_type: event.target.value, stage_version: stage?.version || '1.0.0', config: defaultConfig(event.target.value) }) }}>{catalog?.stages.map(stage => <option key={stage.stage_type}>{stage.stage_type}</option>)}</select></label>
              <div className="inspector-facts"><span>执行类别<strong>{classLabel[selectedNode.data.descriptor?.class || ''] || selectedNode.data.descriptor?.class}</strong></span><span>阶段版本<strong>{selectedNode.data.node.stage_version}</strong></span></div>
              <label>配置 JSON<textarea rows={15} value={configText} readOnly={!editable} onChange={event => setConfigText(event.target.value)} onBlur={applyConfig} /></label>
              {editable && <button className="inspector-apply" onClick={applyConfig}><CheckCircle2 size={13} />应用配置</button>}
              <small>{selectedNode.data.descriptor?.description}</small>
            </> : <><div className="inspector-head"><div><p>FLOW SETTINGS</p><h3>流程属性</h3></div></div>
              <label>Pipeline ID<input value={definition.pipeline_id} readOnly={!editable || draftRevision > 0} onChange={event => updateDefinition({ pipeline_id: event.target.value })} /></label>
              <label>版本<input value={definition.version} readOnly={!editable} onChange={event => updateDefinition({ version: event.target.value })} /></label>
              <label>用途<textarea rows={5} value={definition.intent} readOnly={!editable} onChange={event => updateDefinition({ intent: event.target.value })} /></label>
              <div className="form-grid compact"><label>最多阶段<input type="number" value={definition.policy.max_stages} readOnly={!editable} onChange={event => updateDefinition({ policy: { ...definition.policy, max_stages: Number(event.target.value) } })} /></label><label>超时秒数<input type="number" value={definition.policy.timeout_seconds} readOnly={!editable} onChange={event => updateDefinition({ policy: { ...definition.policy, timeout_seconds: Number(event.target.value) } })} /></label><label>模型调用<input type="number" value={definition.policy.max_model_calls} readOnly={!editable} onChange={event => updateDefinition({ policy: { ...definition.policy, max_model_calls: Number(event.target.value) } })} /></label></div>
              <label>运行输入 JSON<textarea rows={9} value={input} onChange={event => setInput(event.target.value)} /></label>
            </>}
          </aside>
        </div>
      </> : <div className="flow-empty"><GitBranch size={31} /><h2>新建或选择一条流程</h2><p>从模板克隆不会修改原版本。</p><button onClick={startBlank}>创建空白流程</button></div>}
    </main>
  </div>
}
