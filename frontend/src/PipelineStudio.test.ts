import { describe, expect, it } from 'vitest'

import { PipelineDefinition } from './api'
import { graphFromDefinition, isCycle, nextPatchVersion } from './PipelineStudio'

const definition: PipelineDefinition = {
  api_version: 'memory-harness.pipeline/v1alpha1', pipeline_id: 'builtin.user-workflows.test', version: '1.2.3',
  name: 'Editable flow', intent: 'Exercise graph round trip.', required_capabilities: [],
  nodes: [
    { id: 'input', stage_type: 'trigger.manual', stage_version: '1.0.0', plugin_id: 'builtin.memory-harness-core', depends_on: [], config: {} },
    { id: 'map', stage_type: 'transform.map', stage_version: '1.0.0', plugin_id: 'builtin.memory-harness-core', depends_on: ['input'], config: { merge: {} } },
  ],
  outputs: [{ name: 'result', node_id: 'map' }], policy: { max_stages: 4, timeout_seconds: 30, max_model_calls: 0 },
  editor: { positions: { input: { x: 80, y: 90 }, map: { x: 340, y: 90 } } },
}

describe('pipeline editor graph rules', () => {
  it('restores saved layout and dependencies', () => {
    const graph = graphFromDefinition(definition, [])
    expect(graph.nodes.map(node => node.position)).toEqual([{ x: 80, y: 90 }, { x: 340, y: 90 }])
    expect(graph.edges.map(edge => `${edge.source}->${edge.target}`)).toEqual(['input->map'])
  })

  it('rejects connections that would create a cycle', () => {
    const edges = graphFromDefinition(definition, []).edges
    expect(isCycle({ source: 'map', target: 'input', sourceHandle: null, targetHandle: null }, edges)).toBe(true)
    expect(isCycle({ source: 'input', target: 'other', sourceHandle: null, targetHandle: null }, edges)).toBe(false)
  })

  it('clones published work onto the next patch version', () => {
    expect(nextPatchVersion('1.2.3')).toBe('1.2.4')
    expect(nextPatchVersion('draft')).toBe('1.0.0')
  })
})
