package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/luoyif/memory-harness/internal/integrationcatalog"
)

var (
	promptBindingPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*\}\}`)
	memoryToolPattern    = regexp.MustCompile(`\bmemory_[a-z0-9_]+\b`)
)

type deepValidationCheck struct {
	Name   string         `json:"name"`
	Status string         `json:"status"`
	Detail string         `json:"detail"`
	Data   map[string]any `json:"data,omitempty"`
}

func appendDeepCheck(report map[string]any, check deepValidationCheck) {
	checks, _ := report["checks"].([]any)
	entry := map[string]any{"name": check.Name, "status": check.Status, "detail": check.Detail}
	if len(check.Data) > 0 {
		entry["data"] = check.Data
	}
	report["checks"] = append(checks, entry)
}

func finalValidationStatus(report map[string]any) string {
	checks, _ := report["checks"].([]any)
	for _, raw := range checks {
		check, _ := raw.(map[string]any)
		if strings.EqualFold(fmt.Sprint(check["status"]), "failed") {
			return "failed"
		}
	}
	return "passed"
}

func uniqueToolRefs(body string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range memoryToolPattern.FindAllString(strings.ToLower(body), -1) {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
func toolCapabilityCheck(body string, name string, structureOnlyOK bool) deepValidationCheck {
	refs := uniqueToolRefs(body)
	if len(refs) == 0 {
		status := "warning"
		detail := "未声明可解析的 memory_* 工具；为安全起见没有执行任何外部调用"
		if structureOnlyOK {
			status = "passed"
			detail = "未声明外部工具；完成结构级 dry-run，没有执行副作用"
		}
		return deepValidationCheck{Name: name, Status: status, Detail: detail, Data: map[string]any{"mode": "no_side_effects", "resolved_tools": []string{}}}
	}
	unknown := []string{}
	permissions := map[string]string{}
	for _, ref := range refs {
		tool, ok := integrationcatalog.ByName(ref)
		if !ok {
			unknown = append(unknown, ref)
			continue
		}
		permissions[ref] = tool.Permission
	}
	if len(unknown) > 0 {
		return deepValidationCheck{Name: name, Status: "failed", Detail: "显式声明了未知 Memory Harness 工具", Data: map[string]any{"mode": "catalog_only", "resolved_tools": refs, "unknown_tools": unknown}}
	}
	return deepValidationCheck{Name: name, Status: "passed", Detail: "工具与权限在本地 capability catalog 中可解析；未执行工具", Data: map[string]any{"mode": "catalog_only", "resolved_tools": refs, "required_permissions": permissions}}
}
func promptFixtureCheck(body string) deepValidationCheck {
	bindings := promptBindingPattern.FindAllStringSubmatch(body, -1)
	rendered := promptBindingPattern.ReplaceAllStringFunc(body, func(value string) string {
		match := promptBindingPattern.FindStringSubmatch(value)
		if len(match) < 2 {
			return value
		}
		return "fixture_" + strings.ReplaceAll(match[1], ".", "_")
	})
	unresolved := strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}")
	if unresolved {
		return deepValidationCheck{Name: "prompt_fixture_render", Status: "failed", Detail: "确定性 fixture 渲染后仍有未解析占位符", Data: map[string]any{"binding_count": len(bindings), "mode": "deterministic_fixture"}}
	}
	return deepValidationCheck{Name: "prompt_fixture_render", Status: "passed", Detail: "确定性 fixture 可完整渲染；没有调用模型", Data: map[string]any{"binding_count": len(bindings), "rendered_chars": len([]rune(rendered)), "mode": "deterministic_fixture"}}
}

var denyMarkers = []string{"must not", "do not", "forbidden", "never", "不允许", "不得", "禁止", "不能", "严禁"}
var allowMarkers = []string{"should", "must", "allow", "prefer", "应该", "应当", "必须", "允许", "优先", "规则", "原则"}

func normativeSignature(body string) (string, string) {
	lower := strings.ToLower(body)
	polarity := ""
	for _, marker := range denyMarkers {
		if strings.Contains(lower, marker) {
			polarity = "deny"
			break
		}
	}
	if polarity == "" {
		for _, marker := range allowMarkers {
			if strings.Contains(lower, marker) {
				polarity = "allow"
				break
			}
		}
	}
	if polarity == "" {
		return "", ""
	}
	for _, marker := range append(append([]string{}, denyMarkers...), allowMarkers...) {
		lower = strings.ReplaceAll(lower, marker, "")
	}
	var b strings.Builder
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
			b.WriteRune(r)
		}
	}
	signature := b.String()
	if len([]rune(signature)) < 4 {
		return polarity, ""
	}
	return polarity, signature
}

func (s *Service) normativeConflictCheck(ctx context.Context, current Object, body string) (deepValidationCheck, error) {
	polarity, signature := normativeSignature(body)
	if signature == "" {
		return deepValidationCheck{Name: "normative_conflict_scan", Status: "warning", Detail: "没有形成足够稳定的规范签名；未做语义相似度猜测", Data: map[string]any{"mode": "exact_signature_only"}}, nil
	}
	rows, err := s.control.DB.QueryContext(ctx, `SELECT o.object_id,r.payload_json FROM harness_objects o
JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision
WHERE o.project_id=? AND o.type_id IN (?,?) AND o.status='active' AND o.object_id<>?`, current.ProjectID, GovernedAgentAssetTypeV3, GovernedAgentAssetTypeV4, current.ObjectID)
	if err != nil {
		return deepValidationCheck{}, err
	}
	defer rows.Close()
	conflicts := []string{}
	for rows.Next() {
		var objectID, payloadRaw string
		if err := rows.Scan(&objectID, &payloadRaw); err != nil {
			return deepValidationCheck{}, err
		}
		var payload map[string]any
		if json.Unmarshal([]byte(payloadRaw), &payload) != nil {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(asString(payload["asset_type"])))
		if kind != "rule" && kind != "constraint" {
			continue
		}
		otherPolarity, otherSignature := normativeSignature(asString(payload["body"]))
		if otherSignature == signature && otherPolarity != "" && otherPolarity != polarity {
			conflicts = append(conflicts, objectID)
		}
	}
	if err := rows.Err(); err != nil {
		return deepValidationCheck{}, err
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return deepValidationCheck{Name: "normative_conflict_scan", Status: "warning", Detail: "发现同项目 Active Rule/Constraint 具有完全相同规范主体但相反极性；需要 Owner 判断，不自动裁决", Data: map[string]any{"mode": "exact_signature_only", "conflicting_object_ids": conflicts}}, nil
	}
	return deepValidationCheck{Name: "normative_conflict_scan", Status: "passed", Detail: "未发现完全同签名、相反极性的 Active Rule/Constraint；未使用语义相似度推断", Data: map[string]any{"mode": "exact_signature_only"}}, nil
}

func mcpContractChecks(body string) []deepValidationCheck {
	lower := strings.ToLower(body)
	transports := []string{}
	for _, transport := range []string{"stdio", "http", "sse"} {
		if strings.Contains(lower, transport) {
			transports = append(transports, transport)
		}
	}
	transportStatus, transportDetail := "passed", "声明了可识别的 MCP transport"
	if len(transports) == 0 {
		transportStatus, transportDetail = "warning", "没有声明 stdio/http/sse transport；不猜测连接方式"
	}
	return []deepValidationCheck{
		{Name: "mcp_transport_contract", Status: transportStatus, Detail: transportDetail, Data: map[string]any{"declared_transports": transports}},
		toolCapabilityCheck(body, "mcp_tool_permission_probe", false),
		{Name: "mcp_connectivity_probe", Status: "not_executed", Detail: "静态 Validator 不建立网络/进程连接；连通性必须由 Owner 显式测试", Data: map[string]any{"mode": "no_side_effects"}},
	}
}

func (s *Service) deepValidateGovernedAgentAsset(ctx context.Context, current Object, payload []byte, validation json.RawMessage) ([]byte, json.RawMessage, error) {
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, nil, err
	}
	var report map[string]any
	if err := json.Unmarshal(validation, &report); err != nil {
		return nil, nil, err
	}
	if report == nil {
		report = map[string]any{}
	}
	assetType := strings.ToLower(strings.TrimSpace(asString(value["asset_type"])))
	body := strings.TrimSpace(asString(value["body"]))
	switch assetType {
	case "prompt":
		appendDeepCheck(report, promptFixtureCheck(body))
	case "skill":
		appendDeepCheck(report, toolCapabilityCheck(body, "skill_tool_dry_run", true))
	case "tool_recipe":
		appendDeepCheck(report, toolCapabilityCheck(body, "tool_recipe_dry_run", false))
	case "rule", "constraint":
		check, err := s.normativeConflictCheck(ctx, current, body)
		if err != nil {
			return nil, nil, err
		}
		appendDeepCheck(report, check)
	case "mcp":
		for _, check := range mcpContractChecks(body) {
			appendDeepCheck(report, check)
		}
	}
	status := finalValidationStatus(report)
	report["status"] = status
	report["validator"] = "governed-agent-asset-v3/deterministic-2"
	value["validation_status"] = status
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	validationRaw, err := json.Marshal(report)
	return canonical, json.RawMessage(validationRaw), err
}
