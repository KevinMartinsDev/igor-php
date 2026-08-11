package reporter

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestLLMOutputSerialization(t *testing.T) {
	now := time.Now()
	output := LLMOutput{
		Warnings: []LLMWarning{
			{
				FilePath:   "src/Service/StatefulService.php",
				Line:       15,
				Message:    "Mutation of property $counter detected",
				Severity:   "WARNING",
				Snippet:    "$this->counter++;",
				ASTDetails: "(assignment_expression (member_access_expression ...))",
				RuleID:     "MUTATION_DETECTED",
				Context: LLMContext{
					Dependencies: LLMServiceContext{
						IsShared:  true,
						IsPublic:  false,
						ServiceID: "app.stateful_service",
					},
					InjectedLines: []string{"LoggerInterface", "EntityManagerInterface"},
				},
			},
		},
		Metadata: LLMMetadata{
			GeneratedAt: now,
			IgorVersion: "dev",
		},
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal LLMOutput: %v", err)
	}

	var decoded LLMOutput
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal LLMOutput: %v", err)
	}

	if len(decoded.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(decoded.Warnings))
	}

	if decoded.Warnings[0].Context.Dependencies.IsShared != true {
		t.Error("Expected IsShared to be true")
	}

	if decoded.Warnings[0].Context.InjectedLines[0] != "LoggerInterface" {
		t.Errorf("Expected dependency 'LoggerInterface', got '%s'", decoded.Warnings[0].Context.InjectedLines[0])
	}
}

func TestLLMReporter_HTMLEscape(t *testing.T) {
	rep := NewLLMReporter("dev")
	status := symbol.AuditStatus{
		FilePath: "test.php",
		Findings: []symbol.Finding{
			{
				Line:    1,
				Message: "Mutation",
				Code:    "$this->prop",
				Snippet: "$this->prop",
			},
		},
	}
	rep.PrintFindings(status, "/root", false)
	rep.PrintHeader(0)
	rep.PrintProjectHeader()
	rep.PrintVendorHeader()

	// Capture stdout
	old := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	rep.PrintSummary([]symbol.AuditStatus{status}, "/root")

	_ = wOut.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, rOut)
	output := buf.String()

	if strings.Contains(output, "\\u003e") {
		t.Error("Output should not contain escaped HTML entities (e.g., \\u003e for >)")
	}
	if !strings.Contains(output, "->") {
		t.Error("Output should contain the literal '->' symbol")
	}
}
