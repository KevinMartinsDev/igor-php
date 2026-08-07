package config

// Config stores linter settings.
type Config struct {
	Exclude                []string  `json:"exclude"`
	SafeNamespaces         []string  `json:"safe_namespaces"`
	ScanVendors            []string  `json:"scan_vendors"`
	IgnoreVendors          bool      `json:"ignore_vendors"`
	IgnoreExternalBaseline bool      `json:"ignore_external_baseline"`
	ConsolePath            string    `json:"console_path"`
	Env                    string    `json:"env"`
	Verbose                bool      `json:"verbose"`
	BaselinePath           string    `json:"baseline"`
	NoAgent                bool      `json:"-"` // Skip Igor Agent even if available
	ProdPackages           []string  `json:"-"` // List of require packages from composer.json
	DevPackages            []string  `json:"-"` // List of require-dev packages from composer.json
	GenerateBaseline       bool      `json:"-"` // Internal: set if --generate-baseline is used
	CheckBaseline          bool      `json:"-"` // Internal: set if --check-baseline is used
	PruneBaseline          bool      `json:"-"` // Internal: set if --prune-baseline is used
	OutputFormat           string    `json:"output"`
	ContainerDump          string    `json:"container_dump"` // Path to a generic container dump (framework-agnostic non-shared service graph)
	LLMConfig              LLMConfig `json:"llm"`
}

// LLMConfig stores settings for LLM-based review.
type LLMConfig struct {
	Provider  string `json:"provider"` // "openai" (default), "gemini", or "ollama"
	APIUrl    string `json:"api_url"`
	APIKeyEnv string `json:"api_key_env"`
	Model     string `json:"model"`
}

// Baseline represents a collection of ignored findings.
type Baseline struct {
	Files map[string][]BaselineEntry `json:"files"`
}

// BaselineEntry represents a single finding in the baseline.
type BaselineEntry struct {
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}
