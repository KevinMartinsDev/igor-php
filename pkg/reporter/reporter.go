package reporter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/igor-php/igor-php/pkg/symbol"
)

// Reporter handles the output formatting of audit results.
type Reporter interface {
	PrintHeader(count int)
	PrintProjectHeader()
	PrintVendorHeader()
	PrintFindings(res symbol.AuditStatus, projectRoot string, isVendor bool)
	PrintSummary(results []symbol.AuditStatus, projectRoot string) bool
}

// JSONReporter implements Reporter for standard JSON output.
type JSONReporter struct {
	StartTime time.Time
	Findings  []symbol.AuditStatus
}

// NewJSONReporter creates a new JSON reporter.
func NewJSONReporter() Reporter {
	return &JSONReporter{
		StartTime: time.Now(),
		Findings:  []symbol.AuditStatus{},
	}
}

func (r *JSONReporter) PrintHeader(_ int)   {}
func (r *JSONReporter) PrintProjectHeader() {}
func (r *JSONReporter) PrintVendorHeader()  {}

func (r *JSONReporter) PrintFindings(res symbol.AuditStatus, _ string, _ bool) {
	if len(res.Findings) > 0 {
		r.Findings = append(r.Findings, res)
	}
}

func (r *JSONReporter) PrintSummary(_ []symbol.AuditStatus, _ string) bool {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(r.Findings); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating JSON export: %v\n", err)
		return false
	}
	return true
}

// CLIReporter implements Reporter for terminal output.
type CLIReporter struct {
	StartTime time.Time
	IsGitHub  bool
}

// NewReporter creates a new CLI reporter.
func NewReporter() Reporter {
	return &CLIReporter{
		StartTime: time.Now(),
		IsGitHub:  os.Getenv("GITHUB_ACTIONS") == "true",
	}
}

// PrintHeader prints the initial message.
func (r *CLIReporter) PrintHeader(count int) {
	fmt.Printf("🧟 Igor is auditing %d unique shared service files for you, Master...\n\n", count)
}

// PrintProjectHeader prints the project services section header.
func (r *CLIReporter) PrintProjectHeader() {
	fmt.Println("\n\033[34m--- 📂 PROJECT SERVICES ---\033[0m")
}

// PrintVendorHeader prints the vendor services section header.
func (r *CLIReporter) PrintVendorHeader() {
	fmt.Println("\n\033[33m--- 📦 VENDOR SERVICES (THIRD-PARTY) ---\033[0m")
}

// PrintFindings displays detailed issues for a given service.
func (r *CLIReporter) PrintFindings(res symbol.AuditStatus, projectRoot string, isVendor bool) {
	if len(res.Findings) == 0 {
		return
	}

	displayPath := res.FilePath
	// Use relative path for cleaner output if possible
	relPath := res.FilePath
	if rel, found := strings.CutPrefix(res.FilePath, projectRoot); found && rel != "" {
		relPath = strings.TrimPrefix(rel, "/")
		displayPath = relPath
	} else {
		// Fallback: try to get relative path from current working directory
		if cwd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(cwd, res.FilePath); err == nil {
				relPath = rel
			}
		}
	}

	fmt.Printf("\n📂 \033[1m%s\033[0m\n", displayPath)
	if res.ServiceID != "N/A" {
		fmt.Printf("   \033[90mService: %s\033[0m\n", res.ServiceID)
	}

	for _, f := range res.Findings {
		severity := "error"
		color := "\033[31m" // Red for Error
		if f.Severity == "WARNING" {
			severity = "warning"
			color = "\033[33m" // Yellow for Warning
		}

		// Source indicator
		sourceIndicator := "\033[34m[PROJECT]\033[0m"
		if isVendor {
			sourceIndicator = "\033[33m[VENDOR]\033[0m"
		}

		// Standard CLI output
		fmt.Printf("  %s %s%s\033[0m\n", sourceIndicator, color, f.Message)
		fmt.Printf("  \033[90m%d | %s\033[0m\n", f.Line, strings.TrimSpace(f.Code))

		if f.Remediation != "" {
			fmt.Printf("  %s💡 Hint: %s\033[0m\n", "\033[36m", f.Remediation)
		}

		// GitHub Action Annotation (Keeps all hints)
		if r.IsGitHub {
			msg := fmt.Sprintf("[Igor] %s", f.Message)
			if f.Remediation != "" {
				msg += fmt.Sprintf(" %%0A 💡 Hint: %s", f.Remediation)
			}
			if isVendor {
				msg += " %0A 💡 Hint: This is third-party code. If you can't fix it, consider setting a 'max_requests' limit in your Worker configuration to mitigate memory leaks. Determine whether the issue is simply a memory leak or the result of a critical data exchange between two requests."
			} else {
				msg += " %0A 💡 Hint: Since this is your code, you should refactor this service to be stateless or implement ResetInterface to clear the state between requests."
			}
			// Format: ::error file={name},line={line},col={col}::{message}
			fmt.Printf("::%s file=%s,line=%d::%s\n", severity, relPath, f.Line, msg)
		}
	}
}

// PrintSummary displays the final audit statistics.
func (r *CLIReporter) PrintSummary(results []symbol.AuditStatus, projectRoot string) bool {
	totalOK := 0
	projKO, projWarn := 0, 0
	vendKO, vendWarn := 0, 0

	for _, res := range results {
		isVendor := res.IsVendor(projectRoot)
		switch res.Status {
		case "✅ OK":
			totalOK++
		case "❌ KO":
			if isVendor {
				vendKO++
			} else {
				projKO++
			}
		case "⚠️  WARN":
			if isVendor {
				vendWarn++
			} else {
				projWarn++
			}
		}
	}

	totalKO := projKO + vendKO
	totalWarn := projWarn + vendWarn

	fmt.Printf("\n--- 🏁 DEEP AUDIT COMPLETE ---")
	fmt.Printf("\nTotal unique service files: %d", totalOK+totalKO+totalWarn)
	fmt.Printf("\n✅ OK (Stateless):           %d", totalOK)
	fmt.Printf("\n❌ KO (Dangerous State):     %d (Project: %d, Vendor: %d)", totalKO, projKO, vendKO)
	fmt.Printf("\n⚠️  WARN (Review reset):      %d (Project: %d, Vendor: %d)", totalWarn, projWarn, vendWarn)
	fmt.Printf("\nTime taken: %v\n", time.Since(r.StartTime))

	// Detailed Recommendations
	if totalKO > 0 || totalWarn > 0 {
		fmt.Println("\n\033[1m💡 RECOMMENDATIONS:\033[0m")

		if projKO > 0 || projWarn > 0 {
			fmt.Println("  \033[34m[PROJECT]\033[0m Since this is your code, you should refactor these services to be stateless")
			fmt.Println("            or implement ResetInterface to clear the state between requests.")
		}

		if vendKO > 0 || vendWarn > 0 {
			fmt.Println("  \033[33m[VENDOR]\033[0m  This is third-party code. If you can't fix it, consider setting a 'max_requests' limit")
			fmt.Println("            in your Worker configuration (e.g. FrankenPHP) to mitigate memory leaks.")
			fmt.Println("            Determine if the issue is a simple leak or a critical data exchange between requests.")
		}
	}

	if totalKO > 0 {
		fmt.Println("\n\033[31m⚠️  DANGER: Your application or its vendors contain services with shared state.")
		fmt.Println("These services will leak data between requests in Worker Mode.\033[0m")
		return false
	}

	if totalWarn > 0 {
		fmt.Println("\n\033[33m⚠️  CAUTION: Your application contains possible state mutations (Warnings).")
		fmt.Println("Please review these services to ensure they are compatible with Worker Mode.\033[0m")
		return true
	}

	fmt.Println("\n\033[32m✨ CONGRATULATIONS: Your application and all its dependencies are compatible with Worker Mode!\033[0m")
	return true
}
