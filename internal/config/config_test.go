package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "igor_config_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	t.Run("Default config when file missing", func(t *testing.T) {
		cfg := LoadConfig(tmpDir, "")
		if len(cfg.Exclude) != 0 {
			t.Errorf("Expected 0 default excludes, got %d", len(cfg.Exclude))
		}
		if len(cfg.SafeNamespaces) == 0 {
			t.Error("Expected default safe namespaces")
		}
	})

	t.Run("Valid config file", func(t *testing.T) {
		content := `{"exclude": ["custom"], "safe_namespaces": ["My\\"], "ignore_vendors": true}`
		err := os.WriteFile(filepath.Join(tmpDir, "igor.json"), []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
		cfg := LoadConfig(tmpDir, "")
		if cfg.Exclude[0] != "custom" {
			t.Errorf("Expected 'custom' exclude, got %v", cfg.Exclude)
		}
		if cfg.SafeNamespaces[0] != "My\\" {
			t.Errorf("Expected 'My\\' namespace, got %v", cfg.SafeNamespaces)
		}
		if !cfg.IgnoreVendors {
			t.Errorf("Expected IgnoreVendors to be true, got %v", cfg.IgnoreVendors)
		}
	})

	t.Run("Corrupted config file", func(t *testing.T) {
		content := `{ invalid json }`
		err := os.WriteFile(filepath.Join(tmpDir, "igor.json"), []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
		cfg := LoadConfig(tmpDir, "")
		// Should fallback to default values (from struct initialization in LoadConfig)
		if len(cfg.Exclude) != 0 {
			t.Errorf("Expected 0 default excludes on parse error, got %d", len(cfg.Exclude))
		}
	})

	t.Run("Composer dev integration", func(t *testing.T) {
		composerContent := `{"require-dev": {"phpunit/phpunit": "^11.0"}}`
		err := os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerContent), 0644)
		if err != nil {
			t.Fatal(err)
		}
		cfg := LoadConfig(tmpDir, "")
		if len(cfg.DevPackages) != 1 || cfg.DevPackages[0] != "phpunit/phpunit" {
			t.Errorf("Expected 'phpunit/phpunit' in dev packages, got %v", cfg.DevPackages)
		}
	})

	t.Run("Custom config path", func(t *testing.T) {
		customPath := filepath.Join(tmpDir, "custom.json")
		content := `{"exclude": ["custom-path"]}`
		err := os.WriteFile(customPath, []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
		cfg := LoadConfig(tmpDir, customPath)
		if len(cfg.Exclude) != 1 || cfg.Exclude[0] != "custom-path" {
		        t.Errorf("Expected 'custom-path' exclude, got %v", cfg.Exclude)
		}
		})

		t.Run("LLM configuration", func(t *testing.T) {
		content := `{
		        "llm": {
		                "api_url": "https://api.openai.com/v1",
		                "api_key_env": "OPENAI_API_KEY",
		                "model": "gpt-4o"
		        }
		}`
		err := os.WriteFile(filepath.Join(tmpDir, "igor.json"), []byte(content), 0644)
		if err != nil {
		        t.Fatal(err)
		}
		cfg := LoadConfig(tmpDir, "")
		if cfg.LLMConfig.APIUrl != "https://api.openai.com/v1" {
		        t.Errorf("Expected API URL, got %s", cfg.LLMConfig.APIUrl)
		}
		if cfg.LLMConfig.APIKeyEnv != "OPENAI_API_KEY" {
		        t.Errorf("Expected API Key Env, got %s", cfg.LLMConfig.APIKeyEnv)
		}
		if cfg.LLMConfig.Model != "gpt-4o" {
		        t.Errorf("Expected Model, got %s", cfg.LLMConfig.Model)
		}
		})
		}
func TestInitConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "igor_init_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	t.Run("Initialize Generic PHP project", func(t *testing.T) {
		projectDir := filepath.Join(tmpDir, "generic")
		_ = os.MkdirAll(projectDir, 0755)

		detected, err := InitConfig(projectDir, "")
		if err != nil {
			t.Fatalf("InitConfig failed: %v", err)
		}
		if detected != "Generic PHP" {
			t.Errorf("Expected 'Generic PHP', got %s", detected)
		}

		configPath := filepath.Join(projectDir, "igor.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Fatal("igor.json was not created")
		}
	})

	t.Run("Initialize Symfony project", func(t *testing.T) {
		projectDir := filepath.Join(tmpDir, "symfony")
		_ = os.MkdirAll(projectDir, 0755)

		composerContent := `{"require": {"symfony/framework-bundle": "^7.0"}}`
		_ = os.WriteFile(filepath.Join(projectDir, "composer.json"), []byte(composerContent), 0644)

		detected, err := InitConfig(projectDir, "")
		if err != nil {
			t.Fatalf("InitConfig failed: %v", err)
		}
		if detected != "Symfony" {
			t.Errorf("Expected 'Symfony', got %s", detected)
		}
	})

	t.Run("Fail if config already exists", func(t *testing.T) {
		projectDir := filepath.Join(tmpDir, "exists")
		_ = os.MkdirAll(projectDir, 0755)
		_ = os.WriteFile(filepath.Join(projectDir, "igor.json"), []byte("{}"), 0644)

		_, err := InitConfig(projectDir, "")
		if err == nil {
			t.Fatal("Expected error when config already exists")
		}
	})
}

