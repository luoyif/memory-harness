import { useEffect, useMemo, useRef, useState } from 'react'
import type { Core, ElementDefinition, StylesheetJson } from 'cytoscape'
import { ArrowLeft, Focus, Maximize2, RotateCcw, Search, ZoomIn, ZoomOut } from 'lucide-react'

export type MemoryGraphData = {
  nodes: Array<{ id: string; layer: string; label: string; status: string }>
  edges: Array<{ from: string; to: string; kind: string }>
}

const layers = [
  { id: 'evidence', index: '01', label: '不可变证据', hint: '保留原始来源', color: '#878c84' },
  { id: 'knowledge', index: '02', label: '知识点', hint: '结构化最小信息', color: '#b88b42' },
  { id: 'episode', index: '03', label: '会话复盘', hint: '目标、行动与结果', color: '#b95e46' },
  { id: 'memory', index: '04', label: '长期记忆', hint: '合并与冲突治理', color: '#52715f' },
  { id: 'living', index: '05', label: '生长知识', hint: '持续修订的文档', color: '#607f8a' },
  { id: 'asset', index: '06', label: '能力资产', hint: '经验证供 Agent 使用', color: '#99594f' },
]

const layerByID = new Map(layers.map(layer => [layer.id, layer]))
const compact = (value: string, limit = 25) => value.length > limit ? `${value.slice(0, limit - 1)}…` : value

const lineageStyles: StylesheetJson = [
  { selector: 'node', style: {
    shape: 'round-rectangle', width: 156, height: 72, 'background-color': 'data(color)',
    'border-width': 4, 'border-color': '#f5f1e8', label: 'data(label)', color: '#fff',
    'font-family': 'Avenir Next, PingFang SC, sans-serif', 'font-size': 11, 'font-weight': 700,
    'text-wrap': 'wrap', 'text-max-width': '128px', 'text-valign': 'center', 'text-halign': 'center',
    'overlay-opacity': 0,
  } },
  { selector: 'node.object', style: { width: 132, height: 52, 'font-size': 9, 'border-width': 3 } },
  { selector: 'node.selected', style: { 'border-color': '#e58b69', 'border-width': 7 } },
  { selector: 'edge', style: {
    width: 'data(width)', 'line-color': '#a8b4ab', 'target-arrow-color': '#71857a',
    'target-arrow-shape': 'triangle', 'curve-style': 'bezier', label: 'data(label)',
    color: '#4d5d52', 'font-family': 'SFMono-Regular, monospace', 'font-size': 8,
    'text-background-color': '#fbf8f1', 'text-background-opacity': .96, 'text-background-padding': '4px',
    'text-border-color': '#d8d1c4', 'text-border-width': 1, 'text-border-opacity': 1,
  } },
]

function neighborhood(graph: MemoryGraphData, rootID: string, depth = 2, limit = 30) {
  const found = new Set([rootID])
  let frontier = [rootID]
  for (let round = 0; round < depth && frontier.length && found.size < limit; round += 1) {
    const next: string[] = []
    graph.edges.forEach(edge => {
      if (frontier.includes(edge.from) && !found.has(edge.to)) next.push(edge.to)
      if (frontier.includes(edge.to) && !found.has(edge.from)) next.push(edge.from)
    })
    frontier = next.slice(0, Math.max(0, limit - found.size))
    frontier.forEach(id => found.add(id))
  }
  return found
}

export function MemoryLineage({ graph }: { graph: MemoryGraphData }) {
  const canvasRef = useRef<HTMLDivElement>(null)
  const cyRef = useRef<Core>()
  const [selectedLayer, setSelectedLayer] = useState('')
  const [selectedNodeID, setSelectedNodeID] = useState('')
  const [query, setQuery] = useState('')
  const nodeByID = useMemo(() => new Map(graph.nodes.map(node => [node.id, node])), [graph.nodes])
  const counts = useMemo(() => new Map(layers.map(layer => [layer.id, graph.nodes.filter(node => node.layer === layer.id).length])), [graph.nodes])
  const transitionCounts = useMemo(() => {
    const result = new Map<string, number>()
    graph.edges.forEach(edge => {
      const from = nodeByID.get(edge.from)?.layer
      const to = nodeByID.get(edge.to)?.layer
      if (!from || !to || from === to) return
      const key = `${from}:${to}`
      result.set(key, (result.get(key) || 0) + 1)
    })
    return result
  }, [graph.edges, nodeByID])
  const selectedNode = nodeByID.get(selectedNodeID)
  const selectedLayerNodes = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase()
    return graph.nodes.filter(node => (!selectedLayer || node.layer === selectedLayer) && (!normalized || node.label.toLocaleLowerCase().includes(normalized)))
  }, [graph.nodes, query, selectedLayer])

  const elements = useMemo<ElementDefinition[]>(() => {
    if (!selectedNodeID) {
      const stageNodes: ElementDefinition[] = layers.map(layer => ({
        group: 'nodes', data: { id: `stage:${layer.id}`, label: `${layer.index}  ${layer.label}\n${counts.get(layer.id) || 0} 个对象`, color: layer.color },
      }))
      const stageEdges: ElementDefinition[] = layers.slice(0, -1).map((layer, index) => {
        const next = layers[index + 1]
        const count = transitionCounts.get(`${layer.id}:${next.id}`) || transitionCounts.get(`${next.id}:${layer.id}`) || 0
        return { group: 'edges', data: { id: `stage-edge:${layer.id}`, source: `stage:${layer.id}`, target: `stage:${next.id}`, label: `${count} 条真实连接`, width: 1.5 + Math.min(5, Math.sqrt(count)) } }
      })
      return [...stageNodes, ...stageEdges]
    }
    const ids = neighborhood(graph, selectedNodeID)
    const objectNodes: ElementDefinition[] = graph.nodes.filter(node => ids.has(node.id)).map(node => ({
      group: 'nodes', data: { id: node.id, label: compact(node.label), color: layerByID.get(node.layer)?.color || '#878c84' }, classes: 'object',
    }))
    const objectEdges: ElementDefinition[] = graph.edges.filter(edge => ids.has(edge.from) && ids.has(edge.to)).map((edge, index) => ({
      group: 'edges', data: { id: `lineage:${index}:${edge.from}:${edge.to}`, source: edge.from, target: edge.to, label: edge.kind.replaceAll('_', ' '), width: 1.6 },
    }))
    return [...objectNodes, ...objectEdges]
  }, [counts, graph, selectedNodeID, transitionCounts])

  useEffect(() => {
    if (!canvasRef.current || typeof ResizeObserver === 'undefined') return
    let disposed = false
    let cy: Core | undefined
    let observer: ResizeObserver | undefined
    const container = canvasRef.current
    void import('cytoscape').then(({ default: createCytoscape }) => {
      if (disposed || !container) return
      cy = createCytoscape({
        container, elements, style: lineageStyles, minZoom: .28, maxZoom: 2.2,
        layout: selectedNodeID
          ? { name: 'breadthfirst', directed: true, spacingFactor: 1.35, padding: 52, fit: true, circle: false }
          : { name: 'grid', rows: 1, padding: 58, fit: true },
      })
      cy.on('tap', 'node', event => {
        const id = event.target.id()
        if (id.startsWith('stage:')) { setSelectedLayer(id.slice(6)); setSelectedNodeID('') }
        else { setSelectedNodeID(id); setSelectedLayer(nodeByID.get(id)?.layer || '') }
      })
      cyRef.current = cy
      observer = new ResizeObserver(() => { cy?.resize(); cy?.fit(undefined, 52) })
      observer.observe(container)
    })
    return () => { disposed = true; observer?.disconnect(); cy?.destroy(); cyRef.current = undefined }
  }, [elements, nodeByID, selectedNodeID])

  useEffect(() => {
    const cy = cyRef.current
    if (!cy) return
    cy.nodes().removeClass('selected')
    if (selectedNodeID) cy.getElementById(selectedNodeID).addClass('selected')
    if (selectedLayer && !selectedNodeID) cy.getElementById(`stage:${selectedLayer}`).addClass('selected')
  }, [elements, selectedLayer, selectedNodeID])

  const fit = () => cyRef.current?.animate({ fit: { eles: cyRef.current.elements(), padding: 52 }, duration: 220 })
  const zoom = (factor: number) => {
    const cy = cyRef.current
    if (!cy) return
    cy.animate({ zoom: Math.min(cy.maxZoom(), Math.max(cy.minZoom(), cy.zoom() * factor)), center: { eles: cy.nodes() }, duration: 180 })
  }
  const reset = () => { setSelectedLayer(''); setSelectedNodeID(''); setQuery('') }

  return <div className="memory-graph-shell graph-workbench lineage-workbench">
    <header className="graph-workbench-head">
      <div><span className="lineage-mark">6L</span><div><strong>{selectedNodeID ? '对象来源邻域' : '六层生长总览'}</strong><span>{graph.nodes.length} 个可追溯对象 · {graph.edges.length} 条持久化连接</span></div></div>
      {selectedNodeID && <button className="graph-back-button" onClick={() => setSelectedNodeID('')}><ArrowLeft size={15} />返回六层总览</button>}
    </header>
    <div className="graph-toolbar">
      <label><Search size={15} /><span className="sr-only">搜索来源对象</span><input value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索来源对象…" /></label>
      <span className="graph-toolbar-copy">{selectedLayer ? `${layerByID.get(selectedLayer)?.label} · ${selectedLayerNodes.length} 个对象` : '点击任一层查看对象；再点对象展开真实邻域'}</span>
      <span className="graph-toolbar-spacer" />
      <button onClick={() => zoom(1.25)} aria-label="放大来源链"><ZoomIn size={16} /></button>
      <button onClick={() => zoom(.8)} aria-label="缩小来源链"><ZoomOut size={16} /></button>
      <button onClick={fit} aria-label="适应来源链画布"><Maximize2 size={16} /></button>
      <button onClick={reset} aria-label="重置来源链"><RotateCcw size={16} /></button>
    </div>
    <div className="graph-workbench-body lineage-body">
      <div ref={canvasRef} className="memory-graph-canvas cytoscape-canvas" role="img" aria-label={selectedNodeID ? `对象 ${selectedNode?.label} 的来源邻域` : '六层来源链总览'} />
      <aside className="graph-inspector lineage-inspector">
        {selectedNode ? <>
          <p className="micro">TRACEABLE OBJECT</p><h3>{selectedNode.label}</h3>
          <div className="graph-node-meta"><span>{layerByID.get(selectedNode.layer)?.label || selectedNode.layer}</span><span>{selectedNode.status}</span></div>
          <p>画布只展开两跳来源邻域，避免整库关系同时挤入。下方关系均来自持久化连接。</p>
          <div className="lineage-relation-list">{graph.edges.filter(edge => edge.from === selectedNodeID || edge.to === selectedNodeID).map((edge, index) => {
            const otherID = edge.from === selectedNodeID ? edge.to : edge.from
            const other = nodeByID.get(otherID)
            return <button key={index} onClick={() => setSelectedNodeID(otherID)}><span>{edge.kind.replaceAll('_', ' ')}</span><strong>{other?.label || otherID}</strong></button>
          })}</div>
        </> : selectedLayer ? <>
          <p className="micro">{layerByID.get(selectedLayer)?.index} · LAYER OBJECTS</p>
          <h3>{layerByID.get(selectedLayer)?.label}</h3><p>{layerByID.get(selectedLayer)?.hint}。选择一个对象即可查看它向前和向后的真实来源。</p>
          <div className="lineage-object-list">{selectedLayerNodes.length ? selectedLayerNodes.slice(0, 80).map(node => <button key={node.id} onClick={() => setSelectedNodeID(node.id)}><i style={{ background: layerByID.get(node.layer)?.color }} /><span><strong>{node.label}</strong><small>{node.status}</small></span><Focus size={14} /></button>) : <p>当前筛选下没有对象。</p>}</div>
        </> : <>
          <p className="micro">PROGRESSIVE LINEAGE</p><h3>六层不是六列对象墙</h3>
          <p>默认只展示阶段和真实连接数量。点击阶段检查对象，再选择对象查看两跳来源，避免之前的全量扇出。</p>
          <ol className="lineage-stage-list">{layers.map(layer => <li key={layer.id}><button onClick={() => setSelectedLayer(layer.id)}><i style={{ background: layer.color }} /><span>{layer.label}</span><strong>{counts.get(layer.id) || 0}</strong></button></li>)}</ol>
        </>}
      </aside>
    </div>
    <details className="graph-adjacency"><summary>全部来源对象（无障碍与逐条核查）</summary><div className="lineage-adjacency">{graph.nodes.map(node => <button key={node.id} onClick={() => { setSelectedLayer(node.layer); setSelectedNodeID(node.id) }}><strong>{node.label}</strong><span>{layerByID.get(node.layer)?.label || node.layer}</span><small>{node.status}</small></button>)}</div></details>
  </div>
}
