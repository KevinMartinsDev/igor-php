package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/igor-php/igor-php/internal/config"
)

func parseFlagsAndInit() (config.Config, string, bool) {
	binName = filepath.Base(os.Args[0])
	if strings.HasPrefix(binName, "main") || strings.HasPrefix(binName, "exe") {
		binName = "igor"
	}

	var configPath string
	versionFlag := flag.Bool("version", false, "Display version information")
	flag.StringVar(&configPath, "config", "", "Custom path to igor.json")
	flag.StringVar(&configPath, "c", "", "Custom path to igor.json (shorthand)")
	baselineFlag := flag.String("baseline", "", "Path to baseline file")
	generateBaselineFlag := flag.Bool("generate-baseline", false, "Generate a baseline file from current findings")
	checkBaselineFlag := flag.Bool("check-baseline", false, "Verify if the baseline is clean (fails if any baseline entries are no longer detected)")
	pruneBaselineFlag := flag.Bool("prune-baseline", false, "Remove stale entries from the baseline automatically")
	consoleFlag := flag.String("console", "", "Custom path to Symfony console (e.g. app/console)")
	envFlag := flag.String("env", "", "Symfony environment (default: dev)")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output to see skipped services and details")
	noAgentFlag := flag.Bool("no-agent", false, "Disable Igor Agent and fallback to standard scan")
	outputFlag := flag.String("output", "cli", "Output format (cli, llm, json)")
	containerDumpFlag := flag.String("container-dump", "", "Path to a generic container dump JSON ({\"services\":[{\"class\":...,\"shared\":bool}]}) used to skip transient (non-shared) classes")
	ignoreExternalBaselineFlag := flag.Bool("ignore-external-baseline", false, "Ignore baseline files defined in external vendor packages")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "🧟 Igor-PHP v%s - The faithful assistant for FrankenPHP Workers\n\n", Version)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [options] <directory>    Audit a project\n", binName)
		fmt.Fprintf(os.Stderr, "  %s init [options] [directory] Initialize a new igor.json config\n", binName)
		fmt.Fprintf(os.Stderr, "  %s review <json_file>       Review an audit JSON export with an LLM\n", binName)
		fmt.Fprintf(os.Stderr, "  %s debug-external-baseline [directory] List all discovered vendor baselines in the project\n\n", binName)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s .\n", binName)
		fmt.Fprintf(os.Stderr, "  %s --output json .\n", binName)
		fmt.Fprintf(os.Stderr, "  %s --container-dump igor-container.json .\n", binName)
		fmt.Fprintf(os.Stderr, "  %s --generate-baseline\n", binName)
		fmt.Fprintf(os.Stderr, "  %s -c custom-igor.json .\n", binName)
		fmt.Fprintf(os.Stderr, "  %s init\n", binName)
		fmt.Fprintf(os.Stderr, "  %s review igor-export.json\n", binName)
		fmt.Fprintf(os.Stderr, "  %s debug-external-baseline .\n", binName)
		fmt.Fprintf(os.Stderr, "  %s --env stage --verbose ./my-project\n", binName)
	}

	flag.Parse()

	if *versionFlag {
		fmt.Fprintf(os.Stderr, "%s version %s\n", binName, Version)
		return config.Config{}, "", true
	}

	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "init":
			handleInitSubcommand(args, configPath)
			return config.Config{}, "", true
		case "review":
			handleReviewSubcommand(args, configPath)
			return config.Config{}, "", true
		case "debug-external-baseline":
			handleDebugExternalBaselineSubcommand(args, configPath)
			return config.Config{}, "", true
		}
	}

	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}
	rootPath, _ := filepath.Abs(args[0])

	cfg := config.LoadConfig(rootPath, configPath)
	applyFlagOverrides(&cfg, consoleFlag, envFlag, verboseFlag, noAgentFlag, outputFlag, generateBaselineFlag, baselineFlag, containerDumpFlag, ignoreExternalBaselineFlag, checkBaselineFlag, pruneBaselineFlag)

	// Display summary of packages
	if len(cfg.ProdPackages) > 0 || len(cfg.DevPackages) > 0 {
		fmt.Fprintf(os.Stderr, "📦 Composer: %d production packages will be inspected, %d dev packages will be ignored.\n",
			len(cfg.ProdPackages), len(cfg.DevPackages))
		if !*verboseFlag && len(cfg.DevPackages) > 0 {
			fmt.Fprintln(os.Stderr, "   (Use --verbose to see which services are being skipped)")
		}
	}

	return cfg, rootPath, false
}

func applyFlagOverrides(cfg *config.Config, consoleFlag, envFlag *string, verboseFlag, noAgentFlag *bool, outputFlag *string, generateBaselineFlag *bool, baselineFlag, containerDumpFlag *string, ignoreExternalBaselineFlag *bool, checkBaselineFlag, pruneBaselineFlag *bool) {
	if *consoleFlag != "" {
		cfg.ConsolePath = *consoleFlag
	}
	if *containerDumpFlag != "" {
		cfg.ContainerDump = *containerDumpFlag
	}
	if *envFlag != "" {
		cfg.Env = *envFlag
	}
	if *verboseFlag {
		cfg.Verbose = true
	}
	if *noAgentFlag {
		cfg.NoAgent = true
	}
	if *outputFlag != "" {
		cfg.OutputFormat = *outputFlag
	}
	if *ignoreExternalBaselineFlag {
		cfg.IgnoreExternalBaseline = true
	}
	if *checkBaselineFlag {
		cfg.CheckBaseline = true
	}
	if *pruneBaselineFlag {
		cfg.PruneBaseline = true
	}
	switch {
	case *generateBaselineFlag:
		cfg.GenerateBaseline = true
		if *baselineFlag != "" {
			cfg.BaselinePath = *baselineFlag
		} else if cfg.BaselinePath == "" {
			cfg.BaselinePath = "igor-baseline.json"
		}
	case *baselineFlag != "":
		cfg.BaselinePath = *baselineFlag
	case cfg.BaselinePath == "" && (cfg.CheckBaseline || cfg.PruneBaseline):
		cfg.BaselinePath = "igor-baseline.json"
	}
}
