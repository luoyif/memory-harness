package memory

import "testing"

func TestClassifyAgentAssetDeterministicTaxonomy(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"prompt", "提示词模板包含变量 {{topic}}、when_to_use 和固定输出格式。", AssetTypePrompt},
		{"skill", "使用 Wails 创建桌面应用，并通过 go test 验证实现。", AssetTypeSkill},
		{"rule", "代码审查原则：高风险修改应该优先检查来源与回滚策略。", AssetTypeRule},
		{"constraint", "生产环境不得直接删除原始 Evidence，任何删除必须先经过人工审批。", AssetTypeConstraint},
		{"procedure", "每次发布前第一步运行完整测试，然后检查 git diff，最后再生成发布包。", AssetTypeProcedure},
		{"tool_recipe", "调用 API 前检查参数 schema，失败时按错误码重试，并验证返回值。", AssetTypeToolRecipe},
		{"mcp", "MCP server 通过 stdio transport 暴露 tool manifest，并声明权限与 capabilities。", AssetTypeMCP},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyAgentAssetText(test.text)
			if got.Type != test.want {
				t.Fatalf("type=%q scores=%v reasons=%v want=%q", got.Type, got.Scores, got.Reasons, test.want)
			}
		})
	}
}

func TestProcedureWinsWhenFrequencyAndConstraintSignalsCoexist(t *testing.T) {
	got := classifyAgentAssetText("每次执行任务时必须先输出计划，然后逐项检查完成状态，最后停止。")
	if got.Type != AssetTypeProcedure {
		t.Fatalf("type=%q scores=%v reasons=%v", got.Type, got.Scores, got.Reasons)
	}
}

func TestClassifyAgentAssetAbstainsWhenSignalsAreInsufficient(t *testing.T) {
	got := classifyAgentAssetText("团队今天讨论了未来方向，大家交换了很多想法。")
	if got.Type != AssetTypeUnclassified || !got.Ambiguous {
		t.Fatalf("expected abstention, got type=%q ambiguous=%v scores=%v", got.Type, got.Ambiguous, got.Scores)
	}
}
