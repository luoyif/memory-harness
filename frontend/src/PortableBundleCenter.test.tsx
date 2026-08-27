import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIClient, HarnessObject } from './api'
import { PortableBundleCenter } from './PortableBundleCenter'

afterEach(()=>{cleanup();window.go=undefined;vi.restoreAllMocks()})

function object(id:string):HarnessObject{return {
  object_id:id,type_id:'builtin.living-asset-vault.document',project_id:'project-ft5',status:'active',protection_class:'standard',current_revision:2,
  created_at:'2026-08-23T10:00:00Z',updated_at:'2026-08-23T11:00:00Z',
  revision:{payload:{title:'Portable'},content_hash:`hash-${id}`,confidence:1,importance:.8,plugin_id:'builtin.living-asset-vault',plugin_version:'2.0.0',source_evidence_ids:['ev-1'],source_object_ids:[],created_at:'2026-08-23T11:00:00Z'}
}}
function apiWith(objects:HarnessObject[]){
  const api=new APIClient({endpoint:'http://local',sessionID:'s',token:'t',csrf:'c',expiresAt:'',version:'2.0.0'})
  vi.spyOn(api,'get').mockImplementation(async path=>{
    const url=new URL(`http://local${path}`)
    const offset=Number(url.searchParams.get('offset')||0)
    const limit=Number(url.searchParams.get('limit')||100)
    const page=objects.slice(offset,offset+limit)
    return {objects:page,total:objects.length,has_more:offset+page.length<objects.length}
  })
  return api
}
const manifest={bundle_id:'bundle-abc',bundle_hash:'sha256:abc',source_project_id:'source',object_count:1,evidence_count:1,required_capabilities:['evidence:v1'],signature:{status:'unsigned',algorithm:'none'}}
const report={compatible:false,blocked:false,missing_capabilities:['plugin:missing'],unmapped_object_types:['foreign.type'],findings:[],degradations:['presentation fallback'],permission_delta:[],presentation_fallback:true,import_mode:'evidence_quarantine+protected_candidate_only'}

describe('PortableBundleCenter',()=>{
  it('pages large Object collections instead of loading 500 at once',async()=>{
    const objects=Array.from({length:205},(_,i)=>object(`root-${i}`))
    render(<PortableBundleCenter api={apiWith(objects)} projectID="project-ft5"/>)
    expect(await screen.findByText('100 / 205')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:/加载更多 Object/}))
    expect(await screen.findByText('200 / 205')).toBeInTheDocument()
    expect(screen.getByRole('button',{name:/加载更多 Object/})).toHaveTextContent('200 / 205')
  })

  it('exports only explicitly selected roots and keeps dependency choice explicit',async()=>{
    const exportPortable=vi.fn().mockResolvedValue({path:'/tmp/test.mhbundle.tar.gz',manifest})
    window.go={main:{DesktopBridge:{Bootstrap:vi.fn(),ExportPortableBundle:exportPortable}}}
    render(<PortableBundleCenter api={apiWith([object('root-1'),object('root-2')])} projectID="project-ft5"/>)
    const boxes=await screen.findAllByRole('checkbox',{name:/builtin.living-asset-vault.document/})
    fireEvent.click(boxes[0])
    fireEvent.click(screen.getByRole('button',{name:/导出所选 Bundle/}))
    await waitFor(()=>expect(exportPortable).toHaveBeenCalledTimes(1))
    expect(exportPortable).toHaveBeenCalledWith('project-ft5',['root-1'],true)
    expect(await screen.findByText(/已导出 1 个对象/)).toBeInTheDocument()
  })

  it('renders a blocked preflight and keeps import disabled',async()=>{
    const blocked={...report,blocked:true,findings:[{severity:'blocked',code:'instruction_injection_signal',subject:'object:x',detail:'matched instruction-like control phrase'}]}
    const preflight=vi.fn().mockResolvedValue({path:'/tmp/blocked.mhbundle.tar.gz',manifest,report:blocked})
    window.go={main:{DesktopBridge:{Bootstrap:vi.fn(),PreflightPortableBundle:preflight}}}
    render(<PortableBundleCenter api={apiWith([])} projectID="project-ft5"/>)
    fireEvent.click(await screen.findByRole('button',{name:/Preflight/}))
    expect((await screen.findAllByText('BLOCKED')).length).toBeGreaterThan(0)
    expect(screen.getByText(/instruction_injection_signal/)).toBeInTheDocument()
    expect(screen.getByRole('button',{name:/确认隔离导入/})).toBeDisabled()
  })

  it('requires explicit confirmation for degraded import and pins the inspected identity',async()=>{
    const preflight=vi.fn().mockResolvedValue({path:'/tmp/degraded.mhbundle.tar.gz',manifest,report})
    const importer=vi.fn().mockResolvedValue({bundle_id:'bundle-abc',target_project_id:'project-ft5',evidence_imported:1,evidence_duplicates:0,candidate_object_ids:['portable-candidate-1'],no_direct_activation:true})
    window.go={main:{DesktopBridge:{Bootstrap:vi.fn(),PreflightPortableBundle:preflight,ImportPortableBundle:importer}}}
    render(<PortableBundleCenter api={apiWith([])} projectID="project-ft5"/>)
    fireEvent.click(await screen.findByRole('button',{name:/Preflight/}))
    expect((await screen.findAllByText('DEGRADED')).length).toBeGreaterThan(0)
    const button=screen.getByRole('button',{name:/确认隔离导入/})
    expect(button).toBeDisabled()
    fireEvent.click(screen.getByRole('checkbox',{name:/我确认导入只产生 quarantine Evidence/}))
    expect(button).not.toBeDisabled()
    fireEvent.click(button)
    await waitFor(()=>expect(importer).toHaveBeenCalledTimes(1))
    expect(importer.mock.calls[0].slice(0,4)).toEqual(['project-ft5','/tmp/degraded.mhbundle.tar.gz','bundle-abc','sha256:abc'])
    expect(await screen.findByText(/NO DIRECT ACTIVATION: YES/)).toBeInTheDocument()
  })
})
