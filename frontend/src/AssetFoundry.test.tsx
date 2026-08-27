import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AssetFoundry } from './AssetFoundry'
import { APIClient } from './api'

describe('AssetFoundry template refinement',()=>{
  it('selects candidates and runs incremental second distillation without rerunning Evidence',async()=>{
    const get=vi.fn(async(path:string)=>{
      if(path.startsWith('/v1/assets?'))return {assets:[{asset_id:'a1',asset_type:'skill',title:'发布检查',summary:'发布前运行测试',status:'candidate',version:'0.3',risk_level:'medium',source_memory_ids:['m1'],validation_status:'not_run',classification_status:'classified',created_at:'2026-08-24T00:00:00Z',updated_at:'2026-08-24T00:00:00Z'}]}
      if(path==='/v1/asset-templates')return {template_version:'v1',schema_version:'1',templates:[{asset_type:'skill',label:'Skill · 可复用技能',description:'标准技能',template_version:'v1',fields:[{key:'purpose',label:'能力目标',description:'解决什么问题',kind:'text',required:true,placeholder:'用于…'}]}]}
      if(path.startsWith('/v1/harness/objects?'))return {objects:[]}
      throw new Error(`unexpected GET ${path}`)
    })
    const post=vi.fn(async()=>({run_id:'run-1',total:1,refined:1,proposed:0,skipped:0,failed:0,model_groups:1,fallback_groups:0}))
    render(<AssetFoundry api={{get,post} as unknown as APIClient} projectID="p1"/>)
    const refineButton=await screen.findByRole('button',{name:/按模板二次沉淀/})
    await waitFor(()=>expect(refineButton).toBeEnabled())
    fireEvent.click(refineButton)
    expect(screen.getByRole('dialog',{name:/按标准模板整理 1 个候选/})).toHaveTextContent('不会重跑原材料')
    fireEvent.click(screen.getByRole('button',{name:'只处理新增或有变化'}))
    await waitFor(()=>expect(post).toHaveBeenCalledWith('/v1/projects/p1/assets/refine',expect.objectContaining({asset_ids:['a1'],mode:'incremental'})))
    expect(await screen.findByText(/二次沉淀完成：新建 1/)).toBeInTheDocument()
  })
})
