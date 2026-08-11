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

func handleInitSubcommand(args []string, configPath string) error {
	targetDir := "."
	if len(args) > 1 {
		targetDir = args[1]
	}
	rootPath, _ := filepath.Abs(targetDir)
	detectedType, err := config.InitConfig(rootPath, configPath)
	if err != nil {
		return err
	}

	actualConfigPath := configPath
	if actualConfigPath == "" {
		actualConfigPath = filepath.Join(rootPath, "igor.json")
	}

	fmt.Fprintf(os.Stderr, "✨ Igor has successfully initialized your project!\n")
	fmt.Fprintf(os.Stderr, "📂 Detected project type: %s\n", detectedType)
	fmt.Fprintf(os.Stderr, "📝 Configuration saved to: %s\n", actualConfigPath)
	fmt.Fprintf(os.Stderr, "👉 You can now customize the configuration to fit your needs.\n")
	return nil
}

func handleReviewSubcommand(args []string, configPath string) error {
	if len(args) < 2 {
		return fmt.Errorf("missing JSON file to review.\nUsage: %s review <json_file>", binName)
	}
	jsonFile := args[1]
	content, err := os.ReadFile(jsonFile)
	if err != nil {
		return fmt.Errorf("could not read file %s: %w", jsonFile, err)
	}

	fmt.Fprintf(os.Stderr, "🧟 Igor is preparing to review %s...\n", jsonFile)

	rootPath, _ := filepath.Abs(".")
	cfg := config.LoadConfig(rootPath, configPath)

	if cfg.LLMConfig.Provider == "gemini" {
		return handleGeminiReview(string(content), cfg)
	}

	if cfg.LLMConfig.Provider == "ollama" || (cfg.LLMConfig.Provider == "openai" && cfg.LLMConfig.APIUrl != "") {
		return handleAPIReview(string(content), cfg)
	}

	err = reporter.GenerateFrictionlessPrompt(string(content))
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "📄 Prompt ready! Copy the content of igor-review-prompt.md to your favorite LLM.")
	return nil
}

func handleGeminiReview(content string, cfg config.Config) error {
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
		return fmt.Errorf("Gemini CLI failed: %w", err)
	}

	err = os.WriteFile("igor-review.md", out.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("error writing review file: %w", err)
	}

	fmt.Fprintln(os.Stderr, "✨ Gemini Review complete! Results saved to igor-review.md")
	return nil
}

func handleAPIReview(content string, cfg config.Config) error {
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
			return nil
		}
	}

	fmt.Fprintf(os.Stderr, "🧠 %s: Sending audit to LLM (%s)...\n", modeName, cfg.LLMConfig.Model)
	client := reporter.NewLLMClient(apiURL, apiKey, cfg.LLMConfig.Model)

	prompt := fmt.Sprintf(reporter.FrictionlessPromptTemplate, content)
	review, err := client.Review(prompt)
	if err != nil {
		return fmt.Errorf("%s failed: %w", modeName, err)
	}

	err = os.WriteFile("igor-review.md", []byte(review), 0644)
	if err != nil {
		return fmt.Errorf("error writing review file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✨ %s Review complete! Results saved to igor-review.md\n", modeName)
	return nil
}

func handleDebugExternalBaselineSubcommand(args []string, configPath string) error {
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
		return nil
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
	return nil
}
