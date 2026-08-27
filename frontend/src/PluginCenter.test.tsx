import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { APIClient, PluginConformanceReport, PluginVersion, Project } from './api'
import { PluginCenter } from './PluginCenter'

afterEach(cleanup)

const plugin: PluginVersion = {
  plugin_id: 'builtin.user-workflows', version: '2.0.0', name: 'User Workflows', publisher: 'memory-harness',
  trust_class: 'declarative', signature_status: 'bundled', status: 'enabled', permissions: ['memory.read', 'memory.materialize'],
  contributions: { pipelines: [{ pipeline_id: 'builtin.user-workflows.capture', version: '1.0.0' }], stages: [], memory_types: [] },
  project_states: [], content_hash: 'sha256-test',
}

const project: Project = {
  project_id: 'project-1', slug: 'personal', name: '个人', description: '', status: 'active', color: '#52715f', default_currency: 'CNY', budget_minor: 0,
}

const conformance: PluginConformanceReport = {
  plugin_id: plugin.plugin_id, version: plugin.version, project_id: 'project-1', memory_harness_version: '2.0.0-memory-harness',
  compatibility_requirement: '>=2.0.0 <3.0.0', compatibility_status: 'compatible', declared_capabilities: plugin.permissions,
  granted_capabilities: plugin.permissions, missing_required: [], optional_not_granted: [], configuration_status: 'not_declared', overall_status: 'passed',
  checks: [{ name: 'external_effect_probe', status: 'not_executed', detail: 'no side effects' }],
}

describe('plugin center project DIY', () => {
  it('shows contributions and saves a project-scoped capability selection', async () => {
    const api = new APIClient({ endpoint: 'http://local', sessionID: 's', token: 't', csrf: 'c', expiresAt: '', version: '2.0.0' })
    vi.spyOn(api, 'get').mockImplementation(async (path) => {
      if (path === '/v1/plugins') return { plugins: [plugin] } as never
      if (path.includes('/impact')) return { plugin_id: plugin.plugin_id, version: plugin.version, current_objects: 0, historical_revisions: 0, historical_runs: 0, pipeline_versions: 1, blueprint_versions: 0, enabled_projects: 0, active_blueprint_refs: 0, package_bytes_reclaimed: 0, can_retire: false, history_preserved: true, blockers: ['built-in plugins are part of the product baseline'] } as never
      if (path.includes('/conformance')) return conformance as never
      throw new Error(`unexpected GET ${path}`)
    })
    const put = vi.spyOn(api, 'put').mockImplementation(async (_path, body) => ({
      ...plugin,
      project_states: [{ project_id: 'project-1', status: 'enabled', granted_capabilities: (body as { capabilities: string[] }).capabilities, config: {} }],
    }))
    render(<PluginCenter api={api} projectID="project-1" projects={[{ project }]} />)

    expect(await screen.findByRole('heading', { name: 'User Workflows' })).toBeInTheDocument()
    expect(await screen.findAllByText('builtin.user-workflows.capture')).toHaveLength(2)
    fireEvent.click(screen.getByText('memory.materialize'))
    fireEvent.click(screen.getByRole('button', { name: /保存项目设置/ }))

    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    expect(put.mock.calls[0][0]).toContain('/projects/project-1')
    expect(put.mock.calls[0][1]).toMatchObject({ status: 'enabled', capabilities: ['memory.read'] })
    expect(await screen.findByText('项目授权与插件配置已保存。')).toBeInTheDocument()
  })

  it('renders schema-driven config and saves only explicit project values', async () => {
    const configured: PluginVersion = {...plugin, project_states:[{project_id:'project-1',status:'enabled',granted_capabilities:plugin.permissions,config:{mode:'safe',limit:5}}]}
    const schemaReport: PluginConformanceReport = {...conformance, configuration_status:'valid', configuration_schema:{type:'object',required:['mode'],properties:{mode:{type:'string',title:'模式',enum:['safe','strict']},limit:{type:'integer',title:'上限',minimum:1,maximum:20}}}}
    const api = new APIClient({ endpoint: 'http://local', sessionID: 's', token: 't', csrf: 'c', expiresAt: '', version: '2.0.0' })
    vi.spyOn(api, 'get').mockImplementation(async (path) => {
      if (path === '/v1/plugins') return { plugins: [configured] } as never
      if (path.includes('/impact')) return { plugin_id: configured.plugin_id, version: configured.version, current_objects: 0, historical_revisions: 0, historical_runs: 0, pipeline_versions: 1, blueprint_versions: 0, enabled_projects: 1, active_blueprint_refs: 0, package_bytes_reclaimed: 0, can_retire: false, history_preserved: true, blockers: ['built-in plugins are part of the product baseline'] } as never
      if (path.includes('/conformance')) return schemaReport as never
      throw new Error(`unexpected GET ${path}`)
    })
    const put=vi.spyOn(api,'put').mockImplementation(async (_path,body)=>({...configured,project_states:[{...configured.project_states[0],config:(body as {config:Record<string,unknown>}).config}]}))
    render(<PluginCenter api={api} projectID="project-1" projects={[{ project }]} />)
    expect(await screen.findByText('Schema 驱动配置')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('combobox',{name:'模式 *'}),{target:{value:'strict'}})
    fireEvent.change(screen.getByRole('spinbutton',{name:'上限'}),{target:{value:'8'}})
    fireEvent.click(screen.getByRole('button',{name:/保存项目设置/}))
    await waitFor(()=>expect(put).toHaveBeenCalledTimes(1))
    expect(put.mock.calls[0][1]).toMatchObject({config:{mode:'strict',limit:8}})
    expect(screen.getByText(/no side effects/)).toBeInTheDocument()
  })

})
