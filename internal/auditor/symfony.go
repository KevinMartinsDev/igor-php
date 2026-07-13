package auditor

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

//go:embed find_class_files.php
var phpHelperScript []byte

// SymfonyBridge handles all interactions with the Symfony Framework.
type SymfonyBridge struct {
	Root        string
	ConsolePath string
	Config      config.Config
	Container   *symbol.SymfonyContainer
	ClassToFile map[string]string
}

// NewSymfonyBridge creates a new bridge for a given project root.
func NewSymfonyBridge(root string, consolePath string, cfg config.Config) *SymfonyBridge {
	return &SymfonyBridge{
		Root:        root,
		ConsolePath: consolePath,
		Config:      cfg,
		ClassToFile: make(map[string]string),
	}
}

// DetectSymfony attempts to find a Symfony project and initialize the bridge.
func DetectSymfony(rootPath string, cfg config.Config) (*SymfonyBridge, error) {
	projectRoot := rootPath
	consolePath := cfg.ConsolePath

	// 1. Check if the console exists at the specified path
	fullPath := filepath.Join(projectRoot, consolePath)
	if _, err := os.Stat(fullPath); err != nil {
		// If custom path was provided but doesn't exist, it's an error
		if consolePath != "bin/console" {
			return nil, fmt.Errorf("symfony console not found at %s", fullPath)
		}

		// Fallback for default bin/console: try parent dir (useful for vendor/bin context)
		projectRoot = filepath.Dir(rootPath)
		fullPath = filepath.Join(projectRoot, consolePath)
		if _, err := os.Stat(fullPath); err != nil {
			return nil, nil // Not a Symfony project
		}
	}

	// 2. If we found a console, try to initialize the bridge
	fmt.Fprintf(os.Stderr, "🔍 Symfony project detected at %s. Initializing Deep Audit...\n", projectRoot)
	sb := NewSymfonyBridge(projectRoot, consolePath, cfg)
	if err := sb.LoadContainer(cfg.Env); err != nil {

		return nil, fmt.Errorf("could not load symfony container: %v\n👉 ensure your project is bootable in '%s' environment", err, cfg.Env)
	}

	return sb, nil
}

// LoadContainer fetches definitions and locates files via PHP Reflection in the configured environment.
func (b *SymfonyBridge) LoadContainer(env string) error {
	// 1. Try to load from Igor Agent map first (faster, no container boot)
	loaded, err := b.tryLoadFromAgent(env)
	if err != nil {
		return err
	}
	if loaded {
		return nil
	}

	// 2. Fallback to debug:container
	consolePath := filepath.Join(b.Root, b.ConsolePath)
	cmd := exec.Command("php", consolePath, "debug:container", "--format=json", "--show-hidden", "--env="+env, "--no-debug")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute debug:container: %v", err)
	}

	strOutput := string(output)
	start := strings.Index(strOutput, "{")
	end := strings.LastIndex(strOutput, "}")
	if start == -1 || end == -1 || end < start {
		return fmt.Errorf("could not find a valid JSON object in Symfony output (check if your %s environment is valid)", env)
	}
	jsonPart := strOutput[start : end+1]

	var container symbol.SymfonyContainer
	if err := json.Unmarshal([]byte(jsonPart), &container); err != nil {
		return fmt.Errorf("failed to parse Symfony container JSON: %v", err)
	}
	b.Container = &container

	// 3. Locate files
	return b.locateFilesViaReflection(jsonPart)
}

func (b *SymfonyBridge) tryLoadFromAgent(env string) (bool, error) {
	if b.Config.NoAgent {
		return false, nil
	}

	// Possible paths for the agent map
	paths := []string{
		filepath.Join(b.Root, "var", "cache", env, "igor_service_map.json"),
		filepath.Join(b.Root, "var", "cache", "igor_service_map.json"),
	}

	found := false
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			found = true
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			var container symbol.SymfonyContainer
			if err := json.Unmarshal(data, &container); err != nil {
				continue
			}

			b.Container = &container
			fmt.Fprintf(os.Stderr, "⚡ Igor Agent detected: Using cached service map from %s\n", path)

			jsonPart, _ := json.Marshal(container)
			if err := b.locateFilesViaReflection(string(jsonPart)); err != nil {
				return false, err
			}
			return true, nil
		}
	}

	if !found {
		return false, fmt.Errorf("igor agent map not found (var/cache/%s/igor_service_map.json).\n   To ensure 100%% accuracy, please run 'php bin/console cache:clear' first.\n   If you want to perform a less precise scan without the agent, use the --no-agent flag", env)
	}

	return false, nil
}
func (b *SymfonyBridge) locateFilesViaReflection(jsonPart string) error {
	tmpHelper, err := os.CreateTemp("", "igor_helper_*.php")
	if err != nil {
		return fmt.Errorf("failed to create temp helper file: %v", err)
	}
	defer func() { _ = os.Remove(tmpHelper.Name()) }()

	if _, err := tmpHelper.Write(phpHelperScript); err != nil {
		_ = tmpHelper.Close()
		return fmt.Errorf("failed to write to temp helper: %v", err)
	}
	if err := tmpHelper.Close(); err != nil {
		return fmt.Errorf("failed to close temp helper: %v", err)
	}

	reflectCmd := exec.Command("php", tmpHelper.Name(), b.Root)
	reflectCmd.Stdin = bytes.NewReader([]byte(jsonPart))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	reflectCmd.Stdout = &stdout
	reflectCmd.Stderr = &stderr

	err = reflectCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to locate files via reflection: %v (stderr: %s)", err, stderr.String())
	}

	strOutput := stdout.String()
	start := strings.Index(strOutput, "{")
	end := strings.LastIndex(strOutput, "}")
	if start == -1 || end == -1 || end < start {
		// Fallback to try unmarshaling the entire stdout if no braces found (highly unlikely if it ran successfully)
		if err := json.Unmarshal(stdout.Bytes(), &b.ClassToFile); err != nil {
			return fmt.Errorf("failed to parse reflection mapping (no JSON object found in output: %q): %v", strOutput, err)
		}
		return nil
	}

	cleanJson := strOutput[start : end+1]
	if err := json.Unmarshal([]byte(cleanJson), &b.ClassToFile); err != nil {
		return fmt.Errorf("failed to parse reflection mapping: %v", err)
	}

	return nil
}

// IsSharedService checks if a FQCN is a shared service in Symfony.
func (b *SymfonyBridge) IsSharedService(className string) bool {
	if b.Container == nil {
		return true
	}
	className = strings.TrimPrefix(className, "\\")
	for _, def := range b.Container.Definitions {
		if strings.TrimPrefix(def.Class, "\\") == className {
			return def.Shared
		}
	}
	return false
}
