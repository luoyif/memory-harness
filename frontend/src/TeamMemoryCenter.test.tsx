import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIClient, HarnessObject } from './api'
import { TeamMemoryCenter } from './TeamMemoryCenter'

afterEach(cleanup)

function object(id:string,type:string,status:string,payload:Record<string,unknown>,revision=1):HarnessObject{
  return {object_id:id,type_id:type,project_id:'project-team',status,protection_class:'protected',current_revision:revision,created_at:'2026-08-24T01:00:00Z',updated_at:'2026-08-24T01:00:00Z',revision:{payload,content_hash:`hash-${id}`,confidence:1,importance:.7,plugin_id:'builtin.team-memory',plugin_version:'1.0.0',source_evidence_ids:[],source_object_ids:[],created_at:'2026-08-24T01:00:00Z'}}
}
const taskPayload={task_id:'team-task-1',project_id:'project-team',title:'Architecture review',member_agent_ids:['agent-a','agent-b'],status:'active',created_at:'2026-08-24T01:00:00Z',expires_at:'2099-08-24T02:00:00Z'}
const task=object('team-task-1','builtin.team-memory.task.v1','active',taskPayload)
const board=object('board-1','builtin.team-memory.blackboard-entry.v1','active',{entry_id:'board-1',task_id:'team-task-1',project_id:'project-team',topic:'architecture',claim_key:'db',claim_value:'sqlite',content:'Use SQLite',direct_share_agent_ids:['agent-b'],meta:{agent_id:'agent-a',source_evidence_ids:[],confidence:.9,epistemic_status:'observed',created_at:'2026-08-24T01:00:00Z',expires_at:'2099-08-24T02:00:00Z'}})
const conflict=object('conflict-1','builtin.team-memory.conflict.v1','needs_review',{conflict_id:'conflict-1',task_id:'team-task-1',project_id:'project-team',topic:'architecture',claim_key:'db',entry_ids:['board-1','board-2'],agent_ids:['agent-a','agent-b'],status:'needs_review',created_at:'2026-08-24T01:10:00Z'})
const durable=object('durable-1','builtin.team-memory.project-durable.v1','candidate',{durable_id:'durable-1',project_id:'project-team',task_id:'team-task-1',entry_ids:['board-1'],title:'Architecture durable',summary:'Owner selected',body:'Use SQLite',source_agent_ids:['agent-a'],source_run_ids:[],source_evidence_ids:[],epistemic_status:'observed',generation_status:'owner_selected',created_at:'2026-08-24T01:20:00Z'})
function mockAPI(opts:{tasks?:HarnessObject[],conflicts?:HarnessObject[],durables?:HarnessObject[]}={}){
  const api=new APIClient({endpoint:'http://local',sessionID:'s',token:'t',csrf:'c',expiresAt:'',version:'2.0.0'})
  const tasks=opts.tasks||[],conflicts=opts.conflicts||[],durables=opts.durables||[]
  const get=vi.spyOn(api,'get').mockImplementation(async path=>{
    if(path.startsWith('/v1/team/tasks?'))return {tasks}
    if(path.startsWith('/v1/team/conflicts?'))return {conflicts,review_status:Object.fromEntries(conflicts.map(item=>[item.object_id,'pending']))}
    if(path.startsWith('/v1/team/durables?'))return {durables}
    if(path==='/v1/agents')return {agents:[{agent_id:'agent-a',name:'Agent A',status:'active',all_projects:false,project_ids:['project-team']},{agent_id:'agent-b',name:'Agent B',status:'active',all_projects:false,project_ids:['project-team']}]}
    if(path==='/v1/team/tasks/team-task-1')return {object:task,task:taskPayload,blackboard:[board]}
    throw new Error(`unexpected GET ${path}`)
  })
  const post=vi.spyOn(api,'post').mockImplementation(async path=>{
    if(path==='/v1/team/tasks')return task
    if(path.includes('/close-proposal'))return {review_id:'review-close'}
    if(path.includes('/activation-proposal'))return {review_id:'review-durable'}
    return durable
  })
  return {api,get,post}
}

describe('TeamMemoryCenter',()=>{
  it('never loads Private Scratch in the Owner page and closes tasks through Review only',async()=>{
    const {api,get,post}=mockAPI({tasks:[task],conflicts:[conflict]})
    render(<TeamMemoryCenter api={api} projectID="project-team"/>)
    fireEvent.click(await screen.findByText('Architecture review'))
    expect(await screen.findByText(/Owner 页面不会加载或展示 Private Scratch/)).toBeInTheDocument()
    expect(screen.getByText(/Use SQLite/)).toBeInTheDocument()
    expect(get.mock.calls.every(([path])=>!String(path).includes('/private'))).toBe(true)
    fireEvent.click(screen.getByRole('button',{name:/提交任务关闭 Review/}))
    await waitFor(()=>expect(post.mock.calls.some(([path])=>String(path).includes('/close-proposal'))).toBe(true))
    expect(post.mock.calls.some(([path])=>String(path).includes('/revision-reviews/')&&String(path).includes('/decision'))).toBe(false)
  })

  it('creates a Team Task only with explicitly selected direct members and TTL',async()=>{
    const {api,post}=mockAPI()
    render(<TeamMemoryCenter api={api} projectID="project-team"/>)
    fireEvent.click(await screen.findByRole('button',{name:/新建 Team Task/}))
    fireEvent.change(screen.getByLabelText('任务名称'),{target:{value:'New team task'}})
    fireEvent.change(screen.getByLabelText(/任务 TTL/),{target:{value:'1800'}})
    fireEvent.click(screen.getByRole('checkbox',{name:/Agent A/}))
    fireEvent.click(screen.getByRole('button',{name:/创建 Team Task/}))
    await waitFor(()=>expect(post.mock.calls.some(([path])=>path==='/v1/team/tasks')).toBe(true))
    const call=post.mock.calls.find(([path])=>path==='/v1/team/tasks')
    expect(call?.[1]).toMatchObject({project_id:'project-team',title:'New team task',member_agent_ids:['agent-a'],ttl_seconds:1800})
  })

  it('submits Durable activation as a pending Review and never approves directly',async()=>{
    const {api,post}=mockAPI({durables:[durable]})
    render(<TeamMemoryCenter api={api} projectID="project-team"/>)
    fireEvent.click(await screen.findByText('Architecture durable'))
    expect(await screen.findByText(/Candidate 不进入默认项目召回/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:/提交激活 Review/}))
    await waitFor(()=>expect(post.mock.calls.some(([path])=>String(path).includes('/activation-proposal'))).toBe(true))
    expect(post.mock.calls.some(([path])=>String(path).includes('/revision-reviews/')&&String(path).includes('/decision'))).toBe(false)
  })
})
