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
	err := handleDebugExternalBaselineSubcommand([]string{"debug-external-baseline", tmpDir}, "")
	if err != nil {
		t.Fatalf("Unexpected error from handleDebugExternalBaselineSubcommand: %v", err)
	}

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

func TestExternalBaseline_SubdirectoryDiscovery(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "igor_subdirs_baseline_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	writeTestFile := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", path, err)
		}
	}

	mkdirAll := func(path string) {
		t.Helper()
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", path, err)
		}
	}

	vendorDir := filepath.Join(tmpDir, "vendor")
	pkg4Dir := filepath.Join(vendorDir, "acme", "package4")
	pkg5Dir := filepath.Join(vendorDir, "acme", "package5")
	pkg6Dir := filepath.Join(vendorDir, "acme", "package6")
	pkg7Dir := filepath.Join(vendorDir, "acme", "package7")

	// Pkg 4: config/ci/igor.json specifying a baseline in the same folder: "igor-baseline.json"
	pkg4ConfigDir := filepath.Join(pkg4Dir, "config", "ci")
	mkdirAll(pkg4ConfigDir)
	pkg4Config := `{
		"baseline": "igor-baseline.json"
	}`
	writeTestFile(filepath.Join(pkg4ConfigDir, "igor.json"), pkg4Config)
	pkg4Baseline := `{
		"files": {
			"src/Service4.php": [
				{"message": "State mutation in Service4"}
			]
		}
	}`
	writeTestFile(filepath.Join(pkg4ConfigDir, "igor-baseline.json"), pkg4Baseline)

	// Pkg 5: config/igor-baseline.json directly, no igor.json
	pkg5ConfigDir := filepath.Join(pkg5Dir, "config")
	mkdirAll(pkg5ConfigDir)
	pkg5Baseline := `{
		"files": {
			"src/Service5.php": [
				{"message": "State mutation in Service5"}
			]
		}
	}`
	writeTestFile(filepath.Join(pkg5ConfigDir, "igor-baseline.json"), pkg5Baseline)

	// Pkg 6: .github/igor.json specifying a baseline in the package root: "root-baseline.json"
	pkg6GithubDir := filepath.Join(pkg6Dir, ".github")
	mkdirAll(pkg6GithubDir)
	pkg6Config := `{
		"baseline": "root-baseline.json"
	}`
	writeTestFile(filepath.Join(pkg6GithubDir, "igor.json"), pkg6Config)
	pkg6Baseline := `{
		"files": {
			"src/Service6.php": [
				{"message": "State mutation in Service6"}
			]
		}
	}`
	writeTestFile(filepath.Join(pkg6Dir, "root-baseline.json"), pkg6Baseline)

	// Pkg 7: config/ci/igor.json with non-existent absolute path "/app/config/ci/igor-baseline.json"
	pkg7ConfigDir := filepath.Join(pkg7Dir, "config", "ci")
	mkdirAll(pkg7ConfigDir)
	pkg7Config := `{
		"baseline": "/app/config/ci/igor-baseline.json"
	}`
	writeTestFile(filepath.Join(pkg7ConfigDir, "igor.json"), pkg7Config)
	pkg7Baseline := `{
		"files": {
			"src/Service7.php": [
				{"message": "State mutation in Service7"}
			]
		}
	}`
	writeTestFile(filepath.Join(pkg7ConfigDir, "igor-baseline.json"), pkg7Baseline)

	cfg := config.Config{
		BaselinePath:           "igor-baseline.json",
		IgnoreExternalBaseline: false,
	}

	// Load and merge baselines
	baseline := loadAuditBaseline(tmpDir, &cfg)

	// Verify package 4 was loaded
	p4Path := filepath.Join("vendor", "acme", "package4", "src/Service4.php")
	p4Entries, found := baseline.Files[p4Path]
	if !found {
		t.Errorf("Expected package4 baseline path %q to be loaded", p4Path)
	} else if len(p4Entries) != 1 || p4Entries[0].Message != "State mutation in Service4" {
		t.Errorf("Unexpected entries for package4: %v", p4Entries)
	}

	// Verify package 5 was loaded
	p5Path := filepath.Join("vendor", "acme", "package5", "src/Service5.php")
	p5Entries, found := baseline.Files[p5Path]
	if !found {
		t.Errorf("Expected package5 baseline path %q to be loaded", p5Path)
	} else if len(p5Entries) != 1 || p5Entries[0].Message != "State mutation in Service5" {
		t.Errorf("Unexpected entries for package5: %v", p5Entries)
	}

	// Verify package 6 was loaded (tests fallback of relative path to package root)
	p6Path := filepath.Join("vendor", "acme", "package6", "src/Service6.php")
	p6Entries, found := baseline.Files[p6Path]
	if !found {
		t.Errorf("Expected package6 baseline path %q to be loaded", p6Path)
	} else if len(p6Entries) != 1 || p6Entries[0].Message != "State mutation in Service6" {
		t.Errorf("Unexpected entries for package6: %v", p6Entries)
	}

	// Verify package 7 was loaded (tests dynamic suffix resolution of non-existent absolute path)
	p7Path := filepath.Join("vendor", "acme", "package7", "src/Service7.php")
	p7Entries, found := baseline.Files[p7Path]
	if !found {
		t.Errorf("Expected package7 baseline path %q to be loaded", p7Path)
	} else if len(p7Entries) != 1 || p7Entries[0].Message != "State mutation in Service7" {
		t.Errorf("Unexpected entries for package7: %v", p7Entries)
	}
}
