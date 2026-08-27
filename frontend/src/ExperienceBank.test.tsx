import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIClient, HarnessObject } from './api'
import { ExperienceBank } from './ExperienceBank'

afterEach(cleanup)

function caseObject(id:string,status='candidate',result='unknown'):HarnessObject{
  return {object_id:id,type_id:'builtin.experience-bank.case.v1',project_id:'project-ft3',status,protection_class:'protected',current_revision:1,created_at:'2026-08-23T10:00:00Z',updated_at:'2026-08-23T10:00:00Z',revision:{content_hash:`hash-${id}`,confidence:1,importance:.7,plugin_id:'builtin.experience-bank',plugin_version:'1.0.0',source_evidence_ids:[],source_object_ids:[],created_at:'2026-08-23T10:00:00Z',payload:{case_id:`case-${id}`,source_run_id:`run-${id}`,plan_id:`plan-${id}`,receipt_id:`receipt-${id}`,blueprint_id:'builtin.memory-harness-core.default',blueprint_version:'1.1.0',adapter_id:'dsh-memory-harness-bridge',runtime:'deepseek-harness',task_features:{run_status:'completed'},delivery:{total:2,delivered:2,trimmed:0,denied:0,failed:0,delivery_unverified:0,evidence_level:'harness_observed',completeness:'complete'},outcome_run_ids:[`outcome-${id}`],outcome_metrics:[{name:'turn_completed',value:1,confidence:1}],cost:{tokens:100,latency_ms:500},evaluation_object_ids:[],result,secondary_failure_dimensions:[],diagnosis:'Context delivery and runtime outcome are observations only; correctness remains unknown until an independent Evaluation is attached.',transfer_scope:['deepseek-harness'],sensitivity:'standard',source_artifact_refs:[`run:${id}`],source_hash:`source-${id}`,generated_at:'2026-08-23T10:00:00Z'}}}
}

function mockAPI(objects:HarnessObject[]){
  const api=new APIClient({endpoint:'http://local',sessionID:'s',token:'t',csrf:'c',expiresAt:'',version:'2.0.0'})
  vi.spyOn(api,'get').mockImplementation(async path=>{
    if(path.startsWith('/v1/experience/cases?'))return {cases:objects}
    if(path.startsWith('/v1/experience/patterns?'))return {patterns:[]}
    if(path.startsWith('/v1/experience/cases/')){const object=objects.find(x=>path.endsWith(x.object_id))!;return {object,case:object.revision.payload,evaluations:[]}}
    throw new Error(`unexpected GET ${path}`)
  })
  const post=vi.spyOn(api,'post').mockResolvedValue({object_id:'evaluation-1'})
  return {api,post}
}
describe('ExperienceBank',()=>{
  it('never renders delivered + completed as task success before Evaluation',async()=>{
    const {api,post}=mockAPI([caseObject('negative')])
    render(<ExperienceBank api={api} projectID="project-ft3"/>)
    expect(await screen.findByText('尚未形成失败诊断')).toBeInTheDocument()
    expect(screen.getByText('待评价 / Unknown')).toBeInTheDocument()
    expect(screen.getByText('2/2')).toBeInTheDocument()
    expect(screen.queryByText('通过评价 / Pass')?.parentElement?.textContent).toContain('0')
    fireEvent.click(screen.getByText('尚未形成失败诊断'))
    expect(await screen.findByText(/“送达”“回合完成”和“任务正确”是三个不同事实/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:/独立评价/}))
    fireEvent.change(screen.getByLabelText('评价结论'),{target:{value:'fail'}})
    fireEvent.change(screen.getByLabelText('观察结果'),{target:{value:'NO_CONTEXT'}})
    fireEvent.click(screen.getByRole('button',{name:/保存独立 Evaluation/}))
    await waitFor(()=>expect(post).toHaveBeenCalled())
    const call=post.mock.calls.find(([path])=>String(path).includes('/evaluations'))
    expect(call?.[1]).toMatchObject({verdict:'fail',observed:'NO_CONTEXT',dimensions:[{name:'task_correctness',verdict:'fail'}]})
  })

  it('requires multiple active governed Cases before creating a Pattern',async()=>{
    const first=caseObject('active-1','active','fail'),second=caseObject('active-2','active','fail')
    const {api,post}=mockAPI([first,second]);render(<ExperienceBank api={api} projectID="project-ft3"/>)
    expect((await screen.findAllByText('尚未形成失败诊断')).length).toBe(2)
    fireEvent.click(screen.getByRole('button',{name:/提出 Pattern/}))
    fireEvent.change(screen.getByLabelText('归一化诊断模式'),{target:{value:'delivered context can be under-used'}})
    fireEvent.change(screen.getByLabelText('预期作用'),{target:{value:'preserve semantic labels'}})
    fireEvent.click(screen.getAllByRole('checkbox',{name:/run-active/})[0])
    fireEvent.click(screen.getByRole('button',{name:/创建 Pattern Candidate/}))
    expect(await screen.findByText(/至少选择两个已治理 active Case/)).toBeInTheDocument()
    expect(post.mock.calls.some(([path])=>String(path)==='/v1/experience/patterns')).toBe(false)
    const boxes=screen.getAllByRole('checkbox',{name:/失败/})
    fireEvent.click(boxes[1])
    fireEvent.click(screen.getByRole('button',{name:/创建 Pattern Candidate/}))
    await waitFor(()=>expect(post.mock.calls.some(([path])=>String(path)==='/v1/experience/patterns')).toBe(true))
    const patternCall=post.mock.calls.find(([path])=>String(path)==='/v1/experience/patterns')
    expect(patternCall?.[1]).toMatchObject({supporting_case_ids:['active-1','active-2'],normalized_pattern:'delivered context can be under-used'})
  })
})
