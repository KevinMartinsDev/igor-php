package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/reporter"
)

func handleInitSubcommand(args []string, configPath string) {
	targetDir := "."
	if len(args) > 1 {
		targetDir = args[1]
	}
	rootPath, _ := filepath.Abs(targetDir)
	detectedType, err := config.InitConfig(rootPath, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}

	actualConfigPath := configPath
	if actualConfigPath == "" {
		actualConfigPath = filepath.Join(rootPath, "igor.json")
	}

	fmt.Fprintf(os.Stderr, "✨ Igor has successfully initialized your project!\n")
	fmt.Fprintf(os.Stderr, "📂 Detected project type: %s\n", detectedType)
	fmt.Fprintf(os.Stderr, "📝 Configuration saved to: %s\n", actualConfigPath)
	fmt.Fprintf(os.Stderr, "👉 You can now customize the configuration to fit your needs.\n")
}

func handleReviewSubcommand(args []string, configPath string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "❌ Error: missing JSON file to review.")
		fmt.Fprintf(os.Stderr, "Usage: %s review <json_file>\n", binName)
		os.Exit(1)
	}
	jsonFile := args[1]
	content, err := os.ReadFile(jsonFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: could not read file %s: %v\n", jsonFile, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "🧟 Igor is preparing to review %s...\n", jsonFile)

	rootPath, _ := filepath.Abs(".")
	cfg := config.LoadConfig(rootPath, configPath)

	if cfg.LLMConfig.Provider == "gemini" {
		handleGeminiReview(string(content), cfg)
		os.Exit(0)
	}

	if cfg.LLMConfig.Provider == "ollama" || (cfg.LLMConfig.Provider == "openai" && cfg.LLMConfig.APIUrl != "") {
		handleAPIReview(string(content), cfg)
		os.Exit(0)
	}

	err = reporter.GenerateFrictionlessPrompt(string(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "📄 Prompt ready! Copy the content of igor-review-prompt.md to your favorite LLM.")
	os.Exit(0)
}

func handleGeminiReview(content string, cfg config.Config) {
	fmt.Fprintln(os.Stderr, "🤖 Gemini CLI Mode: Sending audit to Gemini...")
	prompt := fmt.Sprintf(reporter.FrictionlessPromptTemplate, content)

	args := []string{"-p", prompt, "--skip-trust"}
	if cfg.LLMConfig.Model != "" {
		args = append(args, "-m", cfg.LLMConfig.Model)
	}

	cmd := exec.Command("gemini", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Gemini CLI failed: %v\n", err)
		os.Exit(1)
	}

	err = os.WriteFile("igor-review.md", out.Bytes(), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error writing review file: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "✨ Gemini Review complete! Results saved to igor-review.md")
}

func handleAPIReview(content string, cfg config.Config) {
	apiURL := cfg.LLMConfig.APIUrl
	apiKey := ""
	modeName := "Expert Mode"

	if cfg.LLMConfig.Provider == "ollama" {
		modeName = "Ollama Mode"
		if apiURL == "" {
			apiURL = "http://localhost:11434/v1"
		}
		apiKey = "ollama"
	} else {
		apiKey = os.Getenv(cfg.LLMConfig.APIKeyEnv)
		if apiKey == "" {
			fmt.Fprintf(os.Stderr, "⚠️  Expert Mode enabled but ENV %s is empty. Falling back to Frictionless Mode.\n", cfg.LLMConfig.APIKeyEnv)
			return
		}
	}

	fmt.Fprintf(os.Stderr, "🧠 %s: Sending audit to LLM (%s)...\n", modeName, cfg.LLMConfig.Model)
	client := reporter.NewLLMClient(apiURL, apiKey, cfg.LLMConfig.Model)

	prompt := fmt.Sprintf(reporter.FrictionlessPromptTemplate, content)
	review, err := client.Review(prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %s failed: %v\n", modeName, err)
		os.Exit(1)
	}

	err = os.WriteFile("igor-review.md", []byte(review), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error writing review file: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "✨ %s Review complete! Results saved to igor-review.md\n", modeName)
	os.Exit(0)
}

func handleDebugExternalBaselineSubcommand(args []string, configPath string) {
	targetDir := "."
	if len(args) > 1 {
		targetDir = args[1]
	}
	rootPath, _ := filepath.Abs(targetDir)

	cfg := config.LoadConfig(rootPath, configPath)
	var baseline config.Baseline
	baseline.Files = make(map[string][]config.BaselineEntry)

	fmt.Fprintf(os.Stderr, "🔍 Debugging external baselines for project at: %s\n\n", rootPath)

	discoverAndMergeExternalBaselines(rootPath, &cfg, &baseline)

	fmt.Println("\n📋 Summary of loaded baseline files:")
	if len(baseline.Files) == 0 {
		fmt.Println("   No baseline files or entries found.")
		return
	}

	for relPath, entries := range baseline.Files {
		fmt.Printf("   - %s (%d rules ignored)\n", relPath, len(entries))
		for _, entry := range entries {
			fmt.Printf("       • %s\n", entry.Message)
			if entry.Reason != "" {
				fmt.Printf("           ◦ Reason: %s\n", entry.Reason)
			}
		}
	}
}
