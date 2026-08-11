package auditor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestNewSymfonyBridge(t *testing.T) {
	cfg := config.Config{}
	sb := NewSymfonyBridge("/path/to/root", "bin/console", cfg)
	if sb.Root != "/path/to/root" {
		t.Errorf("Expected Root to be /path/to/root, got %s", sb.Root)
	}
	if sb.ConsolePath != "bin/console" {
		t.Errorf("Expected ConsolePath to be bin/console, got %s", sb.ConsolePath)
	}
	if sb.ClassToFile == nil {
		t.Error("Expected ClassToFile to be initialized")
	}
}

func TestSymfonyBridge_IsSharedService(t *testing.T) {
	// Case 1: Container is nil
	sb := &SymfonyBridge{Container: nil}
	if !sb.IsSharedService("AnyClass") {
		t.Error("Expected IsSharedService to return true when Container is nil")
	}

	// Case 2: Container has definitions
	container := &symbol.SymfonyContainer{
		Definitions: map[string]symbol.SymfonyService{
			"app.shared": {
				Class:  "App\\Shared",
				Shared: true,
			},
			"app.non_shared": {
				Class:  "\\App\\NonShared", // with leading backslash
				Shared: false,
			},
		},
	}
	sb.Container = container

	tests := []struct {
		className string
		expected  bool
	}{
		{"App\\Shared", true},
		{"\\App\\Shared", true},
		{"App\\NonShared", false},
		{"\\App\\NonShared", false},
		{"App\\Unknown", false}, // not found, returns false
	}

	for _, tt := range tests {
		t.Run(tt.className, func(t *testing.T) {
			if got := sb.IsSharedService(tt.className); got != tt.expected {
				t.Errorf("IsSharedService(%q) = %v, expected %v", tt.className, got, tt.expected)
			}
		})
	}
}

func TestDetectSymfony_NoConsole(t *testing.T) {
	// Create a temp directory that does NOT contain a console
	tempDir, err := os.MkdirTemp("", "igor_test_detect_symfony_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Case 1: Default console path does not exist -> returns nil, nil
	cfg := config.Config{ConsolePath: "bin/console"}
	sb, err := DetectSymfony(tempDir, cfg)
	if err != nil {
		t.Errorf("Expected no error when console not found at default path, got: %v", err)
	}
	if sb != nil {
		t.Errorf("Expected nil bridge when console not found at default path, got: %v", sb)
	}

	// Case 2: Custom console path does not exist -> returns error
	cfgCustom := config.Config{ConsolePath: "custom/console"}
	_, err = DetectSymfony(tempDir, cfgCustom)
	if err == nil {
		t.Error("Expected error when custom console path is missing, got nil")
	} else if !strings.Contains(err.Error(), "symfony console not found") {
		t.Errorf("Expected console not found error, got: %v", err)
	}
}

func TestSymfonyBridge_tryLoadFromAgent(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "igor_test_agent_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Prepare an agent file under var/cache/dev/igor_service_map.json
	cacheDir := filepath.Join(tempDir, "var", "cache", "dev")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	container := symbol.SymfonyContainer{
		Definitions: map[string]symbol.SymfonyService{
			"app.my_service": {
				Class:  "DateTime", // Use a built-in class so reflection succeeds easily!
				Shared: true,
			},
		},
	}
	data, err := json.Marshal(container)
	if err != nil {
		t.Fatalf("failed to marshal container: %v", err)
	}

	mapPath := filepath.Join(cacheDir, "igor_service_map.json")
	if err := os.WriteFile(mapPath, data, 0644); err != nil {
		t.Fatalf("failed to write map file: %v", err)
	}

	cfg := config.Config{Env: "dev"}
	sb := NewSymfonyBridge(tempDir, "bin/console", cfg)

	// Case 1: Config has NoAgent = true
	sb.Config.NoAgent = true
	loaded, err := sb.tryLoadFromAgent("dev")
	if err != nil {
		t.Errorf("unexpected error with NoAgent: %v", err)
	}
	if loaded {
		t.Error("expected loaded to be false with NoAgent")
	}

	// Case 2: Load succeeds with NoAgent = false
	sb.Config.NoAgent = false
	loaded, err = sb.tryLoadFromAgent("dev")
	if err != nil {
		t.Errorf("unexpected error during tryLoadFromAgent: %v", err)
	}
	if !loaded {
		t.Error("expected loaded to be true when map exists")
	}
	if sb.Container == nil {
		t.Error("expected Container to be populated")
	}

	// Case 3: Load fails when agent map does not exist for a different env
	sbMissing := NewSymfonyBridge(tempDir, "bin/console", cfg)
	_, err = sbMissing.tryLoadFromAgent("prod")
	if err == nil {
		t.Error("expected error when agent map doesn't exist, got nil")
	} else if !strings.Contains(err.Error(), "igor agent map not found") {
		t.Errorf("expected agent map not found error, got: %v", err)
	}
}

func TestSymfonyBridge_LoadContainer_Fallback(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "igor_test_fallback_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create bin directory and console script
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// We'll write a mock PHP bin/console script
	consoleScript := `<?php
// mock console
if (in_array('debug:container', $argv)) {
    echo json_encode([
        'definitions' => [
            'app.my_service' => [
                'class' => 'DateTime',
                'shared' => true,
            ]
        ],
        'aliases' => (object)[]
    ]);
    exit(0);
}
echo "Unknown command\n";
exit(1);
`
	consolePath := filepath.Join(binDir, "console")
	if err := os.WriteFile(consolePath, []byte(consoleScript), 0755); err != nil {
		t.Fatalf("failed to write mock console: %v", err)
	}

	cfg := config.Config{
		ConsolePath: "bin/console",
		NoAgent:     true, // bypass agent check
		Env:         "dev",
	}

	sb := NewSymfonyBridge(tempDir, "bin/console", cfg)

	// Running LoadContainer should execute the mock PHP console, get the JSON container,
	// and run reflection mapping successfully (since DateTime is a standard PHP class).
	err = sb.LoadContainer("dev")
	if err != nil {
		t.Fatalf("LoadContainer failed: %v", err)
	}

	if sb.Container == nil {
		t.Fatal("expected Container to be loaded")
	}
	if !sb.IsSharedService("DateTime") {
		t.Error("expected DateTime service to be shared")
	}
}

func TestDetectSymfony_Success(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "igor_test_detect_success_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create bin directory and console script
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	consoleScript := `<?php
if (in_array('debug:container', $argv)) {
    echo json_encode([
        'definitions' => [
            'App\Service\MyService' => [
                'class' => 'DateTime',
                'shared' => true,
            ]
        ],
        'aliases' => (object)[]
    ]);
    exit(0);
}
`
	consolePath := filepath.Join(binDir, "console")
	if err := os.WriteFile(consolePath, []byte(consoleScript), 0755); err != nil {
		t.Fatalf("failed to write mock console: %v", err)
	}

	cfg := config.Config{
		ConsolePath: "bin/console",
		NoAgent:     true,
		Env:         "dev",
	}

	sb, err := DetectSymfony(tempDir, cfg)
	if err != nil {
		t.Fatalf("DetectSymfony failed: %v", err)
	}
	if sb == nil {
		t.Fatal("expected SymfonyBridge, got nil")
	}
	if sb.Container == nil {
		t.Error("expected Container to be populated")
	}
}

func TestSymfonyBridge_LoadContainer_MalformedJSON(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_test_malformed_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binDir := filepath.Join(tempDir, "bin")
	_ = os.MkdirAll(binDir, 0755)

	consoleScript := `<?php
echo "invalid { json";
exit(0);
`
	_ = os.WriteFile(filepath.Join(binDir, "console"), []byte(consoleScript), 0755)

	cfg := config.Config{
		ConsolePath: "bin/console",
		NoAgent:     true,
		Env:         "dev",
	}

	sb := NewSymfonyBridge(tempDir, "bin/console", cfg)
	err = sb.LoadContainer("dev")
	if err == nil {
		t.Error("expected error for malformed json, got nil")
	}
}

func TestSymfonyBridge_LoadContainer_CmdError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_test_cmderror_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binDir := filepath.Join(tempDir, "bin")
	_ = os.MkdirAll(binDir, 0755)

	consoleScript := `<?php
exit(1);
`
	_ = os.WriteFile(filepath.Join(binDir, "console"), []byte(consoleScript), 0755)

	cfg := config.Config{
		ConsolePath: "bin/console",
		NoAgent:     true,
		Env:         "dev",
	}

	sb := NewSymfonyBridge(tempDir, "bin/console", cfg)
	err = sb.LoadContainer("dev")
	if err == nil {
		t.Error("expected error when console fails, got nil")
	}
}

func TestSymfonyBridge_tryLoadFromAgent_MalformedJSON(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "igor_test_agent_malformed_")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cacheDir := filepath.Join(tempDir, "var", "cache", "dev")
	_ = os.MkdirAll(cacheDir, 0755)
	_ = os.WriteFile(filepath.Join(cacheDir, "igor_service_map.json"), []byte("invalid json"), 0644)

	cfg := config.Config{Env: "dev"}
	sb := NewSymfonyBridge(tempDir, "bin/console", cfg)

	loaded, err := sb.tryLoadFromAgent("dev")
	if err != nil {
		t.Errorf("expected no error when agent map is malformed (should be ignored), got: %v", err)
	}
	if loaded {
		t.Error("expected loaded to be false when agent map is malformed")
	}
}
