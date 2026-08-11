package reporter

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateFrictionlessPrompt(t *testing.T) {
	// Cleanup any pre-existing file
	const filename = "igor-review-prompt.md"
	_ = os.Remove(filename)
	defer func() {
		_ = os.Remove(filename)
	}()

	payload := `{"test_payload": "hello"}`
	err := GenerateFrictionlessPrompt(payload)
	if err != nil {
		t.Fatalf("Expected no error from GenerateFrictionlessPrompt, got: %v", err)
	}

	// Verify file exists and has correct content
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Expected file %s to exist, got error: %v", filename, err)
	}

	content := string(data)
	if !strings.Contains(content, "JSON PAYLOAD:") {
		t.Error("Expected prompt template content not found")
	}
	if !strings.Contains(content, payload) {
		t.Error("Expected payload to be embedded in the prompt file")
	}
}
