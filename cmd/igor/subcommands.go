package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/igor-php/igor-php/internal/auditor"
	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/reporter"
	"github.com/igor-php/igor-php/pkg/symbol"
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

func diagnoseFindings(findings []symbol.Finding) (bool, bool, bool, bool, []string) {
	hasStatic := false
	hasTerminator := false
	hasSuperglobal := false
	hasClosureLeak := false
	var reasons []string

	for _, f := range findings {
		msg := strings.ToLower(f.Message)
		snippet := f.Snippet
		line := f.Line

		switch {
		case strings.Contains(msg, "static"):
			hasStatic = true
			reasons = append(reasons, fmt.Sprintf("❌ [Static] %s (line %d)", f.Message, line))
		case strings.Contains(msg, "exit") || strings.Contains(msg, "die"):
			hasTerminator = true
			reasons = append(reasons, fmt.Sprintf("❌ [Terminator] %s (line %d)", f.Message, line))
		case strings.Contains(msg, "superglobal") || strings.Contains(msg, "superglobals"):
			hasSuperglobal = true
			reasons = append(reasons, fmt.Sprintf("❌ [Superglobal] %s (line %d)", f.Message, line))
		case strings.Contains(msg, "closure") || strings.Contains(msg, "memory leak"):
			hasClosureLeak = true
			reasons = append(reasons, fmt.Sprintf("❌ [Closure Leak] %s (line %d)", f.Message, line))
		default:
			reasons = append(reasons, fmt.Sprintf("❌ [Mutation] %s (line %d: `%s`)", f.Message, line, strings.TrimSpace(snippet)))
		}
	}
	return hasStatic, hasTerminator, hasSuperglobal, hasClosureLeak, reasons
}

func formatExplainRow(status symbol.AuditStatus, className string) (string, []string) {
	hasStatic, hasTerminator, hasSuperglobal, hasClosureLeak, reasons := diagnoseFindings(status.Findings)

	sharedStr := "NO"
	if status.IsShared {
		sharedStr = "YES"
	}

	staticStr := "NO"
	if hasStatic {
		staticStr = "YES"
	}

	termStr := "NO"
	if hasTerminator {
		termStr = "YES"
	}

	superStr := "NO"
	if hasSuperglobal {
		superStr = "YES"
	}

	leakStr := "NO"
	if hasClosureLeak {
		leakStr = "YES"
	}

	verdict := "✅ OK (Stateless)"
	if len(status.Findings) > 0 {
		if hasStatic || hasTerminator || hasSuperglobal || hasClosureLeak {
			verdict = "❌ KO (State Poison)"
		} else {
			verdict = "❌ KO (State Mutation)"
		}
	}

	paddedClass := className
	if len(paddedClass) > 39 {
		paddedClass = paddedClass[:36] + "..."
	} else {
		paddedClass += strings.Repeat(" ", 39-len(paddedClass))
	}

	paddedVerdict := verdict
	if len(paddedVerdict) > 23 {
		paddedVerdict = paddedVerdict[:20] + "..."
	} else {
		paddedVerdict += strings.Repeat(" ", 23-len(paddedVerdict))
	}

	row := fmt.Sprintf("| %s | %-6s | %-6s | %-5s | %-5s | %-5s | %s |", paddedClass, sharedStr, staticStr, termStr, superStr, leakStr, paddedVerdict)

	if len(reasons) == 0 {
		reasons = []string{
			"✅ No mutated static state, superglobals or exit/die statements detected.",
			"✅ Injected dependencies are read-only or semantically transient.",
		}
	}

	return row, reasons
}

func handleExplainSubcommand(args []string, configPath string) error {
	targetDir := "."
	if len(args) > 1 {
		targetDir = args[1]
	}
	rootPath, _ := filepath.Abs(targetDir)

	cfg := config.LoadConfig(rootPath, configPath)

	aud := auditor.NewAuditor(cfg)

	// Detect Symfony
	symfony, err := auditor.DetectSymfony(rootPath, cfg)
	if err == nil {
		aud.Symfony = symfony
	}

	fmt.Fprintf(os.Stderr, "🔍 Analyzing Igor's Semantics - Explain Matrix for: %s\n\n", rootPath)

	// Load Baseline if specified
	var baseline config.Baseline
	baseline.Files = make(map[string][]config.BaselineEntry)
	if cfg.BaselinePath != "" {
		baselineFile := cfg.BaselinePath
		if !filepath.IsAbs(baselineFile) {
			baselineFile = filepath.Join(rootPath, baselineFile)
		}
		if loaded, err := config.LoadBaseline(baselineFile); err == nil {
			baseline = loaded
			fmt.Fprintf(os.Stderr, "🛡️  Baseline loaded: %d files will be partially ignored in diagnostics.\n", len(baseline.Files))
		}
	}
	discoverAndMergeExternalBaselines(rootPath, &cfg, &baseline)

	auditList := collectFiles(rootPath, cfg, aud)
	if len(auditList) == 0 {
		fmt.Println("No services found to audit.")
		return nil
	}

	results := executeAudit(auditList, aud, cfg, baseline, rootPath)

	// Build the table
	fmt.Println("🔍 Igor Explanation Matrix - Services Audit Diagnoses")
	fmt.Println("=====================================================")
	fmt.Println("")

	// Table header
	fmt.Printf("+-----------------------------------------+--------+--------+-------+-------+-------+-------------------------+\n")
	fmt.Printf("| Service Class                           | Shared | Static | Term  | Super | Leak  | Verdict                 |\n")
	fmt.Printf("+-----------------------------------------+--------+--------+-------+-------+-------+-------------------------+\n")

	type diagnosisDetail struct {
		class   string
		reasons []string
	}
	var details []diagnosisDetail

	for _, status := range results {
		// Get class name
		className := status.ServiceID
		if className == "N/A" || className == "" {
			// fallback to filename
			className = filepath.Base(status.FilePath)
		}

		row, reasons := formatExplainRow(status, className)
		fmt.Println(row)

		details = append(details, diagnosisDetail{
			class:   className,
			reasons: reasons,
		})
	}

	fmt.Printf("+-----------------------------------------+--------+--------+-------+-------+-------+-------------------------+\n")
	fmt.Println("")
	fmt.Println("🔬 Detailed Criteria Diagnoses:")
	fmt.Println("--------------------------------")

	for _, d := range details {
		fmt.Printf("• %s:\n", d.class)
		for _, r := range d.reasons {
			fmt.Printf("  - %s\n", r)
		}
		fmt.Println("")
	}

	return nil
}
