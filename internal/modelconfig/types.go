package modelconfig

type Pricing struct {
	Currency              string `json:"currency,omitempty"`
	InputPerMillionMinor  int64  `json:"input_per_million_minor,omitempty"`
	OutputPerMillionMinor int64  `json:"output_per_million_minor,omitempty"`
	Configured            bool   `json:"configured"`
}

type PricingInput struct {
	Currency              string `json:"currency"`
	InputPerMillionMinor  int64  `json:"input_per_million_minor"`
	OutputPerMillionMinor int64  `json:"output_per_million_minor"`
}

type Provider struct {
	ProviderID     string  `json:"provider_id"`
	Name           string  `json:"name"`
	Kind           string  `json:"kind"`
	Protocol       string  `json:"protocol"`
	BaseURL        string  `json:"base_url"`
	Model          string  `json:"model"`
	Status         string  `json:"status"`
	Enabled        bool    `json:"enabled"`
	HasSecret      bool    `json:"has_secret"`
	LastTestStatus string  `json:"last_test_status,omitempty"`
	LastTestAt     string  `json:"last_test_at,omitempty"`
	LastError      string  `json:"last_error,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	Pricing        Pricing `json:"pricing"`
}

type ProviderInput struct {
	ProviderID string        `json:"provider_id,omitempty"`
	Name       string        `json:"name"`
	Kind       string        `json:"kind"`
	Protocol   string        `json:"protocol,omitempty"`
	BaseURL    string        `json:"base_url"`
	Model      string        `json:"model"`
	APIKey     string        `json:"api_key,omitempty"`
	ClearKey   bool          `json:"clear_api_key,omitempty"`
	Enabled    bool          `json:"enabled"`
	Pricing    *PricingInput `json:"pricing,omitempty"`
}

type Runtime struct {
	Mode             string `json:"mode"`
	ActiveProviderID string `json:"active_provider_id,omitempty"`
	FallbackToRules  bool   `json:"fallback_to_rules"`
	UpdatedAt        string `json:"updated_at"`
}

type RuntimeInput struct {
	Mode             string `json:"mode"`
	ActiveProviderID string `json:"active_provider_id,omitempty"`
	FallbackToRules  bool   `json:"fallback_to_rules"`
}

type Preset struct {
	PresetID     string `json:"preset_id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	BaseURL      string `json:"base_url"`
	ExampleModel string `json:"example_model"`
	RequiresKey  bool   `json:"requires_api_key"`
	Description  string `json:"description"`
}

type ModelKnowledge struct {
	ProviderKind string   `json:"provider_kind"`
	ModelID      string   `json:"model_id"`
	Name         string   `json:"name"`
	Protocol     string   `json:"protocol"`
	Input        []string `json:"input"`
	BestFor      string   `json:"best_for"`
	Source       string   `json:"source"`
}

type ProbeResult struct {
	Status        string           `json:"status"`
	ProviderID    string           `json:"provider_id"`
	Models        []string         `json:"models"`
	ModelDetails  []ModelKnowledge `json:"model_details,omitempty"`
	SelectedModel string           `json:"selected_model"`
	SelectedFound bool             `json:"selected_model_found"`
	CheckedAt     string           `json:"checked_at"`
}

type JSONGenerationRequest struct {
	SystemPrompt string
	Input        []byte
	OutputSchema []byte
	MaxTokens    int
}

type JSONGenerationResult struct {
	Output     []byte `json:"output"`
	ProviderID string `json:"provider_id"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
}
