package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func setupTestExternalBaselineDir(t *testing.T) (string, func()) {
	t.Helper()
	// Create a temporary root directory for the parent project
	tmpDir, err := os.MkdirTemp("", "igor_external_baseline_test")
	if err != nil {
		t.Fatal(err)
	}

	// Create vendor structure
	vendorDir := filepath.Join(tmpDir, "vendor")
	pkg1Dir := filepath.Join(vendorDir, "acme", "package1")
	pkg2Dir := filepath.Join(vendorDir, "acme", "package2")

	if err := os.MkdirAll(pkg1Dir, 0755); err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatal(err)
	}
	if err := os.MkdirAll(pkg2Dir, 0755); err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	// 1. Create a default baseline file for package1 (igor-baseline.json)
	pkg1BaselineContent := `{
		"files": {
			"src/Service1.php": [
				{
					"message": "State mutation detected in Service1",
					"reason": "Legacy code needing refactor"
				}
			]
		}
	}`
	err = os.WriteFile(filepath.Join(pkg1Dir, "igor-baseline.json"), []byte(pkg1BaselineContent), 0644)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	// 2. Create a custom baseline file + custom config for package2 (custom-baseline.json & igor.json)
	pkg2BaselineContent := `{
		"files": {
			"src/Service2.php": [
				{"message": "State mutation detected in Service2"}
			]
		}
	}`
	err = os.WriteFile(filepath.Join(pkg2Dir, "custom-baseline.json"), []byte(pkg2BaselineContent), 0644)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	pkg2ConfigContent := `{
		"baseline": "custom-baseline.json"
	}`
	err = os.WriteFile(filepath.Join(pkg2Dir, "igor.json"), []byte(pkg2ConfigContent), 0644)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	// Create root project baseline
	rootBaselineContent := `{
		"files": {
			"src/AppService.php": [
				{"message": "State mutation detected in AppService"}
			]
		}
	}`
	rootBaselinePath := filepath.Join(tmpDir, "igor-baseline.json")
	err = os.WriteFile(rootBaselinePath, []byte(rootBaselineContent), 0644)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	return tmpDir, func() { _ = os.RemoveAll(tmpDir) }
}

func TestExternalBaseline_AutoDiscoveryAndMerging(t *testing.T) {
	tmpDir, cleanup := setupTestExternalBaselineDir(t)
	defer cleanup()

	cfg := config.Config{
		BaselinePath:           "igor-baseline.json",
		IgnoreExternalBaseline: false,
	}

	baseline := loadAuditBaseline(tmpDir, &cfg)

	// Check root baseline
	if _, found := baseline.Files["src/AppService.php"]; !found {
		t.Error("Expected root baseline entry to be loaded")
	}

	// Check package1 auto-discovered baseline with translated paths
	p1Path := filepath.Join("vendor", "acme", "package1", "src/Service1.php")
	p1Entries, found := baseline.Files[p1Path]
	if !found {
		t.Errorf("Expected package1 baseline path %q to be loaded", p1Path)
	} else if len(p1Entries) != 1 || p1Entries[0].Message != "State mutation detected in Service1" {
		t.Errorf("Unexpected entries for package1: %v", p1Entries)
	}

	// Check package2 auto-discovered baseline with custom baseline path & translated paths
	p2Path := filepath.Join("vendor", "acme", "package2", "src/Service2.php")
	p2Entries, found := baseline.Files[p2Path]
	if !found {
		t.Errorf("Expected package2 baseline path %q to be loaded", p2Path)
	} else if len(p2Entries) != 1 || p2Entries[0].Message != "State mutation detected in Service2" {
		t.Errorf("Unexpected entries for package2: %v", p2Entries)
	}
}

func TestExternalBaseline_IgnoreExternal(t *testing.T) {
	tmpDir, cleanup := setupTestExternalBaselineDir(t)
	defer cleanup()

	cfg := config.Config{
		BaselinePath:           "igor-baseline.json",
		IgnoreExternalBaseline: true,
	}

	baseline := loadAuditBaseline(tmpDir, &cfg)

	// Check root baseline
	if _, found := baseline.Files["src/AppService.php"]; !found {
		t.Error("Expected root baseline entry to be loaded")
	}

	// Check external baselines are ignored
	p1Path := filepath.Join("vendor", "acme", "package1", "src/Service1.php")
	if _, found := baseline.Files[p1Path]; found {
		t.Errorf("Expected package1 baseline path %q to be ignored", p1Path)
	}

	p2Path := filepath.Join("vendor", "acme", "package2", "src/Service2.php")
	if _, found := baseline.Files[p2Path]; found {
		t.Errorf("Expected package2 baseline path %q to be ignored", p2Path)
	}
}

func TestExternalBaseline_CheckBaseline(t *testing.T) {
	tmpDir, cleanup := setupTestExternalBaselineDir(t)
	defer cleanup()

	cfg := config.Config{
		BaselinePath:  "igor-baseline.json",
		CheckBaseline: true,
	}

	baseline := loadAuditBaseline(tmpDir, &cfg)

	// Check root baseline
	if _, found := baseline.Files["src/AppService.php"]; !found {
		t.Error("Expected root baseline entry to be loaded")
	}

	// Check external baselines are ignored under check-baseline
	p1Path := filepath.Join("vendor", "acme", "package1", "src/Service1.php")
	if _, found := baseline.Files[p1Path]; found {
		t.Errorf("Expected package1 baseline path %q to be ignored under check-baseline", p1Path)
	}
}

func TestExternalBaseline_PruneBaseline(t *testing.T) {
	tmpDir, cleanup := setupTestExternalBaselineDir(t)
	defer cleanup()

	cfg := config.Config{
		BaselinePath:  "igor-baseline.json",
		PruneBaseline: true,
	}

	baseline := loadAuditBaseline(tmpDir, &cfg)

	// Check root baseline
	if _, found := baseline.Files["src/AppService.php"]; !found {
		t.Error("Expected root baseline entry to be loaded")
	}

	// Check external baselines are ignored under prune-baseline
	p1Path := filepath.Join("vendor", "acme", "package1", "src/Service1.php")
	if _, found := baseline.Files[p1Path]; found {
		t.Errorf("Expected package1 baseline path %q to be ignored under prune-baseline", p1Path)
	}
}

func TestExternalBaseline_SymlinkedBundle(t *testing.T) {
	// Create a temporary root directory for the parent project
	tmpDir, err := os.MkdirTemp("", "igor_symlink_baseline_test")
	if err != nil {
		t.Fatal(err)
	}
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create vendor structure
	vendorDir := filepath.Join(tmpDir, "vendor")
	orgDir := filepath.Join(vendorDir, "acme")
	if err := os.MkdirAll(orgDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create target bundle directory outside vendor
	localPackagesDir := filepath.Join(tmpDir, "local-packages")
	pkg3Dir := filepath.Join(localPackagesDir, "package3")
	if err := os.MkdirAll(filepath.Join(pkg3Dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create baseline file for package3
	pkg3BaselineContent := `{
		"files": {
			"src/Service3.php": [
				{"message": "State mutation detected in Service3"}
			]
		}
	}`
	err = os.WriteFile(filepath.Join(pkg3Dir, "igor-baseline.json"), []byte(pkg3BaselineContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create symbolic link under vendor pointing to package3 outside vendor
	symlinkPath := filepath.Join(orgDir, "package3")
	err = os.Symlink(pkg3Dir, symlinkPath)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		BaselinePath:           "igor-baseline.json",
		IgnoreExternalBaseline: false,
	}

	// Load the baseline
	baseline := loadAuditBaseline(tmpDir, &cfg)

	p3Path := filepath.Join("vendor", "acme", "package3", "src/Service3.php")
	p3Entries, found := baseline.Files[p3Path]
	if !found {
		t.Fatalf("Expected package3 baseline path %q to be loaded", p3Path)
	}
	if len(p3Entries) != 1 || p3Entries[0].Message != "State mutation detected in Service3" {
		t.Errorf("Unexpected entries for package3: %v", p3Entries)
	}

	// Test path translation during FilterFindings (simulating PHP reflection's real path)
	realFilePath := filepath.Join(pkg3Dir, "src/Service3.php")
	normalizedPath := cfg.NormalizePath(realFilePath)
	findings := []symbol.Finding{
		{Message: "State mutation detected in Service3"},
	}

	filtered := config.FilterFindings(baseline, normalizedPath, findings, tmpDir)
	if len(filtered) != 0 {
		t.Errorf("Expected findings for symlinked file %q (normalized: %q) to be filtered out by baseline, but got: %v", realFilePath, normalizedPath, filtered)
	}
}

func TestExternalBaseline_DebugSubcommand(t *testing.T) {
	tmpDir, cleanup := setupTestExternalBaselineDir(t)
	defer cleanup()

	// Redirect stdout to capture the output of the subcommand
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run subcommand
	handleDebugExternalBaselineSubcommand([]string{"debug-external-baseline", tmpDir}, "")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify stdout contains the summary of loaded baseline files
	if !strings.Contains(output, "Summary of loaded baseline files:") {
		t.Errorf("Expected output to contain summary title, got:\n%s", output)
	}

	// Verify it contains the path of package1 baseline translation
	expectedP1 := filepath.Join("vendor", "acme", "package1", "src/Service1.php")
	if !strings.Contains(output, expectedP1) {
		t.Errorf("Expected output to contain package1 relative path %q, got:\n%s", expectedP1, output)
	}

	// Verify it contains package2 path translation
	expectedP2 := filepath.Join("vendor", "acme", "package2", "src/Service2.php")
	if !strings.Contains(output, expectedP2) {
		t.Errorf("Expected output to contain package2 relative path %q, got:\n%s", expectedP2, output)
	}

	// Verify it contains the reason for package1 on a separate indented line
	expectedReason := "           ◦ Reason: Legacy code needing refactor"
	if !strings.Contains(output, expectedReason) {
		t.Errorf("Expected output to contain package1 reason %q, got:\n%s", expectedReason, output)
	}
}
