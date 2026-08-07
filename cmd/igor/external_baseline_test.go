package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/igor-php/igor-php/internal/config"
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
				{"message": "State mutation detected in Service1"}
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

	baseline := loadAuditBaseline(tmpDir, cfg)

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

	baseline := loadAuditBaseline(tmpDir, cfg)

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

	baseline := loadAuditBaseline(tmpDir, cfg)

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

	baseline := loadAuditBaseline(tmpDir, cfg)

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
