package assettemplate

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	TemplateVersion = "memory-harness.agent-asset-template/v1"
	SchemaVersion   = "1.0.0"
)

// SchemaV4 keeps the common governance envelope stable while the type-specific
// contract is checked by ValidatePayload. The local schema engine intentionally
// implements a bounded JSON Schema subset and does not support oneOf/$ref.
const SchemaV4 = `{"type":"object","required":["asset_id","asset_type","title","summary","body","template_version","spec","source_memory_ids","source_asset_ids","refinement","validation_status"],"properties":{"asset_id":{"type":"string","maxLength":200},"asset_type":{"type":"string","enum":["prompt","skill","rule","constraint","procedure","tool_recipe","mcp"]},"title":{"type":"string","maxLength":240},"summary":{"type":"string","maxLength":4000},"body":{"type":"string","maxLength":48000},"template_version":{"type":"string","enum":["memory-harness.agent-asset-template/v1"]},"spec":{"type":"object","maxProperties":32},"source_memory_ids":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":240}},"source_asset_ids":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":240}},"refinement":{"type":"object","required":["mode","generated_at","source_updated_at","missing_fields"],"properties":{"mode":{"type":"string","enum":["model","template_fallback","owner_edit"]},"model":{"type":"string","maxLength":240},"generated_at":{"type":"string","maxLength":80},"source_updated_at":{"type":"string","maxLength":80},"missing_fields":{"type":"array","maxItems":32,"items":{"type":"string","maxLength":120}}},"additionalProperties":false},"validation_status":{"type":"string","enum":["not_run","passed","failed","manual_passed"]}},"additionalProperties":false}`

type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder"`
}

type Template struct {
	AssetType       string  `json:"asset_type"`
	Label           string  `json:"label"`
	Description     string  `json:"description"`
	TemplateVersion string  `json:"template_version"`
	Fields          []Field `json:"fields"`
}

type Source struct {
	AssetID         string
	AssetType       string
	Title           string
	Body            string
	SourceMemoryIDs []string
	UpdatedAt       string
}

type Refinement struct {
	Mode            string   `json:"mode"`
	Model           string   `json:"model,omitempty"`
	GeneratedAt     string   `json:"generated_at"`
	SourceUpdatedAt string   `json:"source_updated_at"`
	MissingFields   []string `json:"missing_fields"`
}

type Payload struct {
	AssetID          string         `json:"asset_id"`
	AssetType        string         `json:"asset_type"`
	Title            string         `json:"title"`
	Summary          string         `json:"summary"`
	Body             string         `json:"body"`
	TemplateVersion  string         `json:"template_version"`
	Spec             map[string]any `json:"spec"`
	SourceMemoryIDs  []string       `json:"source_memory_ids"`
	SourceAssetIDs   []string       `json:"source_asset_ids"`
	Refinement       Refinement     `json:"refinement"`
	ValidationStatus string         `json:"validation_status"`
}

type ValidationCheck struct {
	Name   string         `json:"name"`
	Status string         `json:"status"`
	Detail string         `json:"detail"`
	Data   map[string]any `json:"data,omitempty"`
}

type ValidationReport struct {
	Status    string            `json:"status"`
	Validator string            `json:"validator"`
	Checks    []ValidationCheck `json:"checks"`
}

var templates = []Template{
	{AssetType: "prompt", Label: "Prompt · Agent 提示", Description: "定义角色、使用时机、变量、指令和输出合同。", Fields: []Field{
		{Key: "prompt_kind", Label: "提示类型", Description: "system、role 或 task", Kind: "text", Required: true, Placeholder: "system"},
		{Key: "role", Label: "Agent 角色", Description: "Agent 应以什么身份工作", Kind: "text", Required: true, Placeholder: "你是……"},
		{Key: "when_to_use", Label: "何时使用", Description: "明确触发场景", Kind: "list", Required: true, Placeholder: "当用户需要……"},
		{Key: "variables", Label: "输入变量", Description: "用 {{name}} 表示变量", Kind: "list", Required: true, Placeholder: "{{project_context}}"},
		{Key: "instructions", Label: "执行指令", Description: "按顺序描述 Agent 要做什么", Kind: "list", Required: true, Placeholder: "先核对来源"},
		{Key: "boundaries", Label: "边界", Description: "禁止行为和升级条件", Kind: "list", Required: true, Placeholder: "不得改写 Evidence"},
		{Key: "output_contract", Label: "输出格式", Description: "必须返回的结构和字段", Kind: "text", Required: true, Placeholder: "返回包含……的 JSON"},
		{Key: "examples", Label: "示例", Description: "可选的输入输出示例", Kind: "list", Placeholder: "输入：…… 输出：……"},
	}},
	{AssetType: "skill", Label: "Skill · 可复用技能", Description: "定义触发条件、输入输出、步骤、工具、失败处理和验收。", Fields: []Field{
		{Key: "purpose", Label: "能力目标", Description: "这项技能解决什么问题", Kind: "text", Required: true, Placeholder: "用于……"},
		{Key: "triggers", Label: "触发条件", Description: "什么请求或情境应使用它", Kind: "list", Required: true, Placeholder: "当用户要求……"},
		{Key: "inputs", Label: "输入", Description: "执行前必须具备的信息", Kind: "list", Required: true, Placeholder: "项目 ID"},
		{Key: "outputs", Label: "输出", Description: "执行完成后交付什么", Kind: "list", Required: true, Placeholder: "验证报告"},
		{Key: "steps", Label: "步骤", Description: "有顺序的可执行动作", Kind: "list", Required: true, Placeholder: "1. 核对范围"},
		{Key: "tools", Label: "允许工具", Description: "明确工具名和所需权限", Kind: "list", Placeholder: "memory_search"},
		{Key: "constraints", Label: "约束", Description: "执行时不可越过的边界", Kind: "list", Required: true, Placeholder: "不得覆盖来源"},
		{Key: "success_checks", Label: "验收条件", Description: "怎样判断真正完成", Kind: "list", Required: true, Placeholder: "来源可精确读回"},
		{Key: "failure_handling", Label: "失败处理", Description: "失败、缺失信息或冲突时怎么办", Kind: "list", Required: true, Placeholder: "停止并请求人工确认"},
	}},
	{AssetType: "rule", Label: "Rule · 判断规则", Description: "定义适用条件、判断、例外、理由和检查方式。", Fields: []Field{
		{Key: "applies_when", Label: "适用条件", Description: "规则在什么情况下成立", Kind: "list", Required: true, Placeholder: "处理正式项目数据时"},
		{Key: "rule", Label: "规则正文", Description: "可判断的 should / should not 语句", Kind: "text", Required: true, Placeholder: "应该……"},
		{Key: "exceptions", Label: "例外", Description: "明确不适用的情况", Kind: "list", Placeholder: "经 Owner 明确授权时"},
		{Key: "rationale", Label: "理由", Description: "为什么需要这条规则", Kind: "text", Required: true, Placeholder: "为了……"},
		{Key: "verification", Label: "检查方式", Description: "怎样判断规则被遵守", Kind: "list", Required: true, Placeholder: "检查审计记录"},
	}},
	{AssetType: "constraint", Label: "Constraint · 强约束", Description: "定义必须/禁止事项、范围、严重度、例外和违规响应。", Fields: []Field{
		{Key: "requirement", Label: "强制要求", Description: "可执行的 must / must not 语句", Kind: "text", Required: true, Placeholder: "不得……"},
		{Key: "severity", Label: "严重度", Description: "critical、high、medium 或 low", Kind: "text", Required: true, Placeholder: "high"},
		{Key: "scope", Label: "适用范围", Description: "项目、数据或操作边界", Kind: "list", Required: true, Placeholder: "全部正式 Evidence"},
		{Key: "exceptions", Label: "例外", Description: "允许例外时必须写清授权条件", Kind: "list", Placeholder: "无"},
		{Key: "violation_response", Label: "违规响应", Description: "发现违反时必须做什么", Kind: "list", Required: true, Placeholder: "阻止执行并记录审计"},
	}},
	{AssetType: "procedure", Label: "Procedure · 标准流程", Description: "定义触发器、前置条件、步骤、完成检查和回滚。", Fields: []Field{
		{Key: "trigger", Label: "触发器", Description: "何时开始流程", Kind: "text", Required: true, Placeholder: "每周五"},
		{Key: "prerequisites", Label: "前置条件", Description: "开始前必须满足的条件", Kind: "list", Required: true, Placeholder: "备份已完成"},
		{Key: "steps", Label: "执行步骤", Description: "严格有序的动作", Kind: "list", Required: true, Placeholder: "1. 检查范围"},
		{Key: "completion_checks", Label: "完成检查", Description: "确认流程真正结束", Kind: "list", Required: true, Placeholder: "所有测试退出码为 0"},
		{Key: "rollback", Label: "回滚方式", Description: "失败后如何安全恢复", Kind: "list", Required: true, Placeholder: "恢复上一版本"},
	}},
	{AssetType: "tool_recipe", Label: "Tool Recipe · 工具配方", Description: "定义工具、参数映射、调用顺序、幂等、重试、副作用和验证。", Fields: []Field{
		{Key: "purpose", Label: "目标", Description: "这一组工具调用要完成什么", Kind: "text", Required: true, Placeholder: "用于……"},
		{Key: "preconditions", Label: "前置条件", Description: "调用前必须确认的状态", Kind: "list", Required: true, Placeholder: "已确认项目范围"},
		{Key: "tools", Label: "工具与权限", Description: "稳定工具名、用途和权限", Kind: "list", Required: true, Placeholder: "memory_search · memory.read"},
		{Key: "ordered_calls", Label: "调用顺序", Description: "包含参数来源和结果去向", Kind: "list", Required: true, Placeholder: "1. 调用……"},
		{Key: "idempotency", Label: "幂等策略", Description: "避免重复副作用", Kind: "text", Required: true, Placeholder: "使用稳定 idempotency_key"},
		{Key: "retry_policy", Label: "重试策略", Description: "可重试错误、次数和退避", Kind: "text", Required: true, Placeholder: "仅网络错误重试 2 次"},
		{Key: "side_effects", Label: "副作用", Description: "会写入或改变什么", Kind: "list", Required: true, Placeholder: "新增一条 Evidence"},
		{Key: "verification", Label: "结果验证", Description: "精确读回和验收方式", Kind: "list", Required: true, Placeholder: "按 ID 精确读回"},
	}},
	{AssetType: "mcp", Label: "MCP · 工具合同", Description: "定义服务身份、传输、工具、权限、配置、健康检查和故障边界。", Fields: []Field{
		{Key: "server_name", Label: "服务名称", Description: "稳定的 MCP server 标识", Kind: "text", Required: true, Placeholder: "memory-harness"},
		{Key: "purpose", Label: "用途", Description: "这个连接允许 Agent 完成什么", Kind: "text", Required: true, Placeholder: "读取项目记忆"},
		{Key: "transports", Label: "传输方式", Description: "stdio、streamable HTTP 或 SSE", Kind: "list", Required: true, Placeholder: "stdio"},
		{Key: "tools", Label: "工具合同", Description: "工具名、输入、输出和副作用", Kind: "list", Required: true, Placeholder: "memory_search(query) → hits"},
		{Key: "permissions", Label: "所需权限", Description: "逐项列出最小权限", Kind: "list", Required: true, Placeholder: "memory.read"},
		{Key: "configuration", Label: "配置", Description: "命令、地址和非敏感参数；不得包含 Token", Kind: "list", Required: true, Placeholder: "command: memoryosd mcp"},
		{Key: "health_check", Label: "健康检查", Description: "如何确认真实可用", Kind: "list", Required: true, Placeholder: "列出项目并精确读回"},
		{Key: "failure_policy", Label: "故障策略", Description: "超时、拒绝和权限不足时怎么办", Kind: "list", Required: true, Placeholder: "失败即停止，不扩大权限"},
	}},
}

func Templates() []Template {
	out := make([]Template, len(templates))
	for index, item := range templates {
		item.TemplateVersion = TemplateVersion
		item.Fields = append([]Field(nil), item.Fields...)
		out[index] = item
	}
	return out
}

func For(assetType string) (Template, bool) {
	assetType = strings.ToLower(strings.TrimSpace(assetType))
	for _, item := range Templates() {
		if item.AssetType == assetType {
			return item, true
		}
	}
	return Template{}, false
}

func emptyValue(kind string) any {
	if kind == "list" {
		return []string{}
	}
	return ""
}

func Skeleton(source Source, generatedAt string) Payload {
	template, _ := For(source.AssetType)
	spec := map[string]any{}
	for _, field := range template.Fields {
		spec[field.Key] = emptyValue(field.Kind)
	}
	generatedAt = strings.TrimSpace(generatedAt)
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	title := truncate(strings.TrimSpace(source.Title), 240)
	if title == "" {
		title = template.Label
	}
	return Payload{
		AssetID: source.AssetID, AssetType: source.AssetType, Title: title,
		Summary: truncate(strings.TrimSpace(source.Body), 4000), TemplateVersion: TemplateVersion, Spec: spec,
		SourceMemoryIDs: normalize(source.SourceMemoryIDs), SourceAssetIDs: normalize([]string{source.AssetID}),
		Refinement:       Refinement{Mode: "template_fallback", GeneratedAt: generatedAt, SourceUpdatedAt: source.UpdatedAt, MissingFields: []string{}},
		ValidationStatus: "not_run",
	}
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return strings.TrimSpace(string(runes[:max-1])) + "…"
}

func normalize(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func valueMissing(value any, kind string) bool {
	if value == nil {
		return true
	}
	if kind == "list" {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				if strings.TrimSpace(fmt.Sprint(item)) != "" {
					return false
				}
			}
		case []string:
			for _, item := range typed {
				if strings.TrimSpace(item) != "" {
					return false
				}
			}
		}
		return true
	}
	return strings.TrimSpace(fmt.Sprint(value)) == ""
}

func stringList(value any) []string {
	out := []string{}
	switch typed := value.(type) {
	case []string:
		out = append(out, typed...)
	case []any:
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
	case string:
		for _, item := range strings.Split(typed, "\n") {
			item = strings.TrimSpace(strings.TrimPrefix(item, "-"))
			if item != "" {
				out = append(out, item)
			}
		}
	}
	return out
}

func RenderBody(payload Payload, template Template) string {
	var body strings.Builder
	fmt.Fprintf(&body, "# %s\n\n%s\n", payload.Title, payload.Summary)
	for _, field := range template.Fields {
		body.WriteString("\n## " + field.Label + "\n")
		if field.Kind == "list" {
			items := stringList(payload.Spec[field.Key])
			if len(items) == 0 {
				body.WriteString("- 待补充\n")
				continue
			}
			for _, item := range items {
				body.WriteString("- " + item + "\n")
			}
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(payload.Spec[field.Key]))
		if text == "" {
			text = "待补充"
		}
		body.WriteString(text + "\n")
	}
	return strings.TrimSpace(body.String())
}

func ValidatePayload(raw []byte) ([]byte, json.RawMessage, error) {
	var payload Payload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, err
	}
	template, ok := For(payload.AssetType)
	if !ok {
		return nil, nil, errors.New("unsupported governed asset template")
	}
	checks := []ValidationCheck{}
	add := func(name, status, detail string, data map[string]any) {
		checks = append(checks, ValidationCheck{Name: name, Status: status, Detail: detail, Data: data})
	}
	if payload.TemplateVersion != TemplateVersion {
		add("template_version", "failed", "资产必须使用当前标准模板版本", map[string]any{"expected": TemplateVersion})
	} else {
		add("template_version", "passed", "标准模板版本已识别", nil)
	}
	identityMissing := []string{}
	if strings.TrimSpace(payload.AssetID) == "" {
		identityMissing = append(identityMissing, "asset_id")
	}
	if strings.TrimSpace(payload.Title) == "" {
		identityMissing = append(identityMissing, "title")
	}
	if strings.TrimSpace(payload.Summary) == "" {
		identityMissing = append(identityMissing, "summary")
	}
	if len(identityMissing) > 0 {
		add("identity", "failed", "资产标识、标题和摘要必须填写", map[string]any{"missing_fields": identityMissing})
	} else {
		add("identity", "passed", "资产标识、标题和摘要完整", nil)
	}
	missing := []string{}
	allowed := map[string]Field{}
	for _, field := range template.Fields {
		allowed[field.Key] = field
		if field.Required && valueMissing(payload.Spec[field.Key], field.Kind) {
			missing = append(missing, field.Key)
		}
	}
	unknown := []string{}
	for key := range payload.Spec {
		if _, exists := allowed[key]; !exists {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	if len(missing) > 0 {
		add("required_template_fields", "failed", "标准模板仍有必填项未完成", map[string]any{"missing_fields": missing})
	} else {
		add("required_template_fields", "passed", "所有必填模板字段均已填写", nil)
	}
	if len(unknown) > 0 {
		add("known_template_fields", "failed", "包含当前模板未声明的字段", map[string]any{"unknown_fields": unknown})
	} else {
		add("known_template_fields", "passed", "字段均属于当前类型模板", nil)
	}
	if len(payload.SourceMemoryIDs) == 0 || len(payload.SourceAssetIDs) == 0 {
		add("source_lineage", "failed", "二次沉淀必须保留来源 Memory 和来源资产", nil)
	} else {
		add("source_lineage", "passed", "来源 Memory 和来源资产均已保留", nil)
	}
	if payload.Refinement.Mode == "template_fallback" {
		add("model_refinement", "failed", "模型不可用时只生成模板骨架；补齐字段或重新运行后才能激活", nil)
	} else {
		add("model_refinement", "passed", "内容已经过模型二次沉淀或 Owner 结构化编辑", map[string]any{"mode": payload.Refinement.Mode})
	}
	payload.Refinement.MissingFields = missing
	payload.Spec = canonicalSpec(payload.Spec, template)
	payload.Body = RenderBody(payload, template)
	payload.SourceMemoryIDs = normalize(payload.SourceMemoryIDs)
	payload.SourceAssetIDs = normalize(payload.SourceAssetIDs)
	status := "passed"
	for _, check := range checks {
		if check.Status == "failed" {
			status = "failed"
			break
		}
	}
	payload.ValidationStatus = status
	report := ValidationReport{Status: status, Validator: "governed-agent-asset-v4/template-1", Checks: checks}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	validation, _ := json.Marshal(report)
	return canonical, json.RawMessage(validation), nil
}

func canonicalSpec(spec map[string]any, template Template) map[string]any {
	out := map[string]any{}
	for _, field := range template.Fields {
		if field.Kind == "list" {
			out[field.Key] = stringList(spec[field.Key])
		} else {
			if spec[field.Key] == nil {
				out[field.Key] = ""
			} else {
				out[field.Key] = strings.TrimSpace(fmt.Sprint(spec[field.Key]))
			}
		}
	}
	return out
}

func BatchOutputSchema(assetType string) (json.RawMessage, error) {
	template, ok := For(assetType)
	if !ok {
		return nil, errors.New("unknown asset type")
	}
	properties := map[string]any{}
	required := []string{}
	for _, field := range template.Fields {
		child := map[string]any{"type": "string", "maxLength": 12000}
		if field.Kind == "list" {
			child = map[string]any{"type": "array", "maxItems": 60, "items": map[string]any{"type": "string", "maxLength": 4000}}
		}
		properties[field.Key] = child
		if field.Required {
			required = append(required, field.Key)
		}
	}
	spec := map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	item := map[string]any{"type": "object", "required": []string{"asset_id", "title", "summary", "spec"}, "properties": map[string]any{
		"asset_id": map[string]any{"type": "string", "maxLength": 200},
		"title":    map[string]any{"type": "string", "maxLength": 240},
		"summary":  map[string]any{"type": "string", "maxLength": 4000},
		"spec":     spec,
	}, "additionalProperties": false}
	schema := map[string]any{"type": "object", "required": []string{"items"}, "properties": map[string]any{"items": map[string]any{"type": "array", "maxItems": 100, "items": item}}, "additionalProperties": false}
	raw, err := json.Marshal(schema)
	return json.RawMessage(raw), err
}
