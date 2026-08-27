package pipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var pipelineIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,159}$`)
var pipelineSemverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$`)

var stageCatalog = map[string]StageDescriptor{
	"trigger.manual":        {"trigger.manual", "1.0.0", "deterministic", nil, "Accept the versioned run input."},
	"memory.compile":        {"memory.compile", "1.0.0", "memory", []string{"evidence.read", "memory.propose", "model.invoke"}, "Distill one canonical Evidence session into versioned knowledge and governed long-term candidates."},
	"memory.semantic_graph": {"memory.semantic_graph", "1.0.0", "memory", []string{"memory.read", "memory.materialize"}, "Project resolved semantic frames into project-scoped entities, assertions and temporal facts."},
	"memory.materialize":    {"memory.materialize", "1.0.0", "memory", []string{"memory.read", "memory.materialize", "asset.propose"}, "Materialize inspectable typed memory objects with source and run provenance."},
	"project.derive":        {"project.derive", "1.0.0", "project", []string{"memory.read", "project.write"}, "Route canonical derivatives and refresh project context, decisions, goals and risks."},
	"search.refresh":        {"search.refresh", "1.0.0", "deterministic", []string{"memory.read"}, "Refresh the project-scoped unified retrieval projection."},
	"transform.map":         {"transform.map", "1.0.0", "deterministic", nil, "Replace or merge a JSON value."},
	"transform.filter":      {"transform.filter", "1.0.0", "deterministic", nil, "Route input using an explicit equality predicate."},
	"policy.require_review": {"policy.require_review", "1.0.0", "human", nil, "Pause the run at a visible owner review gate."},
	"llm.extract":           {"llm.extract", "1.0.0", "model", []string{"model.invoke"}, "Transform untrusted JSON with the active model and validate its schema."},
	"object.materialize":    {"object.materialize", "1.0.0", "deterministic", []string{"memory.materialize"}, "Validate and commit a versioned generic memory object."},
}

func validateStageConfig(node Node) error {
	if len(node.Config) == 0 || !json.Valid(node.Config) {
		return errors.New("config must be valid JSON")
	}
	switch node.StageType {
	case "trigger.manual", "memory.compile", "memory.semantic_graph", "memory.materialize", "project.derive", "search.refresh":
		var value map[string]any
		if err := json.Unmarshal(node.Config, &value); err != nil {
			return errors.New("config must be an object")
		}
	case "transform.map":
		var value struct {
			Value json.RawMessage `json:"value"`
			Merge json.RawMessage `json:"merge"`
		}
		if err := json.Unmarshal(node.Config, &value); err != nil {
			return err
		}
		if len(value.Value) > 0 && len(value.Merge) > 0 {
			return errors.New("value and merge are mutually exclusive")
		}
		if len(value.Merge) > 0 {
			var object map[string]any
			if err := json.Unmarshal(value.Merge, &object); err != nil {
				return errors.New("merge must be an object")
			}
		}
	case "transform.filter":
		var value struct {
			Field string `json:"field"`
		}
		if err := json.Unmarshal(node.Config, &value); err != nil {
			return err
		}
		if strings.TrimSpace(value.Field) == "" {
			return errors.New("field is required")
		}
	case "policy.require_review":
		var value struct {
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(node.Config, &value); err != nil {
			return err
		}
		if strings.TrimSpace(value.Reason) == "" {
			return errors.New("reason is required")
		}
	case "llm.extract":
		var value struct {
			Prompt       string          `json:"prompt"`
			OutputSchema json.RawMessage `json:"output_schema"`
			MaxTokens    int             `json:"max_tokens"`
		}
		if err := json.Unmarshal(node.Config, &value); err != nil {
			return err
		}
		if strings.TrimSpace(value.Prompt) == "" || len(value.OutputSchema) == 0 || !json.Valid(value.OutputSchema) {
			return errors.New("prompt and valid output_schema are required")
		}
		if value.MaxTokens < 0 || value.MaxTokens > 32000 {
			return errors.New("max_tokens must be between 0 and 32000")
		}
	case "object.materialize":
		var value struct {
			TypeID string `json:"type_id"`
		}
		if err := json.Unmarshal(node.Config, &value); err != nil {
			return err
		}
		if strings.TrimSpace(value.TypeID) == "" {
			return errors.New("type_id is required")
		}
	}
	return nil
}

func OwnerCapabilities() []string {
	return []string{
		"evidence.read", "evidence.capture", "memory.read", "memory.propose", "memory.materialize",
		"project.read", "project.write", "finance.read", "finance.write", "asset.read", "asset.propose",
		"asset.activate", "model.invoke", "connector.invoke", "notification.emit", "trace.read_payload",
	}
}

func StageCatalog() []StageDescriptor {
	items := make([]StageDescriptor, 0, len(stageCatalog))
	for _, item := range stageCatalog {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StageType < items[j].StageType })
	return items
}

func parseDefinition(raw []byte) (Definition, []byte, error) {
	if len(raw) == 0 || len(raw) > 1<<20 {
		return Definition{}, nil, errors.New("pipeline definition must be between 1 byte and 1 MiB")
	}
	if len(bytes.TrimSpace(raw)) > 0 && bytes.TrimSpace(raw)[0] == '{' {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var definition Definition
		if err := decoder.Decode(&definition); err != nil {
			return Definition{}, nil, err
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			if err == nil {
				return Definition{}, nil, errors.New("pipeline must contain one JSON object")
			}
			return Definition{}, nil, err
		}
		return normalizeStructuredDefinition(definition)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var definition Definition
	if err := decoder.Decode(&definition); err != nil {
		return Definition{}, nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return Definition{}, nil, errors.New("pipeline must contain one document")
		}
		return Definition{}, nil, err
	}
	for i := range definition.Nodes {
		config, err := json.Marshal(definition.Nodes[i].ConfigYAML)
		if err != nil {
			return Definition{}, nil, err
		}
		if string(config) == "null" {
			config = []byte(`{}`)
		}
		definition.Nodes[i].Config = config
		definition.Nodes[i].ConfigYAML = nil
	}
	canonical, err := json.Marshal(definition)
	return definition, canonical, err
}

func normalizeStructuredDefinition(definition Definition) (Definition, []byte, error) {
	for index := range definition.Nodes {
		if len(definition.Nodes[index].Config) == 0 {
			definition.Nodes[index].Config = json.RawMessage(`{}`)
		}
		var value any
		decoder := json.NewDecoder(bytes.NewReader(definition.Nodes[index].Config))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return Definition{}, nil, fmt.Errorf("node %q config: %w", definition.Nodes[index].ID, err)
		}
		canonical, err := json.Marshal(value)
		if err != nil {
			return Definition{}, nil, err
		}
		definition.Nodes[index].Config = canonical
		definition.Nodes[index].ConfigYAML = nil
	}
	if definition.Editor.Positions == nil {
		definition.Editor.Positions = map[string]NodePosition{}
	}
	canonical, err := json.Marshal(definition)
	return definition, canonical, err
}

func validateDefinition(pluginID string, definition Definition) ([]Node, error) {
	if definition.APIVersion != APIVersion {
		return nil, fmt.Errorf("apiVersion must be %s", APIVersion)
	}
	if !pipelineIDPattern.MatchString(definition.PipelineID) || !strings.HasPrefix(definition.PipelineID, pluginID+".") {
		return nil, errors.New("pipeline_id must be a stable identifier namespaced by plugin_id")
	}
	if !pipelineSemverPattern.MatchString(definition.Version) || strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.Intent) == "" {
		return nil, errors.New("pipeline semantic version, name and intent are required")
	}
	if len(definition.Nodes) == 0 || len(definition.Nodes) > 128 {
		return nil, errors.New("pipeline must contain between 1 and 128 nodes")
	}
	if definition.Policy.MaxStages == 0 {
		definition.Policy.MaxStages = 128
	}
	if definition.Policy.MaxStages < len(definition.Nodes) || definition.Policy.MaxStages > 512 {
		return nil, errors.New("policy.maxStages must cover the graph and be at most 512")
	}
	nodes := map[string]Node{}
	declaredCapabilities := map[string]bool{}
	for _, capability := range definition.RequiredCapabilities {
		declaredCapabilities[capability] = true
	}
	dependents := map[string][]string{}
	indegree := map[string]int{}
	modelCalls := 0
	for _, node := range definition.Nodes {
		if !pipelineIDPattern.MatchString(node.ID) || node.StageVersion == "" || node.PluginID == "" {
			return nil, fmt.Errorf("node %q must have stable id, stage version and plugin id", node.ID)
		}
		if _, exists := nodes[node.ID]; exists {
			return nil, fmt.Errorf("duplicate node %q", node.ID)
		}
		descriptor, exists := stageCatalog[node.StageType]
		if !exists {
			return nil, fmt.Errorf("node %q uses unknown stage type %q", node.ID, node.StageType)
		}
		if node.StageVersion != descriptor.Version {
			return nil, fmt.Errorf("node %q pins unsupported stage version %q", node.ID, node.StageVersion)
		}
		if err := validateStageConfig(node); err != nil {
			return nil, fmt.Errorf("node %q config: %w", node.ID, err)
		}
		if descriptor.Class == "model" {
			modelCalls++
		}
		for _, capability := range descriptor.Capabilities {
			if !declaredCapabilities[capability] {
				return nil, fmt.Errorf("node %q requires undeclared capability %q", node.ID, capability)
			}
		}
		seenDependencies := map[string]bool{}
		for _, dependency := range node.DependsOn {
			if seenDependencies[dependency] {
				return nil, fmt.Errorf("node %q repeats dependency %q", node.ID, dependency)
			}
			seenDependencies[dependency] = true
		}
		nodes[node.ID] = node
		indegree[node.ID] = len(node.DependsOn)
	}
	for nodeID := range definition.Editor.Positions {
		if _, exists := nodes[nodeID]; !exists {
			return nil, fmt.Errorf("editor position references missing node %q", nodeID)
		}
	}
	if modelCalls > definition.Policy.MaxModelCalls {
		return nil, fmt.Errorf("pipeline contains %d model calls but policy allows %d", modelCalls, definition.Policy.MaxModelCalls)
	}
	for _, node := range definition.Nodes {
		for _, dependency := range node.DependsOn {
			if _, exists := nodes[dependency]; !exists {
				return nil, fmt.Errorf("node %q depends on missing node %q", node.ID, dependency)
			}
			dependents[dependency] = append(dependents[dependency], node.ID)
		}
	}
	queue := []string{}
	for id, count := range indegree {
		if count == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)
	ordered := make([]Node, 0, len(nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		ordered = append(ordered, nodes[id])
		for _, dependent := range dependents[id] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				queue = append(queue, dependent)
				sort.Strings(queue)
			}
		}
	}
	if len(ordered) != len(nodes) {
		return nil, errors.New("pipeline graph contains a cycle")
	}
	if len(definition.Outputs) == 0 {
		return nil, errors.New("pipeline must declare at least one output")
	}
	for _, output := range definition.Outputs {
		if strings.TrimSpace(output.Name) == "" {
			return nil, errors.New("pipeline output name is required")
		}
		if _, exists := nodes[output.NodeID]; !exists {
			return nil, fmt.Errorf("output %q references missing node %q", output.Name, output.NodeID)
		}
	}
	return ordered, nil
}
