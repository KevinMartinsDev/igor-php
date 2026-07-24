package symbol

import (
	"path/filepath"
	"strings"
)

// Finding represents a single issue detected by the linter.
type Finding struct {
	Message      string   `json:"message"`
	Code         string   `json:"code"`
	Snippet      string   `json:"snippet"`
	Remediation  string   `json:"remediation"`
	Severity     string   `json:"severity"` // "ERROR" or "WARNING"
	Line         int      `json:"line"`
	ASTDetails   string   `json:"ast_details"`
	Dependencies []string `json:"dependencies"`
}

// Result groups findings by file.
type Result struct {
	FilePath string    `json:"file_path"`
	Findings []Finding `json:"findings"`
}

// SymfonyContainer represents the output of debug:container --format=json.
type SymfonyContainer struct {
	Definitions map[string]SymfonyService `json:"definitions"`
	Aliases     map[string]interface{}    `json:"aliases"`
}

// SymfonyService represents a single service definition in Symfony.
type SymfonyService struct {
	Class      string `json:"class"`
	Public     bool   `json:"public"`
	Shared     bool   `json:"shared"`
	Arguments  []any  `json:"arguments"`
	Resettable bool   `json:"resettable"`
	Tags       any    `json:"tags"`
}

// IsResettable checks if this service is resettable in any supported format.
func (s *SymfonyService) IsResettable() bool {
	if s.Resettable {
		return true
	}
	if s.Tags == nil {
		return false
	}
	// Try parsing as array/slice of maps (standard format)
	if slice, ok := s.Tags.([]any); ok {
		for _, item := range slice {
			if m, ok := item.(map[string]any); ok {
				if name, ok := m["name"].(string); ok && name == "kernel.reset" {
					return true
				}
			}
		}
	}
	// Try parsing as map (alternative format)
	if m, ok := s.Tags.(map[string]any); ok {
		if _, exists := m["kernel.reset"]; exists {
			return true
		}
	}
	return false
}

// AuditStatus represents the audit state of a single service.
type AuditStatus struct {
	ServiceID    string    `json:"service_id"`
	FilePath     string    `json:"file_path"`
	Status       string    `json:"status"` // "✅ OK", "❌ KO", "⚠️  WARN", "❓ MISSING"
	Findings     []Finding `json:"findings"`
	Dependencies []string  `json:"dependencies"`
	IsShared     bool      `json:"is_shared"`
	IsPublic     bool      `json:"is_public"`
}

// IsVendor returns true if the file is part of the vendor directory.
func (a AuditStatus) IsVendor(projectRoot string) bool {
	relPath := a.FilePath
	if rel, found := strings.CutPrefix(a.FilePath, projectRoot); found && rel != "" {
		relPath = strings.TrimPrefix(rel, "/")
	}

	if strings.HasPrefix(relPath, "vendor/") {
		return true
	}

	absPath, _ := filepath.Abs(a.FilePath)
	return strings.Contains(absPath, "/vendor/")
}
