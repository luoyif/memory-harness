package modelconfig

const (
	ProtocolOpenAIChat      = "openai_chat"
	ProtocolOpenAIResponses = "openai_responses"
	ProtocolAnthropic       = "anthropic_messages"
	ModelCatalogUpdatedAt   = "2026-08-25"
)

func Presets() []Preset {
	return []Preset{
		{PresetID: "openai-responses", Kind: "openai", Name: "OpenAI · Responses", Protocol: ProtocolOpenAIResponses, BaseURL: "https://api.openai.com/v1", ExampleModel: "gpt-5.6", RequiresKey: true, Description: "适合 OpenAI 新模型、推理模型和 Responses-only 模型。"},
		{PresetID: "openai-chat", Kind: "openai", Name: "OpenAI · Chat Completions", Protocol: ProtocolOpenAIChat, BaseURL: "https://api.openai.com/v1", ExampleModel: "gpt-5.6", RequiresKey: true, Description: "用于仍要求 Chat Completions 的模型或兼容服务。"},
		{PresetID: "anthropic", Kind: "anthropic", Name: "Anthropic · Claude", Protocol: ProtocolAnthropic, BaseURL: "https://api.anthropic.com/v1", ExampleModel: "claude-sonnet-4-6", RequiresKey: true, Description: "使用 Anthropic Messages 协议，不经过 OpenAI 兼容层。"},
		{PresetID: "opencode-go", Kind: "opencode_go", Name: "OpenCode Go", Protocol: ProtocolOpenAIResponses, BaseURL: "https://opencode.ai/zen/go/v1", ExampleModel: "gpt-5.6-luna", RequiresKey: true, Description: "同一订阅包含 Responses、Chat Completions 和 Messages 三类模型；选择模型后自动匹配协议。"},
		{PresetID: "deepseek", Kind: "deepseek", Name: "DeepSeek", Protocol: ProtocolOpenAIChat, BaseURL: "https://api.deepseek.com/v1", ExampleModel: "deepseek-chat", RequiresKey: true, Description: "使用 OpenAI-compatible Chat Completions。"},
		{PresetID: "compatible-chat", Kind: "openai_compatible", Name: "OpenAI Compatible / Local", Protocol: ProtocolOpenAIChat, BaseURL: "http://127.0.0.1:11434/v1", ExampleModel: "local-model", RequiresKey: false, Description: "适合 Ollama、LM Studio、vLLM 等兼容 Chat Completions 的本地服务。"},
		{PresetID: "compatible-responses", Kind: "openai_compatible", Name: "OpenAI-compatible Responses", Protocol: ProtocolOpenAIResponses, BaseURL: "http://127.0.0.1:8000/v1", ExampleModel: "local-model", RequiresKey: false, Description: "适合实现了 /responses 的代理、网关或本地服务。"},
	}
}

func ModelCatalog() []ModelKnowledge {
	models := []ModelKnowledge{
		{ProviderKind: "openai", ModelID: "gpt-5.6", Name: "GPT-5.6", Protocol: ProtocolOpenAIResponses, Input: []string{"text", "image"}, BestFor: "通用推理与结构化沉淀", Source: "OpenAI"},
		{ProviderKind: "openai", ModelID: "gpt-5.6-sol", Name: "GPT-5.6 Sol", Protocol: ProtocolOpenAIResponses, Input: []string{"text", "image"}, BestFor: "复杂推理与高质量 Agent 工作", Source: "OpenAI"},
		{ProviderKind: "openai", ModelID: "gpt-5.5", Name: "GPT-5.5", Protocol: ProtocolOpenAIResponses, Input: []string{"text", "image"}, BestFor: "稳定结构化输出与知识整理", Source: "OpenAI"},
		{ProviderKind: "anthropic", ModelID: "claude-opus-4-8", Name: "Claude Opus 4.8", Protocol: ProtocolAnthropic, Input: []string{"text", "image"}, BestFor: "复杂推理与长程 Agent 任务", Source: "Anthropic"},
		{ProviderKind: "anthropic", ModelID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Protocol: ProtocolAnthropic, Input: []string{"text", "image"}, BestFor: "速度与质量平衡的日常沉淀", Source: "Anthropic"},
		{ProviderKind: "anthropic", ModelID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Protocol: ProtocolAnthropic, Input: []string{"text", "image"}, BestFor: "快速、低延迟整理", Source: "Anthropic"},
	}
	opencode := []struct{ id, name, protocol, bestFor string }{
		{"grok-4.5", "Grok 4.5", ProtocolOpenAIResponses, "复杂编码与推理"},
		{"gpt-5.6-luna", "GPT 5.6 Luna", ProtocolOpenAIResponses, "高性价比通用编码与整理"},
		{"glm-5.3", "GLM-5.3", ProtocolOpenAIChat, "通用编码与中文任务"},
		{"glm-5.2", "GLM-5.2", ProtocolOpenAIChat, "通用编码"},
		{"glm-5.1", "GLM-5.1", ProtocolOpenAIChat, "通用编码"},
		{"kimi-k3", "Kimi K3", ProtocolOpenAIChat, "长上下文编码与推理"},
		{"kimi-k2.7-code", "Kimi K2.7 Code", ProtocolOpenAIChat, "代码任务"},
		{"kimi-k2.6", "Kimi K2.6", ProtocolOpenAIChat, "通用编码"},
		{"longcat-2.0", "LongCat-2.0", ProtocolOpenAIChat, "高性价比编码"},
		{"deepseek-v4-pro", "DeepSeek V4 Pro", ProtocolOpenAIChat, "复杂编码"},
		{"deepseek-v4-flash", "DeepSeek V4 Flash", ProtocolOpenAIChat, "快速编码"},
		{"deepseek-v4-flash-vision-exp", "DeepSeek V4 Flash Vision Exp", ProtocolOpenAIChat, "图文与代码实验"},
		{"mimo-v2.5", "MiMo-V2.5", ProtocolOpenAIChat, "低成本批量整理"},
		{"mimo-v2.5-pro", "MiMo-V2.5-Pro", ProtocolOpenAIChat, "高性价比复杂任务"},
		{"minimax-m3", "MiniMax M3", ProtocolAnthropic, "长上下文编码与沉淀"},
		{"minimax-m2.7", "MiniMax M2.7", ProtocolAnthropic, "通用编码"},
		{"minimax-m2.5", "MiniMax M2.5", ProtocolAnthropic, "通用编码"},
		{"muse-spark-1.2-contributor", "Muse Spark 1.2 Contributor", ProtocolOpenAIResponses, "低成本实验；使用前查看数据条款"},
		{"qwen3.8-max", "Qwen3.8 Max", ProtocolAnthropic, "复杂编码与中文任务"},
		{"qwen3.7-max", "Qwen3.7 Max", ProtocolAnthropic, "复杂编码"},
		{"qwen3.7-plus", "Qwen3.7 Plus", ProtocolAnthropic, "通用编码"},
		{"qwen3.6-plus", "Qwen3.6 Plus", ProtocolAnthropic, "通用编码"},
		{"hy3", "Hy3", ProtocolOpenAIChat, "高性价比编码"},
		{"ox-alpha-free", "Ox Alpha Free", ProtocolOpenAIChat, "限时免费实验"},
	}
	for _, item := range opencode {
		models = append(models, ModelKnowledge{ProviderKind: "opencode_go", ModelID: item.id, Name: item.name, Protocol: item.protocol, Input: []string{"text"}, BestFor: item.bestFor, Source: "OpenCode Go"})
	}
	return models
}

func catalogProtocol(kind, model string) string {
	for _, item := range ModelCatalog() {
		if item.ProviderKind == kind && item.ModelID == model {
			return item.Protocol
		}
	}
	return ""
}

func modelKnowledge(kind, model, fallbackProtocol string) ModelKnowledge {
	for _, item := range ModelCatalog() {
		if item.ProviderKind == kind && item.ModelID == model {
			return item
		}
	}
	return ModelKnowledge{ProviderKind: kind, ModelID: model, Name: model, Protocol: fallbackProtocol, Input: []string{"text"}, BestFor: "提供商返回的可用模型；能力和限制以提供商说明为准", Source: "Provider /models"}
}

func defaultProtocol(kind, model string) string {
	if protocol := catalogProtocol(kind, model); protocol != "" {
		return protocol
	}
	switch kind {
	case "openai":
		return ProtocolOpenAIResponses
	case "anthropic":
		return ProtocolAnthropic
	default:
		return ProtocolOpenAIChat
	}
}
