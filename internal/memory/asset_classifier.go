package memory

import "strings"

const (
	AssetTypePrompt       = "prompt"
	AssetTypeSkill        = "skill"
	AssetTypeRule         = "rule"
	AssetTypeConstraint   = "constraint"
	AssetTypeProcedure    = "procedure"
	AssetTypeToolRecipe   = "tool_recipe"
	AssetTypeMCP          = "mcp"
	AssetTypeUnclassified = "unclassified"
)

type assetClassification struct {
	Type      string
	Scores    map[string]int
	Reasons   []string
	Ambiguous bool
}

type assetSignal struct {
	kind   string
	weight int
	terms  []string
	reason string
}

var governedAgentAssetTypes = []string{
	AssetTypePrompt, AssetTypeSkill, AssetTypeRule, AssetTypeConstraint,
	AssetTypeProcedure, AssetTypeToolRecipe, AssetTypeMCP,
}

var assetSignals = []assetSignal{
	{AssetTypeMCP, 10, []string{"mcp server", "mcp tool", "model context protocol", "mcp服务", "mcp 服务", "mcp工具", "mcp 工具", "stdio", "streamable http", "sse transport"}, "mcp-contract"},
	{AssetTypeMCP, 5, []string{"tool manifest", "server manifest", "transport", "capabilities", "permission schema", "工具清单", "传输协议", "权限声明"}, "mcp-surface"},
	{AssetTypeToolRecipe, 7, []string{"调用工具", "工具调用", "调用 api", "调用api", "执行命令", "命令行", "cli", "tool call", "api call", "shell command"}, "ordered-tool-use"},
	{AssetTypeToolRecipe, 4, []string{"重试", "幂等", "参数 schema", "参数schema", "返回值", "错误码", "verification", "retry", "idempotency", "arguments"}, "tool-contract"},
	{AssetTypePrompt, 8, []string{"提示词", "prompt template", "system prompt", "user prompt", "prompt_text", "提示模板", "系统提示", "提示模板"}, "prompt-template"},
	{AssetTypePrompt, 4, []string{"变量", "占位符", "variables", "placeholder", "when_to_use", "instruction", "输出格式", "few-shot", "示例输入"}, "prompt-contract"},
	{AssetTypeConstraint, 7, []string{"不得", "禁止", "不允许", "只允许", "严禁", "必须先经过", "不能直接", "must not", "forbidden", "prohibited", "only allow", "permission", "权限边界"}, "hard-boundary"},
	{AssetTypeConstraint, 5, []string{"约束", "边界", "限制", "合规", "安全要求", "强制", "requirement", "constraint", "security boundary"}, "enforced-requirement"},
	{AssetTypeRule, 6, []string{"规则", "原则", "准则", "策略", "判断标准", "适用条件", "例外", "rule", "policy", "principle", "exception"}, "normative-rule"},
	{AssetTypeRule, 3, []string{"应该", "应当", "建议", "优先", "避免", "should ", "prefer ", "recommend"}, "soft-rule"},
	{AssetTypeProcedure, 6, []string{"第一步", "第二步", "第三步", "步骤", "流程", "工作流", "runbook", "procedure", "workflow", "step 1", "step one"}, "ordered-steps"},
	{AssetTypeProcedure, 5, []string{"每次", "每天", "每日", "每周", "定期", "一旦", "触发", "当…时", "当...时", "whenever", "every time", "daily", "weekly", "trigger"}, "trigger-or-frequency"},
	{AssetTypeProcedure, 3, []string{"先", "然后", "接着", "最后", "再", "before", "then", "finally", "checklist", "检查清单"}, "ordered-actions"},
	{AssetTypeSkill, 5, []string{"如何", "方法", "技巧", "使用", "创建", "生成", "制作", "调试", "分析", "部署", "实现", "操作", "how to", "using ", "create ", "build ", "debug", "deploy"}, "capability-method"},
	{AssetTypeSkill, 3, []string{"输入", "输出", "成功检查", "验收", "验证", "inputs", "outputs", "success checks", "validate"}, "capability-contract"},
}

func IsGovernedAgentAssetType(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range governedAgentAssetTypes {
		if value == candidate {
			return true
		}
	}
	return false
}

func classifyAgentAsset(unit KnowledgeUnit) assetClassification {
	text := strings.ToLower(strings.TrimSpace(unit.Statement + " " + unit.Structure.Frame.Action + " " + unit.Structure.Frame.Context))
	scores := map[string]int{}
	reasons := map[string][]string{}
	for _, kind := range governedAgentAssetTypes {
		scores[kind] = 0
	}
	for _, signal := range assetSignals {
		for _, term := range signal.terms {
			if strings.Contains(text, strings.ToLower(term)) {
				scores[signal.kind] += signal.weight
				reasons[signal.kind] = appendUnique(reasons[signal.kind], signal.reason)
				break
			}
		}
	}
	if unit.Structure.Epistemic.Modality == "normative" {
		scores[AssetTypeRule] += 2
		reasons[AssetTypeRule] = appendUnique(reasons[AssetTypeRule], "normative-modality")
	}
	if strings.TrimSpace(unit.Structure.Frame.Action) != "" {
		scores[AssetTypeSkill] += 2
		reasons[AssetTypeSkill] = appendUnique(reasons[AssetTypeSkill], "explicit-action")
	}
	// Strong surface contracts are more specific than generic workflow/rule words.
	for _, kind := range []string{AssetTypeMCP, AssetTypeToolRecipe, AssetTypePrompt} {
		if scores[kind] >= 8 {
			return assetClassification{Type: kind, Scores: scores, Reasons: reasons[kind]}
		}
	}
	// A repeated trigger plus ordered actions is a Procedure, even if individual
	// steps contain normative words such as "must".
	if scores[AssetTypeProcedure] >= 8 && scores[AssetTypeProcedure] >= scores[AssetTypeConstraint] {
		return assetClassification{Type: AssetTypeProcedure, Scores: scores, Reasons: reasons[AssetTypeProcedure]}
	}
	best, second := governedAgentAssetTypes[0], governedAgentAssetTypes[1]
	if scores[second] > scores[best] {
		best, second = second, best
	}
	for _, kind := range governedAgentAssetTypes[2:] {
		if scores[kind] > scores[best] {
			second, best = best, kind
		} else if scores[kind] > scores[second] {
			second = kind
		}
	}
	if scores[best] < 5 || scores[best] == scores[second] {
		return assetClassification{Type: AssetTypeUnclassified, Scores: scores, Reasons: []string{"insufficient-or-tied-signals"}, Ambiguous: true}
	}
	return assetClassification{Type: best, Scores: scores, Reasons: reasons[best]}
}

func classifyAgentAssetText(statement string) assetClassification {
	return classifyAgentAsset(KnowledgeUnit{Statement: statement})
}
