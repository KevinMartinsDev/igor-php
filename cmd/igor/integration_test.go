package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/igor-php/igor-php/internal/auditor"
	"github.com/igor-php/igor-php/internal/config"
)

func setupMockSymfonyProject(t *testing.T) (string, string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mock_symfony_deep")
	if err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(tmpDir, "bin")
	vendorDir := filepath.Join(tmpDir, "vendor")
	srcDir := filepath.Join(tmpDir, "src", "Service")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	serviceContent := `<?php
namespace App\Service;
class MyService {
    private $state;
    public function set($v) { $this->state = $v; }
}
`
	servicePath := filepath.Join(srcDir, "MyService.php")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		t.Fatal(err)
	}

	autoloaderContent := `<?php
spl_autoload_register(function ($class) {
    if ($class === 'App\\Service\\MyService') {
        require_once '` + servicePath + `';
    }
});
`
	if err := os.WriteFile(filepath.Join(vendorDir, "autoload.php"), []byte(autoloaderContent), 0644); err != nil {
		t.Fatal(err)
	}

	mockConsoleContent := `<?php
if ($argv[1] === 'debug:container') {
    echo json_encode([
        'definitions' => [
            'app.my_service' => [
                'class' => 'App\\Service\\MyService',
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

	return tmpDir, servicePath
}

func TestSymfonyIntegration(t *testing.T) {
	tmpDir, servicePath := setupMockSymfonyProject(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	t.Run("Bridge should load container and LOCATE files via Reflection", func(t *testing.T) {
		requirePHP(t)
		bridge := auditor.NewSymfonyBridge(tmpDir, "bin/console", config.Config{NoAgent: true})
		err := bridge.LoadContainer("prod")
		if err != nil {
			t.Fatalf("LoadContainer failed: %v", err)
		}

		if bridge.Container == nil {
			t.Fatal("Expected container to be loaded")
		}

		filePath, found := bridge.ClassToFile["App\\Service\\MyService"]
		if !found {
			t.Fatal("Expected App\\Service\\MyService to be located via reflection")
		}

		realServicePath, _ := filepath.EvalSymlinks(servicePath)
		realLocatedPath, _ := filepath.EvalSymlinks(filePath)

		if realLocatedPath != realServicePath {
			t.Errorf("Expected path %s, got %s", realServicePath, realLocatedPath)
		}
	})

	t.Run("Bridge should survive PHP deprecations or warnings during reflection", func(t *testing.T) {
		requirePHP(t)

		noiseAutoloader := `<?php
fwrite(STDERR, "PHP Deprecated:  Some deprecated library feature in autoload\n");
echo "Psr\\Log\\InvalidArgumentException is loaded\n";

spl_autoload_register(function ($class) {
    if ($class === 'App\\Service\\MyService') {
        require_once '` + servicePath + `';
    }
});
`
		if err := os.WriteFile(filepath.Join(tmpDir, "vendor", "autoload.php"), []byte(noiseAutoloader), 0644); err != nil {
			t.Fatal(err)
		}

		defer func() {
			originalAutoloader := `<?php
spl_autoload_register(function ($class) {
    if ($class === 'App\\Service\\MyService') {
        require_once '` + servicePath + `';
    }
});
`
			_ = os.WriteFile(filepath.Join(tmpDir, "vendor", "autoload.php"), []byte(originalAutoloader), 0644)
		}()

		bridge := auditor.NewSymfonyBridge(tmpDir, "bin/console", config.Config{NoAgent: true})
		err := bridge.LoadContainer("prod")
		if err != nil {
			t.Fatalf("LoadContainer failed with stdout/stderr noise: %v", err)
		}

		if bridge.Container == nil {
			t.Fatal("Expected container to be loaded")
		}

		filePath, found := bridge.ClassToFile["App\\Service\\MyService"]
		if !found {
			t.Fatal("Expected App\\Service\\MyService to be located via reflection even with noise")
		}

		realServicePath, _ := filepath.EvalSymlinks(servicePath)
		realLocatedPath, _ := filepath.EvalSymlinks(filePath)

		if realLocatedPath != realServicePath {
			t.Errorf("Expected path %s, got %s", realServicePath, realLocatedPath)
		}
	})

	t.Run("Bridge should support custom console path", func(t *testing.T) {
		requirePHP(t)
		customBinDir := filepath.Join(tmpDir, "app")
		if err := os.MkdirAll(customBinDir, 0755); err != nil {
			t.Fatal(err)
		}
		customConsole := filepath.Join(customBinDir, "console")
		if err := os.Rename(filepath.Join(tmpDir, "bin", "console"), customConsole); err != nil {
			t.Fatal(err)
		}

		bridge := auditor.NewSymfonyBridge(tmpDir, "app/console", config.Config{NoAgent: true})
		if err := bridge.LoadContainer("prod"); err != nil {
			t.Fatalf("LoadContainer with custom path failed: %v", err)
		}

		if _, found := bridge.ClassToFile["App\\Service\\MyService"]; !found {
			t.Error("Should still be able to find services with custom console path")
		}
	})

	t.Run("Auditor should correctly audit the mocked service", func(t *testing.T) {
		aud := auditor.NewAuditor(config.Config{})
		findings, err := aud.Audit(servicePath, nil)
		if err != nil {
			t.Fatalf("Audit failed: %v", err)
		}
		if len(findings) == 0 {
			t.Error("Expected 1 finding for stateful service, got 0")
		}
	})

	t.Run("Bridge should prioritize Igor Agent service map", func(t *testing.T) {
		requirePHP(t)
		cacheDir := filepath.Join(tmpDir, "var", "cache", "test")
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatal(err)
		}

		mapContent := `{"definitions": {"agent.service": {"class": "App\\Service\\MyService", "public": true, "shared": true}}, "aliases": {}}`
		if err := os.WriteFile(filepath.Join(cacheDir, "igor_service_map.json"), []byte(mapContent), 0644); err != nil {
			t.Fatal(err)
		}

		bridge := auditor.NewSymfonyBridge(tmpDir, "bin/console", config.Config{})
		if err := bridge.LoadContainer("test"); err != nil {
			t.Fatalf("LoadContainer failed: %v", err)
		}

		if _, found := bridge.Container.Definitions["agent.service"]; !found {
			t.Error("Expected service map to be loaded from Agent")
		}
	})
}
