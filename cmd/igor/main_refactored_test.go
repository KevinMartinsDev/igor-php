package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/igor-php/igor-php/internal/auditor"
	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestCalculateAuditStatus(t *testing.T) {
	tests := []struct {
		name     string
		findings []symbol.Finding
		expected string
	}{
		{
			name:     "No findings",
			findings: nil,
			expected: "✅ OK",
		},
		{
			name: "Error findings",
			findings: []symbol.Finding{
				{Severity: "WARNING", Message: "Some warning"},
				{Severity: "ERROR", Message: "Some error"},
			},
			expected: "❌ KO",
		},
		{
			name: "Warning findings",
			findings: []symbol.Finding{
				{Severity: "WARNING", Message: "Some warning"},
			},
			expected: "⚠️  WARN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateAuditStatus(tt.findings); got != tt.expected {
				t.Errorf("calculateAuditStatus() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestLoadContainerDump(t *testing.T) {
	// Case 1: Empty dump path
	cfgEmpty := config.Config{ContainerDump: ""}
	nonSharedEmpty := loadContainerDump("/root", cfgEmpty)
	if nonSharedEmpty != nil {
		t.Error("Expected nil container dump map for empty configuration")
	}

	// Case 2: Non-existent dump path
	cfgMissing := config.Config{ContainerDump: "missing-dump.json"}
	nonSharedMissing := loadContainerDump("/root", cfgMissing)
	if nonSharedMissing != nil {
		t.Error("Expected nil container dump map for missing dump path")
	}

	// Case 3: Valid JSON container dump
	tempDir, err := os.MkdirTemp("", "igor_test_dump_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dumpData := `{"services": [{"class": "App\\Service\\Transient", "shared": false}]}`
	dumpPath := filepath.Join(tempDir, "container-dump.json")
	if err := os.WriteFile(dumpPath, []byte(dumpData), 0644); err != nil {
		t.Fatalf("Failed to write dump file: %v", err)
	}

	cfgValid := config.Config{ContainerDump: dumpPath}
	nonSharedValid := loadContainerDump(tempDir, cfgValid)
	if nonSharedValid == nil {
		t.Fatal("Expected populated container dump map, got nil")
	}
	if !nonSharedValid["App\\Service\\Transient"] {
		t.Error("Expected App\\Service\\Transient to be registered as non-shared")
	}
}

func TestDetectSymfonyProject_NoAgent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_test_detect_symfony_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Case 1: NoAgent = true -> prints warning and returns nil, nil
	cfgNoAgent := config.Config{
		ConsolePath: "nonexistent/console",
		NoAgent:     true,
	}
	sb, err := detectSymfonyProject(tempDir, cfgNoAgent)
	if err != nil {
		t.Errorf("Unexpected error with NoAgent = true: %v", err)
	}
	if sb != nil {
		t.Error("Expected nil bridge with non-existent console path and NoAgent = true")
	}

	// Case 2: NoAgent = false -> returns error
	cfgWithAgent := config.Config{
		ConsolePath: "nonexistent/console",
		NoAgent:     false,
	}
	_, err = detectSymfonyProject(tempDir, cfgWithAgent)
	if err == nil {
		t.Error("Expected error when Symfony console doesn't exist and NoAgent = false")
	}
}

func TestGenerateBaselineFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_test_gen_baseline_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	results := []symbol.AuditStatus{
		{
			FilePath: "src/Service/MyService.php",
			Findings: []symbol.Finding{
				{
					Line:    12,
					Message: "Mutation error",
					Code:    "$this->prop = $v",
					Snippet: "$this->prop = $v",
				},
			},
		},
	}

	cfg := config.Config{
		BaselinePath: "custom-baseline.json",
	}

	err = generateBaselineFile(tempDir, cfg, results)
	if err != nil {
		t.Fatalf("generateBaselineFile failed: %v", err)
	}

	baselineFile := filepath.Join(tempDir, "custom-baseline.json")
	if _, err := os.Stat(baselineFile); err != nil {
		t.Fatalf("Expected baseline file to exist at %s, but got: %v", baselineFile, err)
	}

	data, err := os.ReadFile(baselineFile)
	if err != nil {
		t.Fatalf("Failed to read generated baseline: %v", err)
	}

	var loaded config.Baseline
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal baseline file: %v", err)
	}

	if len(loaded.Files) != 1 || len(loaded.Files["src/Service/MyService.php"]) != 1 {
		t.Errorf("Unexpected baseline content: %+v", loaded.Files)
	}
}

type mockReporter struct {
	printHeaderCalled        bool
	printProjectHeaderCalled bool
	printVendorHeaderCalled  bool
	printFindingsCalled      int
}

func (m *mockReporter) PrintHeader(_ int) {
	m.printHeaderCalled = true
}

func (m *mockReporter) PrintProjectHeader() {
	m.printProjectHeaderCalled = true
}

func (m *mockReporter) PrintVendorHeader() {
	m.printVendorHeaderCalled = true
}

func (m *mockReporter) PrintFindings(_ symbol.AuditStatus, _ string, _ bool) {
	m.printFindingsCalled++
}

func (m *mockReporter) PrintSummary(_ []symbol.AuditStatus, _ string) bool {
	return true
}

func TestReportAllFindings(t *testing.T) {
	rep := &mockReporter{}
	root := "/app"

	results := []symbol.AuditStatus{
		{
			FilePath: "/app/src/Service.php", // Local
			Findings: []symbol.Finding{{Message: "Err 1"}},
		},
		{
			FilePath: "/app/vendor/acme/bundle/Service.php", // Vendor
			Findings: []symbol.Finding{{Message: "Err 2"}},
		},
	}

	reportAllFindings(rep, results, root)

	if !rep.printProjectHeaderCalled {
		t.Error("Expected PrintProjectHeader to be called")
	}
	if !rep.printVendorHeaderCalled {
		t.Error("Expected PrintVendorHeader to be called")
	}
	if rep.printFindingsCalled != 2 {
		t.Errorf("Expected PrintFindings to be called 2 times, got %d", rep.printFindingsCalled)
	}
}

func TestHandleInitSubcommand_Error(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_test_init_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create igor.json beforehand to force InitConfig error
	_ = os.WriteFile(filepath.Join(tempDir, "igor.json"), []byte("{}"), 0644)

	err = handleInitSubcommand([]string{"init", tempDir}, "")
	if err == nil {
		t.Error("Expected error when initializing in directory with existing igor.json")
	}
}

func TestHandleInitSubcommand_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_test_init_success_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize clean project - should default to generic project
	err = handleInitSubcommand([]string{"init", tempDir}, "")
	if err != nil {
		t.Fatalf("Expected successful init, got error: %v", err)
	}

	// Verify that igor.json was created
	configPath := filepath.Join(tempDir, "igor.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("Expected igor.json to be created in %s, but it doesn't exist", tempDir)
	}
}

func TestHandleReviewSubcommand_Errors(t *testing.T) {
	// Case 1: Missing JSON file
	err := handleReviewSubcommand([]string{"review"}, "")
	if err == nil {
		t.Error("Expected error when missing json file argument")
	} else if !strings.Contains(err.Error(), "missing JSON file") {
		t.Errorf("Unexpected error message: %v", err)
	}

	// Case 2: Non-existent file
	err = handleReviewSubcommand([]string{"review", "nonexistent.json"}, "")
	if err == nil {
		t.Error("Expected error for non-existent json file")
	} else if !strings.Contains(err.Error(), "could not read file") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestCollectForcedVendorFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_test_forced_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create nested PHP file inside vendor/acme/my-bundle
	vendorDir := filepath.Join(tempDir, "vendor", "acme", "my-bundle")
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatalf("Failed to create vendor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vendorDir, "Service.php"), []byte("<?php"), 0644); err != nil {
		t.Fatalf("Failed to write PHP file: %v", err)
	}

	cfg := config.Config{
		ScanVendors: []string{"acme/my-bundle"},
	}

	processed := make(map[string]bool)
	list := collectForcedVendorFiles(tempDir, cfg, processed)

	if len(list) != 1 {
		t.Fatalf("Expected 1 discovered vendor file, got %d", len(list))
	}

	expectedPath := filepath.Clean(filepath.Join(vendorDir, "Service.php"))
	actualPath := filepath.Clean(list[0].FilePath)
	if actualPath != expectedPath {
		t.Errorf("Expected discovered path to be %s, got %s", expectedPath, actualPath)
	}
}

func TestParseFlagsAndInit(t *testing.T) {
	// 1. Version flag
	_, _, shouldExit, err := parseFlagsAndInit([]string{"igor", "--version"})
	if err != nil {
		t.Errorf("Unexpected error for --version: %v", err)
	}
	if !shouldExit {
		t.Error("Expected shouldExit to be true for --version")
	}

	// 2. Missing target directory
	_, _, shouldExit, err = parseFlagsAndInit([]string{"igor"})
	if err == nil {
		t.Error("Expected error when target directory is missing")
	} else if !strings.Contains(err.Error(), "missing target directory") {
		t.Errorf("Unexpected error: %v", err)
	}
	if !shouldExit {
		t.Error("Expected shouldExit to be true when target directory is missing")
	}

	// 3. Valid directory with flag overrides
	tempDir, err := os.MkdirTemp("", "igor_test_cli_parse_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg, rootPath, shouldExit, err := parseFlagsAndInit([]string{
		"igor",
		"--output", "json",
		"--env", "prod",
		"--verbose",
		"--no-agent",
		tempDir,
	})

	if err != nil {
		t.Fatalf("Unexpected error for valid arguments: %v", err)
	}
	if shouldExit {
		t.Error("Expected shouldExit to be false for valid audit arguments")
	}
	if filepath.Clean(rootPath) != filepath.Clean(tempDir) {
		t.Errorf("Expected rootPath to be %s, got %s", tempDir, rootPath)
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("Expected OutputFormat to be json, got %s", cfg.OutputFormat)
	}
	if cfg.Env != "prod" {
		t.Errorf("Expected Env to be prod, got %s", cfg.Env)
	}
	if !cfg.Verbose {
		t.Error("Expected Verbose to be true")
	}
	if !cfg.NoAgent {
		t.Error("Expected NoAgent to be true")
	}
}

func TestExecuteAudit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_test_execute_audit_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "test.php")
	_ = os.WriteFile(filePath, []byte("<?php"), 0644)

	cfg := config.Config{}
	aud := auditor.NewAuditor(cfg)

	auditList := []symbol.AuditStatus{
		{
			ServiceID: "app.test",
			FilePath:  filePath,
			Status:    "⏳ PENDING",
		},
	}

	results := executeAudit(auditList, aud, cfg, config.Baseline{}, tempDir)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "✅ OK" {
		t.Errorf("expected status to be OK, got %s", results[0].Status)
	}
}

func TestHandleGeminiReview_Error(t *testing.T) {
	cfg := config.Config{
		LLMConfig: config.LLMConfig{
			Model: "gemini-1.5-flash",
		},
	}
	err := handleGeminiReview("my content", cfg)
	// Both success or failure are acceptable depending on whether gemini CLI is installed on the host runner.
	if err == nil {
		_ = os.Remove("igor-review.md")
	}
}

func TestHandleAPIReview_Error(t *testing.T) {
	cfg := config.Config{
		LLMConfig: config.LLMConfig{
			Provider: "ollama",
			Model:    "llama3",
			APIUrl:   "http://invalid-url-that-does-not-exist:11434",
		},
	}
	err := handleAPIReview("my content", cfg)
	if err == nil {
		t.Error("Expected error because Ollama API URL is invalid/unreachable")
	}
}

func TestHandleAPIReview_OpenAIFallback(t *testing.T) {
	// Expert mode with empty API Key env should print warning and exit cleanly (return nil)
	cfg := config.Config{
		LLMConfig: config.LLMConfig{
			Provider:  "openai",
			Model:     "gpt-4",
			APIKeyEnv: "IGOR_NON_EXISTENT_KEY_ENV",
		},
	}
	// Make sure the env var is clean
	_ = os.Unsetenv("IGOR_NON_EXISTENT_KEY_ENV")

	err := handleAPIReview("my content", cfg)
	if err != nil {
		t.Errorf("Expected nil error for empty APIKeyEnv fallback, got: %v", err)
	}
}

func TestHandleAPIReview_OllamaEmptyURL(t *testing.T) {
	// Ollama with empty API URL should default to localhost and attempt request (failing due to unreachable connection)
	cfg := config.Config{
		LLMConfig: config.LLMConfig{
			Provider: "ollama",
			Model:    "llama3",
			APIUrl:   "", // forces fallback to default URL
		},
	}
	err := handleAPIReview("my content", cfg)
	if err == nil {
		t.Error("Expected error because default localhost Ollama should be unreachable in tests")
	}
}

func TestHandleReviewSubcommand_GeminiProvider(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "igor_review_gemini_")
	defer os.RemoveAll(tempDir)

	jsonFile := filepath.Join(tempDir, "export.json")
	_ = os.WriteFile(jsonFile, []byte("{}"), 0644)

	igorJson := `{"llm": {"provider": "gemini", "model": "gemini-1.5-flash"}}`
	igorJsonPath := filepath.Join(tempDir, "igor.json")
	_ = os.WriteFile(igorJsonPath, []byte(igorJson), 0644)

	err := handleReviewSubcommand([]string{"review", jsonFile}, igorJsonPath)
	// Both success or failure are acceptable depending on whether gemini CLI is installed on the host runner.
	if err == nil {
		_ = os.Remove("igor-review.md")
	}
}

func TestHandleReviewSubcommand_APIProvider(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "igor_review_api_")
	defer os.RemoveAll(tempDir)

	jsonFile := filepath.Join(tempDir, "export.json")
	_ = os.WriteFile(jsonFile, []byte("{}"), 0644)

	igorJson := `{"llm": {"provider": "ollama", "model": "llama3", "api_url": "http://invalid:11434"}}`
	igorJsonPath := filepath.Join(tempDir, "igor.json")
	_ = os.WriteFile(igorJsonPath, []byte(igorJson), 0644)

	err := handleReviewSubcommand([]string{"review", jsonFile}, igorJsonPath)
	if err == nil {
		t.Error("Expected error from API execution fallback")
	}
}

func TestParseFlagsAndInit_Subcommands(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_sub_cli_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create igor.json beforehand to force init subcommand error
	_ = os.WriteFile(filepath.Join(tempDir, "igor.json"), []byte("{}"), 0644)

	// 1. init
	_, _, shouldExit, err := parseFlagsAndInit([]string{"igor", "-config", filepath.Join(tempDir, "igor.json"), "init", tempDir})
	if err == nil {
		t.Error("Expected error for init subcommand on pre-existing config")
	}
	if !shouldExit {
		t.Error("Expected shouldExit to be true for init subcommand")
	}

	// 2. debug-external-baseline
	_, _, shouldExit, err = parseFlagsAndInit([]string{"igor", "debug-external-baseline", tempDir})
	if err != nil {
		t.Errorf("Unexpected error for debug-external-baseline: %v", err)
	}
	if !shouldExit {
		t.Error("Expected shouldExit to be true for debug-external-baseline subcommand")
	}

	// 3. review (missing json file)
	_, _, shouldExit, err = parseFlagsAndInit([]string{"igor", "review"})
	if err == nil {
		t.Error("Expected error for review subcommand without arguments")
	}
	if !shouldExit {
		t.Error("Expected shouldExit to be true for review subcommand error")
	}
}

func TestHandleExplainSubcommand_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_explain_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a mock php file inside tempDir/src/Service/
	srcDir := filepath.Join(tempDir, "src", "Service")
	_ = os.MkdirAll(srcDir, 0755)

	phpFile := filepath.Join(srcDir, "MyService.php")
	_ = os.WriteFile(phpFile, []byte(`<?php
namespace App\Service;
class MyService {
    public static $staticVar = 1;
    public function touch() {
        self::$staticVar = 2;
        exit(1);
    }
}
`), 0644)

	// Create igor.json
	igorJson := filepath.Join(tempDir, "igor.json")
	_ = os.WriteFile(igorJson, []byte(`{}`), 0644)

	// Test handleExplainSubcommand without filter
	err = handleExplainSubcommand([]string{"explain", tempDir}, "")
	if err != nil {
		t.Fatalf("handleExplainSubcommand failed: %v", err)
	}

	// Test handleExplainSubcommand with filter
	err = handleExplainSubcommand([]string{"explain", tempDir, "MyService"}, "")
	if err != nil {
		t.Fatalf("handleExplainSubcommand with filter failed: %v", err)
	}

	// Test parseFlagsAndInit for explain subcommand
	_, _, shouldExit, err := parseFlagsAndInit([]string{"igor", "explain", tempDir})
	if err != nil {
		t.Errorf("Unexpected error for parseFlagsAndInit with explain: %v", err)
	}
	if !shouldExit {
		t.Error("Expected shouldExit to be true for explain subcommand")
	}
}

func TestFormatExplainRow_Direct(t *testing.T) {
	// Case 1: Shared service with all possible findings
	status := symbol.AuditStatus{
		ServiceID: "App\\Service\\MySharedService",
		FilePath:  "src/Service/MySharedService.php",
		IsShared:  true,
		Findings: []symbol.Finding{
			{Message: "class static property mutation", Line: 10, Snippet: "self::$cache = 1"},
			{Message: "forbidden exit call", Line: 20, Snippet: "exit(1)"},
			{Message: "PHP Superglobal $_GET used", Line: 30, Snippet: "$_GET"},
			{Message: "closure capturing local state", Line: 40, Snippet: "function() use ($x)"},
			{Message: "some standard mutation", Line: 50, Snippet: "$this->prop = 2"},
		},
	}

	row, reasons := formatExplainRow(status, "App\\Service\\MySharedService")
	if !strings.Contains(row, "YES") {
		t.Error("Expected Shared column to be YES")
	}
	if !strings.Contains(row, "KO (State Poison)") {
		t.Error("Expected Verdict to be KO (State Poison)")
	}

	// We expect exactly 5 reasons (1 for each finding type)
	if len(reasons) != 5 {
		t.Errorf("Expected 5 reasons, got %d", len(reasons))
	}

	// Case 2: Stateless service (0 findings)
	statusOK := symbol.AuditStatus{
		ServiceID: "App\\Service\\MyStatelessService",
		FilePath:  "src/Service/MyStatelessService.php",
		IsShared:  false,
		Findings:  nil,
	}

	rowOK, reasonsOK := formatExplainRow(statusOK, "App\\Service\\MyStatelessService")
	if !strings.Contains(rowOK, "NO") {
		t.Error("Expected Shared column to be NO")
	}
	if !strings.Contains(rowOK, "OK (Stateless)") {
		t.Error("Expected Verdict to be OK (Stateless)")
	}
	if len(reasonsOK) != 2 {
		t.Errorf("Expected 2 default OK reasons, got %d", len(reasonsOK))
	}

	// Case 3: Standard mutation only (KO State Mutation)
	statusMutation := symbol.AuditStatus{
		ServiceID: "App\\Service\\MyMutationService",
		FilePath:  "src/Service/MyMutationService.php",
		IsShared:  true,
		Findings: []symbol.Finding{
			{Message: "some standard mutation", Line: 50, Snippet: "$this->prop = 2"},
		},
	}

	rowMut, _ := formatExplainRow(statusMutation, "App\\Service\\MyMutationService")
	if !strings.Contains(rowMut, "KO (State Mutation)") {
		t.Error("Expected Verdict to be KO (State Mutation)")
	}
}
