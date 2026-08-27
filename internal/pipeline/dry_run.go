package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/luoyif/memory-harness/internal/harness"
)

func previewDeterministicStage(node Node, inputRaw json.RawMessage) (json.RawMessage, error) {
	switch node.StageType {
	case "trigger.manual":
		return inputRaw, nil
	case "transform.map":
		var config struct {
			Value json.RawMessage `json:"value"`
			Merge json.RawMessage `json:"merge"`
		}
		if err := json.Unmarshal(node.Config, &config); err != nil {
			return nil, err
		}
		if len(config.Value) > 0 {
			return config.Value, nil
		}
		if len(config.Merge) > 0 {
			var base, addition map[string]any
			if err := json.Unmarshal(inputRaw, &base); err != nil {
				return nil, errors.New("transform.map merge requires object input")
			}
			if err := json.Unmarshal(config.Merge, &addition); err != nil {
				return nil, errors.New("transform.map merge must be an object")
			}
			for key, value := range addition {
				base[key] = value
			}
			out, _ := json.Marshal(base)
			return out, nil
		}
		return inputRaw, nil
	case "transform.filter":
		var config struct {
			Field  string `json:"field"`
			Equals any    `json:"equals"`
		}
		if err := json.Unmarshal(node.Config, &config); err != nil {
			return nil, err
		}
		var value map[string]any
		if err := json.Unmarshal(inputRaw, &value); err != nil {
			return nil, errors.New("transform.filter requires object input")
		}
		actual, _ := json.Marshal(value[config.Field])
		expected, _ := json.Marshal(config.Equals)
		if string(actual) != string(expected) {
			return json.RawMessage(`null`), nil
		}
		return inputRaw, nil
	case "policy.require_review":
		return inputRaw, nil
	default:
		return nil, fmt.Errorf("stage %s is not a pure preview stage", node.StageType)
	}
}

func dryRunStageFlags(descriptor StageDescriptor) (wouldWrite, wouldInvokeModel bool) {
	switch descriptor.Class {
	case "memory", "project":
		wouldWrite = true
	case "model":
		wouldInvokeModel = true
	}
	if descriptor.StageType == "search.refresh" || descriptor.StageType == "object.materialize" {
		wouldWrite = true
	}
	return
}

// DryRun performs no Run creation, model invocation, object materialization or
// business writes. Pure JSON stages are evaluated in memory; stateful stages
// are planned and validated as far as their known input permits.
func (s *Service) DryRun(ctx context.Context, input ExecuteInput) (DryRunResult, error) {
	version, err := s.Version(ctx, input.PipelineID, input.PipelineVersion)
	if err != nil {
		return DryRunResult{}, err
	}
	ordered, err := validateDefinition(version.PluginID, version.Definition)
	if err != nil {
		return DryRunResult{}, err
	}
	if err := s.checkPluginProject(ctx, version, input.ProjectID, input.EffectiveCapabilities); err != nil {
		return DryRunResult{}, err
	}
	if len(input.Input) == 0 || !json.Valid(input.Input) {
		return DryRunResult{}, errors.New("input must be valid JSON")
	}
	result := DryRunResult{
		ProjectID: input.ProjectID, PipelineID: version.PipelineID, PipelineVersion: version.Version,
		PipelineHash: version.ContentHash, Nodes: []DryRunNode{}, Warnings: []string{}, NoWritesPerformed: true,
	}
	outputs := map[string]json.RawMessage{}
	known := map[string]bool{}
	for _, node := range ordered {
		descriptor := stageCatalog[node.StageType]
		item := DryRunNode{NodeID: node.ID, StageType: node.StageType, Class: descriptor.Class}
		wouldWrite, wouldModel := dryRunStageFlags(descriptor)
		item.WouldWrite, item.WouldInvokeModel = wouldWrite, wouldModel
		if wouldWrite {
			result.PlannedWrites++
		}
		if wouldModel {
			result.PlannedModelCalls++
		}
		dependenciesKnown := true
		for _, dependency := range node.DependsOn {
			dependenciesKnown = dependenciesKnown && known[dependency]
		}
		if !dependenciesKnown {
			item.Status = "blocked_by_unknown_input"
			item.Detail = "上游包含未执行的有状态或模型阶段；Dry Run 不伪造其输出。"
			result.Nodes = append(result.Nodes, item)
			continue
		}
		nodeInput := dependencyInput(node, outputs, input.Input)
		item.InputHash = hashBytes(nodeInput)
		switch node.StageType {
		case "trigger.manual", "transform.map", "transform.filter", "policy.require_review":
			preview, err := previewDeterministicStage(node, nodeInput)
			if err != nil {
				item.Status, item.Detail = "invalid", err.Error()
				result.Nodes = append(result.Nodes, item)
				return result, fmt.Errorf("dry run node %s: %w", node.ID, err)
			}
			item.Preview, item.OutputHash = preview, hashBytes(preview)
			if node.StageType == "policy.require_review" {
				item.Status, item.ReviewGate = "would_wait_for_owner", true
				item.Detail = "会停在 Owner Review；Dry Run 继续使用同一输入预览后续纯阶段。"
				result.ReviewGates++
			} else {
				item.Status = "previewed"
				item.Detail = "纯 JSON 阶段已在内存中执行；无持久化。"
				result.PurePreviewed++
			}
			outputs[node.ID], known[node.ID] = preview, true
		case "object.materialize":
			var config struct {
				TypeID string `json:"type_id"`
			}
			if err := json.Unmarshal(node.Config, &config); err != nil {
				return result, err
			}
			typeDef, err := s.harness.Type(ctx, strings.TrimSpace(config.TypeID))
			if err != nil {
				item.Status, item.Detail = "invalid", err.Error()
				result.Nodes = append(result.Nodes, item)
				return result, fmt.Errorf("dry run object type: %w", err)
			}
			if _, err := harness.ValidateAgainstSchema(typeDef.Schema, nodeInput); err != nil {
				item.Status, item.Detail = "invalid_payload", err.Error()
				result.Nodes = append(result.Nodes, item)
				return result, fmt.Errorf("dry run materialize %s: %w", node.ID, err)
			}
			item.Status = "would_materialize"
			item.Detail = "Schema 验证通过；真实执行会创建/修订 Harness Object，但 Dry Run 没有写入。"
		case "llm.extract":
			item.Status = "would_invoke_model"
			item.Detail = "模型调用被跳过；输出保持 unknown，因此依赖它的后续阶段不会被伪造。"
		case "memory.compile", "memory.semantic_graph", "memory.materialize", "project.derive", "search.refresh":
			item.Status = "would_execute_stateful_stage"
			item.Detail = "有状态业务阶段被跳过；已验证 Pipeline/权限/输入边界，但没有触发数据库或索引写入。"
		default:
			item.Status = "planned"
			item.Detail = "阶段只做计划展示。"
		}
		result.Nodes = append(result.Nodes, item)
	}
	if result.PlannedModelCalls > 0 {
		result.Warnings = append(result.Warnings, "模型阶段未执行，后续依赖模型输出的节点只能显示计划。")
	}
	if result.PlannedWrites > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("真实执行预计触发 %d 个有状态写入阶段；Dry Run 已全部跳过。", result.PlannedWrites))
	}
	return result, nil
}
