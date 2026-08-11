package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
