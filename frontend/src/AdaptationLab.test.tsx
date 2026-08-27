import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AdaptationLab } from './AdaptationLab'
import { APIClient, HarnessObject } from './api'

afterEach(cleanup)

const baseHash='sha256:base'
const effectiveHash='sha256:overlay'
function object(id:string,type:string,status:string,payload:Record<string,unknown>,revision=1):HarnessObject{
  return {object_id:id,type_id:type,project_id:'project-ft4',status,protection_class:'protected',current_revision:revision,created_at:'2026-08-23T10:00:00Z',updated_at:'2026-08-23T10:00:00Z',revision:{payload,content_hash:`hash-${id}`,confidence:1,importance:.8,plugin_id:type.startsWith('builtin.adaptation')?'builtin.adaptation-lab':'builtin.experience-bank',plugin_version:'1.0.0',source_evidence_ids:[],source_object_ids:[],created_at:'2026-08-23T10:00:00Z'}}
}
function failedCase(){return object('case-fail','builtin.experience-bank.case.v1','active',{case_id:'case-fail',source_run_id:'run-fail',result:'fail',primary_failure_dimension:'task_correctness',diagnosis:'delivered context was not used correctly'})}
function proposal(status='candidate'){return object('proposal-1','builtin.adaptation-lab.change-proposal.v1',status,{proposal_id:'proposal-1',source_case_ids:['case-fail'],base_blueprint_hash:baseHash,effective_blueprint_hash:effectiveHash,patch:{role:'context.presentation-policy',config:{object:'verbatim'}},predicted_fix:'preserve explicit goal labels',predicted_regressions:['larger context'],evaluation_suite:['task-correctness','regression-check'],minimum_sample:2,stop_conditions:{max_regression_rate:.25,stop_on_safety_failure:true},proposer_id:'agent-evolver',verifier_id:status==='active'?'verifier-1':undefined,evaluation_object_ids:status==='active'?['eval-pre']:[],canary_scope:'case-only',overlay_ttl_seconds:3600})}
function overlay(status='active'){return object('overlay-1','builtin.adaptation-lab.case-overlay.v1',status,{overlay_id:'overlay-1',proposal_id:'proposal-1',base_blueprint_hash:baseHash,effective_blueprint_hash:effectiveHash,patch:{role:'context.presentation-policy',config:{object:'verbatim'}},permission_delta:[],ttl_seconds:3600,expires_at:'2099-08-23T10:00:00Z'},2)}

const preEval={evaluation_id:'eval-pre',evaluator_id:'evaluator-pre',verdict:'pass' as const,sample_size:2,baseline_ref:baseHash,challenger_ref:effectiveHash,dimensions:[{name:'conformance',verdict:'pass'}]}
const canaryEval={evaluation_id:'eval-canary',evaluator_id:'evaluator-canary',verdict:'fail' as const,sample_size:2,baseline_ref:baseHash,challenger_ref:effectiveHash,dimensions:[{name:'improvement',verdict:'unknown'},{name:'regression',verdict:'fail'}]}
function mockAPI(opts:{proposals?:HarnessObject[],overlays?:HarnessObject[],cases?:HarnessObject[],evaluations?:Array<typeof preEval | typeof canaryEval>}={}){
  const api=new APIClient({endpoint:'http://local',sessionID:'s',token:'t',csrf:'c',expiresAt:'',version:'2.0.0'})
  const proposals=opts.proposals||[],overlays=opts.overlays||[],cases=opts.cases||[]
  vi.spyOn(api,'get').mockImplementation(async path=>{
    if(path.startsWith('/v1/adaptation/proposals?'))return {proposals}
    if(path.startsWith('/v1/adaptation/overlays?'))return {overlays}
    if(path.startsWith('/v1/experience/cases?'))return {cases}
    if(path.startsWith('/v1/adaptation/proposals/')){const item=proposals.find(x=>path.endsWith(x.object_id))!;return {object:item,proposal:item.revision.payload,evaluations:opts.evaluations||[]}}
    if(path.startsWith('/v1/adaptation/overlays/')){const item=overlays.find(x=>path.endsWith(x.object_id))!;return {object:item,overlay:item.revision.payload}}
    throw new Error(`unexpected GET ${path}`)
  })
  const post=vi.spyOn(api,'post').mockImplementation(async(path)=>{
    if(path==='/v1/adaptation/proposals/dry-run')return {base_blueprint_hash:baseHash,effective_blueprint_hash:effectiveHash,target_role:'context.presentation-policy',base_config:{object:'summary'},effective_config:{object:'verbatim'},permission_delta:[],no_writes_performed:true}
    if(path==='/v1/adaptation/proposals')return proposal()
    if(path.includes('/canary'))return {run_id:'canary-run',status:'stopped_fallback_base',samples:2,improved_samples:0,regressed_samples:2,regression_rate:1,safety_failure:false,global_blueprint_unchanged:true}
    return {review_id:'review-1'}
  })
  return {api,post}
}

describe('AdaptationLab',()=>{
  it('requires zero-write Dry Run before creating a Change Proposal',async()=>{
    const {api,post}=mockAPI({cases:[failedCase()]})
    render(<AdaptationLab api={api} projectID="project-ft4"/>)
    const newProposal=await screen.findByRole('button',{name:/新建 Change Proposal/})
    fireEvent.click(newProposal)
    await screen.findByRole('checkbox',{name:/task_correctness/})
    const create=screen.getByRole('button',{name:/创建 Proposal Candidate/})
    expect(create).toBeDisabled()
    fireEvent.click(screen.getByRole('checkbox',{name:/task_correctness/}))
    fireEvent.click(screen.getByRole('button',{name:/Dry Run/}))
    expect(await screen.findByText('ZERO WRITE')).toBeInTheDocument()
    expect(create).not.toBeDisabled()
    fireEvent.click(create)
    await waitFor(()=>expect(post.mock.calls.some(([path])=>path==='/v1/adaptation/proposals')).toBe(true))
    const paths=post.mock.calls.map(([path])=>String(path))
    expect(paths.indexOf('/v1/adaptation/proposals/dry-run')).toBeLessThan(paths.indexOf('/v1/adaptation/proposals'))
  })

  it('submits proposal approval as a review request and never approves directly',async()=>{
    const candidate=proposal('candidate')
    const {api,post}=mockAPI({proposals:[candidate],cases:[failedCase()],evaluations:[preEval]})
    render(<AdaptationLab api={api} projectID="project-ft4"/>)
    fireEvent.click(await screen.findByText('preserve explicit goal labels'))
    fireEvent.click(await screen.findByRole('button',{name:/提交 Owner Review/}))
    fireEvent.change(screen.getByLabelText('Verifier ID'),{target:{value:'verifier-independent'}})
    fireEvent.click(screen.getByRole('button',{name:/创建待审核 Revision/}))
    await waitFor(()=>expect(post.mock.calls.some(([path])=>String(path).includes('/approval-proposal'))).toBe(true))
    expect(post.mock.calls.some(([path])=>String(path).includes('/revision-reviews/')&&String(path).includes('/decision'))).toBe(false)
  })

  it('feeds Canary only evaluations that contain an explicit regression dimension',async()=>{
    const activeProposal=proposal('active'),activeOverlay=overlay('active')
    const {api,post}=mockAPI({proposals:[activeProposal],overlays:[activeOverlay],evaluations:[preEval,canaryEval]})
    render(<AdaptationLab api={api} projectID="project-ft4"/>)
    const overlayText=await screen.findByText(/Base sha256:base/)
    fireEvent.click(overlayText.closest('button')!)
    fireEvent.click(await screen.findByRole('button',{name:/运行 Canary/}))
    await waitFor(()=>expect(post.mock.calls.some(([path])=>String(path).endsWith('/canary'))).toBe(true))
    const call=post.mock.calls.find(([path])=>String(path).endsWith('/canary'))
    expect(call?.[1]).toMatchObject({verifier_id:'verifier-1',evaluation_object_ids:['eval-canary']})
    expect(await screen.findByText('stopped_fallback_base')).toBeInTheDocument()
    expect(screen.getByText('UNCHANGED')).toBeInTheDocument()
  })
})
