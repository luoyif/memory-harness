package modelconfig_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/modelusage"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestProviderProbeRuntimeAndAgentExtraction(t *testing.T) {
	const secret = "test-secret-value"
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "memory-model"}}})
		case "/v1/chat/completions":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["model"] != "memory-model" {
				t.Fatalf("request=%#v", request)
			}
			if _, exists := request["reasoning_split"]; exists {
				t.Fatalf("portable provider received MiniMax-only field: %#v", request)
			}
			content := `{"candidates":[{"evidence_id":"ev-agent","statement":"MemoryOS uses project-scoped Agent permissions.","unit_type":"decision","tier_hint":"semantic","risk_tier":"B","confidence":0.94}]}`
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	a, _ := testutil.Open(t)
	secrets := modelconfig.NewMemorySecretStore()
	service := modelconfig.New(a.Control, secrets, mock.Client())
	provider, err := service.SaveProvider(context.Background(), modelconfig.ProviderInput{Name: "Acceptance Model", Kind: "openai_compatible", BaseURL: mock.URL + "/v1", Model: "memory-model", APIKey: secret, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.HasSecret || strings.Contains(mustJSON(t, provider), secret) {
		t.Fatalf("provider=%#v", provider)
	}
	probe, err := service.Probe(t.Context(), provider.ProviderID)
	if err != nil || probe.Status != "pass" || !probe.SelectedFound {
		t.Fatalf("probe=%#v err=%v", probe, err)
	}
	runtime, err := service.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID, FallbackToRules: true})
	if err != nil || runtime.Mode != "agent" {
		t.Fatalf("runtime=%#v err=%v", runtime, err)
	}
	result, err := service.Extract(t.Context(), memory.ExtractionRequest{SessionID: "session-agent", Turns: []memory.ExtractionTurn{{EvidenceID: "ev-agent", Role: "user", Text: "Use project-scoped permissions"}}})
	if err != nil || len(result.Candidates) != 1 || result.Candidates[0].EvidenceID != "ev-agent" || !strings.HasPrefix(result.Compiler, "agent/") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if !strings.Contains(service.Compiler(t.Context()), "memory-model") {
		t.Fatalf("compiler=%q", service.Compiler(t.Context()))
	}
}

func TestProviderValidationAndRulesDefault(t *testing.T) {
	a, _ := testutil.Open(t)
	service := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), nil)
	runtime, err := service.Runtime(t.Context())
	if err != nil || runtime.Mode != "rules" || service.Compiler(t.Context()) != memory.CompilerVersion {
		t.Fatalf("runtime=%#v err=%v", runtime, err)
	}
	if _, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: "Unsafe", Kind: "openai_compatible", BaseURL: "http://example.com/v1", Model: "x"}); err == nil {
		t.Fatal("expected insecure remote provider URL rejection")
	}
	if _, err := service.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: "missing", FallbackToRules: true}); err == nil {
		t.Fatal("expected missing provider rejection")
	}
}

func TestGenerateJSONUsesMiniMaxControlsAndSelectsTheSchemaMatchingValue(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			ReasoningSplit      bool           `json:"reasoning_split"`
			MaxCompletionTokens int            `json:"max_completion_tokens"`
			MaxTokens           int            `json:"max_tokens"`
			Temperature         float64        `json:"temperature"`
			Thinking            map[string]any `json:"thinking"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if !request.ReasoningSplit || request.MaxCompletionTokens != 12000 || request.MaxTokens != 0 || request.Temperature <= 0 || request.Thinking["type"] != "disabled" {
			t.Fatalf("MiniMax structured-output controls are incomplete: %#v", request)
		}
		content := `<think>先比较候选。这里的 {"debug":true} 不是最终结果。</think>` + "\n```json\n" + `{"items":[{"asset_id":"asset-1"}]}` + "\n```"
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
	}))
	defer mock.Close()

	a, _ := testutil.Open(t)
	service := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
	provider, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: "MiniMax JSON", Kind: "openai_compatible", BaseURL: mock.URL + "/v1", Model: "MiniMax-M3", APIKey: "secret", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID, FallbackToRules: true}); err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"type":"object","required":["items"],"properties":{"items":{"type":"array","items":{"type":"object","required":["asset_id"],"properties":{"asset_id":{"type":"string"}},"additionalProperties":false}}},"additionalProperties":false}`)
	result, err := service.GenerateJSON(t.Context(), modelconfig.JSONGenerationRequest{SystemPrompt: "Return assets.", Input: json.RawMessage(`{"source":"test"}`), OutputSchema: schema, MaxTokens: 12000})
	if err != nil || string(result.Output) != `{"items":[{"asset_id":"asset-1"}]}` {
		t.Fatalf("result=%s err=%v", result.Output, err)
	}
}

func TestGenerateJSONKeepsPortableRequestPortableAndDoesNotLeakInvalidOutput(t *testing.T) {
	const privateOutput = "PRIVATE_MODEL_OUTPUT_MUST_NOT_LEAK"
	var calls atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if _, exists := request["reasoning_split"]; exists || request["max_tokens"] != float64(700) {
			t.Fatalf("portable provider request was changed: %#v", request)
		}
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": `{"ok":true}`}}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"finish_reason": "length", "message": map[string]string{"content": privateOutput}}}})
	}))
	defer mock.Close()

	a, _ := testutil.Open(t)
	service := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
	provider, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: "Portable JSON", Kind: "openai_compatible", BaseURL: mock.URL + "/v1", Model: "portable-model", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID, FallbackToRules: true}); err != nil {
		t.Fatal(err)
	}
	request := modelconfig.JSONGenerationRequest{SystemPrompt: "Return status.", Input: json.RawMessage(`{"source":"test"}`), OutputSchema: json.RawMessage(`{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}},"additionalProperties":false}`), MaxTokens: 700}
	if result, err := service.GenerateJSON(t.Context(), request); err != nil || string(result.Output) != `{"ok":true}` {
		t.Fatalf("result=%s err=%v", result.Output, err)
	}
	if _, err := service.GenerateJSON(t.Context(), request); err == nil || strings.Contains(err.Error(), privateOutput) || !strings.Contains(err.Error(), "finish_reason=length") {
		t.Fatalf("invalid output diagnostics were unsafe or incomplete: %v", err)
	}
}

func TestGenerateJSONRepairsOnlyAnUnambiguousTopLevelArray(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `[{"asset_id":"asset-1","title":"发布检查"}]`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}}}})
	}))
	defer mock.Close()

	a, _ := testutil.Open(t)
	service := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
	provider, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: "Array Repair Model", Kind: "openai_compatible", BaseURL: mock.URL + "/v1", Model: "array-model", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID, FallbackToRules: true}); err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"type":"object","required":["items"],"properties":{"items":{"type":"array","items":{"type":"object","required":["asset_id","title"],"properties":{"asset_id":{"type":"string"},"title":{"type":"string"}},"additionalProperties":false}}},"additionalProperties":false}`)
	result, err := service.GenerateJSON(t.Context(), modelconfig.JSONGenerationRequest{SystemPrompt: "Return items.", Input: json.RawMessage(`{"source":"test"}`), OutputSchema: schema, MaxTokens: 500})
	if err != nil || string(result.Output) != `{"items":[{"asset_id":"asset-1","title":"发布检查"}]}` {
		t.Fatalf("result=%s err=%v", result.Output, err)
	}
}

func TestGenerateJSONRepairsOneRedundantBatchArrayLayer(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `{"items":[[{"asset_id":"asset-1","title":"发布检查"}]]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}}}})
	}))
	defer mock.Close()

	a, _ := testutil.Open(t)
	service := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
	provider, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: "Nested Array Repair Model", Kind: "openai_compatible", BaseURL: mock.URL + "/v1", Model: "nested-array-model", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID, FallbackToRules: true}); err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"type":"object","required":["items"],"properties":{"items":{"type":"array","items":{"type":"object","required":["asset_id","title"],"properties":{"asset_id":{"type":"string"},"title":{"type":"string"}},"additionalProperties":false}}},"additionalProperties":false}`)
	result, err := service.GenerateJSON(t.Context(), modelconfig.JSONGenerationRequest{SystemPrompt: "Return items.", Input: json.RawMessage(`{"source":"test"}`), OutputSchema: schema, MaxTokens: 500})
	if err != nil || string(result.Output) != `{"items":[{"asset_id":"asset-1","title":"发布检查"}]}` {
		t.Fatalf("result=%s err=%v", result.Output, err)
	}
}

func TestGenerateJSONSupportsResponsesAndAnthropicMessages(t *testing.T) {
	tests := []struct {
		name          string
		kind          string
		protocol      string
		model         string
		expectedPath  string
		expectBearer  bool
		expectXAPIKey bool
		response      map[string]any
		expectedUsage [3]int
		validateBody  func(*testing.T, map[string]any)
	}{
		{
			name: "OpenAI Responses", kind: "openai", protocol: modelconfig.ProtocolOpenAIResponses, model: "gpt-response-test", expectedPath: "/v1/responses", expectBearer: true,
			response:      map[string]any{"status": "completed", "output": []map[string]any{{"type": "message", "content": []map[string]any{{"type": "output_text", "text": `{"answer":"ok"}`}}}}, "usage": map[string]any{"input_tokens": 21, "output_tokens": 4, "total_tokens": 25}},
			expectedUsage: [3]int{21, 4, 0},
			validateBody: func(t *testing.T, body map[string]any) {
				if body["instructions"] == nil || body["input"] == nil || body["max_output_tokens"] != float64(400) || body["store"] != false {
					t.Fatalf("invalid Responses request: %#v", body)
				}
				text, _ := body["text"].(map[string]any)
				format, _ := text["format"].(map[string]any)
				if format["type"] != "json_schema" || format["schema"] == nil {
					t.Fatalf("Responses request lost its output schema: %#v", body)
				}
			},
		},
		{
			name: "Anthropic Messages", kind: "anthropic", protocol: modelconfig.ProtocolAnthropic, model: "claude-test", expectedPath: "/v1/messages", expectXAPIKey: true,
			response:      map[string]any{"stop_reason": "end_turn", "content": []map[string]any{{"type": "text", "text": `{"answer":"ok"}`}}, "usage": map[string]any{"input_tokens": 18, "output_tokens": 5, "cache_read_input_tokens": 7}},
			expectedUsage: [3]int{18, 5, 7},
			validateBody: func(t *testing.T, body map[string]any) {
				if body["system"] == nil || body["messages"] == nil || body["max_tokens"] != float64(400) {
					t.Fatalf("invalid Messages request: %#v", body)
				}
			},
		},
		{
			name: "OpenCode Go Messages", kind: "opencode_go", protocol: modelconfig.ProtocolAnthropic, model: "minimax-m3", expectedPath: "/v1/messages", expectBearer: true, expectXAPIKey: true,
			response: map[string]any{"stop_reason": "end_turn", "content": []map[string]any{{"type": "text", "text": `{"answer":"ok"}`}}},
			validateBody: func(t *testing.T, body map[string]any) {
				if body["model"] != "minimax-m3" || body["messages"] == nil {
					t.Fatalf("invalid OpenCode Messages request: %#v", body)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != test.expectedPath {
					t.Fatalf("path=%s want=%s", r.URL.Path, test.expectedPath)
				}
				if (r.Header.Get("Authorization") == "Bearer protocol-secret") != test.expectBearer {
					t.Fatalf("unexpected bearer header for %s", test.name)
				}
				if (r.Header.Get("x-api-key") == "protocol-secret") != test.expectXAPIKey {
					t.Fatalf("unexpected x-api-key header for %s", test.name)
				}
				if test.expectXAPIKey && r.Header.Get("anthropic-version") != "2023-06-01" {
					t.Fatalf("missing Anthropic version header")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				test.validateBody(t, body)
				_ = json.NewEncoder(w).Encode(test.response)
			}))
			defer mock.Close()
			a, _ := testutil.Open(t)
			service := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
			provider, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: test.name, Kind: test.kind, Protocol: test.protocol, BaseURL: mock.URL + "/v1", Model: test.model, APIKey: "protocol-secret", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID, FallbackToRules: true}); err != nil {
				t.Fatal(err)
			}
			callContext := modelusage.WithContext(t.Context(), modelusage.ContextInfo{RunID: "run-" + strings.ReplaceAll(test.name, " ", "-")})
			result, err := service.GenerateJSON(callContext, modelconfig.JSONGenerationRequest{SystemPrompt: "Return answer.", Input: json.RawMessage(`{"question":"test"}`), OutputSchema: json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}},"additionalProperties":false}`), MaxTokens: 400})
			if err != nil || string(result.Output) != `{"answer":"ok"}` {
				t.Fatalf("result=%s err=%v", result.Output, err)
			}
			observations, err := modelusage.ListByRun(t.Context(), a.Control.DB, "run-"+strings.ReplaceAll(test.name, " ", "-"))
			if err != nil || len(observations) != 1 {
				t.Fatalf("observations=%#v err=%v", observations, err)
			}
			if test.expectedUsage != [3]int{} && (observations[0].PromptTokens != test.expectedUsage[0] || observations[0].CompletionTokens != test.expectedUsage[1] || observations[0].CachedPromptTokens != test.expectedUsage[2]) {
				t.Fatalf("protocol usage was not normalized: %#v", observations[0])
			}
		})
	}
}

func TestOpenCodeCatalogSelectsProtocolAndNormalizesFullEndpoint(t *testing.T) {
	a, _ := testutil.Open(t)
	service := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), nil)
	responses, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: "OpenCode Luna", Kind: "opencode_go", BaseURL: "https://opencode.ai/zen/go/v1/responses", Model: "gpt-5.6-luna", APIKey: "secret", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if responses.Protocol != modelconfig.ProtocolOpenAIResponses || responses.BaseURL != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("Responses model was not normalized: %#v", responses)
	}
	updated, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{ProviderID: responses.ProviderID, Name: "OpenCode Luna Updated", Kind: "opencode_go", BaseURL: responses.BaseURL, Model: responses.Model, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Protocol != modelconfig.ProtocolOpenAIResponses {
		t.Fatalf("legacy update omitted protocol and changed the saved choice: %#v", updated)
	}
	messages, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: "OpenCode MiniMax", Kind: "opencode_go", BaseURL: "https://opencode.ai/zen/go/v1", Model: "minimax-m3", APIKey: "secret", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if messages.Protocol != modelconfig.ProtocolAnthropic {
		t.Fatalf("OpenCode model knowledge selected wrong protocol: %#v", messages)
	}
	catalog := modelconfig.ModelCatalog()
	found := map[string]string{}
	for _, item := range catalog {
		if item.ProviderKind == "opencode_go" {
			found[item.ModelID] = item.Protocol
		}
	}
	if found["glm-5.3"] != modelconfig.ProtocolOpenAIChat || found["gpt-5.6-luna"] != modelconfig.ProtocolOpenAIResponses || found["qwen3.8-max"] != modelconfig.ProtocolAnthropic {
		t.Fatalf("OpenCode catalog protocols are incomplete: %#v", found)
	}
}

func TestAgentExtractionChunksLongTranscriptAndKeepsSuccessfulChunksWhenOneFails(t *testing.T) {
	var calls atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		call := calls.Add(1)
		var request struct {
			ReasoningSplit      bool           `json:"reasoning_split"`
			MaxCompletionTokens int            `json:"max_completion_tokens"`
			MaxTokens           int            `json:"max_tokens"`
			Thinking            map[string]any `json:"thinking"`
			Messages            []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Messages) != 2 || !strings.Contains(request.Messages[1].Content, "Transcript chunk:") {
			t.Fatalf("missing bounded chunk metadata: %#v", request.Messages)
		}
		if !request.ReasoningSplit {
			t.Fatal("MiniMax request did not enable reasoning_split")
		}
		if request.MaxCompletionTokens != 16000 || request.MaxTokens != 0 || request.Thinking["type"] != "disabled" {
			t.Fatalf("MiniMax structured-output controls are incomplete: %#v", request)
		}
		if strings.Contains(request.Messages[1].Content, "Transcript chunk: 2/") {
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "<think>truncated before final JSON"}}}})
			return
		}
		content := `<think>先判断这段证据是否包含独立信息。</think>` + "\n```json\n" + `{"candidates":[{"evidence_id":"ev-long","statement":"第` + string(rune('0'+call)) + `段确认采用分段提炼策略。","unit_type":"decision","tier_hint":"semantic","risk_tier":"B","confidence":0.94}]}` + "\n```"
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}}}})
	}))
	defer mock.Close()

	a, _ := testutil.Open(t)
	service := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
	provider, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{Name: "Chunk Model", Kind: "openai_compatible", BaseURL: mock.URL + "/v1", Model: "MiniMax-M3", APIKey: "secret", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID, FallbackToRules: true}); err != nil {
		t.Fatal(err)
	}
	transcript := strings.Repeat("我们确认这段录音需要先去除口语噪声，再抽取可独立理解的项目决定。", 430)
	result, err := service.Extract(t.Context(), memory.ExtractionRequest{SessionID: "long-session", Turns: []memory.ExtractionTurn{{EvidenceID: "ev-long", Role: "user", Text: transcript}}})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() < 4 || len(result.Candidates) != result.SucceededChunks || !strings.Contains(result.Compiler, "+partial-") {
		t.Fatalf("expected successful bounded chunks to survive one failure, calls=%d result=%#v", calls.Load(), result)
	}
	if !result.Degraded || result.FailedChunks != 1 || result.SucceededChunks+result.FailedChunks != result.TotalChunks {
		t.Fatalf("partial extraction diagnostics are incomplete: %#v", result)
	}
	for _, candidate := range result.Candidates {
		if candidate.EvidenceID != "ev-long" {
			t.Fatalf("candidate lost evidence provenance: %#v", candidate)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
