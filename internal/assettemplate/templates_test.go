package assettemplate_test

import (
	"encoding/json"
	"testing"

	"github.com/luoyif/memory-harness/internal/assettemplate"
)

func TestSkillTemplateRequiresExecutableContractAndPreservesLineage(t *testing.T) {
	payload := assettemplate.Payload{
		AssetID: "asset-skill-1", AssetType: "skill", Title: "安全发布检查", Summary: "在发布前完成验证并保留来源。",
		TemplateVersion: assettemplate.TemplateVersion,
		Spec: map[string]any{
			"purpose": "在发布前验证交付物", "triggers": []string{"用户要求发布时"}, "inputs": []string{"候选版本"},
			"outputs": []string{"验收报告"}, "steps": []string{"核对范围", "运行测试", "检查回滚"}, "tools": []string{"测试运行器"},
			"constraints": []string{"不得改写原材料"}, "success_checks": []string{"全部测试明确通过"}, "failure_handling": []string{"停止发布并报告失败"},
		},
		SourceMemoryIDs: []string{"memory-1"}, SourceAssetIDs: []string{"asset-skill-1"},
		Refinement: assettemplate.Refinement{Mode: "owner_edit", GeneratedAt: "2026-08-24T00:00:00Z", SourceUpdatedAt: "2026-08-23T00:00:00Z", MissingFields: []string{}},
	}
	raw, _ := json.Marshal(payload)
	canonical, validation, err := assettemplate.ValidatePayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	var got assettemplate.Payload
	if err := json.Unmarshal(canonical, &got); err != nil {
		t.Fatal(err)
	}
	if got.ValidationStatus != "passed" || got.Body == "" {
		t.Fatalf("payload=%#v validation=%s", got, validation)
	}
	if got.Refinement.Mode != "owner_edit" || len(got.SourceMemoryIDs) != 1 || len(got.SourceAssetIDs) != 1 {
		t.Fatalf("lineage/refinement lost: %#v", got)
	}
}

func TestFallbackSkeletonCannotMasqueradeAsValidatedAsset(t *testing.T) {
	payload := assettemplate.Skeleton(assettemplate.Source{AssetID: "asset-rule-1", AssetType: "rule", Title: "发布规则", Body: "发布前先测试", SourceMemoryIDs: []string{"memory-1"}, UpdatedAt: "2026-08-23T00:00:00Z"}, "2026-08-24T00:00:00Z")
	raw, _ := json.Marshal(payload)
	canonical, validation, err := assettemplate.ValidatePayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	var got assettemplate.Payload
	_ = json.Unmarshal(canonical, &got)
	if got.ValidationStatus != "failed" || len(got.Refinement.MissingFields) == 0 {
		t.Fatalf("fallback became usable: payload=%#v validation=%s", got, validation)
	}
}

func TestAllSevenTemplatesExposeDistinctRequiredFields(t *testing.T) {
	templates := assettemplate.Templates()
	if len(templates) != 7 {
		t.Fatalf("templates=%d", len(templates))
	}
	seen := map[string]bool{}
	for _, template := range templates {
		if seen[template.AssetType] || len(template.Fields) < 5 {
			t.Fatalf("invalid template %#v", template)
		}
		seen[template.AssetType] = true
		required := 0
		for _, field := range template.Fields {
			if field.Required {
				required++
			}
		}
		if required < 4 {
			t.Fatalf("template %s has only %d required fields", template.AssetType, required)
		}
	}
}
