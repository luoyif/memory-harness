import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { APIClient, HarnessObject } from './api'
import { ProfileCenter } from './ProfileCenter'

afterEach(cleanup)

function profileObject(locked=false):HarnessObject{
  return {
    object_id:'profile-project-1-dynamic',type_id:'builtin.living-asset-vault.profile-projection.v1',project_id:'project-1',status:'active',protection_class:'standard',current_revision:locked?2:1,
    revision:{content_hash:'hash-current',confidence:1,importance:.75,plugin_id:'builtin.living-asset-vault',plugin_version:'2.0.0',source_evidence_ids:[],source_object_ids:[],created_at:'2026-08-23T08:00:00Z',payload:{
      profile_id:'profile-id-1',view_kind:'dynamic_project',profile_class:'dynamic',title:'Dynamic Project',summary:'当前项目上下文',locked_block_ids:locked?['project:goals']:[],generation_status:locked?'human_mixed':'auto',generated_from_revision:3,generated_at:'2026-08-23T08:00:00Z',
      blocks:[{block_id:'project:goals',label:'当前目标',content:'- 完成 Profile Compiler',source_refs:[],source_object_ids:[],source_hash:'source-hash',last_verified_at:'2026-08-23T08:00:00Z',confidence:1,locked,review_status:'current'}],
    }},created_at:'2026-08-23T08:00:00Z',updated_at:'2026-08-23T08:00:00Z',
  }
}

describe('ProfileCenter',()=>{
  it('renders governed projections and locks a block through the Owner API',async()=>{
    let current=profileObject(false)
    const get=vi.fn(async()=>({profiles:[current],total:1}))
    const post=vi.fn(async()=>({status:'refreshed',profiles:[current],total:1}))
    const put=vi.fn(async(path:string,body:unknown)=>{void path;void body;current=profileObject(true);return {status:'updated',object:current}})
    render(<ProfileCenter api={{get,post,put} as unknown as APIClient} projectID="project-1"/>)
    expect(await screen.findByText('Dynamic Project')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:/Dynamic Project/}))
    expect(await screen.findByText('当前目标')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:'锁定此 Block'}))
    await waitFor(()=>expect(put).toHaveBeenCalledTimes(1))
    expect(put.mock.calls[0][0]).toBe('/v1/projects/project-1/profiles/dynamic_project/locks')
    expect(put.mock.calls[0][1]).toEqual({block_ids:['project:goals']})
    expect(await screen.findByRole('button',{name:'解除锁定'})).toBeInTheDocument()
  })
})
