package pipeline_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/pipeline"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestModelStageProducesValidatedOutputAndEffectReceipt(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer local-test-secret" {
			http.Error(w, "missing secret", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": `{"statement":"Durable model output"}`}}}})
	}))
	defer mock.Close()

	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "model-stage", Name: "Model Stage", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	models := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
	provider, err := models.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: "Mock Model", Kind: "openai_compatible", BaseURL: mock.URL + "/v1", Model: "mock", APIKey: "local-test-secret", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = models.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID}); err != nil {
		t.Fatal(err)
	}
	service := pipeline.New(a.Control, a.Harness, models)
	_, err = a.Harness.RegisterType(t.Context(), harness.RegisterTypeInput{
		TypeID: "builtin.model.note", PluginID: "builtin.model", DisplayName: "Model Note", SchemaVersion: "1.0.0",
		Schema:    json.RawMessage(`{"type":"object","required":["statement"],"properties":{"statement":{"type":"string"}},"additionalProperties":false}`),
		Lifecycle: harness.Lifecycle{Initial: "active", States: []string{"active"}, Transitions: map[string][]string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.Publish(t.Context(), "builtin.model", []byte(`apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.model.extract-note
version: 1.0.0
name: Model extract note
intent: Exercise the governed model stage.
requiredCapabilities: [model.invoke, memory.materialize]
nodes:
  - {id: input, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {}}
  - id: extract
    stageType: llm.extract
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [input]
    config:
      prompt: Extract one durable statement.
      output_schema: {type: object, required: [statement], properties: {statement: {type: string}}, additionalProperties: false}
      max_tokens: 200
  - id: materialize
    stageType: object.materialize
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [extract]
    config: {type_id: builtin.model.note, plugin_id: builtin.model, plugin_version: 1.0.0}
outputs: [{name: object, nodeId: materialize}]
policy: {maxStages: 8, timeoutSeconds: 30, maxModelCalls: 1}
`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Execute(t.Context(), pipeline.ExecuteInput{ProjectID: project.ProjectID, CallerType: "owner", CallerID: "owner", Channel: "desktop", PipelineID: published.PipelineID, PipelineVersion: published.Version, IdempotencyKey: "model-run", Input: json.RawMessage(`{"conversation":"untrusted"}`), EffectiveCapabilities: []string{"model.invoke", "memory.materialize"}})
	if err != nil || result.Status != "completed" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	detail, err := a.Harness.RunDetail(t.Context(), result.RunID)
	if err != nil || len(detail.Effects) != 1 {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	effect := detail.Effects[0]
	if effect.Status != "materialized" || effect.Outcome != "confirmed" || effect.ResultHash == "" {
		t.Fatalf("effect=%#v", effect)
	}
	if string(effect.Receipt) == "" || string(effect.Receipt) == "{}" {
		t.Fatalf("missing provider receipt: %#v", effect)
	}
	if strings.Contains(string(effect.Receipt), "local-test-secret") {
		t.Fatal("secret leaked into effect receipt")
	}
}
