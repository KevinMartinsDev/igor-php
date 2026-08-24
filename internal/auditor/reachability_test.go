package auditor

import (
	"path/filepath"
	"testing"

	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestMarkReachability(t *testing.T) {
	cfg := config.Config{}
	auditor := NewAuditor(cfg)

	appPath := filepath.Join("..", "..", "test", "fixtures", "reachability_app.php")
	vendorPath := filepath.Join("..", "..", "test", "fixtures", "reachability_vendor.php")

	appFindings, err := auditor.Audit(appPath, nil)
	if err != nil {
		t.Fatalf("failed to audit app fixture: %v", err)
	}
	vendorFindings, err := auditor.Audit(vendorPath, nil)
	if err != nil {
		t.Fatalf("failed to audit vendor fixture: %v", err)
	}
	auditor.recordProjectClass("App\\Controller\\AppController")

	if len(vendorFindings) != 2 {
		t.Fatalf("expected 2 findings in vendor fixture (mutate + neverCalled), got %d: %+v", len(vendorFindings), vendorFindings)
	}

	results := []symbol.AuditStatus{
		{FilePath: appPath, Findings: appFindings},
		{FilePath: vendorPath, Findings: vendorFindings},
	}

	auditor.MarkReachability(results)

	rankByMethod := make(map[string]string)
	for _, f := range results[1].Findings {
		rankByMethod[f.ContextMethod] = f.Reachability
	}

	if rankByMethod["mutate"] != "HIGH" {
		t.Errorf("expected VendorService::mutate() to be HIGH (reachable via AppController::run -> entryPoint -> mutate), got %q", rankByMethod["mutate"])
	}
	if rankByMethod["neverCalled"] != "INFO" {
		t.Errorf("expected VendorService::neverCalled() to be INFO (no call site), got %q", rankByMethod["neverCalled"])
	}
}
