package auditor

import (
	"os"
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

func TestMarkReachability_InheritedMethodPromotedToHigh(t *testing.T) {
	cfg := config.Config{}
	auditor := NewAuditor(cfg)

	appPath := filepath.Join("..", "..", "test", "fixtures", "reachability_inheritance_app.php")
	vendorPath := filepath.Join("..", "..", "test", "fixtures", "reachability_inheritance_vendor.php")

	appFindings, err := auditor.Audit(appPath, nil)
	if err != nil {
		t.Fatalf("failed to audit app fixture: %v", err)
	}
	vendorFindings, err := auditor.Audit(vendorPath, nil)
	if err != nil {
		t.Fatalf("failed to audit vendor fixture: %v", err)
	}
	auditor.recordProjectClass("App\\Controller\\InheritanceController")

	results := []symbol.AuditStatus{
		{FilePath: appPath, Findings: appFindings},
		{FilePath: vendorPath, Findings: vendorFindings},
	}

	auditor.MarkReachability(results)

	rankByClassMethod := make(map[string]string)
	for _, f := range results[1].Findings {
		rankByClassMethod[f.ContextClass+"::"+f.ContextMethod] = f.Reachability
	}

	cases := []struct {
		key  string
		want string
	}{
		// Inherited (not overridden) method: promoted to HIGH through a direct
		// extends (InheritedChildService) and a multi-level extends
		// (InheritedGrandchildService -> InheritedChildService -> InheritedBaseService).
		{"Vendor\\Lib\\InheritedBaseService::mutate", "HIGH"},
		// Never invoked anywhere, directly or through a subclass.
		{"Vendor\\Lib\\InheritedBaseService::neverCalled", "INFO"},
		// Overridden: only the child's own mutate() is called, so the parent's
		// declaration must NOT inherit that reachability.
		{"Vendor\\Lib\\OverriddenBaseService::mutate", "INFO"},
		{"Vendor\\Lib\\OverridingChildService::mutate", "HIGH"},
		{"Vendor\\Lib\\GeneratorBaseService::getOutputFromHtml", "HIGH"},
		{"Vendor\\Lib\\GeneratorBaseService::createTemporaryFile", "HIGH"},
	}
	for _, tc := range cases {
		if got := rankByClassMethod[tc.key]; got != tc.want {
			t.Errorf("expected %s to be %s, got %q (all: %v)", tc.key, tc.want, got, rankByClassMethod)
		}
	}
}

func TestResolveDeclaringAncestor(t *testing.T) {
	auditor := NewAuditor(config.Config{})
	auditor.RecordClassParent("App\\Child", "App\\Parent")
	auditor.RecordClassParent("App\\Grandchild", "App\\Child")
	auditor.RecordMethodDeclared("App\\Parent", "mutate")
	auditor.RecordMethodDeclared("App\\Child", "override")

	tests := []struct {
		name   string
		class  string
		method string
		want   string
	}{
		{"direct child inherits from parent", "App\\Child", "mutate", "App\\Parent"},
		{"multi-level grandchild inherits from parent", "App\\Grandchild", "mutate", "App\\Parent"},
		{"override stops at the declaring child", "App\\Grandchild", "override", "App\\Child"},
		{"class declaring its own method resolves to itself", "App\\Parent", "mutate", "App\\Parent"},
		{"unknown method resolves to nothing", "App\\Child", "unknown", ""},
		{"unknown class resolves to nothing", "App\\Unrelated", "mutate", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auditor.resolveDeclaringAncestor(tt.class, tt.method); got != tt.want {
				t.Errorf("resolveDeclaringAncestor(%q, %q) = %q, want %q", tt.class, tt.method, got, tt.want)
			}
		})
	}
}

func TestResolveDeclaringAncestor_CycleGuard(t *testing.T) {
	auditor := NewAuditor(config.Config{})
	// A malformed/self-referential hierarchy must not hang the walk.
	auditor.RecordClassParent("App\\A", "App\\B")
	auditor.RecordClassParent("App\\B", "App\\A")

	if got := auditor.resolveDeclaringAncestor("App\\A", "mutate"); got != "" {
		t.Errorf("expected empty result for a cyclic hierarchy with no declaration, got %q", got)
	}
}

func TestAudit_SymlinkedVendorFileIsNotProjectClass(t *testing.T) {
	realDir := t.TempDir()
	phpFile := filepath.Join(realDir, "SymlinkedService.php")
	content := []byte(`<?php
namespace Vendor\Lib;

class SymlinkedService
{
    private array $data = [];

    public function mutate(string $value): void
    {
        $this->data[] = $value;
    }
}
`)
	if err := os.WriteFile(phpFile, content, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	auditor := NewAuditor(config.Config{
		SymlinkMap: map[string]string{
			realDir: "vendor/acme/symlinked-package",
		},
	})
	if _, err := auditor.Audit(phpFile, nil); err != nil {
		t.Fatalf("audit failed: %v", err)
	}

	if auditor.projectClasses["Vendor\\Lib\\SymlinkedService"] {
		t.Error("expected symlinked vendor class to be excluded from projectClasses")
	}
}
