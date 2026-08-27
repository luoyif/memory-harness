import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ConnectionsPage } from './App'
import { APIClient } from './api'

afterEach(()=>{cleanup();vi.restoreAllMocks()})

function modelPayload(){return {
  runtime:{mode:'agent',active_provider_id:'provider-1',fallback_to_rules:true},
  providers:[{provider_id:'provider-1',name:'Local Model',kind:'openai_compatible',protocol:'openai_chat',base_url:'http://127.0.0.1:11434/v1',model:'model-a',status:'ready',enabled:true,has_secret:false,pricing:{configured:false}}],
  presets:[
    {preset_id:'openai-responses',kind:'openai',name:'OpenAI · Responses',protocol:'openai_responses',base_url:'https://api.openai.com/v1',example_model:'gpt-5.6',description:'OpenAI 新接口'},
    {preset_id:'opencode-go',kind:'opencode_go',name:'OpenCode Go',protocol:'openai_responses',base_url:'https://opencode.ai/zen/go/v1',example_model:'gpt-5.6-luna',description:'自动匹配不同模型的调用方式'},
  ],
  model_catalog:[
    {provider_kind:'opencode_go',model_id:'gpt-5.6-luna',name:'GPT 5.6 Luna',protocol:'openai_responses',input:['text'],best_for:'通用编码与整理',source:'OpenCode Go'},
    {provider_kind:'opencode_go',model_id:'minimax-m3',name:'MiniMax M3',protocol:'anthropic_messages',input:['text'],best_for:'长上下文编码与沉淀',source:'OpenCode Go'},
  ],
  model_catalog_updated_at:'2026-08-25',
  model_catalog_notice:'内置目录只帮助选择正确调用方式。',
  usage:{window_hours:24,generated_at:'2026-08-24T06:00:00Z',
    health:{calls:3,successful_calls:2,failed_calls:1,provider_reported_calls:2,prompt_tokens:300,completion_tokens:120,total_tokens:420,reasoning_tokens:10,cached_prompt_tokens:30,total_latency_ms:1500,max_latency_ms:900,estimated_cost_microminor:0,cost_status:'unavailable'},
    providers:[{provider_id:'provider-1',provider:'openai_compatible',model:'model-a',last_call_at:'2026-08-24T05:59:00Z',health:{calls:3,successful_calls:2,failed_calls:1,provider_reported_calls:2,prompt_tokens:300,completion_tokens:120,total_tokens:420,reasoning_tokens:10,cached_prompt_tokens:30,total_latency_ms:1500,max_latency_ms:900,estimated_cost_microminor:0,cost_status:'unavailable'}}]},
  privacy_notice:'Provider receives selected Evidence only in Agent mode.',secret_store:'test',secret_persistent:false
}}

function mockAPI(){
  const api=new APIClient({endpoint:'http://local',sessionID:'s',token:'t',csrf:'c',expiresAt:'',version:'2.0.0'})
  vi.spyOn(api,'get').mockImplementation(async path=>{
    if(path==='/v1/model/config')return modelPayload()
    if(path==='/v1/agents')return {agents:[],allowed_permissions:[]}
    if(path==='/v1/integrations/capabilities')return {tools:[],command:''}
    if(path==='/v1/sources')return {sources:[]}
    throw new Error(`unexpected GET ${path}`)
  })
  const post=vi.spyOn(api,'post').mockResolvedValue({})
  const put=vi.spyOn(api,'put').mockResolvedValue({})
  return {api,post,put}
}

describe('Model Center health',()=>{
  it('shows provider-reported health without inventing cost',async()=>{
    const {api}=mockAPI()
    render(<ConnectionsPage api={api} projects={[]}/>)
    expect(await screen.findByText('420')).toBeInTheDocument()
    expect(screen.getByText('1 次失败')).toBeInTheDocument()
    expect(screen.getByText('2/3 次含 usage')).toBeInTheDocument()
    expect(screen.getByText('未配置定价')).toBeInTheDocument()
    expect(screen.getByText('未自动猜测价格')).toBeInTheDocument()
    expect(screen.getByText('未配置定价 · 不估算费用')).toBeInTheDocument()
    expect(screen.getByText(/模型知识更新于 2026-08-25/)).toBeInTheDocument()
  })

  it('submits only Owner-entered pricing with a new provider',async()=>{
    const {api,post}=mockAPI()
    render(<ConnectionsPage api={api} projects={[]}/>)
    fireEvent.click(await screen.findByRole('button',{name:'添加模型'}))
    fireEvent.change(screen.getByLabelText('显示名称'),{target:{value:'Priced Model'}})
    fireEvent.change(screen.getByLabelText(/输入单价/),{target:{value:'300'}})
    fireEvent.change(screen.getByLabelText(/输出单价/),{target:{value:'1200'}})
    fireEvent.click(screen.getByRole('button',{name:'保存提供方'}))
    await waitFor(()=>expect(post).toHaveBeenCalled())
    const call=post.mock.calls.find(([path])=>path==='/v1/model/providers')
    expect(call).toBeTruthy()
    const body=call?.[1] as Record<string,unknown>
    expect(body.pricing).toEqual({currency:'USD',input_per_million_minor:300,output_per_million_minor:1200})
  })
  it('lets Owner add pricing to an existing provider without replacing its secret',async()=>{
    const {api,put}=mockAPI()
    render(<ConnectionsPage api={api} projects={[]}/>)
    fireEvent.click(await screen.findByRole('button',{name:'编辑'}))
    expect(screen.getByRole('heading',{name:'编辑模型提供方'})).toBeInTheDocument()
    expect(screen.getByDisplayValue('Local Model')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText(/输入单价/),{target:{value:'250'}})
    fireEvent.change(screen.getByLabelText(/输出单价/),{target:{value:'900'}})
    fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
    await waitFor(()=>expect(put).toHaveBeenCalledTimes(1))
    expect(put.mock.calls[0][0]).toBe('/v1/model/providers/provider-1')
    const body=put.mock.calls[0][1] as Record<string,unknown>
    expect(body.api_key).toBe('')
    expect(body.pricing).toEqual({currency:'USD',input_per_million_minor:250,output_per_million_minor:900})
  })

  it('reports the provider live model count after a connection test',async()=>{
    const {api,post}=mockAPI()
    post.mockResolvedValueOnce({models:['model-a','model-b'],selected_model_found:true})
    render(<ConnectionsPage api={api} projects={[]}/>)
    fireEvent.click(await screen.findByRole('button',{name:'测试'}))
    expect(await screen.findByText(/当前账号返回 2 个模型，已找到所选模型/)).toBeInTheDocument()
  })

  it('uses model knowledge to select the OpenCode Go protocol',async()=>{
    const {api,post}=mockAPI()
    render(<ConnectionsPage api={api} projects={[]}/>)
    fireEvent.click(await screen.findByRole('button',{name:'添加模型'}))
    fireEvent.click(screen.getByRole('button',{name:/OpenCode Go/}))
    expect(screen.getByDisplayValue('https://opencode.ai/zen/go/v1')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('模型'),{target:{value:'minimax-m3'}})
    expect(screen.getByLabelText('调用方式')).toHaveValue('anthropic_messages')
    expect(screen.getByText('长上下文编码与沉淀')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:'保存提供方'}))
    await waitFor(()=>expect(post).toHaveBeenCalled())
    const call=post.mock.calls.find(([path])=>path==='/v1/model/providers')
    expect(call?.[1]).toMatchObject({kind:'opencode_go',protocol:'anthropic_messages',base_url:'https://opencode.ai/zen/go/v1',model:'minimax-m3'})
  })

})
