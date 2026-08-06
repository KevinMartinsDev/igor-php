package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/igor-php/igor-php/pkg/symbol"
)

// LoadBaseline loads the baseline configuration from a file.
func LoadBaseline(path string) (Baseline, error) {
	var b Baseline
	data, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	err = json.Unmarshal(data, &b)
	return b, err
}

// SaveBaseline generates a baseline file from the audit results.
func SaveBaseline(path string, results []symbol.AuditStatus, rootPath string) error {
	b := Baseline{
		Files: make(map[string][]BaselineEntry),
	}

	for _, res := range results {
		if len(res.Findings) == 0 {
			continue
		}

		relPath, err := filepath.Rel(rootPath, res.FilePath)
		if err != nil {
			relPath = res.FilePath
		}

		entries := []BaselineEntry{}
		for _, f := range res.Findings {
			entries = append(entries, BaselineEntry{
				Message: f.Message,
				Reason:  "TODO: Explain why this state mutation is a false positive or safe",
			})
		}
		b.Files[relPath] = entries
	}

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// WriteBaseline writes a Baseline struct to a file.
func WriteBaseline(path string, b Baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// FilterFindings removes findings that are present in the baseline.
func FilterFindings(baseline Baseline, filePath string, findings []symbol.Finding, rootPath string) []symbol.Finding {
	if baseline.Files == nil {
		return findings
	}

	relPath, err := filepath.Rel(rootPath, filePath)
	if err != nil {
		relPath = filePath
	}

	ignoredEntries, found := baseline.Files[relPath]
	if !found {
		return findings
	}

	filtered := []symbol.Finding{}
	for _, f := range findings {
		isIgnored := false
		for _, entry := range ignoredEntries {
			if entry.Message == f.Message {
				isIgnored = true
				break
			}
		}
		if !isIgnored {
			filtered = append(filtered, f)
		}
	}

	return filtered
}

// StaleBaselineEntry represents a stale entry in the baseline.
type StaleBaselineEntry struct {
	FilePath string
	Message  string
}

// IdentifyStaleEntries compares the actual raw findings with the baseline to find entries that are no longer detected.
// To avoid false positives when only scanning a subset of files, we only check files that were actually scanned.
func IdentifyStaleEntries(baseline Baseline, results []symbol.AuditStatus, rootPath string) []StaleBaselineEntry {
	var stale []StaleBaselineEntry

	// Set of files that were actually scanned during this audit
	scannedFiles := make(map[string]bool)
	// Map of active findings: filePath -> message -> true
	activeFindings := make(map[string]map[string]bool)

	for _, res := range results {
		relPath, err := filepath.Rel(rootPath, res.FilePath)
		if err != nil {
			relPath = res.FilePath
		}
		scannedFiles[relPath] = true
		if _, exists := activeFindings[relPath]; !exists {
			activeFindings[relPath] = make(map[string]bool)
		}
		for _, f := range res.Findings {
			activeFindings[relPath][f.Message] = true
		}
	}

	// Iterate over the baseline files
	for relPath, entries := range baseline.Files {
		// If the file was not scanned during this run, skip checking its entries to avoid marking them as stale
		if !scannedFiles[relPath] {
			continue
		}

		activeForFile := activeFindings[relPath]
		for _, entry := range entries {
			// Since the file WAS scanned, if there is no active finding matching the entry's message, it is stale
			if activeForFile == nil || !activeForFile[entry.Message] {
				stale = append(stale, StaleBaselineEntry{
					FilePath: relPath,
					Message:  entry.Message,
				})
			}
		}
	}

	return stale
}

// PruneBaseline removes stale entries from the baseline.
func PruneBaseline(baseline Baseline, stale []StaleBaselineEntry) Baseline {
	// Create a map of stale entries for fast lookup: filePath -> set of stale messages
	staleMap := make(map[string]map[string]bool)
	for _, s := range stale {
		if _, exists := staleMap[s.FilePath]; !exists {
			staleMap[s.FilePath] = make(map[string]bool)
		}
		staleMap[s.FilePath][s.Message] = true
	}

	prunedFiles := make(map[string][]BaselineEntry)

	for relPath, entries := range baseline.Files {
		var activeEntries []BaselineEntry
		for _, entry := range entries {
			if staleMap[relPath] == nil || !staleMap[relPath][entry.Message] {
				activeEntries = append(activeEntries, entry)
			}
		}
		if len(activeEntries) > 0 {
			prunedFiles[relPath] = activeEntries
		}
	}

	return Baseline{Files: prunedFiles}
}
