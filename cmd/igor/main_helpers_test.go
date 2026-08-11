package main

import (
	"strings"
	"testing"

	"github.com/igor-php/igor-php/internal/auditor"
	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/reporter"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestSetupReporter(t *testing.T) {
	tests := []struct {
		format   string
		expected string
	}{
		{"json", "*reporter.JSONReporter"},
		{"llm", "*reporter.LLMReporter"},
		{"cli", "*reporter.CLIReporter"},
		{"", "*reporter.CLIReporter"},
	}

	for _, tt := range tests {
		cfg := config.Config{OutputFormat: tt.format}
		rep := setupReporter(cfg)
		var actual string
		switch rep.(type) {
		case *reporter.JSONReporter:
			actual = "*reporter.JSONReporter"
		case *reporter.LLMReporter:
			actual = "*reporter.LLMReporter"
		case *reporter.CLIReporter:
			actual = "*reporter.CLIReporter"
		}
		if actual != tt.expected {
			t.Errorf("setupReporter(format=%q) = %s, expected %s", tt.format, actual, tt.expected)
		}
	}
}

func TestShouldSkipServiceMeta(t *testing.T) {
	cfg := config.Config{
		SafeNamespaces: []string{"Symfony\\"},
	}
	aud := auditor.NewAuditor(cfg)

	tests := []struct {
		name         string
		id           string
		def          symbol.SymfonyService
		expectedSkip bool
		expectedMsg  string
	}{
		{
			name:         "Errored service ID",
			id:           ".errored.my_service",
			def:          symbol.SymfonyService{Shared: true, Class: "App\\Service"},
			expectedSkip: true,
			expectedMsg:  "container error",
		},
		{
			name:         "Non-shared service",
			id:           "app.my_service",
			def:          symbol.SymfonyService{Shared: false, Class: "App\\Service"},
			expectedSkip: true,
			expectedMsg:  "non-shared (prototype)",
		},
		{
			name:         "No class defined",
			id:           "app.my_service",
			def:          symbol.SymfonyService{Shared: true, Class: ""},
			expectedSkip: true,
			expectedMsg:  "no class defined",
		},
		{
			name:         "Safe namespace class",
			id:           "app.my_service",
			def:          symbol.SymfonyService{Shared: true, Class: "Symfony\\Component\\HttpKernel\\Kernel"},
			expectedSkip: true,
			expectedMsg:  "safe namespace",
		},
		{
			name:         "Valid shared service",
			id:           "app.my_service",
			def:          symbol.SymfonyService{Shared: true, Class: "App\\Service\\MySharedService"},
			expectedSkip: false,
			expectedMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, reason := shouldSkipServiceMeta(tt.id, tt.def, aud)
			if skip != tt.expectedSkip {
				t.Errorf("shouldSkipServiceMeta() skip = %t, expected %t", skip, tt.expectedSkip)
			}
			if tt.expectedMsg != "" && !strings.Contains(reason, tt.expectedMsg) {
				t.Errorf("shouldSkipServiceMeta() reason = %q, expected containing %q", reason, tt.expectedMsg)
			}
		})
	}
}

func TestShouldSkipServicePath(t *testing.T) {
	cfg := config.Config{
		Exclude:       []string{"exclude-dir"},
		IgnoreVendors: true,
		DevPackages:   []string{"phpunit/phpunit"},
	}
	aud := auditor.NewAuditor(cfg)

	tests := []struct {
		name         string
		path         string
		expectedSkip bool
		expectedMsg  string
	}{
		{
			name:         "Excluded path",
			path:         "/app/exclude-dir/file.php",
			expectedSkip: true,
			expectedMsg:  "is excluded",
		},
		{
			name:         "Dev package path",
			path:         "/app/vendor/phpunit/phpunit/src/TestCase.php",
			expectedSkip: true,
			expectedMsg:  "belongs to a dev package",
		},
		{
			name:         "Vendor path when IgnoreVendors is true",
			path:         "/app/vendor/symfony/http-kernel/Kernel.php",
			expectedSkip: true,
			expectedMsg:  "belongs to vendor directory",
		},
		{
			name:         "Valid local path",
			path:         "/app/src/Service/MyService.php",
			expectedSkip: false,
			expectedMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, reason := shouldSkipServicePath("app.service", tt.path, cfg, aud, "/app")
			if skip != tt.expectedSkip {
				t.Errorf("shouldSkipServicePath() skip = %t, expected %t (path: %s)", skip, tt.expectedSkip, tt.path)
			}
			if tt.expectedMsg != "" && !strings.Contains(reason, tt.expectedMsg) {
				t.Errorf("shouldSkipServicePath() reason = %q, expected containing %q", reason, tt.expectedMsg)
			}
		})
	}
}

func TestApplyFlagOverrides(t *testing.T) {
	cfg := config.Config{}

	consoleFlag := "custom/console"
	envFlag := "test-env"
	verboseFlag := true
	noAgentFlag := true
	outputFlag := "json"
	generateBaselineFlag := true
	baselineFlag := "custom-baseline.json"
	containerDumpFlag := "dump.json"
	ignoreExternalBaselineFlag := true
	checkBaselineFlag := true
	pruneBaselineFlag := true

	applyFlagOverrides(
		&cfg,
		&consoleFlag,
		&envFlag,
		&verboseFlag,
		&noAgentFlag,
		&outputFlag,
		&generateBaselineFlag,
		&baselineFlag,
		&containerDumpFlag,
		&ignoreExternalBaselineFlag,
		&checkBaselineFlag,
		&pruneBaselineFlag,
	)

	if cfg.ConsolePath != "custom/console" {
		t.Errorf("Expected ConsolePath override, got %s", cfg.ConsolePath)
	}
	if cfg.ContainerDump != "dump.json" {
		t.Errorf("Expected ContainerDump override, got %s", cfg.ContainerDump)
	}
	if cfg.Env != "test-env" {
		t.Errorf("Expected Env override, got %s", cfg.Env)
	}
	if !cfg.Verbose {
		t.Error("Expected Verbose override to be true")
	}
	if !cfg.NoAgent {
		t.Error("Expected NoAgent override to be true")
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("Expected OutputFormat override, got %s", cfg.OutputFormat)
	}
	if !cfg.IgnoreExternalBaseline {
		t.Error("Expected IgnoreExternalBaseline override to be true")
	}
	if !cfg.CheckBaseline {
		t.Error("Expected CheckBaseline override to be true")
	}
	if !cfg.PruneBaseline {
		t.Error("Expected PruneBaseline override to be true")
	}
	if !cfg.GenerateBaseline {
		t.Error("Expected GenerateBaseline override to be true")
	}
	if cfg.BaselinePath != "custom-baseline.json" {
		t.Errorf("Expected BaselinePath override, got %s", cfg.BaselinePath)
	}
}
