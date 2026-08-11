package main

import (
	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/reporter"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func setupReporter(cfg config.Config) reporter.Reporter {
	switch cfg.OutputFormat {
	case "llm":
		return reporter.NewLLMReporter(Version)
	case "json":
		return reporter.NewJSONReporter()
	default:
		return reporter.NewReporter()
	}
}

func reportAllFindings(rep reporter.Reporter, results []symbol.AuditStatus, rootPath string) {
	// Report Project Results first
	hasProjectFindings := false
	for _, res := range results {
		isVendor := res.IsVendor(rootPath)
		if !isVendor && len(res.Findings) > 0 {
			if !hasProjectFindings {
				rep.PrintProjectHeader()
				hasProjectFindings = true
			}
			rep.PrintFindings(res, rootPath, isVendor)
		}
	}

	// Report Vendor Results second
	hasVendorFindings := false
	for _, res := range results {
		isVendor := res.IsVendor(rootPath)
		if isVendor && len(res.Findings) > 0 {
			if !hasVendorFindings {
				rep.PrintVendorHeader()
				hasVendorFindings = true
			}
			rep.PrintFindings(res, rootPath, isVendor)
		}
	}
}
