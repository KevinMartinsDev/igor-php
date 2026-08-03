package reporter

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/igor-php/igor-php/pkg/symbol"
)

// LLMOutput represents the strictly structured JSON format for LLMs.
type LLMOutput struct {
	Warnings []LLMWarning `json:"warnings"`
	Metadata LLMMetadata  `json:"metadata"`
}

// LLMWarning contains detailed context for a single detection.
type LLMWarning struct {
	FilePath   string     `json:"file_path"`
	Line       int        `json:"line"`
	Message    string     `json:"message"`
	Severity   string     `json:"severity"`
	Snippet    string     `json:"snippet"`
	ASTDetails string     `json:"ast_details"`
	RuleID     string     `json:"rule_id"`
	Context    LLMContext `json:"context"`
}

// LLMContext provides additional technical background for the warning.
type LLMContext struct {
	Dependencies  LLMServiceContext `json:"dependencies"`
	InjectedLines []string          `json:"injected_dependencies,omitempty"`
}

// LLMServiceContext holds metadata about the service where the finding was detected.
type LLMServiceContext struct {
	IsShared  bool   `json:"is_shared"`
	IsPublic  bool   `json:"is_public"`
	ServiceID string `json:"service_id"`
}

// LLMMetadata holds execution environment information.
type LLMMetadata struct {
	GeneratedAt time.Time `json:"generated_at"`
	IgorVersion string    `json:"igor_version"`
}

// LLMReporter implements the Reporter interface for JSON output.
type LLMReporter struct {
	IgorVersion string
	Warnings    []LLMWarning
}

// NewLLMReporter creates a new instance of LLMReporter.
func NewLLMReporter(version string) Reporter {
	return &LLMReporter{
		IgorVersion: version,
		Warnings:    []LLMWarning{},
	}
}

// PrintHeader is a no-op for LLMReporter.
func (r *LLMReporter) PrintHeader(_ int) {}

// PrintProjectHeader is a no-op for LLMReporter.
func (r *LLMReporter) PrintProjectHeader() {}

// PrintVendorHeader is a no-op for LLMReporter.
func (r *LLMReporter) PrintVendorHeader() {}

// PrintFindings collects warnings to be exported later.
func (r *LLMReporter) PrintFindings(res symbol.AuditStatus, _ string, _ bool) {
	for _, f := range res.Findings {
		w := LLMWarning{
			FilePath:   res.FilePath,
			Line:       f.Line,
			Message:    f.Message,
			Severity:   f.Severity,
			Snippet:    f.Snippet,
			ASTDetails: f.ASTDetails,
			RuleID:     "STATE_MUTATION", // Default for now
			Context: LLMContext{
				Dependencies: LLMServiceContext{
					IsShared:  res.IsShared,
					IsPublic:  res.IsPublic,
					ServiceID: res.ServiceID,
				},
				InjectedLines: f.Dependencies,
			},
		}
		r.Warnings = append(r.Warnings, w)
	}
}

// PrintSummary outputs the accumulated warnings as a JSON object.
func (r *LLMReporter) PrintSummary(_ []symbol.AuditStatus, _ string) bool {
	output := LLMOutput{
		Warnings: r.Warnings,
		Metadata: LLMMetadata{
			GeneratedAt: time.Now(),
			IgorVersion: r.IgorVersion,
		},
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating LLM export: %v\n", err)
		return false
	}

	// For LLM output, generating the JSON is a success regardless of findings.
	// The triage is handled by the LLM in the next step.
	return true
}
