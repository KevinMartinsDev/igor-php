package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/igor-php/igor-php/internal/analyzer"
	"github.com/igor-php/igor-php/internal/auditor"
	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func executeAudit(auditList []symbol.AuditStatus, aud *auditor.Auditor, cfg config.Config, baseline config.Baseline, rootPath string) []symbol.AuditStatus {
	resultsChan := runParallelAudit(auditList, aud)
	var finalResults []symbol.AuditStatus

	for res := range resultsChan {
		res.FilePath = cfg.NormalizePath(res.FilePath)
		if !cfg.GenerateBaseline && !cfg.CheckBaseline && !cfg.PruneBaseline && baseline.Files != nil {
			res.Findings = config.FilterFindings(baseline, res.FilePath, res.Findings, rootPath)
			res.Status = calculateAuditStatus(res.Findings)
		}
		finalResults = append(finalResults, res)
	}

	return finalResults
}

func calculateAuditStatus(findings []symbol.Finding) string {
	if len(findings) == 0 {
		return "✅ OK"
	}
	for _, f := range findings {
		if f.Severity == "ERROR" {
			return "❌ KO"
		}
	}
	return "⚠️  WARN"
}

func loadContainerDump(rootPath string, cfg config.Config) (analyzer.NonSharedServiceMap, analyzer.AliasesMap) {
	if cfg.ContainerDump == "" {
		return nil, nil
	}

	dumpPath := cfg.ContainerDump
	if !filepath.IsAbs(dumpPath) {
		dumpPath = filepath.Join(rootPath, dumpPath)
	}

	nonShared, aliases, err := analyzer.LoadContainerDump(dumpPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Could not load container dump from %s: %v\n", dumpPath, err)
		return nil, nil
	}

	fmt.Fprintf(os.Stderr, "📦 Container dump loaded: %d non-shared (transient) classes will be skipped.\n", len(nonShared))
	if len(aliases) > 0 {
		fmt.Fprintf(os.Stderr, "   %d interface aliases will be used for reachability.\n", len(aliases))
	}
	return nonShared, aliases
}

func runParallelAudit(auditList []symbol.AuditStatus, aud *auditor.Auditor) <-chan symbol.AuditStatus {
	resultsChan := make(chan symbol.AuditStatus, len(auditList))
	jobsChan := make(chan symbol.AuditStatus, len(auditList))
	var wg sync.WaitGroup

	for w := 1; w <= 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobsChan {
				findings, err := aud.Audit(job.FilePath, job.Dependencies)
				if err != nil {
					job.Status = "❌ ERROR"
					resultsChan <- job
					continue
				}
				job.Findings = findings
				job.Status = "✅ OK"
				if len(findings) > 0 {
					hasError := false
					for _, f := range findings {
						if f.Severity == "ERROR" {
							hasError = true
							break
						}
					}
					if hasError {
						job.Status = "❌ KO"
					} else {
						job.Status = "⚠️  WARN"
					}
				}
				resultsChan <- job
			}
		}()
	}

	for _, job := range auditList {
		jobsChan <- job
	}
	close(jobsChan)

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	return resultsChan
}

func detectSymfonyProject(rootPath string, cfg config.Config) (*auditor.SymfonyBridge, error) {
	sb, err := auditor.DetectSymfony(rootPath, cfg)
	if err != nil {
		if cfg.NoAgent {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: Symfony Deep Audit disabled: %v\n", err)
			fmt.Fprintln(os.Stderr, "   Falling back to standard directory scan.")
			return nil, nil
		} else {
			return nil, err
		}
	}
	return sb, nil
}
