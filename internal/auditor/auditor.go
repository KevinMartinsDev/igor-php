// Package main provides the core auditing logic for igor-php.
package auditor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/igor-php/igor-php/internal/analyzer"
	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
	sitter "github.com/tree-sitter/go-tree-sitter"
	php "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

// Auditor orchestrates the analysis of PHP files.
type Auditor struct {
	Config            config.Config
	Symfony           *SymfonyBridge
	NonSharedServices analyzer.NonSharedServiceMap
	AuditedClasses    map[string]bool
	mu                sync.Mutex
}

// NewAuditor creates a new instance of the Auditor.
func NewAuditor(cfg config.Config) *Auditor {
	return &Auditor{
		Config:         cfg,
		AuditedClasses: make(map[string]bool),
	}
}

// Audit analyzes a single PHP file and returns findings.
func (a *Auditor) Audit(path string, dependencies []string) ([]symbol.Finding, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", path, err)
	}

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	if err := p.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("failed to set language for %s: %v", path, err)
	}

	tree := p.Parse(content, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse %s", path)
	}
	defer tree.Close()

	v := analyzer.NewVisitor(content, a)
	v.SetDependencies(dependencies)
	v.SetNonSharedServices(a.NonSharedServices)
	v.Walk(tree.RootNode())

	return v.Findings(), nil
}

// ExtractFQCN extracts the full class name from a file.
func (a *Auditor) ExtractFQCN(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)

	tree := p.Parse(content, nil)
	if tree == nil {
		return "", fmt.Errorf("failed to parse %s", path)
	}
	defer tree.Close()

	var namespace, className string
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil {
			return
		}
		switch n.Kind() {
		case "namespace_definition":
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				namespace = string(content[nameNode.StartByte():nameNode.EndByte()])
			}
		case "class_declaration", "trait_declaration":
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				className = string(content[nameNode.StartByte():nameNode.EndByte()])
			}
		}
		if className != "" {
			return
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(tree.RootNode())

	if className == "" {
		return "", nil
	}
	if namespace == "" {
		return className, nil
	}
	return namespace + "\\" + className, nil
}

func (a *Auditor) RecordClassAudited(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.AuditedClasses[name] = true
}

func (a *Auditor) IsSafeNamespace(className string) bool {
	className = strings.TrimPrefix(className, "\\")
	for _, ns := range a.Config.SafeNamespaces {
		if strings.HasPrefix(className, strings.TrimPrefix(ns, "\\")) {
			return true
		}
	}
	return false
}

// IsDataPath returns true if the file path belongs to a directory that usually contains only data (Entity, DTO, etc.)
func (a *Auditor) IsDataPath(path string) bool {
	dataFolders := []string{"Entity", "DTO", "Dto", "ApiResource", "Migrations", "Document", "tests", "Tests"}
	for _, folder := range dataFolders {
		if strings.Contains(path, string(os.PathSeparator)+folder+string(os.PathSeparator)) ||
			strings.HasSuffix(filepath.Dir(path), string(os.PathSeparator)+folder) {
			return true
		}
	}
	return false
}

func normalizeClassName(s string) string {
	s = strings.ReplaceAll(s, "/", "\\")
	s = strings.TrimSpace(s)
	return strings.TrimPrefix(s, "\\")
}

func resolveAliasTarget(val interface{}) string {
	if s, ok := val.(string); ok {
		return s
	}
	if m, ok := val.(map[string]interface{}); ok {
		for _, key := range []string{"service", "id", "target"} {
			if target, ok := m[key].(string); ok {
				return target
			}
		}
	}
	return ""
}

func (a *Auditor) resolveAliases(className string) []string {
	if a.Symfony == nil || a.Symfony.Container == nil {
		return []string{normalizeClassName(className)}
	}

	normClassName := normalizeClassName(className)
	possibleIDs := []string{normClassName}
	visited := map[string]bool{normClassName: true}
	current := normClassName

	for {
		var foundTarget string
		for aliasKey, val := range a.Symfony.Container.Aliases {
			if normalizeClassName(aliasKey) == current {
				target := resolveAliasTarget(val)
				if target != "" {
					foundTarget = normalizeClassName(target)
					break
				}
			}
		}

		if foundTarget != "" {
			current = foundTarget
			if !visited[current] {
				visited[current] = true
				possibleIDs = append(possibleIDs, current)
				continue
			}
		}
		break
	}

	return possibleIDs
}

func (a *Auditor) IsExplicitlyNonShared(className string) bool {
	if a.Symfony == nil || a.Symfony.Container == nil {
		return false
	}
	possibleIDs := a.resolveAliases(className)

	for defID, def := range a.Symfony.Container.Definitions {
		normDefID := normalizeClassName(defID)
		normDefClass := normalizeClassName(def.Class)

		for _, id := range possibleIDs {
			if normDefID == id || normDefClass == id {
				return !def.Shared
			}
		}
	}
	return false
}

func (a *Auditor) IsResettable(className string) bool {
	if a.Symfony == nil || a.Symfony.Container == nil {
		return false
	}
	possibleIDs := a.resolveAliases(className)

	for defID, def := range a.Symfony.Container.Definitions {
		normDefID := normalizeClassName(defID)
		normDefClass := normalizeClassName(def.Class)

		for _, id := range possibleIDs {
			if normDefID == id || normDefClass == id {
				if def.IsResettable() {
					return true
				}
			}
		}
	}
	return false
}

// IsDevPackagePath returns true if the file path belongs to a dev package in vendor/.
func (a *Auditor) IsDevPackagePath(path string) bool {
	// Convert to slash for cross-platform comparison
	path = filepath.ToSlash(path)
	for _, pkg := range a.Config.DevPackages {
		vendorPath := "vendor/" + pkg + "/"
		if strings.Contains(path, vendorPath) {
			return true
		}
	}
	return false
}
