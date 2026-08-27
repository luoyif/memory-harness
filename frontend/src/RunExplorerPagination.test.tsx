import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RunsPage } from './App'
import { APIClient, HarnessRun, RunDetail } from './api'

afterEach(cleanup)

function run(index:number):HarnessRun{
  return {run_id:`run-${index}`,project_id:'project-runs',caller_type:'owner',caller_id:'owner',channel:'desktop',pipeline_id:`pipeline-${index}`,pipeline_version:'1.0.0',pipeline_hash:`hash-${index}`,status:'completed',created_at:`2026-08-24T01:${String(index%60).padStart(2,'0')}:00Z`}
}

function detail(id:string):RunDetail{
  return {run:{...run(999),run_id:id,pipeline_id:'pipeline-requested'},spans:[],events:[],effects:[],stage_outputs:[]}
}

function mockAPI(total=120){
  const api=new APIClient({endpoint:'http://local',sessionID:'s',token:'t',csrf:'c',expiresAt:'',version:'2.0.0'})
  const get=vi.spyOn(api,'get').mockImplementation(async path=>{
    if(path.includes('/v1/harness/runs?')){
      const url=new URL(`http://local${path}`)
      const offset=Number(url.searchParams.get('offset')||0)
      const limit=Number(url.searchParams.get('limit')||50)
      const items=Array.from({length:Math.max(0,Math.min(limit,total-offset))},(_,i)=>run(offset+i))
      return {runs:items,total,limit,offset,has_more:offset+items.length<total}
    }
    if(path.startsWith('/v1/harness/runs/'))return detail(path.split('/').pop()!)
    throw new Error(`unexpected GET ${path}`)
  })
  return {api,get}
}
describe('Run Explorer pagination',()=>{
  it('loads 50 runs first and appends the next page on demand',async()=>{
    const {api,get}=mockAPI(120)
    render(<RunsPage api={api} projectID="project-runs" requestedRun=""/>)
    expect(await screen.findByText('50 / 120 次运行')).toBeInTheDocument()
    const more=screen.getByRole('button',{name:/加载更多运行记录/})
    fireEvent.click(more)
    expect(await screen.findByText('100 / 120 次运行')).toBeInTheDocument()
    expect(get.mock.calls.some(([path])=>String(path).includes('limit=50&offset=50'))).toBe(true)
    expect(screen.getByRole('button',{name:/加载更多运行记录/})).toHaveTextContent('100 / 120')
  })

  it('opens a requested Run detail even when that Run is outside the first page',async()=>{
    const {api,get}=mockAPI(1000)
    render(<RunsPage api={api} projectID="project-runs" requestedRun="run-outside-page"/>)
    expect(await screen.findByText('50 / 1000 次运行')).toBeInTheDocument()
    expect(await screen.findByRole('heading',{name:'pipeline-requested'})).toBeInTheDocument()
    await waitFor(()=>expect(get.mock.calls.some(([path])=>String(path)==='/v1/harness/runs/run-outside-page')).toBe(true))
    expect(get.mock.calls.some(([path])=>String(path).includes('limit=50&offset=0'))).toBe(true)
  })
})

it('shows provider-reported Run Health without inventing pricing',async()=>{
  const {api,get}=mockAPI(20)
  get.mockImplementation(async path=>{
    if(path.includes('/v1/harness/runs?')){
      const url=new URL(`http://local${path}`); const offset=Number(url.searchParams.get('offset')||0); const limit=Number(url.searchParams.get('limit')||50)
      const items=Array.from({length:Math.max(0,Math.min(limit,20-offset))},(_,i)=>run(offset+i))
      return {runs:items,total:20,limit,offset,has_more:false}
    }
    if(path==='/v1/harness/runs/run-health-ui')return {
      ...detail('run-health-ui'),
      model_health:{calls:2,successful_calls:1,failed_calls:1,provider_reported_calls:2,prompt_tokens:220,completion_tokens:80,total_tokens:300,reasoning_tokens:10,cached_prompt_tokens:40,total_latency_ms:1800,max_latency_ms:1200,estimated_cost_microminor:0,cost_status:'unavailable'},
      model_calls:[
        {call_id:'call-1',run_id:'run-health-ui',node_id:'compile',project_id:'project-runs',stage_type:'memory.compile',provider_id:'p1',provider:'openai_compatible',model:'test-model',status:'success',usage_source:'provider_reported',prompt_tokens:120,completion_tokens:30,total_tokens:150,reasoning_tokens:7,cached_prompt_tokens:20,latency_ms:600,pricing_source:'unavailable',created_at:'2026-08-24T01:00:00Z'},
        {call_id:'call-2',run_id:'run-health-ui',node_id:'compile',project_id:'project-runs',stage_type:'memory.compile',provider_id:'p1',provider:'openai_compatible',model:'test-model',status:'failed',usage_source:'provider_reported',prompt_tokens:100,completion_tokens:50,total_tokens:150,latency_ms:1200,pricing_source:'unavailable',error_code:'output_contract_error',created_at:'2026-08-24T01:00:01Z'}
      ]
    }
    throw new Error(`unexpected GET ${path}`)
  })
  render(<RunsPage api={api} projectID="project-runs" requestedRun="run-health-ui"/>)
  expect(await screen.findByText('300')).toBeInTheDocument()
  expect(screen.getByText('未配置定价')).toBeInTheDocument()
  expect(screen.getByText('不会自动猜测价格')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button',{name:/模型调用 2/}))
  expect(await screen.findByText('output_contract_error')).toBeInTheDocument()
})

it('explains completed-with-warnings asset refinement in ordinary language',async()=>{
  const {api,get}=mockAPI(1)
  get.mockImplementation(async path=>{
    if(path.includes('/v1/harness/runs?'))return {runs:[run(1)],total:1,limit:50,offset:0,has_more:false}
    if(path==='/v1/harness/runs/run-warning-ui')return {
      ...detail('run-warning-ui'),
      run:{...detail('run-warning-ui').run,status:'completed_with_warnings',pipeline_id:'builtin.agent-assets.template-refinement'},
      spans:[{span_id:'span-1',node_id:'refine-skill-01',stage_type:'asset.template_refine',status:'completed',started_at:'2026-08-24T01:00:00Z',ended_at:'2026-08-24T01:00:10Z',detail:{result:{failed:0,used_model:false,model_error:'model did not return one bounded JSON value matching the required schema'}}}]
    }
    throw new Error(`unexpected GET ${path}`)
  })
  render(<RunsPage api={api} projectID="project-runs" requestedRun="run-warning-ui"/>)
  const warning=await screen.findByRole('alert')
  expect(warning).toHaveTextContent('结果没有全部成功')
  expect(warning).toHaveTextContent('1 个批次只保存了待补齐骨架')
  expect(warning).toHaveTextContent('模型没有返回符合模板的标准结果')
})
