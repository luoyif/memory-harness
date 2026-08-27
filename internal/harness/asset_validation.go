package harness

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

type assetValidationCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func hasAny(text string, markers ...string) bool {
	text = strings.ToLower(text)
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func validateGovernedAgentAssetPayload(raw []byte) ([]byte, json.RawMessage, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, err
	}
	assetType := strings.ToLower(strings.TrimSpace(asString(payload["asset_type"])))
	title := strings.TrimSpace(asString(payload["title"]))
	body := strings.TrimSpace(asString(payload["body"]))
	sources, _ := payload["source_memory_ids"].([]any)
	checks := []assetValidationCheck{}
	add := func(name string, passed bool, detail string) {
		status := "failed"
		if passed {
			status = "passed"
		}
		checks = append(checks, assetValidationCheck{Name: name, Status: status, Detail: detail})
	}
	add("title", title != "", "资产标题必须存在")
	add("body", utf8.RuneCountInString(body) >= 12, "资产正文至少需要 12 个字符")
	add("source_lineage", len(sources) > 0, "受治理资产必须保留至少一条来源 Memory")
	switch assetType {
	case "prompt":
		balanced := strings.Count(body, "{{") == strings.Count(body, "}}")
		add("prompt_render", balanced, "Prompt 占位符必须成对闭合")
	case "skill":
		add("skill_action", hasAny(body, "使用", "创建", "实现", "操作", "输入", "输出", "how to", "build", "use "), "Skill 需要可执行动作或输入输出信号")
	case "rule":
		add("rule_normative", hasAny(body, "规则", "原则", "应该", "应当", "优先", "避免", "rule", "policy", "should"), "Rule 需要明确的规范性判断")
	case "constraint":
		add("constraint_boundary", hasAny(body, "不得", "禁止", "不允许", "不能", "必须", "严禁", "must not", "forbidden", "only allow"), "Constraint 需要明确的强制边界")
	case "procedure":
		ordered := 0
		for _, marker := range []string{"第一", "第二", "步骤", "先", "然后", "最后", "每次", "step", "then", "finally"} {
			if hasAny(body, marker) {
				ordered++
			}
		}
		add("procedure_steps", ordered >= 2, "Procedure 需要至少两个顺序/触发信号")
	case "tool_recipe":
		add("tool_reference", hasAny(body, "工具", "调用", "api", "命令", "tool", "cli", "shell"), "Tool Recipe 需要明确工具或调用接口")
		add("tool_verification", hasAny(body, "验证", "检查", "重试", "幂等", "verify", "check", "retry", "idempot"), "Tool Recipe 需要结果检查、重试或幂等策略")
	case "mcp":
		add("mcp_identity", hasAny(body, "mcp", "model context protocol"), "MCP Asset 必须明确 MCP 身份")
		add("mcp_contract", hasAny(body, "tool", "工具", "transport", "stdio", "http", "sse", "权限", "permission", "manifest"), "MCP Asset 需要工具、传输或权限合同")
	default:
		return nil, nil, errors.New("unsupported governed asset type")
	}
	status := "passed"
	for _, check := range checks {
		if check.Status != "passed" {
			status = "failed"
			break
		}
	}
	payload["validation_status"] = status
	canonical, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	validation, _ := json.Marshal(map[string]any{
		"status": status, "validator": "governed-agent-asset-v3/deterministic-1", "checks": checks,
	})
	return canonical, json.RawMessage(validation), nil
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}
