package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/igor-php/igor-php/internal/auditor"
	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestVendorExclusionBug(t *testing.T) {
	// Setup a mock project
	tmpDir, err := os.MkdirTemp("", "repro_vendor_bug")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	binDir := filepath.Join(tmpDir, "bin")
	vendorDir := filepath.Join(tmpDir, "vendor", "some-pkg")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Service in vendor
	servicePath := filepath.Join(vendorDir, "VendorService.php")
	serviceContent := `<?php
namespace SomePkg;
class VendorService {
    private $state;
    public function set($v) { $this->state = $v; }
}
`
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Mock console to return this service as shared
	mockConsoleContent := `<?php
if ($argv[1] === 'debug:container') {
    echo json_encode([
        'definitions' => [
            'some_pkg.vendor_service' => [
                'class' => 'SomePkg\\VendorService',
                'public' => true,
                'shared' => true
            ]
        ],
        'aliases' => new stdClass()
    ]);
}
`
	if err := os.WriteFile(filepath.Join(binDir, "console"), []byte(mockConsoleContent), 0755); err != nil {
		t.Fatal(err)
	}

	// Config with vendor excluded
	cfg := config.Config{
		Exclude:     []string{"vendor"},
		ConsolePath: "bin/console",
		NoAgent:     true,
	}

	aud := auditor.NewAuditor(cfg)
	bridge := auditor.NewSymfonyBridge(tmpDir, cfg.ConsolePath, cfg)

	// Manually inject the ClassToFile mapping since reflection won't work easily here without proper autoloader
	bridge.Container = &symbol.SymfonyContainer{
		Definitions: map[string]symbol.SymfonyService{
			"some_pkg.vendor_service": {
				Class:  "SomePkg\\VendorService",
				Public: true,
				Shared: true,
			},
		},
	}
	bridge.ClassToFile = map[string]string{
		"SomePkg\\VendorService": servicePath,
	}
	aud.Symfony = bridge

	// Now run collectFiles
	auditList := collectFiles(tmpDir, cfg, aud)

	// We expect auditList to NOT contain VendorService because vendor is excluded
	found := false
	for _, status := range auditList {
		if status.ServiceID == "some_pkg.vendor_service" {
			found = true
			break
		}
	}

	if found {
		t.Errorf("Expected some_pkg.vendor_service to be excluded because its path is in vendor/, but it was found in audit list")
	}

	t.Run("Should handle trailing slashes in exclude patterns", func(t *testing.T) {
		cfgSlash := config.Config{
			Exclude:     []string{"vendor/"},
			ConsolePath: "bin/console",
			NoAgent:     true,
		}
		auditListSlash := collectFiles(tmpDir, cfgSlash, aud)
		foundSlash := false
		for _, status := range auditListSlash {
			if status.ServiceID == "some_pkg.vendor_service" {
				foundSlash = true
				break
			}
		}
		if foundSlash {
			t.Errorf("Expected some_pkg.vendor_service to be excluded with 'vendor/' pattern, but it was found")
		}
	})
}

func TestIgnoreVendors(t *testing.T) {
	// Setup a mock project
	tmpDir, err := os.MkdirTemp("", "test_ignore_vendors")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	binDir := filepath.Join(tmpDir, "bin")
	vendorDir := filepath.Join(tmpDir, "vendor", "some-pkg")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Service in vendor
	servicePath := filepath.Join(vendorDir, "VendorService.php")
	serviceContent := `<?php
namespace SomePkg;
class VendorService {
    private $state;
    public function set($v) { $this->state = $v; }
}
`
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Config with IgnoreVendors: true
	cfg := config.Config{
		IgnoreVendors: true,
		ConsolePath:   "bin/console",
		NoAgent:       true,
	}

	aud := auditor.NewAuditor(cfg)
	bridge := auditor.NewSymfonyBridge(tmpDir, cfg.ConsolePath, cfg)

	bridge.Container = &symbol.SymfonyContainer{
		Definitions: map[string]symbol.SymfonyService{
			"some_pkg.vendor_service": {
				Class:  "SomePkg\\VendorService",
				Public: true,
				Shared: true,
			},
		},
	}
	bridge.ClassToFile = map[string]string{
		"SomePkg\\VendorService": servicePath,
	}
	aud.Symfony = bridge

	// Now run collectFiles
	auditList := collectFiles(tmpDir, cfg, aud)

	// We expect auditList to NOT contain VendorService because IgnoreVendors is true
	found := false
	for _, status := range auditList {
		if status.ServiceID == "some_pkg.vendor_service" {
			found = true
			break
		}
	}
	if found {
		t.Errorf("Expected some_pkg.vendor_service to be excluded because IgnoreVendors is true, but it was found in audit list")
	}

	// Config with IgnoreVendors: false
	cfgFalse := config.Config{
		IgnoreVendors: false,
		ConsolePath:   "bin/console",
		NoAgent:       true,
	}
	auditListFalse := collectFiles(tmpDir, cfgFalse, aud)
	foundFalse := false
	for _, status := range auditListFalse {
		if status.ServiceID == "some_pkg.vendor_service" {
			foundFalse = true
			break
		}
	}
	if !foundFalse {
		t.Errorf("Expected some_pkg.vendor_service to be included because IgnoreVendors is false, but it was not found")
	}
}
