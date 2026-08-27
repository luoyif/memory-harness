package builtin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/luoyif/memory-harness/internal/adaptation"
	"github.com/luoyif/memory-harness/internal/assettemplate"
	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/experience"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/pipeline"
	"github.com/luoyif/memory-harness/internal/plugins"
	"github.com/luoyif/memory-harness/internal/portablebundle"
	"github.com/luoyif/memory-harness/internal/teammemory"
)

type Services struct {
	Harness    *harness.Service
	Pipelines  *pipeline.Service
	Plugins    *plugins.Service
	Blueprints *blueprint.Service
}

type typeSpec struct {
	pluginID string
	typeID   string
	name     string
	schema   string
	states   []string
	protect  string
}

var builtinTypes = []typeSpec{
	{"builtin.core-memory-growth", "builtin.core-memory-growth.knowledge-point", "知识点", `{"type":"object","required":["statement"],"properties":{"statement":{"type":"string","maxLength":4000},"kind":{"type":"string"},"scope":{"type":"string"}},"additionalProperties":false}`, []string{"candidate", "active", "superseded", "rejected"}, "standard"},
	{"builtin.core-memory-growth", memory.StructuredKnowledgeUnitTypeV2, "结构化知识单元 v2", `{"type":"object","required":["unit_id","episode_id","evidence_id","unit_type","tier_hint","statement","normalized_key","confidence","risk_tier","status","scopes","observed_at","created_at","schema_version","structure"],"properties":{"unit_id":{"type":"string","maxLength":200},"episode_id":{"type":"string","maxLength":200},"evidence_id":{"type":"string","maxLength":240},"unit_type":{"type":"string","enum":["fact","state","event","decision","goal","risk","outcome","procedure","identity","correction"]},"tier_hint":{"type":"string","enum":["episodic","semantic","procedural","identity_core"]},"statement":{"type":"string","minLength":1,"maxLength":4000},"normalized_key":{"type":"string","maxLength":4000},"confidence":{"type":"number","minimum":0,"maximum":1},"risk_tier":{"type":"string","enum":["A","B","C","D"]},"status":{"type":"string","maxLength":80},"scopes":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":240}},"observed_at":{"type":"string","maxLength":80},"created_at":{"type":"string","maxLength":80},"processed_at":{"type":"string","maxLength":80},"schema_version":{"type":"string","enum":["memory-harness.knowledge-unit/v2"]},"structure":{"type":"object"}},"additionalProperties":false}`, []string{"active", "superseded", "rejected"}, "standard"},
	{"builtin.core-memory-growth", memory.StructuredMemoryRecordTypeV1, "结构化长期记忆 v1", `{"type":"object","required":["memory_id","tier","asset_form","domain","status","summary","body","canonical_key","confidence","importance","strength","source_evidence_ids","source_episode_ids","scopes","visibility","observed_at","created_at","updated_at"],"properties":{"memory_id":{"type":"string","maxLength":200},"tier":{"type":"string","enum":["episodic","semantic","procedural","identity_core"]},"asset_form":{"type":"string","maxLength":120},"domain":{"type":"string","maxLength":240},"status":{"type":"string","maxLength":80},"summary":{"type":"string","maxLength":1200},"body":{"type":"string","minLength":1,"maxLength":12000},"canonical_key":{"type":"string","maxLength":12000},"confidence":{"type":"number","minimum":0,"maximum":1},"importance":{"type":"number","minimum":0,"maximum":1},"strength":{"type":"number","minimum":0},"source_evidence_ids":{"type":"array","maxItems":500,"items":{"type":"string","maxLength":240}},"source_episode_ids":{"type":"array","maxItems":500,"items":{"type":"string","maxLength":240}},"scopes":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":240}},"visibility":{"type":"string","maxLength":80},"observed_at":{"type":"string","maxLength":80},"created_at":{"type":"string","maxLength":80},"updated_at":{"type":"string","maxLength":80},"last_reinforced_at":{"type":"string","maxLength":80}},"additionalProperties":false}`, []string{"active", "superseded", "rejected"}, "protected"},
	{"builtin.core-memory-growth", "builtin.core-memory-growth.episodic", "情景记忆", `{"type":"object","required":["summary"],"properties":{"summary":{"type":"string","maxLength":12000},"outcome":{"type":"string"},"participants":{"type":"array","maxItems":100,"items":{"type":"string"}}},"additionalProperties":false}`, []string{"candidate", "active", "superseded", "rejected"}, "standard"},
	{"builtin.core-memory-growth", "builtin.core-memory-growth.semantic", "语义记忆", `{"type":"object","required":["statement"],"properties":{"statement":{"type":"string","maxLength":8000},"domain":{"type":"string"}},"additionalProperties":false}`, []string{"candidate", "active", "superseded", "rejected"}, "standard"},
	{"builtin.core-memory-growth", "builtin.core-memory-growth.procedural", "程序记忆", `{"type":"object","required":["procedure"],"properties":{"procedure":{"type":"string","maxLength":16000},"trigger":{"type":"string"}},"additionalProperties":false}`, []string{"candidate", "review_required", "active", "superseded", "rejected"}, "protected"},
	{"builtin.core-memory-growth", "builtin.core-memory-growth.identity", "身份记忆", `{"type":"object","required":["statement"],"properties":{"statement":{"type":"string","maxLength":8000},"stability":{"type":"string"}},"additionalProperties":false}`, []string{"candidate", "review_required", "active", "superseded", "rejected"}, "protected"},
	{"builtin.living-asset-vault", "builtin.living-asset-vault.document", "生长文档", `{"type":"object","required":["title","summary"],"properties":{"title":{"type":"string","maxLength":240},"summary":{"type":"string","maxLength":8000},"format":{"type":"string"}},"additionalProperties":false}`, []string{"candidate", "active", "superseded", "archived"}, "standard"},
	{"builtin.living-asset-vault", harness.KnowledgeProductTypeV1, "用户知识产品 v1", `{"type":"object","required":["product_id","product_type","title","summary","body","source_refs","locked_fields","generation_status"],"properties":{"product_id":{"type":"string","maxLength":200},"product_type":{"type":"string","enum":["report","diary","personal_capability","personal_profile","project_brief","decision_log","risk_report"]},"title":{"type":"string","maxLength":240},"summary":{"type":"string","maxLength":4000},"body":{"type":"string","maxLength":48000},"format":{"type":"string","enum":["markdown","plain_text"]},"source_refs":{"type":"array","maxItems":500,"items":{"type":"string"}},"locked_fields":{"type":"array","maxItems":10,"items":{"type":"string","enum":["title","summary","body"]}},"generation_status":{"type":"string","enum":["auto","human_mixed","human"]}},"additionalProperties":false}`, []string{"active", "superseded", "archived"}, "standard"},
	{"builtin.living-asset-vault", harness.ProfileProjectionTypeV1, "Profile Projection v1", `{"type":"object","required":["profile_id","view_kind","profile_class","title","summary","blocks","locked_block_ids","generation_status","generated_from_revision","generated_at"],"properties":{"profile_id":{"type":"string","maxLength":200},"view_kind":{"type":"string","enum":["owner_identity","agent_identity","stable_preference","dynamic_project","relationship","session_resume"]},"profile_class":{"type":"string","enum":["static","dynamic"]},"title":{"type":"string","maxLength":240},"summary":{"type":"string","maxLength":4000},"blocks":{"type":"array","maxItems":80,"items":{"type":"object","required":["block_id","label","content","source_refs","source_object_ids","source_hash","last_verified_at","confidence","locked","review_status"],"properties":{"block_id":{"type":"string","maxLength":200},"label":{"type":"string","maxLength":240},"content":{"type":"string","maxLength":12000},"source_refs":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},"source_object_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},"source_hash":{"type":"string","maxLength":128},"candidate_source_hash":{"type":"string","maxLength":128},"valid_from":{"type":"string","maxLength":80},"valid_until":{"type":"string","maxLength":80},"last_verified_at":{"type":"string","maxLength":80},"confidence":{"type":"number","minimum":0,"maximum":1},"locked":{"type":"boolean"},"review_status":{"type":"string","enum":["current","stale_locked"]}},"additionalProperties":false}},"locked_block_ids":{"type":"array","maxItems":80,"items":{"type":"string","maxLength":200}},"generation_status":{"type":"string","enum":["auto","human_mixed"]},"generated_from_revision":{"type":"integer","minimum":0},"generated_at":{"type":"string","maxLength":80}},"additionalProperties":false}`, []string{"active", "superseded", "archived"}, "standard"},
	{experience.PluginID, experience.EvaluationTypeV1, "Evaluation Object v1", experience.EvaluationSchemaV1, []string{"active", "superseded", "archived"}, "protected"},
	{experience.PluginID, experience.EvaluationTypeV2, "Evaluation Object v2", experience.EvaluationSchemaV2, []string{"active", "superseded", "archived"}, "protected"},
	{experience.PluginID, experience.CaseTypeV1, "Experience Case v1", experience.CaseSchemaV1, []string{"candidate", "active", "rejected", "superseded", "archived"}, "protected"},
	{experience.PluginID, experience.PatternTypeV1, "Experience Pattern v1", experience.PatternSchemaV1, []string{"candidate", "active", "rejected", "superseded", "archived"}, "protected"},
	{adaptation.PluginID, adaptation.ChangeProposalTypeV1, "Harness Change Proposal v1", adaptation.ChangeProposalSchemaV1, []string{"candidate", "active", "rejected", "superseded", "archived"}, "protected"},
	{adaptation.PluginID, adaptation.CaseOverlayTypeV1, "Run-scoped Case Overlay v1", adaptation.CaseOverlaySchemaV1, []string{"candidate", "active", "rejected", "superseded", "archived"}, "protected"},
	{portablebundle.PluginID, portablebundle.ImportCandidateTypeV1, "Portable Import Candidate v1", portablebundle.ImportCandidateSchemaV1, []string{"candidate", "rejected", "archived"}, "protected"},
	{teammemory.PluginID, teammemory.TaskTypeV1, "Team Task v1", teammemory.TaskSchemaV1, []string{"active", "closed", "archived"}, "protected"},
	{teammemory.PluginID, teammemory.PrivateScratchTypeV1, "Private Scratchpad v1", teammemory.PrivateScratchSchemaV1, []string{"active", "expired", "archived"}, "standard"},
	{teammemory.PluginID, teammemory.BlackboardEntryTypeV1, "Task Blackboard Entry v1", teammemory.BlackboardEntrySchemaV1, []string{"active", "expired", "archived"}, "standard"},
	{teammemory.PluginID, teammemory.ConflictTypeV1, "Team Conflict v1", teammemory.ConflictSchemaV1, []string{"candidate", "active", "resolved", "archived"}, "protected"},
	{teammemory.PluginID, teammemory.ProjectDurableTypeV1, "Project Durable Team Memory v1", teammemory.ProjectDurableSchemaV1, []string{"candidate", "active", "superseded", "archived"}, "protected"},
	{"builtin.agent-assets", "builtin.agent-assets.asset", "Agent 能力资产", `{"type":"object","required":["asset_type","title","body"],"properties":{"asset_type":{"type":"string","enum":["prompt","skill","rule","constraint","procedure","mcp","tool_recipe"]},"title":{"type":"string"},"body":{"type":"string","maxLength":32000}},"additionalProperties":false}`, []string{"candidate", "review_required", "active", "superseded", "rolled_back"}, "protected"},
	{"builtin.agent-assets", harness.GovernedAgentAssetTypeV2, "受治理能力资产 v2", `{"type":"object","required":["asset_id","asset_type","title","body","source_memory_ids"],"properties":{"asset_id":{"type":"string","maxLength":200},"asset_type":{"type":"string","enum":["policy","skill","routine","factset"]},"title":{"type":"string","maxLength":240},"body":{"type":"string","maxLength":32000},"source_memory_ids":{"type":"array","maxItems":100,"items":{"type":"string"}},"validation_status":{"type":"string"}},"additionalProperties":false}`, []string{"candidate", "review_required", "active", "superseded", "rolled_back"}, "protected"},
	{"builtin.agent-assets", harness.GovernedAgentAssetTypeV3, "受治理能力资产 v3", `{"type":"object","required":["asset_id","asset_type","title","body","source_memory_ids","validation_status"],"properties":{"asset_id":{"type":"string","maxLength":200},"asset_type":{"type":"string","enum":["prompt","skill","rule","constraint","procedure","tool_recipe","mcp"]},"title":{"type":"string","maxLength":240},"body":{"type":"string","maxLength":32000},"source_memory_ids":{"type":"array","maxItems":100,"items":{"type":"string"}},"validation_status":{"type":"string","enum":["not_run","passed","failed","manual_passed"]}},"additionalProperties":false}`, []string{"candidate", "review_required", "active", "superseded", "rolled_back"}, "protected"},
	{"builtin.agent-assets", harness.GovernedAgentAssetTypeV4, "模板化能力资产 v4", assettemplate.SchemaV4, []string{"candidate", "review_required", "active", "superseded", "rolled_back"}, "protected"},
	{"builtin.opportunity-intelligence", "builtin.opportunity-intelligence.need", "客户需求", `{"type":"object","required":["title","evidence"],"properties":{"title":{"type":"string"},"evidence":{"type":"string"},"customer":{"type":"string"}},"additionalProperties":false}`, []string{"candidate", "qualified", "rejected", "superseded"}, "standard"},
	{"builtin.opportunity-intelligence", "builtin.opportunity-intelligence.opportunity", "商机", `{"type":"object","required":["title","score","next_action"],"properties":{"title":{"type":"string"},"score":{"type":"integer","minimum":0,"maximum":100},"next_action":{"type":"string"},"currency":{"type":"string"},"value_minor":{"type":"integer"}},"additionalProperties":false}`, []string{"candidate", "review_required", "qualified", "won", "lost", "superseded"}, "protected"},
}

var builtinPlugins = []plugins.BuiltinSpec{
	{PluginID: "builtin.memory-harness-core", Version: "2.0.0", Name: "流程运行核心", Permissions: pipeline.OwnerCapabilities()},
	{PluginID: "builtin.user-workflows", Version: "2.0.0", Name: "我的流程", Permissions: pipeline.OwnerCapabilities()},
	{PluginID: "builtin.core-memory-growth", Version: "2.0.0", Name: "核心记忆生长", Permissions: []string{"evidence.read", "memory.propose", "memory.materialize", "model.invoke"}},
	{PluginID: "builtin.project-operations", Version: "2.0.0", Name: "项目经营", Permissions: []string{"project.read", "project.write", "memory.read"}},
	{PluginID: "builtin.opportunity-intelligence", Version: "2.0.0", Name: "商机智能", Permissions: []string{"memory.read", "memory.propose", "memory.materialize", "project.write", "model.invoke"}},
	{PluginID: "builtin.business-finance", Version: "2.0.0", Name: "经营资金", Permissions: []string{"finance.read", "finance.write", "project.read"}},
	{PluginID: "builtin.agent-assets", Version: "2.0.0", Name: "Agent 能力资产", Permissions: []string{"asset.read", "asset.propose", "asset.activate", "memory.read"}},
	{PluginID: "builtin.conversation-connectors", Version: "2.0.0", Name: "会话连接器", Permissions: []string{"evidence.capture", "connector.invoke"}},
	{PluginID: "builtin.living-asset-vault", Version: "2.0.0", Name: "生长资产库", Permissions: []string{"memory.read", "memory.materialize", "asset.read"}},
	{PluginID: experience.PluginID, Version: experience.PluginVersion, Name: "经验与评估", Permissions: []string{"trace.read_payload", "memory.read", "memory.materialize"}},
	{PluginID: adaptation.PluginID, Version: adaptation.PluginVersion, Name: "受治理适配实验室", Permissions: []string{"trace.read_payload", "memory.read", "memory.materialize", "project.read"}},
	{PluginID: portablebundle.PluginID, Version: portablebundle.PluginVersion, Name: "可迁移记忆包", Permissions: []string{"evidence.read", "evidence.capture", "memory.read", "memory.materialize", "project.read"}},
	{PluginID: teammemory.PluginID, Version: teammemory.PluginVersion, Name: "多 Agent 团队记忆", Permissions: []string{"team.private", "team.blackboard.read", "team.blackboard.write", "team.blackboard.share", "memory.materialize", "project.read"}},
	{PluginID: "builtin.palace-organization", Version: "1.0.0", Name: "宫殿组织策略", Permissions: []string{"evidence.read", "memory.read"}},
	{PluginID: "builtin.progressive-recall", Version: "1.0.0", Name: "渐进召回", Permissions: []string{"memory.read"}},
	{PluginID: "builtin.context-policy", Version: "1.0.0", Name: "上下文编译策略", Permissions: []string{"memory.read", "project.read"}},
	{PluginID: "builtin.hybrid-retrieval", Version: "1.0.0", Name: "混合深度检索", Permissions: []string{"memory.read", "model.invoke"}},
	{PluginID: "builtin.dsh-bridge", Version: "0.1.0", Name: "DeepSeek Harness Bridge", Permissions: []string{"memory.read", "evidence.capture", "connector.invoke"}, Status: "experimental"},
}

var builtinStrategyComponents = map[string][]plugins.StrategyComponentContribution{
	"builtin.core-memory-growth": {
		{ComponentID: "builtin.core-memory-growth.evidence", Version: "1.0.0", DisplayName: "不可变 Evidence", Description: "先保存原文与来源，再允许任何派生。", Role: "growth.evidence", Kind: "policy", Capabilities: []string{"evidence.read"}},
		{ComponentID: "builtin.core-memory-growth.knowledge-point", Version: "1.0.0", DisplayName: "知识点", Description: "从来源抽样出的最小事实、意图与约束。", Role: "growth.knowledge", Kind: "memory_type", Capabilities: []string{"memory.materialize"}},
		{ComponentID: "builtin.core-memory-growth.episode", Version: "1.0.0", DisplayName: "情景复盘", Description: "把一次经历的目标、行动和结果连成 Episode。", Role: "growth.episode", Kind: "memory_type", Capabilities: []string{"memory.materialize"}},
		{ComponentID: "builtin.core-memory-growth.long-term", Version: "1.0.0", DisplayName: "长期记忆", Description: "形成语义、程序、身份等可治理记忆。", Role: "growth.long-term", Kind: "memory_type", Capabilities: []string{"memory.materialize"}},
	},
	"builtin.living-asset-vault": {
		{ComponentID: "builtin.living-asset-vault.growth", Version: "1.0.0", DisplayName: "生长知识", Description: "由持续证据修订的可读资产。", Role: "growth.living", Kind: "memory_type", Capabilities: []string{"memory.read", "memory.materialize"}},
	},
	"builtin.agent-assets": {
		{ComponentID: "builtin.agent-assets.activation", Version: "1.0.0", DisplayName: "Agent 能力资产", Description: "经验证和审核后供 Agent 使用。", Role: "growth.agent-asset", Kind: "memory_type", Capabilities: []string{"memory.read", "asset.propose"}},
	},
	"builtin.palace-organization": {
		{ComponentID: "builtin.palace-organization.project-scope", Version: "1.0.0", DisplayName: "Project / Wing", Description: "先以项目或人物形成硬隔离范围。", Role: "organization.scope", Kind: "provider", Capabilities: []string{"evidence.read"}},
		{ComponentID: "builtin.palace-organization.topic-room", Version: "1.0.0", DisplayName: "Topic / Room", Description: "在范围内按主题、模块或子项目定位。", Role: "organization.topic", Kind: "provider", Capabilities: []string{"memory.read"}},
		{ComponentID: "builtin.palace-organization.concept-hall", Version: "1.0.0", DisplayName: "Concept / Hall", Description: "用实体、事件和概念建立交叉指针。", Role: "organization.concept", Kind: "provider", Capabilities: []string{"memory.read"}},
		{ComponentID: "builtin.palace-organization.evidence-drawer", Version: "1.0.0", DisplayName: "Evidence / Drawer", Description: "指针最终回到可验证的原始内容。", Role: "organization.evidence", Kind: "provider", Capabilities: []string{"evidence.read"}},
	},
	"builtin.progressive-recall": {
		{ComponentID: "builtin.progressive-recall.identity", Version: "1.0.0", DisplayName: "L0 身份", Description: "固定加载最小身份与边界。", Role: "recall.identity", Kind: "policy", Capabilities: []string{"memory.read"}},
		{ComponentID: "builtin.progressive-recall.essential", Version: "1.0.0", DisplayName: "L1 核心上下文", Description: "加载当前项目最重要且已审核的背景。", Role: "recall.essential", Kind: "stage", StageType: "memory.retrieve", Capabilities: []string{"memory.read"}},
		{ComponentID: "builtin.progressive-recall.scoped", Version: "1.0.0", DisplayName: "L2 按需召回", Description: "按项目和主题在预算内扩大范围。", Role: "recall.scoped", Kind: "stage", StageType: "memory.retrieve", Capabilities: []string{"memory.read"}},
		{ComponentID: "builtin.progressive-recall.exact-deep", Version: "1.0.0", DisplayName: "L3 精确深搜", Description: "不依赖模型的全文与结构化深度检索。", Role: "recall.deep", Kind: "stage", StageType: "memory.retrieve", Capabilities: []string{"memory.read"}},
	},
	"builtin.context-policy": {
		{ComponentID: "builtin.context-policy.profile-compiler", Version: "1.0.0", DisplayName: "用途画像编译", Description: "在任务开始前优先组织项目动态与会话恢复画像，不暴露 Owner-only 身份画像。", Role: "context.profile-compiler", Kind: "policy", Configuration: `{"views":["dynamic_project","session_resume"],"max_profiles":2,"max_chars":7000}`, Capabilities: []string{"memory.read", "project.read"}},
		{ComponentID: "builtin.context-policy.retrieval", Version: "1.0.0", DisplayName: "上下文召回策略", Description: "限定 Context Plan 可选的权威来源类型与候选规模。", Role: "context.retrieval-policy", Kind: "policy", Configuration: `{"kinds":["object","memory","evidence"],"max_candidates":40}`, Capabilities: []string{"memory.read"}},
		{ComponentID: "builtin.context-policy.presentation", Version: "1.0.0", DisplayName: "上下文呈现策略", Description: "决定画像、对象与 Evidence 在 Adapter 中的建议呈现方式。", Role: "context.presentation-policy", Kind: "policy", Configuration: `{"profile":"profile","object":"summary","evidence":"verbatim"}`, Capabilities: []string{"memory.read"}},
		{ComponentID: "builtin.context-policy.budget", Version: "1.0.0", DisplayName: "上下文预算策略", Description: "给 Profile 与 Recall 分配确定性字符和 Token 预算。", Role: "context.budget-policy", Kind: "policy", Configuration: `{"profile_max_tokens":1600,"profile_max_chars":7000,"candidate_multiplier":4}`, Capabilities: []string{"project.read"}},
	},
	"builtin.hybrid-retrieval": {
		{ComponentID: "builtin.hybrid-retrieval.deep", Version: "1.0.0", DisplayName: "L3 混合深搜", Description: "结构过滤、全文、向量、时间与关系信号融合；LLM 重排保持可选。", Role: "recall.deep", Kind: "stage", StageType: "memory.retrieve", Capabilities: []string{"memory.read"}},
	},
	"builtin.dsh-bridge": {
		{ComponentID: "builtin.dsh-bridge.deep-recall", Version: "1.0.0", DisplayName: "DSH 外部深搜", Description: "把召回委托给显式授权的 DeepSeek Harness 适配器。", Role: "recall.deep", Kind: "stage", StageType: "connector.call", Capabilities: []string{"memory.read", "connector.invoke"}},
	},
}

func Bootstrap(ctx context.Context, services Services) error {
	contributions := map[string]plugins.Contributions{}
	for _, item := range builtinTypes {
		lifecycle := harness.Lifecycle{Initial: item.states[0], States: item.states, Transitions: map[string][]string{}}
		for index := 0; index+1 < len(item.states); index++ {
			lifecycle.Transitions[item.states[index]] = []string{item.states[index+1]}
		}
		if item.typeID == harness.GovernedAgentAssetTypeV2 || item.typeID == harness.GovernedAgentAssetTypeV3 || item.typeID == harness.GovernedAgentAssetTypeV4 {
			lifecycle.Transitions = map[string][]string{
				"candidate":       {"review_required", "active"},
				"review_required": {"candidate", "active"},
				"active":          {"active", "superseded", "rolled_back"},
				"rolled_back":     {"active", "superseded"},
			}
		}
		if item.typeID == experience.CaseTypeV1 || item.typeID == experience.PatternTypeV1 || item.typeID == adaptation.ChangeProposalTypeV1 || item.typeID == adaptation.CaseOverlayTypeV1 {
			lifecycle.Transitions = map[string][]string{
				"candidate":  {"active", "rejected"},
				"active":     {"superseded", "archived"},
				"rejected":   {"archived"},
				"superseded": {"archived"},
			}
		}
		if item.typeID == experience.EvaluationTypeV1 || item.typeID == experience.EvaluationTypeV2 {
			lifecycle.Transitions = map[string][]string{
				"active":     {"superseded", "archived"},
				"superseded": {"archived"},
			}
		}
		if item.typeID == teammemory.PrivateScratchTypeV1 || item.typeID == teammemory.BlackboardEntryTypeV1 {
			lifecycle.Transitions = map[string][]string{
				"active":  {"active", "expired"},
				"expired": {"archived"},
			}
		}
		if item.typeID == teammemory.ConflictTypeV1 {
			lifecycle.Transitions = map[string][]string{
				"candidate": {"active"},
				"active":    {"resolved", "archived"},
				"resolved":  {"archived"},
			}
		}
		if item.typeID == teammemory.ProjectDurableTypeV1 {
			lifecycle.Transitions = map[string][]string{
				"candidate":  {"active"},
				"active":     {"superseded", "archived"},
				"superseded": {"archived"},
			}
		}
		if _, err := services.Harness.RegisterType(ctx, harness.RegisterTypeInput{
			TypeID: item.typeID, PluginID: item.pluginID, DisplayName: item.name, SchemaVersion: "1.0.0",
			Schema: json.RawMessage(item.schema), Lifecycle: lifecycle, ProtectionClass: item.protect,
			Renderer: json.RawMessage(`{"mode":"generic-card"}`),
		}); err != nil {
			return fmt.Errorf("bootstrap type %s: %w", item.typeID, err)
		}
		entry := contributions[item.pluginID]
		entry.MemoryTypes = append(entry.MemoryTypes, plugins.MemoryTypeContribution{TypeID: item.typeID, DisplayName: item.name, SchemaVersion: "1.0.0", Lifecycle: lifecycle, ProtectionClass: item.protect})
		contributions[item.pluginID] = entry
	}
	for _, item := range defaultPipelines {
		entry := contributions[item.pluginID]
		entry.Pipelines = append(entry.Pipelines, plugins.PipelineContribution{PipelineID: item.pipelineID, Version: item.version, Definition: "builtin"})
		contributions[item.pluginID] = entry
	}
	for pluginID, components := range builtinStrategyComponents {
		entry := contributions[pluginID]
		entry.StrategyComponents = append(entry.StrategyComponents, components...)
		contributions[pluginID] = entry
	}
	for _, item := range defaultBlueprints {
		entry := contributions[item.pluginID]
		entry.Blueprints = append(entry.Blueprints, plugins.BlueprintContribution{BlueprintID: item.definition.BlueprintID, Version: item.definition.Version, Definition: "builtin"})
		contributions[item.pluginID] = entry
	}
	core := contributions["builtin.memory-harness-core"]
	for _, stage := range pipeline.StageCatalog() {
		core.Stages = append(core.Stages, plugins.StageContribution{StageType: stage.StageType, Version: stage.Version, Class: stage.Class, Capabilities: stage.Capabilities})
	}
	contributions["builtin.memory-harness-core"] = core
	for _, spec := range builtinPlugins {
		spec.Contributions = contributions[spec.PluginID]
		if _, err := services.Plugins.RegisterBuiltin(ctx, spec); err != nil {
			return fmt.Errorf("bootstrap plugin %s: %w", spec.PluginID, err)
		}
	}
	for _, item := range defaultPipelines {
		if _, err := services.Pipelines.Version(ctx, item.pipelineID, item.version); err == nil {
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect bootstrap pipeline %s: %w", item.name, err)
		}
		if _, err := services.Pipelines.Publish(ctx, item.pluginID, []byte(item.definition)); err != nil {
			return fmt.Errorf("bootstrap pipeline %s: %w", item.name, err)
		}
	}
	if services.Blueprints != nil {
		for _, item := range defaultBlueprints {
			if _, err := services.Blueprints.Version(ctx, item.definition.BlueprintID, item.definition.Version); err == nil {
				continue
			} else if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("inspect bootstrap blueprint %s: %w", item.definition.Name, err)
			}
			if _, err := services.Blueprints.Publish(ctx, item.pluginID, item.definition); err != nil {
				return fmt.Errorf("bootstrap blueprint %s: %w", item.definition.Name, err)
			}
		}
	}
	return nil
}

type blueprintSpec struct {
	pluginID   string
	definition blueprint.Definition
}

var defaultBlueprints = []blueprintSpec{{pluginID: "builtin.memory-harness-core", definition: blueprint.Definition{
	APIVersion: blueprint.APIVersion, BlueprintID: blueprint.DefaultBlueprintID, Version: blueprint.DefaultBlueprintVersion,
	Name: "默认可编程记忆", Description: "六层语义生长、宫殿式结构组织与四级渐进召回的本地优先组合。",
	Intent: "在保留原文和完整来源的前提下，让记忆逐层生长，并只按当前任务成本加载所需上下文。",
	Policy: blueprint.Policy{EvidenceMode: "normalized_with_verbatim", ModelBoundary: "configured_provider", DefaultContextBudget: 12000, CrossProjectRecall: false},
	Tracks: []blueprint.Track{
		{TrackID: "growth", Role: "growth", DisplayName: "语义生长", Description: "一条来源最终能够变成什么。", Nodes: []blueprint.NodeBinding{
			{NodeID: "evidence", Role: "growth.evidence", DisplayName: "不可变证据", BindingKind: "policy", PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", ComponentID: "builtin.core-memory-growth.evidence", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"evidence.read"}, Config: json.RawMessage(`{"preserve_raw":true}`)},
			{NodeID: "knowledge", Role: "growth.knowledge", DisplayName: "知识点", BindingKind: "memory_type", PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", ComponentID: "builtin.core-memory-growth.knowledge-point", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.materialize"}, Config: json.RawMessage(`{"review":"risk_based"}`)},
			{NodeID: "episode", Role: "growth.episode", DisplayName: "情景复盘", BindingKind: "memory_type", PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", ComponentID: "builtin.core-memory-growth.episode", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.materialize"}, Config: json.RawMessage(`{"group_by":"session"}`)},
			{NodeID: "long-term", Role: "growth.long-term", DisplayName: "长期记忆", BindingKind: "memory_type", PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", ComponentID: "builtin.core-memory-growth.long-term", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.materialize"}, Config: json.RawMessage(`{"protected_types":["identity","procedure","behavior"]}`)},
			{NodeID: "living", Role: "growth.living", DisplayName: "生长知识", BindingKind: "memory_type", PluginID: "builtin.living-asset-vault", PluginVersion: "2.0.0", ComponentID: "builtin.living-asset-vault.growth", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read", "memory.materialize"}, Config: json.RawMessage(`{"revisioned":true}`)},
			{NodeID: "agent-asset", Role: "growth.agent-asset", DisplayName: "Agent 能力资产", BindingKind: "memory_type", PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", ComponentID: "builtin.agent-assets.activation", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"asset.propose", "memory.read"}, Config: json.RawMessage(`{"activation_review":true}`)},
		}},
		{TrackID: "organization", Role: "organization", DisplayName: "空间组织", Description: "内容在哪里，以及如何先缩小检索范围。", Nodes: []blueprint.NodeBinding{
			{NodeID: "scope", Role: "organization.scope", DisplayName: "Project / Wing", BindingKind: "provider", PluginID: "builtin.palace-organization", PluginVersion: "1.0.0", ComponentID: "builtin.palace-organization.project-scope", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"evidence.read"}, Config: json.RawMessage(`{"isolation":"project"}`)},
			{NodeID: "topic", Role: "organization.topic", DisplayName: "Topic / Room", BindingKind: "provider", PluginID: "builtin.palace-organization", PluginVersion: "1.0.0", ComponentID: "builtin.palace-organization.topic-room", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read"}, Config: json.RawMessage(`{"max_depth":2}`)},
			{NodeID: "concept", Role: "organization.concept", DisplayName: "Concept / Hall", BindingKind: "provider", PluginID: "builtin.palace-organization", PluginVersion: "1.0.0", ComponentID: "builtin.palace-organization.concept-hall", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read"}, Config: json.RawMessage(`{"entity_links":true,"temporal_links":true}`)},
			{NodeID: "drawer", Role: "organization.evidence", DisplayName: "Evidence / Drawer", BindingKind: "provider", PluginID: "builtin.palace-organization", PluginVersion: "1.0.0", ComponentID: "builtin.palace-organization.evidence-drawer", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"evidence.read"}, Config: json.RawMessage(`{"return_verbatim":true}`)},
		}},
		{TrackID: "recall", Role: "recall", DisplayName: "召回成本", Description: "只加载当前任务真正需要的上下文。", Nodes: []blueprint.NodeBinding{
			{NodeID: "l0", Role: "recall.identity", DisplayName: "L0 身份", BindingKind: "policy", PluginID: "builtin.progressive-recall", PluginVersion: "1.0.0", ComponentID: "builtin.progressive-recall.identity", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read"}, Config: json.RawMessage(`{"load":"always","budget_chars":600}`)},
			{NodeID: "l1", Role: "recall.essential", DisplayName: "L1 核心上下文", BindingKind: "stage", PluginID: "builtin.progressive-recall", PluginVersion: "1.0.0", ComponentID: "builtin.progressive-recall.essential", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read"}, Config: json.RawMessage(`{"load":"always","budget_chars":3200,"approved_only":true}`)},
			{NodeID: "l2", Role: "recall.scoped", DisplayName: "L2 按需召回", BindingKind: "stage", PluginID: "builtin.progressive-recall", PluginVersion: "1.0.0", ComponentID: "builtin.progressive-recall.scoped", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read"}, Config: json.RawMessage(`{"load":"on_demand","budget_chars":4000,"scope_first":true}`)},
			{NodeID: "l3", Role: "recall.deep", DisplayName: "L3 混合深搜", BindingKind: "stage", PluginID: "builtin.hybrid-retrieval", PluginVersion: "1.0.0", ComponentID: "builtin.hybrid-retrieval.deep", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read"}, Config: json.RawMessage(`{"load":"on_demand","budget_chars":12000,"signals":["structure","lexical","vector","temporal","relations"],"llm_rerank":false}`)},
		}},
	},
}}}

func init() {
	contextual := defaultBlueprints[0].definition
	contextual.Version = blueprint.ContextBlueprintVersion
	contextual.Name = "默认可编程记忆 · Context"
	contextual.Description = "保留原有生长、组织与召回三轨，并增加可选、确定性的上下文编译轨。"
	contextual.Tracks = append(append([]blueprint.Track(nil), contextual.Tracks...), blueprint.Track{
		TrackID: "context", Role: "context", DisplayName: "上下文编译", Description: "决定哪些受治理画像与召回结果进入 Context Plan；不扩大权限。",
		Nodes: []blueprint.NodeBinding{
			{NodeID: "profile", Role: "context.profile-compiler", DisplayName: "用途画像编译", BindingKind: "policy", PluginID: "builtin.context-policy", PluginVersion: "1.0.0", ComponentID: "builtin.context-policy.profile-compiler", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read", "project.read"}, Config: json.RawMessage(`{"views":["dynamic_project","session_resume"],"max_profiles":2,"max_chars":7000}`)},
			{NodeID: "retrieval", Role: "context.retrieval-policy", DisplayName: "上下文召回策略", BindingKind: "policy", PluginID: "builtin.context-policy", PluginVersion: "1.0.0", ComponentID: "builtin.context-policy.retrieval", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read"}, Config: json.RawMessage(`{"kinds":["object","memory","evidence"],"max_candidates":40}`)},
			{NodeID: "presentation", Role: "context.presentation-policy", DisplayName: "上下文呈现策略", BindingKind: "policy", PluginID: "builtin.context-policy", PluginVersion: "1.0.0", ComponentID: "builtin.context-policy.presentation", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"memory.read"}, Config: json.RawMessage(`{"profile":"profile","object":"summary","evidence":"verbatim"}`)},
			{NodeID: "budget", Role: "context.budget-policy", DisplayName: "上下文预算策略", BindingKind: "policy", PluginID: "builtin.context-policy", PluginVersion: "1.0.0", ComponentID: "builtin.context-policy.budget", ComponentVersion: "1.0.0", Enabled: true, RequiredCapabilities: []string{"project.read"}, Config: json.RawMessage(`{"profile_max_tokens":1600,"profile_max_chars":7000,"candidate_multiplier":4}`)},
		},
	})
	defaultBlueprints = append(defaultBlueprints, blueprintSpec{pluginID: "builtin.memory-harness-core", definition: contextual})
}

type pipelineSpec struct{ pluginID, pipelineID, version, name, definition string }

var defaultPipelines = []pipelineSpec{
	{pluginID: "builtin.core-memory-growth", pipelineID: "builtin.core-memory-growth.default-import", version: "3.0.0", name: "default-memory-growth", definition: `apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.core-memory-growth.default-import
version: 3.0.0
name: 默认六层记忆生长
intent: 从不可变 Evidence 出发，真实执行结构化提炼、项目归属、语义与时间投影、六层对象物化及检索刷新；每一步均保留输入、输出和来源。
requiredCapabilities: [evidence.read, memory.read, memory.propose, memory.materialize, project.write, asset.propose, model.invoke]
nodes:
  - {id: evidence, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {preserve: canonical}}
  - {id: compile, stageType: memory.compile, stageVersion: 1.0.0, pluginId: builtin.core-memory-growth, dependsOn: [evidence], config: {knowledge_schema: memory-harness.knowledge-unit/v2, attribution_policy: never_assume_owner}}
  - {id: project, stageType: project.derive, stageVersion: 1.0.0, pluginId: builtin.core-memory-growth, dependsOn: [compile], config: {route: explicit_project, derive: [context, goals, decisions, risks]}}
  - {id: semantic, stageType: memory.semantic_graph, stageVersion: 1.0.0, pluginId: builtin.core-memory-growth, dependsOn: [project], config: {graphs: [semantic, temporal], ambiguous: review}}
  - {id: materialize, stageType: memory.materialize, stageVersion: 1.0.0, pluginId: builtin.core-memory-growth, dependsOn: [semantic], config: {layers: [knowledge, episode, long_term, living, agent_asset], source_bound: true}}
  - {id: index, stageType: search.refresh, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [materialize], config: {scope: project}}
outputs: [{name: result, nodeId: index}]
policy: {maxStages: 8, timeoutSeconds: 600, maxModelCalls: 0}
editor:
  positions:
    evidence: {x: 80, y: 180}
    compile: {x: 360, y: 180}
    project: {x: 640, y: 180}
    semantic: {x: 920, y: 180}
    materialize: {x: 1200, y: 180}
    index: {x: 1480, y: 180}
`},
	{pluginID: "builtin.core-memory-growth", pipelineID: "builtin.core-memory-growth.default-import", version: "2.1.0", name: "default-memory-distillation", definition: `apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.core-memory-growth.default-import
version: 2.1.0
name: 默认记忆沉淀
intent: 把一次真实导入依次变成高质量知识点、长期记忆、项目投影、生长知识和能力资产，并留下完整运行轨迹。
requiredCapabilities: []
nodes:
  - {id: evidence, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {}}
  - {id: distill, stageType: transform.filter, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [evidence], config: {field: quality_gate}}
  - {id: consolidate, stageType: transform.map, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [distill], config: {merge: {layer: long_term}}}
  - {id: project, stageType: transform.map, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [consolidate], config: {merge: {projection: project_workspace}}}
  - {id: assets, stageType: transform.map, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [project], config: {merge: {projection: living_and_agent_assets}}}
outputs: [{name: result, nodeId: assets}]
policy: {maxStages: 8, timeoutSeconds: 600, maxModelCalls: 16}
`},
	{pluginID: "builtin.opportunity-intelligence", pipelineID: "builtin.opportunity-intelligence.qualify", version: "1.0.0", name: "opportunity-qualification", definition: `apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.opportunity-intelligence.qualify
version: 1.0.0
name: 商机识别与审核
intent: 从已选择的需求候选中形成可解释评分，并在物化商机前等待 Owner 审核。
requiredCapabilities: []
nodes:
  - {id: capture, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {}}
  - id: score
    stageType: transform.map
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [capture]
    config: {merge: {score: 50, qualification: needs_review}}
  - {id: review, stageType: policy.require_review, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [score], config: {reason: protected_opportunity}}
outputs: [{name: candidate, nodeId: review}]
policy: {maxStages: 8, timeoutSeconds: 120, maxModelCalls: 1}
`},
	{pluginID: "builtin.core-memory-growth", pipelineID: "builtin.core-memory-growth.inspect-candidate", version: "1.0.0", name: "memory-candidate", definition: `apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.core-memory-growth.inspect-candidate
version: 1.0.0
name: 记忆候选检查
intent: 在物化受保护长期记忆之前显示候选和人工审核轨迹。
requiredCapabilities: []
nodes:
  - {id: candidate, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {}}
  - {id: review, stageType: policy.require_review, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [candidate], config: {reason: protected_memory}}
outputs: [{name: candidate, nodeId: review}]
policy: {maxStages: 4, timeoutSeconds: 120, maxModelCalls: 0}
`},
}
