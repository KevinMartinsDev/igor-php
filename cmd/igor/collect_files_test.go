package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/igor-php/igor-php/internal/auditor"
	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestCollectFiles_SymfonyActive_SkipsLocalFiles(t *testing.T) {
	// 1. Create a temporary project directory
	tmpDir, err := os.MkdirTemp("", "collect_files_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}

	// 2. Create a service file and a non-service (entity) file
	servicePath := filepath.Join(srcDir, "MyService.php")
	if err := os.WriteFile(servicePath, []byte("<?php class MyService {}"), 0644); err != nil {
		t.Fatalf("Failed to write service file: %v", err)
	}

	entityPath := filepath.Join(srcDir, "MyEntity.php")
	if err := os.WriteFile(entityPath, []byte("<?php class MyEntity {}"), 0644); err != nil {
		t.Fatalf("Failed to write entity file: %v", err)
	}

	cfg := config.Config{
		NoAgent: true,
	}

	// 3. Scenario A: Symfony bridge is ACTIVE
	aud := auditor.NewAuditor(cfg)
	bridge := auditor.NewSymfonyBridge(tmpDir, "bin/console", cfg)
	bridge.Container = &symbol.SymfonyContainer{
		Definitions: map[string]symbol.SymfonyService{
			"app.my_service": {
				Class:  "App\\Service\\MyService",
				Public: true,
				Shared: true,
			},
		},
	}
	bridge.ClassToFile = map[string]string{
		"App\\Service\\MyService": servicePath,
	}
	aud.Symfony = bridge

	auditList := collectFiles(tmpDir, cfg, aud)

	// We expect only MyService.php to be audited (from Symfony bridge).
	// MyEntity.php must be skipped since it's not a service and local scan is disabled when Symfony is detected.
	hasService := false
	hasEntity := false
	for _, item := range auditList {
		if filepath.Base(item.FilePath) == "MyService.php" {
			hasService = true
		}
		if filepath.Base(item.FilePath) == "MyEntity.php" {
			hasEntity = true
		}
	}

	if !hasService {
		t.Error("Expected MyService.php to be audited as a registered Symfony service, but it was skipped")
	}
	if hasEntity {
		t.Error("Expected MyEntity.php to be skipped when Symfony is active, but it was collected for audit")
	}

	// 4. Scenario B: Symfony bridge is INACTIVE (non-Symfony project)
	audInactive := auditor.NewAuditor(cfg) // audInactive.Symfony is nil

	auditListInactive := collectFiles(tmpDir, cfg, audInactive)

	// We expect BOTH files to be collected via standard directory scan
	hasServiceInactive := false
	hasEntityInactive := false
	for _, item := range auditListInactive {
		if filepath.Base(item.FilePath) == "MyService.php" {
			hasServiceInactive = true
		}
		if filepath.Base(item.FilePath) == "MyEntity.php" {
			hasEntityInactive = true
		}
	}

	if !hasServiceInactive || !hasEntityInactive {
		t.Errorf("Expected both files to be collected when Symfony is inactive. Got MyService.php: %t, MyEntity.php: %t", hasServiceInactive, hasEntityInactive)
	}
}

func TestCollectFiles_SkipsSafeNamespacesInherited(t *testing.T) {
	// 1. Create a temporary project directory
	tmpDir, err := os.MkdirTemp("", "collect_files_safe_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}

	servicePath := filepath.Join(srcDir, "MyService.php")
	if err := os.WriteFile(servicePath, []byte("<?php class MyService {}"), 0644); err != nil {
		t.Fatalf("Failed to write service file: %v", err)
	}

	inheritedPath := filepath.Join(srcDir, "MyInheritedSafeService.php")
	if err := os.WriteFile(inheritedPath, []byte("<?php class MyInheritedSafeService {}"), 0644); err != nil {
		t.Fatalf("Failed to write inherited file: %v", err)
	}

	cfg := config.Config{
		NoAgent:        true,
		SafeNamespaces: []string{"Symfony\\", "App\\Safe\\"},
	}

	aud := auditor.NewAuditor(cfg)
	bridge := auditor.NewSymfonyBridge(tmpDir, "bin/console", cfg)
	bridge.Container = &symbol.SymfonyContainer{
		Definitions: map[string]symbol.SymfonyService{
			"app.my_service": {
				Class:  "App\\Service\\MyService",
				Public: true,
				Shared: true,
			},
		},
	}
	bridge.ClassToFile = map[string]string{
		"App\\Service\\MyService":           servicePath,
		"App\\Safe\\MyInheritedSafeService": inheritedPath,
	}
	aud.Symfony = bridge

	auditList := collectFiles(tmpDir, cfg, aud)

	// App\\Safe\\MyInheritedSafeService must be skipped since it is in a safe namespace.
	hasService := false
	hasInherited := false
	for _, item := range auditList {
		if filepath.Base(item.FilePath) == "MyService.php" {
			hasService = true
		}
		if filepath.Base(item.FilePath) == "MyInheritedSafeService.php" {
			hasInherited = true
		}
	}

	if !hasService {
		t.Error("Expected MyService.php to be collected")
	}
	if hasInherited {
		t.Error("Expected App\\Safe\\MyInheritedSafeService to be skipped due to safe namespace prefix, but it was collected")
	}
}
