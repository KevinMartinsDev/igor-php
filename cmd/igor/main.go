package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/igor-php/igor-php/internal/auditor"
	"github.com/igor-php/igor-php/internal/config"
)

var Version = "dev"
var binName = "igor"

func main() {
	cfg, rootPath, shouldExit, err := parseFlagsAndInit(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	if shouldExit {
		return
	}

	// 1. Initialize Components
	aud := auditor.NewAuditor(cfg)
	rep := setupReporter(cfg)

	// 1b. Load generic container dump (non-shared/transient classes to skip)
	aud.NonSharedServices = loadContainerDump(rootPath, cfg)

	// 2. Detect Symfony project
	symfony, err := detectSymfonyProject(rootPath, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	aud.Symfony = symfony

	// 3. Load Baseline
	baseline := loadAuditBaseline(rootPath, &cfg)

	// 4. Collect Files to Audit
	auditList := collectFiles(rootPath, cfg, aud)

	// 5. Silence header for machine-readable formats
	if cfg.OutputFormat != "llm" && cfg.OutputFormat != "json" {
		rep.PrintHeader(len(auditList))
	}

	// 6. Run Audit
	results := executeAudit(auditList, aud, cfg, baseline, rootPath)

	// 7a. Handle Baseline Checking
	if cfg.CheckBaseline {
		staleEntries := config.IdentifyStaleEntries(baseline, results, rootPath)
		if len(staleEntries) > 0 {
			fmt.Fprintf(os.Stderr, "❌ Baseline check failed: %d stale entries detected in baseline (%s):\n", len(staleEntries), cfg.BaselinePath)
			for _, entry := range staleEntries {
				fmt.Fprintf(os.Stderr, "  - %s: %s\n", entry.FilePath, entry.Message)
			}
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "✨ Baseline check passed! The baseline (%s) is perfectly clean and up to date.\n", cfg.BaselinePath)
		return
	}

	// 7b. Handle Baseline Pruning
	if cfg.PruneBaseline {
		staleEntries := config.IdentifyStaleEntries(baseline, results, rootPath)
		if len(staleEntries) == 0 {
			fmt.Fprintf(os.Stderr, "✨ No stale entries found. The baseline (%s) is already up to date.\n", cfg.BaselinePath)
			return
		}

		fmt.Fprintf(os.Stderr, "⚙️  Pruning %d stale entries from baseline (%s)...\n", len(staleEntries), cfg.BaselinePath)
		prunedBaseline := config.PruneBaseline(baseline, staleEntries)

		baselineFile := cfg.BaselinePath
		if !filepath.IsAbs(baselineFile) {
			baselineFile = filepath.Join(rootPath, baselineFile)
		}

		err := config.WriteBaseline(baselineFile, prunedBaseline)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error saving pruned baseline: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "✨ Baseline successfully pruned and saved to: %s\n", baselineFile)
		return
	}

	// 7. Handle Baseline Generation
	if cfg.GenerateBaseline {
		err := generateBaselineFile(rootPath, cfg, results)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 8. Report Results
	reportAllFindings(rep, results, rootPath)

	if !rep.PrintSummary(results, rootPath) {
		os.Exit(1)
	}
}
