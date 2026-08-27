import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AssetGovernanceDrawer } from './AssetGovernance'
import { APIClient } from './api'

afterEach(cleanup)

function detailWithValidation(status:string, checks:Array<Record<string,unknown>>) {
  const payload={asset_id:'asset-mcp',asset_type:'mcp',title:'MCP Contract',body:'MCP uses stdio transport and tool memory_search with permission boundaries.',source_memory_ids:['mem-1'],validation_status:status}
  return {
    asset:{asset_id:'asset-mcp',asset_type:'mcp',title:'MCP Contract',summary:'contract',status:'candidate',version:'3',risk_level:'medium',source_memory_ids:['mem-1'],validation_status:status,created_at:'2026-08-24T00:00:00Z',updated_at:'2026-08-24T00:00:00Z'},
    memories:[],
    governance:{
      object:{object_id:'obj-mcp',type_id:'builtin.agent-assets.governed-asset.v3',project_id:'project-1',status:'candidate',current_revision:1,revision:{object_id:'obj-mcp',revision:1,status:'candidate',payload,content_hash:'hash-current',confidence:1,importance:.8,plugin_id:'builtin.agent-assets',plugin_version:'2.0.0',created_at:'2026-08-24T00:00:00Z'}},
      revisions:[],
      reviews:[{review_id:'review-1',object_id:'obj-mcp',revision:2,base_revision:1,status:'pending',target_status:'active',requested_by:'owner',diff:{changed_fields:['body'],before:{body:'old'},after:{body:payload.body}},validation:{status,validator:'governed-agent-asset-v3/deterministic-2',checks},created_at:'2026-08-24T00:01:00Z',proposed_revision:{object_id:'obj-mcp',revision:2,status:'active',payload,content_hash:'hash-next',confidence:1,importance:.8,plugin_id:'builtin.agent-assets',plugin_version:'2.0.0',created_at:'2026-08-24T00:01:00Z'}}],
    },
  }
}
describe('Asset governance validation report',()=>{
  it('renders failed checks and disables activation',async()=>{
    const get=vi.fn(async()=>detailWithValidation('failed',[{name:'tool_recipe_dry_run',status:'failed',detail:'显式声明了未知 Memory Harness 工具',data:{mode:'catalog_only',unknown_tools:['memory_delete_everything']}}]))
    const post=vi.fn(async()=>({}))
    render(<AssetGovernanceDrawer api={{get,post} as unknown as APIClient} assetID="asset-mcp" onClose={()=>{}} />)
    fireEvent.click(await screen.findByRole('button',{name:/版本与审核/}))
    expect(await screen.findByText('显式声明了未知 Memory Harness 工具')).toBeInTheDocument()
    expect(screen.getByText(/memory_delete_everything/)).toBeInTheDocument()
    const approve=screen.getByRole('button',{name:'验证失败，不能激活'})
    expect(approve).toBeDisabled()
    fireEvent.click(approve)
    expect(post).not.toHaveBeenCalled()
  })

  it('shows warning and not-executed distinctly while preserving Owner review',async()=>{
    const checks=[
      {name:'normative_conflict_scan',status:'warning',detail:'发现相反极性规则',data:{conflicting_object_ids:['obj-rule-a']}},
      {name:'mcp_connectivity_probe',status:'not_executed',detail:'静态 Validator 不建立网络/进程连接',data:{mode:'no_side_effects'}},
    ]
    const get=vi.fn(async()=>detailWithValidation('passed',checks))
    const post=vi.fn(async()=>({}))
    render(<AssetGovernanceDrawer api={{get,post} as unknown as APIClient} assetID="asset-mcp" onClose={()=>{}} />)
    fireEvent.click(await screen.findByRole('button',{name:/版本与审核/}))
    expect(await screen.findByText('发现相反极性规则')).toBeInTheDocument()
    expect(screen.getByText('未执行')).toBeInTheDocument()
    expect(screen.getByText(/静态 Validator 不建立网络/)).toBeInTheDocument()
    const approve=screen.getByRole('button',{name:'批准并激活'})
    expect(approve).not.toBeDisabled()
    fireEvent.click(approve)
    await waitFor(()=>expect(post).toHaveBeenCalledWith('/v1/harness/revision-reviews/review-1/decision',{decision:'approve',note:'Owner 检查 Diff 与验证结果后批准'}))
  })
})
