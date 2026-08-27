package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/luoyif/memory-harness/internal/buildinfo"
	"github.com/luoyif/memory-harness/internal/harness"
)

type ConformanceCheck struct {
	Name   string         `json:"name"`
	Status string         `json:"status"`
	Detail string         `json:"detail"`
	Data   map[string]any `json:"data,omitempty"`
}

type PluginConformanceReport struct {
	PluginID                 string             `json:"plugin_id"`
	Version                  string             `json:"version"`
	ProjectID                string             `json:"project_id,omitempty"`
	MemoryHarnessVersion     string             `json:"memory_harness_version"`
	CompatibilityRequirement string             `json:"compatibility_requirement"`
	CompatibilityStatus      string             `json:"compatibility_status"`
	DeclaredCapabilities     []string           `json:"declared_capabilities"`
	GrantedCapabilities      []string           `json:"granted_capabilities"`
	MissingRequired          []string           `json:"missing_required"`
	OptionalNotGranted       []string           `json:"optional_not_granted"`
	ConfigurationSchema      json.RawMessage    `json:"configuration_schema,omitempty"`
	ConfigurationStatus      string             `json:"configuration_status"`
	OverallStatus            string             `json:"overall_status"`
	Checks                   []ConformanceCheck `json:"checks"`
}

func versionTriple(value string) ([3]int, bool) {
	var out [3]int
	base := strings.SplitN(strings.TrimSpace(value), "-", 2)[0]
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i := range parts {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func compareVersion(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func compatibilityStatus(requirement string) string {
	current, ok := versionTriple(buildinfo.Version)
	if !ok {
		return "unknown"
	}
	tokens := strings.Fields(strings.TrimSpace(requirement))
	if len(tokens) == 0 {
		return "unknown"
	}
	for _, token := range tokens {
		op, raw := "=", token
		for _, candidate := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(token, candidate) {
				op, raw = candidate, strings.TrimSpace(strings.TrimPrefix(token, candidate))
				break
			}
		}
		other, parsed := versionTriple(raw)
		if !parsed {
			return "unknown"
		}
		cmp := compareVersion(current, other)
		if (op == ">=" && cmp < 0) || (op == "<=" && cmp > 0) || (op == ">" && cmp <= 0) || (op == "<" && cmp >= 0) || (op == "=" && cmp != 0) {
			return "incompatible"
		}
	}
	return "compatible"
}
func (s *Service) configurationSchema(ctx context.Context, plugin PluginVersion) (json.RawMessage, error) {
	var manifest Manifest
	if len(plugin.Manifest) == 0 || json.Unmarshal(plugin.Manifest, &manifest) != nil || strings.TrimSpace(manifest.Configuration.Schema) == "" {
		return nil, nil
	}
	var blob []byte
	if err := s.control.DB.QueryRowContext(ctx, `SELECT package_blob FROM harness_plugin_versions WHERE plugin_id=? AND version=?`, plugin.PluginID, plugin.Version).Scan(&blob); err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}
	if len(blob) == 0 {
		return nil, fmt.Errorf("plugin package is unavailable")
	}
	pkg, err := parsePackage(blob)
	if err != nil {
		return nil, err
	}
	raw, ok := pkg.Files[manifest.Configuration.Schema]
	if !ok {
		return nil, fmt.Errorf("configuration schema %q is missing", manifest.Configuration.Schema)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("configuration schema invalid JSON: %w", err)
	}
	if schema["type"] != "object" {
		return nil, fmt.Errorf("configuration schema root type must be object")
	}
	return append(json.RawMessage(nil), raw...), nil
}

func difference(left, right []string) []string {
	set := map[string]bool{}
	for _, value := range right {
		set[value] = true
	}
	out := []string{}
	for _, value := range left {
		if !set[value] {
			out = append(out, value)
		}
	}
	return out
}

func projectStateFor(plugin PluginVersion, projectID string) *ProjectState {
	for i := range plugin.ProjectStates {
		if plugin.ProjectStates[i].ProjectID == projectID {
			return &plugin.ProjectStates[i]
		}
	}
	return nil
}

func addConformanceCheck(report *PluginConformanceReport, name, status, detail string, data map[string]any) {
	report.Checks = append(report.Checks, ConformanceCheck{Name: name, Status: status, Detail: detail, Data: data})
	if status == "failed" {
		report.OverallStatus = "failed"
	} else if report.OverallStatus != "failed" && status == "warning" {
		report.OverallStatus = "warning"
	}
}
func (s *Service) Conformance(ctx context.Context, pluginID, version, projectID string) (PluginConformanceReport, error) {
	plugin, err := s.Plugin(ctx, strings.TrimSpace(pluginID), strings.TrimSpace(version))
	if err != nil {
		return PluginConformanceReport{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(plugin.Manifest, &manifest); err != nil {
		return PluginConformanceReport{}, err
	}
	report := PluginConformanceReport{
		PluginID: plugin.PluginID, Version: plugin.Version, ProjectID: strings.TrimSpace(projectID), MemoryHarnessVersion: buildinfo.Version,
		CompatibilityRequirement: manifest.Compatibility.MemoryHarness, CompatibilityStatus: compatibilityStatus(manifest.Compatibility.MemoryHarness),
		DeclaredCapabilities: append([]string(nil), plugin.Permissions...), OverallStatus: "passed", ConfigurationStatus: "not_declared", Checks: []ConformanceCheck{},
	}
	compatCheck := "passed"
	compatDetail := "声明的 Memory Harness 版本范围与当前内核兼容。"
	if report.CompatibilityStatus == "incompatible" {
		compatCheck, compatDetail = "failed", "插件声明的 Memory Harness 版本范围不包含当前内核。"
	}
	if report.CompatibilityStatus == "unknown" {
		compatCheck, compatDetail = "warning", "兼容性表达式超出当前确定性检查器支持范围；未猜测兼容。"
	}
	addConformanceCheck(&report, "compatibility", compatCheck, compatDetail, map[string]any{"required": report.CompatibilityRequirement, "current": buildinfo.Version})

	signatureStatus := "failed"
	if plugin.SignatureStatus == "verified" || plugin.SignatureStatus == "bundled" {
		signatureStatus = "passed"
	}
	if plugin.SignatureStatus == "developer_unsigned" {
		signatureStatus = "warning"
	}
	addConformanceCheck(&report, "signature_trust", signatureStatus, "签名/信任状态由安装时校验结果重放，不重新执行插件。", map[string]any{"signature_status": plugin.SignatureStatus})

	packageStatus := "passed"
	packageDetail := "安装包可重放并与固定 content hash 一致。"
	if strings.HasPrefix(plugin.PluginID, "builtin.") {
		packageDetail = "内置插件属于应用基线，不要求外部包重放。"
	} else {
		var blob []byte
		if err := s.control.DB.QueryRowContext(ctx, `SELECT package_blob FROM harness_plugin_versions WHERE plugin_id=? AND version=?`, plugin.PluginID, plugin.Version).Scan(&blob); err != nil {
			return PluginConformanceReport{}, err
		}
		if len(blob) == 0 {
			packageStatus, packageDetail = "warning", "插件已退役或包体不可用；历史对象仍可读，但无法重放包级 conformance。"
		} else if pkg, parseErr := parsePackage(blob); parseErr != nil {
			packageStatus, packageDetail = "failed", parseErr.Error()
		} else if pkg.Hash != plugin.ContentHash {
			packageStatus, packageDetail = "failed", "安装包重新计算的 hash 与固定 content hash 不一致。"
		} else if _, validateErr := validateManifest(pkg); validateErr != nil {
			packageStatus, packageDetail = "failed", validateErr.Error()
		}
	}
	addConformanceCheck(&report, "deterministic_package", packageStatus, packageDetail, nil)

	state := projectStateFor(plugin, report.ProjectID)
	if report.ProjectID != "" {
		if state != nil {
			report.GrantedCapabilities = append([]string(nil), state.GrantedCapabilities...)
		} else if strings.HasPrefix(plugin.PluginID, "builtin.") && plugin.Status == "enabled" {
			report.GrantedCapabilities = append([]string(nil), plugin.Permissions...)
		}
		report.MissingRequired = difference(manifest.Permissions.Required, report.GrantedCapabilities)
		report.OptionalNotGranted = difference(manifest.Permissions.Optional, report.GrantedCapabilities)
		permissionStatus := "passed"
		permissionDetail := "当前项目授权覆盖全部 required capabilities。"
		if len(report.MissingRequired) > 0 {
			permissionStatus, permissionDetail = "failed", "当前项目缺少 required capabilities，插件不能安全启用。"
		}
		addConformanceCheck(&report, "permission_diff", permissionStatus, permissionDetail, map[string]any{"required_missing": report.MissingRequired, "optional_not_granted": report.OptionalNotGranted, "granted": report.GrantedCapabilities})
	}

	schema, schemaErr := s.configurationSchema(ctx, plugin)
	if schemaErr != nil {
		report.ConfigurationStatus = "unavailable"
		addConformanceCheck(&report, "configuration_schema", "warning", schemaErr.Error(), nil)
	} else if len(schema) > 0 {
		report.ConfigurationSchema = schema
		config := json.RawMessage(`{}`)
		enabled := false
		if state != nil {
			config = state.Config
			enabled = state.Status == "enabled"
		}
		if _, err := harness.ValidateAgainstSchema(schema, config); err != nil {
			report.ConfigurationStatus = "invalid"
			status := "warning"
			if enabled {
				status = "failed"
			}
			addConformanceCheck(&report, "configuration_schema", status, "当前项目配置不满足插件声明 Schema："+err.Error(), nil)
		} else {
			report.ConfigurationStatus = "valid"
			addConformanceCheck(&report, "configuration_schema", "passed", "当前项目配置通过内核 bounded JSON Schema 校验。", nil)
		}
	}

	lifecycleStatus := "passed"
	lifecycleDetail := "插件版本可参与受治理项目配置。"
	if plugin.Status == "quarantined" {
		lifecycleStatus, lifecycleDetail = "failed", "插件已隔离，不能参与任何新运行。"
	}
	if plugin.Status == "uninstalled" {
		lifecycleStatus, lifecycleDetail = "warning", "插件已退役，仅保留历史可读性。"
	}
	addConformanceCheck(&report, "lifecycle", lifecycleStatus, lifecycleDetail, map[string]any{"status": plugin.Status})
	addConformanceCheck(&report, "external_effect_probe", "not_executed", "Conformance 不执行插件、网络连接或工具副作用；运行期效果必须由 Run/Effect receipt 验证。", map[string]any{"mode": "no_side_effects"})
	return report, nil
}
