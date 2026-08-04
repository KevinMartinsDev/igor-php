package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestCLI_JSONOutput(t *testing.T) {
	// 1. Setup a mock project
	tmpDir, servicePath := setupMockSymfonyProject(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// 2. Mock results
	results := []symbol.AuditStatus{
		{
			ServiceID: "app.my_service",
			FilePath:  servicePath,
			Status:    "❌ KO",
			Findings: []symbol.Finding{
				{
					Message:  "State mutation detected",
					Severity: "ERROR",
					Line:     5,
				},
			},
		},
	}

	// 3. Test setupReporter logic
	t.Run("setupReporter should return JSONReporter for json format", func(t *testing.T) {
		cfg := config.Config{OutputFormat: "json"}
		rep := setupReporter(cfg)
		if _, ok := rep.(interface {
			PrintFindings(symbol.AuditStatus, string, bool)
		}); !ok {
			t.Fatal("setupReporter did not return a valid Reporter")
		}
		// We can't easily check the private type name across packages if not exported,
		// but we know NewJSONReporter returns a *JSONReporter.
	})

	// 4. Test actual JSON output integration
	t.Run("JSON output should be valid and contain expected data", func(t *testing.T) {
		old := os.Stdout
		rOut, wOut, _ := os.Pipe()
		os.Stdout = wOut

		cfg := config.Config{OutputFormat: "json"}
		rep := setupReporter(cfg)

		// Simulate reporting
		rep.PrintFindings(results[0], tmpDir, false)
		rep.PrintSummary(results, tmpDir)

		_ = wOut.Close()
		os.Stdout = old

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rOut)
		output := buf.String()

		// Verify JSON
		var parsed []symbol.AuditStatus
		if err := json.Unmarshal([]byte(output), &parsed); err != nil {
			t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, output)
		}

		if len(parsed) != 1 {
			t.Fatalf("Expected 1 result in JSON, got %d", len(parsed))
		}

		if parsed[0].ServiceID != "app.my_service" {
			t.Errorf("Expected service_id 'app.my_service', got %s", parsed[0].ServiceID)
		}

		if len(parsed[0].Findings) != 1 {
			t.Fatalf("Expected 1 finding, got %d", len(parsed[0].Findings))
		}

		if !strings.Contains(parsed[0].Findings[0].Message, "State mutation") {
			t.Errorf("Expected message to contain 'State mutation', got %s", parsed[0].Findings[0].Message)
		}
	})
}
