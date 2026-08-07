package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DefaultConfig returns the standard linter configuration.
func DefaultConfig() Config {
	return Config{
		Exclude: []string{},
		SafeNamespaces: []string{
			"Symfony\\",
			"Doctrine\\",
			"Psr\\",
			"IgorPhp\\IgorBundle\\",
		},
		ConsolePath: "bin/console",
		Env:         "dev",
		Verbose:     false,
	}
}

// LoadConfig loads the configuration from a file (defaulting to igor.json) in the given root directory.
func LoadConfig(root string, customConfigPath string) Config {
	c := DefaultConfig()

	// Auto-detect packages from composer.json
	if prod, dev, err := ParseComposer(root); err == nil {
		c.ProdPackages = prod
		c.DevPackages = dev
	}

	configPath := customConfigPath
	if configPath == "" {
		configPath = filepath.Join(root, "igor.json")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return c
	}
	var userConfig Config
	if err := json.Unmarshal(data, &userConfig); err != nil {
		// Log or handle error if needed, for now fallback to defaults
		return c
	}

	// Merge logic (optional): here we just override if provided
	if len(userConfig.Exclude) > 0 {
		c.Exclude = userConfig.Exclude
	}
	if len(userConfig.SafeNamespaces) > 0 {
		c.SafeNamespaces = userConfig.SafeNamespaces
	}
	if userConfig.IgnoreVendors {
		c.IgnoreVendors = true
	}
	if userConfig.IgnoreExternalBaseline {
		c.IgnoreExternalBaseline = true
	}
	if userConfig.BaselinePath != "" {
		c.BaselinePath = userConfig.BaselinePath
	}
	if userConfig.LLMConfig.Provider != "" || userConfig.LLMConfig.APIUrl != "" {
		c.LLMConfig = userConfig.LLMConfig
		if c.LLMConfig.Provider == "" {
			c.LLMConfig.Provider = "openai"
		}
	}
	return c
}

// IsExcluded returns true if the given path matches any of the excluded patterns.
func (c Config) IsExcluded(path string, rootPath string) bool {
	rel, err := filepath.Rel(rootPath, path)
	if err != nil {
		return false
	}
	for _, ex := range c.Exclude {
		// Normalize exclusion pattern by removing trailing slashes
		ex = strings.TrimRight(ex, "/\\")
		if rel == ex || strings.HasPrefix(rel, ex+string(os.PathSeparator)) || strings.HasPrefix(rel, ex+"/") {
			return true
		}
	}
	return false
}

// InitConfig detects the project type and generates a default configuration file.
func InitConfig(root string, customConfigPath string) (string, error) {
	configPath := customConfigPath
	if configPath == "" {
		configPath = filepath.Join(root, "igor.json")
	}

	// Check if already exists
	if _, err := os.Stat(configPath); err == nil {
		return "", os.ErrExist
	}

	// Minimal base configuration
	c := Config{
		Exclude:        []string{},
		SafeNamespaces: []string{"Psr\\"},
		ConsolePath:    "bin/console",
		Env:            "prod",
		Verbose:        false,
	}
	projectType := "Generic PHP"

	// 1. Detect Frameworks via composer.json
	composerPath := filepath.Join(root, "composer.json")
	if data, err := os.ReadFile(composerPath); err == nil {
		content := string(data)
		if strings.Contains(content, "symfony/framework-bundle") {
			projectType = "Symfony"
			c.SafeNamespaces = append(c.SafeNamespaces, "Symfony\\", "Doctrine\\")
		}
	}

	// 2. Additional folder detection
	if _, err := os.Stat(filepath.Join(root, "bin/console")); err == nil && projectType == "Generic PHP" {
		projectType = "Symfony (detected via bin/console)"
		c.SafeNamespaces = append(c.SafeNamespaces, "Symfony\\", "Doctrine\\")
	}

	// Deduplicate
	c.SafeNamespaces = uniqueStrings(c.SafeNamespaces)
	c.Exclude = uniqueStrings(c.Exclude)

	// 3. Write file
	file, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(configPath, file, 0644); err != nil {
		return "", err
	}

	return projectType, nil
}

func uniqueStrings(input []string) []string {
	unique := make(map[string]bool)
	result := []string{}
	for _, s := range input {
		if s == "" {
			continue
		}
		if !unique[s] {
			unique[s] = true
			result = append(result, s)
		}
	}
	return result
}
