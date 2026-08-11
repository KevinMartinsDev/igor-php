package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestSaveAndLoadBaseline(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "igor_baseline_test_save")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filePath := filepath.Join(tmpDir, "service.php")
	_ = os.WriteFile(filePath, []byte("<?php class Service {}"), 0644)

	findings := []symbol.Finding{
		{Message: "Error 1", Severity: "ERROR"},
		{Message: "Warning 1", Severity: "WARNING"},
	}

	results := []symbol.AuditStatus{
		{
			FilePath: filePath,
			Findings: findings,
		},
	}

	baselinePath := filepath.Join(tmpDir, "igor-baseline.json")

	err := SaveBaseline(baselinePath, results, tmpDir)
	if err != nil {
		t.Fatalf("SaveBaseline failed: %v", err)
	}

	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadBaseline failed: %v", err)
	}

	relPath, _ := filepath.Rel(tmpDir, filePath)
	if len(baseline.Files[relPath]) != 2 {
		t.Errorf("Expected 2 findings in baseline for %s, got %d", relPath, len(baseline.Files[relPath]))
	}

	// Verify the pre-filled reason is present and correct
	for _, entry := range baseline.Files[relPath] {
		expectedReason := "TODO: Explain why this state mutation is a false positive or safe"
		if entry.Reason != expectedReason {
			t.Errorf("Expected reason %q, got %q", expectedReason, entry.Reason)
		}
	}
}

func TestFilterFindings(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "igor_baseline_test_filter")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filePath := filepath.Join(tmpDir, "service.php")
	_ = os.WriteFile(filePath, []byte("<?php class Service {}"), 0644)

	findings := []symbol.Finding{
		{Message: "Error 1", Severity: "ERROR"},
		{Message: "Warning 1", Severity: "WARNING"},
	}

	results := []symbol.AuditStatus{
		{
			FilePath: filePath,
			Findings: findings,
		},
	}

	baselinePath := filepath.Join(tmpDir, "igor-baseline.json")
	_ = SaveBaseline(baselinePath, results, tmpDir)

	baseline, _ := LoadBaseline(baselinePath)

	// 1. Full match - should result in 0 findings
	filtered := FilterFindings(baseline, filePath, findings, tmpDir)
	if len(filtered) != 0 {
		t.Errorf("Expected 0 findings after filtering, got %d", len(filtered))
	}

	// 2. New finding - should not be filtered
	findingsWithNew := append([]symbol.Finding(nil), findings...)
	findingsWithNew = append(findingsWithNew, symbol.Finding{Message: "New Error", Severity: "ERROR"})
	filtered = FilterFindings(baseline, filePath, findingsWithNew, tmpDir)
	if len(filtered) != 1 || filtered[0].Message != "New Error" {
		t.Errorf("Expected 1 new finding, got %v", filtered)
	}

	// 3. Different file - should not be filtered
	otherFile := filepath.Join(tmpDir, "other.php")
	filtered = FilterFindings(baseline, otherFile, findings, tmpDir)
	if len(filtered) != 2 {
		t.Errorf("Expected 2 findings for unbaselined file, got %d", len(filtered))
	}
}

func TestBackwardCompatibilityWithLegacyBaselines(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "igor_baseline_test_compat")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filePath := filepath.Join(tmpDir, "service.php")
	findings := []symbol.Finding{
		{Message: "Error 1", Severity: "ERROR"},
		{Message: "Warning 1", Severity: "WARNING"},
	}

	// Create a legacy baseline JSON content without 'reason' fields
	legacyJSON := `{
		"files": {
			"service.php": [
				{
					"message": "Error 1"
				},
				{
					"message": "Warning 1"
				}
			]
		}
	}`

	legacyPath := filepath.Join(tmpDir, "legacy-baseline.json")
	err := os.WriteFile(legacyPath, []byte(legacyJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write legacy baseline: %v", err)
	}

	// 1. Check that loading doesn't fail
	baseline, err := LoadBaseline(legacyPath)
	if err != nil {
		t.Fatalf("Failed to load legacy baseline: %v", err)
	}

	// 2. Check that reason is empty
	entries := baseline.Files["service.php"]
	if len(entries) != 2 {
		t.Fatalf("Expected 2 legacy entries, got %d", len(entries))
	}
	for _, entry := range entries {
		if entry.Reason != "" {
			t.Errorf("Expected empty reason for legacy entry, got %q", entry.Reason)
		}
	}

	// 3. Verify filtering still functions correctly on legacy baselines
	filtered := FilterFindings(baseline, filePath, findings, tmpDir)
	if len(filtered) != 0 {
		t.Errorf("Expected 0 findings after filtering with legacy baseline, got %d", len(filtered))
	}
}

func TestIdentifyAndPruneStaleBaselineEntries(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "igor_baseline_test_prune")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Define a baseline
	base := Baseline{
		Files: map[string][]BaselineEntry{
			"service.php": {
				{Message: "Active Error"},
				{Message: "Stale Error 1"},
			},
			"other_service.php": {
				{Message: "Stale Error 2"},
			},
			"not_scanned.php": {
				{Message: "Should Not Be Stale"},
			},
		},
	}

	// Define actual audit results (only service.php and other_service.php are scanned)
	// active findings for service.php is "Active Error"
	// active findings for other_service.php is empty (fixed)
	scannedResults := []symbol.AuditStatus{
		{
			FilePath: filepath.Join(tmpDir, "service.php"),
			Findings: []symbol.Finding{
				{Message: "Active Error"},
			},
		},
		{
			FilePath: filepath.Join(tmpDir, "other_service.php"),
			Findings: []symbol.Finding{},
		},
	}

	// 1. Identify stale entries
	stale := IdentifyStaleEntries(base, scannedResults, tmpDir)

	// Expected stale entries:
	// - "service.php": "Stale Error 1"
	// - "other_service.php": "Stale Error 2"
	// Note that "not_scanned.php" should NOT be identified as stale because it wasn't in the scannedResults.
	if len(stale) != 2 {
		t.Fatalf("Expected 2 stale entries, got %d: %v", len(stale), stale)
	}

	hasStale1 := false
	hasStale2 := false
	for _, s := range stale {
		if s.FilePath == "service.php" && s.Message == "Stale Error 1" {
			hasStale1 = true
		}
		if s.FilePath == "other_service.php" && s.Message == "Stale Error 2" {
			hasStale2 = true
		}
	}

	if !hasStale1 {
		t.Errorf("Expected stale entry 'Stale Error 1' in service.php")
	}
	if !hasStale2 {
		t.Errorf("Expected stale entry 'Stale Error 2' in other_service.php")
	}

	// 2. Prune the baseline
	pruned := PruneBaseline(base, stale)

	// Pruned baseline should have:
	// - "service.php": only "Active Error"
	// - "not_scanned.php": "Should Not Be Stale"
	// - "other_service.php": should be completely removed because it has 0 entries left
	if len(pruned.Files) != 2 {
		t.Fatalf("Expected 2 files in pruned baseline, got %d: %v", len(pruned.Files), pruned.Files)
	}

	if len(pruned.Files["service.php"]) != 1 || pruned.Files["service.php"][0].Message != "Active Error" {
		t.Errorf("Unexpected entries for service.php in pruned baseline: %v", pruned.Files["service.php"])
	}

	if len(pruned.Files["not_scanned.php"]) != 1 || pruned.Files["not_scanned.php"][0].Message != "Should Not Be Stale" {
		t.Errorf("Unexpected entries for not_scanned.php in pruned baseline: %v", pruned.Files["not_scanned.php"])
	}

	if _, exists := pruned.Files["other_service.php"]; exists {
		t.Errorf("Expected other_service.php to be completely removed from pruned baseline")
	}
}

func TestWriteBaseline(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "igor_write_baseline_test")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	b := Baseline{
		Files: map[string][]BaselineEntry{
			"test.php": {
				{
					Message: "Mutation error",
					Reason:  "Safe",
				},
			},
		},
	}

	path := filepath.Join(tmpDir, "custom-baseline.json")
	err := WriteBaseline(path, b)
	if err != nil {
		t.Fatalf("WriteBaseline failed: %v", err)
	}

	loaded, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline failed: %v", err)
	}

	if len(loaded.Files["test.php"]) != 1 || loaded.Files["test.php"][0].Message != "Mutation error" {
		t.Errorf("Unexpected loaded baseline content: %v", loaded.Files["test.php"])
	}
}
