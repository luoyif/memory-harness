package modelconfig

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type providerTextResponse struct {
	Content      string
	FinishReason string
}

func providerEndpoint(provider Provider) string {
	path := "/chat/completions"
	switch provider.Protocol {
	case ProtocolOpenAIResponses:
		path = "/responses"
	case ProtocolAnthropic:
		path = "/messages"
	}
	if strings.HasSuffix(provider.BaseURL, path) {
		return provider.BaseURL
	}
	return strings.TrimRight(provider.BaseURL, "/") + path
}

func buildProviderPayload(provider Provider, systemPrompt, userPrompt string, maxTokens int, outputSchema []byte) map[string]any {
	switch provider.Protocol {
	case ProtocolOpenAIResponses:
		payload := map[string]any{
			"model": provider.Model, "instructions": systemPrompt, "input": userPrompt,
			"max_output_tokens": maxTokens, "store": false,
		}
		if provider.Kind == "openai" && len(outputSchema) > 0 {
			var schema any
			if json.Unmarshal(outputSchema, &schema) == nil {
				payload["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "memory_harness_output", "strict": false, "schema": schema}}
			}
		}
		return payload
	case ProtocolAnthropic:
		return map[string]any{
			"model": provider.Model, "system": systemPrompt, "max_tokens": maxTokens, "temperature": 0,
			"messages": []map[string]any{{"role": "user", "content": userPrompt}},
		}
	default:
		payload := map[string]any{
			"model": provider.Model, "temperature": 0, "max_tokens": maxTokens,
			"messages": []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": userPrompt}},
		}
		applyStructuredOutputControls(payload, provider, maxTokens)
		return payload
	}
}

func setProviderAuthHeaders(request *http.Request, provider Provider, secret string) {
	request.Header.Set("Content-Type", "application/json")
	if secret == "" {
		return
	}
	if provider.Protocol == ProtocolAnthropic {
		request.Header.Set("x-api-key", secret)
		request.Header.Set("anthropic-version", "2023-06-01")
		if provider.Kind == "opencode_go" {
			request.Header.Set("Authorization", "Bearer "+secret)
		}
		return
	}
	request.Header.Set("Authorization", "Bearer "+secret)
}

func parseProviderText(provider Provider, raw []byte) (providerTextResponse, error) {
	switch provider.Protocol {
	case ProtocolOpenAIResponses:
		var response struct {
			Status            string `json:"status"`
			OutputText        string `json:"output_text"`
			IncompleteDetails struct {
				Reason string `json:"reason"`
			} `json:"incomplete_details"`
			Output []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			return providerTextResponse{}, errors.New("provider Responses response is not compatible JSON")
		}
		parts := []string{}
		if strings.TrimSpace(response.OutputText) != "" {
			parts = append(parts, response.OutputText)
		} else {
			for _, item := range response.Output {
				for _, content := range item.Content {
					if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
						parts = append(parts, content.Text)
					}
				}
			}
		}
		finish := strings.TrimSpace(response.Status)
		if reason := strings.TrimSpace(response.IncompleteDetails.Reason); reason != "" {
			finish = strings.Trim(strings.Join([]string{finish, reason}, "/"), "/")
		}
		if len(parts) == 0 {
			return providerTextResponse{}, errors.New("provider Responses response contains no output_text")
		}
		return providerTextResponse{Content: strings.Join(parts, "\n"), FinishReason: finish}, nil
	case ProtocolAnthropic:
		var response struct {
			StopReason string `json:"stop_reason"`
			Content    []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			return providerTextResponse{}, errors.New("provider Messages response is not compatible JSON")
		}
		parts := []string{}
		for _, content := range response.Content {
			if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
				parts = append(parts, content.Text)
			}
		}
		if len(parts) == 0 {
			return providerTextResponse{}, errors.New("provider Messages response contains no text block")
		}
		return providerTextResponse{Content: strings.Join(parts, "\n"), FinishReason: strings.TrimSpace(response.StopReason)}, nil
	default:
		var response struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &response); err != nil || len(response.Choices) == 0 {
			return providerTextResponse{}, errors.New("provider Chat Completions response is not compatible JSON")
		}
		return providerTextResponse{Content: response.Choices[0].Message.Content, FinishReason: strings.TrimSpace(response.Choices[0].FinishReason)}, nil
	}
}
