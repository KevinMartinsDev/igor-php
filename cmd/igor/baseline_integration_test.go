package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestEnv prepares a temporary directory with a resolved symlink path.
func setupTestEnv(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "igor_baseline_integration_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("Failed to evaluate symlinks for temp dir: %v", err)
	}
	return tmpDir
}

func TestBaselineCheck_GenerateAndCheck(t *testing.T) {
	tmpDir := setupTestEnv(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a PHP file containing a state mutation (static state leak)
	phpFileContent := `<?php
class LeakyService {
    public static $data = [];
    public function add($item) {
        self::$data[] = $item;
    }
}
`
	phpFilePath := filepath.Join(tmpDir, "LeakyService.php")
	err := os.WriteFile(phpFilePath, []byte(phpFileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write php file: %v", err)
	}

	baselinePath := filepath.Join(tmpDir, "igor-baseline.json")

	// Generate baseline
	cmdGen := exec.Command("go", "run", ".", "--generate-baseline", "--baseline", baselinePath, tmpDir)
	outGen, err := cmdGen.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to generate baseline: %v\nOutput: %s", err, string(outGen))
	}

	// Verify baseline content and pre-filled reason
	baselineData, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("Failed to read generated baseline: %v", err)
	}

	var parsedBaseline struct {
		Files map[string][]struct {
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"files"`
	}
	err = json.Unmarshal(baselineData, &parsedBaseline)
	if err != nil {
		t.Fatalf("Failed to parse baseline JSON: %v", err)
	}

	relPath := "LeakyService.php"
	entries, ok := parsedBaseline.Files[relPath]
	if !ok || len(entries) == 0 {
		t.Fatalf("Expected findings for %s in baseline, got nothing", relPath)
	}

	expectedReason := "TODO: Explain why this state mutation is a false positive or safe"
	if entries[0].Reason != expectedReason {
		t.Errorf("Expected pre-filled reason %q, got %q", expectedReason, entries[0].Reason)
	}

	// Run --check-baseline (should pass because the leak is still present in the file)
	cmdCheckPass := exec.Command("go", "run", ".", "--check-baseline", "--baseline", baselinePath, tmpDir)
	outCheckPass, err := cmdCheckPass.CombinedOutput()
	if err != nil {
		t.Errorf("Expected check-baseline to pass, but it failed: %v\nOutput: %s", err, string(outCheckPass))
	}

	if !strings.Contains(string(outCheckPass), "Baseline check passed!") {
		t.Errorf("Expected success message, got: %s", string(outCheckPass))
	}
}

func TestBaselineCheck_CheckStaleAndPrune(t *testing.T) {
	tmpDir := setupTestEnv(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a clean PHP file (no leak)
	cleanPHPContent := `<?php
class LeakyService {
    public function add($item) {
        // Safe, no static mutation
    }
}
`
	phpFilePath := filepath.Join(tmpDir, "LeakyService.php")
	err := os.WriteFile(phpFilePath, []byte(cleanPHPContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write clean php file: %v", err)
	}

	baselinePath := filepath.Join(tmpDir, "igor-baseline.json")

	// Create a baseline containing a stale leak entry
	staleBaseline := `{
		"files": {
			"LeakyService.php": [
				{
					"message": "Mutation of state 'static::$data' in LeakyService::add()",
					"reason": "TODO"
				}
			]
		}
	}`
	err = os.WriteFile(baselinePath, []byte(staleBaseline), 0644)
	if err != nil {
		t.Fatalf("Failed to write stale baseline: %v", err)
	}

	// Run --check-baseline (should FAIL because the leak is fixed, hence the baseline has a stale entry)
	cmdCheckFail := exec.Command("go", "run", ".", "--check-baseline", "--baseline", baselinePath, tmpDir)
	outCheckFail, err := cmdCheckFail.CombinedOutput()
	if err == nil {
		t.Errorf("Expected check-baseline to fail because of stale entries, but it exited with 0.\nOutput: %s", string(outCheckFail))
	}

	outCheckFailStr := string(outCheckFail)
	if !strings.Contains(outCheckFailStr, "Baseline check failed") {
		t.Errorf("Expected baseline check failed error, got: %s", outCheckFailStr)
	}
	if !strings.Contains(outCheckFailStr, "stale entries detected") {
		t.Errorf("Expected stale entry mention, got: %s", outCheckFailStr)
	}

	// Run --prune-baseline (should prune the stale entry and save the baseline)
	cmdPrune := exec.Command("go", "run", ".", "--prune-baseline", "--baseline", baselinePath, tmpDir)
	outPrune, err := cmdPrune.CombinedOutput()
	if err != nil {
		t.Fatalf("Expected prune-baseline to succeed, but it failed: %v\nOutput: %s", err, string(outPrune))
	}

	if !strings.Contains(string(outPrune), "Baseline successfully pruned and saved") {
		t.Errorf("Expected prune success message, got: %s", string(outPrune))
	}

	// Verify baseline is now clean
	prunedBaselineData, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("Failed to read pruned baseline: %v", err)
	}

	var parsedPrunedBaseline struct {
		Files map[string][]struct {
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"files"`
	}
	err = json.Unmarshal(prunedBaselineData, &parsedPrunedBaseline)
	if err != nil {
		t.Fatalf("Failed to parse pruned baseline JSON: %v", err)
	}

	relPath := "LeakyService.php"
	if len(parsedPrunedBaseline.Files[relPath]) > 0 {
		t.Errorf("Expected %s to be pruned from baseline, but entries remain: %v", relPath, parsedPrunedBaseline.Files[relPath])
	}
}
