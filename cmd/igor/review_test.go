package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/igor-php/igor-php/pkg/reporter"
)

func TestReviewCommand_InvalidFile(t *testing.T) {
	// Run the review command with a non-existent file
	cmd := exec.Command("go", "run", ".", "review", "non_existent_file.json")
	output, err := cmd.CombinedOutput()

	// It should fail
	if err == nil {
		t.Error("Expected review command to fail with non-existent file, but it succeeded")
	}

	outStr := strings.ToLower(string(output))
	expectedError := "error: file not found" // Adjust based on planned implementation
	if !strings.Contains(outStr, expectedError) && 
		!strings.Contains(outStr, "no such file") && 
		!strings.Contains(outStr, "cannot find the file") &&
		!strings.Contains(outStr, "does not exist") {
		t.Errorf("Expected error message about missing file, got: %s", string(output))
	}
}

func TestReviewCommand_ExpertMode(t *testing.T) {
	// 1. Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := reporter.ChatCompletionResponse{
			Choices: []struct {
				Message reporter.Message `json:"message"`
			}{
				{Message: reporter.Message{Role: "assistant", Content: "Expert review content."}},
			},
		}
		err := json.NewEncoder(w).Encode(resp)
		if err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// 2. Create config with LLM settings
	configContent := fmt.Sprintf(`{
		"llm": {
			"api_url": "%s",
			"api_key_env": "IGOR_TEST_KEY",
			"model": "test-model"
		}
	}`, server.URL)
	if err := os.WriteFile("igor.json", []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Remove("igor.json")
	}()
	err := os.Setenv("IGOR_TEST_KEY", "test-key")
	if err != nil {
		t.Fatalf("Failed to set env: %v", err)
	}

	// 3. Create a dummy JSON export
	jsonFile := "test-export-expert.json"
	err = os.WriteFile(jsonFile, []byte(`{"warnings": []}`), 0644)
	if err != nil {
		t.Fatalf("Failed to write json file: %v", err)
	}
	defer func() {
		_ = os.Remove(jsonFile)
	}()
	defer func() {
		_ = os.Remove("igor-review.md")
	}()

	// 4. Run the review command
	cmd := exec.Command("go", "run", ".", "review", jsonFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Review command failed: %v\nOutput: %s", err, string(output))
	}

	// 5. Verify terminal output
	outStr := string(output)
	if !strings.Contains(outStr, "🧠 Expert Mode: Sending audit to LLM") {
		t.Errorf("Expected Expert Mode message in output, got: %s", outStr)
	}

	// 6. Verify generated file
	reviewContent, err := os.ReadFile("igor-review.md")
	if err != nil {
		t.Fatal("Expected igor-review.md to be created")
	}
	if string(reviewContent) != "Expert review content." {
		t.Errorf("Expected 'Expert review content.', got '%s'", string(reviewContent))
	}
}
