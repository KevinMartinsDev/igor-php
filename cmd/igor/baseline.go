package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func loadAuditBaseline(rootPath string, cfg *config.Config) config.Baseline {
	if cfg.GenerateBaseline {
		return config.Baseline{}
	}

	var baseline config.Baseline
	baseline.Files = make(map[string][]config.BaselineEntry)

	if cfg.BaselinePath != "" {
		baselineFile := cfg.BaselinePath
		if !filepath.IsAbs(baselineFile) {
			baselineFile = filepath.Join(rootPath, baselineFile)
		}

		loaded, err := config.LoadBaseline(baselineFile)
		if err == nil {
			baseline = loaded
			fmt.Fprintf(os.Stderr, "🛡️  Baseline loaded: %d files will be partially ignored.\n", len(baseline.Files))
		} else {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: Could not load baseline from %s: %v\n", baselineFile, err)
		}
	}

	discoverAndMergeExternalBaselines(rootPath, cfg, &baseline)

	return baseline
}

func discoverAndMergeExternalBaselines(rootPath string, cfg *config.Config, baseline *config.Baseline) {
	vendorDir := filepath.Join(rootPath, "vendor")
	vendors, err := os.ReadDir(vendorDir)
	if err != nil {
		return
	}

	if cfg.SymlinkMap == nil {
		cfg.SymlinkMap = make(map[string]string)
	}

	externalCount := 0
	for _, vDir := range vendors {
		isDir := vDir.IsDir()
		if !isDir && (vDir.Type()&os.ModeSymlink != 0) {
			path := filepath.Join(vendorDir, vDir.Name())
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				isDir = true
			}
		}
		if !isDir {
			continue
		}
		vName := vDir.Name()
		if vName == "bin" || vName == "composer" || strings.HasPrefix(vName, ".") {
			continue
		}

		externalCount += discoverVendorPackages(vendorDir, vName, cfg, baseline)
	}
	if externalCount > 0 {
		fmt.Fprintf(os.Stderr, "🛡️  Loaded %d external baseline paths from vendor dependencies.\n", externalCount)
	}
}

func discoverVendorPackages(vendorDir, vName string, cfg *config.Config, baseline *config.Baseline) int {
	packagesDir := filepath.Join(vendorDir, vName)
	pkgs, err := os.ReadDir(packagesDir)
	if err != nil {
		return 0
	}

	count := 0
	for _, pDir := range pkgs {
		isPkgDir := pDir.IsDir()
		isSymlink := pDir.Type()&os.ModeSymlink != 0
		if !isPkgDir && isSymlink {
			path := filepath.Join(packagesDir, pDir.Name())
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				isPkgDir = true
			}
		}
		if !isPkgDir {
			continue
		}
		pName := pDir.Name()
		packagePath := filepath.Join(packagesDir, pName)

		if isSymlink {
			realPath, err := filepath.EvalSymlinks(packagePath)
			if err == nil {
				cfg.SymlinkMap[realPath] = filepath.Join("vendor", vName, pName)
			}
		}

		if !cfg.IgnoreExternalBaseline && !cfg.CheckBaseline && !cfg.PruneBaseline {
			count += loadAndMergePackageBaseline(packagePath, vName, pName, isSymlink, *cfg, baseline)
		}
	}
	return count
}

func loadAndMergePackageBaseline(packagePath, vName, pName string, isSymlink bool, cfg config.Config, baseline *config.Baseline) int {
	configCandidates := []string{
		"igor.json",
		filepath.Join("config", "ci", "igor.json"),
		filepath.Join("config", "igor.json"),
		filepath.Join(".github", "igor.json"),
	}

	var configPath string
	for _, c := range configCandidates {
		p := filepath.Join(packagePath, c)
		if _, err := os.Stat(p); err == nil {
			configPath = p
			break
		}
	}

	var baselinePath string

	if configPath != "" {
		pkgCfg := config.LoadConfig(packagePath, configPath)
		if pkgCfg.BaselinePath != "" {
			baselinePath = pkgCfg.BaselinePath
			if !isAbsPath(baselinePath) {
				// Try relative to the config file's directory first
				relToConfigDir := filepath.Join(filepath.Dir(configPath), baselinePath)
				if _, err := os.Stat(relToConfigDir); err == nil {
					baselinePath = relToConfigDir
				} else {
					// Fallback to relative to the package root directory
					baselinePath = filepath.Join(packagePath, baselinePath)
				}
			} else {
				// If the absolute path doesn't exist, try to resolve it as a suffix under packagePath
				if _, err := os.Stat(baselinePath); err != nil {
					resolved := resolveAbsPathUnderPackage(packagePath, baselinePath)
					if resolved != "" {
						baselinePath = resolved
					}
				}
			}
		}
	}

	if baselinePath == "" {
		baselineCandidates := []string{
			"igor-baseline.json",
			filepath.Join("config", "ci", "igor-baseline.json"),
			filepath.Join("config", "igor-baseline.json"),
			filepath.Join(".github", "igor-baseline.json"),
		}
		for _, b := range baselineCandidates {
			p := filepath.Join(packagePath, b)
			if _, err := os.Stat(p); err == nil {
				baselinePath = p
				break
			}
		}
	}

	if baselinePath != "" {
		externalBaseline, err := config.LoadBaseline(baselinePath)
		if err == nil {
			linkType := "regular"
			if isSymlink {
				linkType = "symlinked"
			}
			fmt.Fprintf(os.Stderr, "🛡️  Found %s external baseline for package %s/%s at: %s (%d files ignored)\n", linkType, vName, pName, baselinePath, len(externalBaseline.Files))

			if baseline.Files == nil {
				baseline.Files = make(map[string][]config.BaselineEntry)
			}
			count := 0
			for relVendorPath, entries := range externalBaseline.Files {
				parentRelPath := filepath.Join("vendor", vName, pName, relVendorPath)
				baseline.Files[parentRelPath] = entries
				count++
			}
			return count
		} else if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: Could not load external baseline from %s: %v\n", baselinePath, err)
		}
	}
	return 0
}

func resolveAbsPathUnderPackage(packagePath, absPath string) string {
	cleaned := filepath.Clean(absPath)
	// Split by path separator, using filepath.ToSlash to handle all OS paths consistently
	parts := strings.Split(filepath.ToSlash(cleaned), "/")

	var elements []string
	for _, p := range parts {
		if p != "" {
			elements = append(elements, p)
		}
	}

	for i := 0; i < len(elements); i++ {
		suffixPath := filepath.Join(elements[i:]...)
		fullPath := filepath.Join(packagePath, suffixPath)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}
	return ""
}

func isAbsPath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return true
	}
	// Check for Windows drive letters (e.g. C:\ or c:/)
	if len(path) >= 2 && path[1] == ':' && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) {
		return true
	}
	return false
}

func generateBaselineFile(rootPath string, cfg config.Config, results []symbol.AuditStatus) {
	baselineFile := cfg.BaselinePath
	if !filepath.IsAbs(baselineFile) {
		baselineFile = filepath.Join(rootPath, baselineFile)
	}
	err := config.SaveBaseline(baselineFile, results, rootPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error saving baseline: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "\n✨ Baseline successfully generated at: %s\n", baselineFile)
	fmt.Fprintln(os.Stderr, "👉 Future audits will ignore these existing findings.")
}
