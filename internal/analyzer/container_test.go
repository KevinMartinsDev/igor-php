package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	php "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

func TestLoadContainerDumpValidatesSuccessfully(t *testing.T) {
	tempFile, err := os.CreateTemp("", "container_test_*.json")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()

	content := `{
        "services": [
            {"class": "App\\Http\\Uri", "shared": false},
            {"class": "App\\Security\\Security", "shared": true}
        ]
    }`

	if _, err := tempFile.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write mock JSON content: %v", err)
	}
	_ = tempFile.Close()

	nonShared, aliases, err := LoadContainerDump(tempFile.Name())
	if err != nil {
		t.Fatalf("Unexpected parsing error: %v", err)
	}

	if !nonShared["App\\Http\\Uri"] {
		t.Error("Uri should have been identified as non-shared")
	}

	if nonShared["App\\Security\\Security"] {
		t.Error("Security is a shared service and should not exist in the non-shared lookup map")
	}

	if len(aliases) != 0 {
		t.Errorf("Expected no aliases, got %d", len(aliases))
	}
}

// TestLoadContainerDumpEmptyPath guarantees callers can invoke the loader
// unconditionally: an empty path yields an empty, non-nil map and no error.
func TestLoadContainerDumpEmptyPath(t *testing.T) {
	nonShared, aliases, err := LoadContainerDump("")
	if err != nil {
		t.Fatalf("Unexpected error for empty path: %v", err)
	}
	if nonShared == nil {
		t.Fatal("Expected a non-nil map for empty path")
	}
	if aliases == nil {
		t.Fatal("Expected a non-nil aliases map for empty path")
	}
	if len(nonShared) != 0 {
		t.Errorf("Expected an empty map for empty path, got %d entries", len(nonShared))
	}
	if len(aliases) != 0 {
		t.Errorf("Expected an empty aliases map for empty path, got %d entries", len(aliases))
	}
}

// TestLoadContainerDumpParsesAliases ensures interface-to-implementation
// aliases are loaded and normalized from the JSON dump.
func TestLoadContainerDumpParsesAliases(t *testing.T) {
	tempFile, err := os.CreateTemp("", "container_aliases_*.json")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()

    content := `{
        "services": [
            {"class": "App\\Service\\MailService", "shared": true},
            {"class": "App\\Service\\Logger", "shared": true}
        ],
        "aliases": {
            "App\\Contract\\MailerInterface": "App\\Service\\MailService",
            "\\App\\Contract\\LoggerInterface": "App\\Service\\Logger",
            "App\\Contract\\FactoryInterface": "app.factory"
        }
    }`

	if _, err := tempFile.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write mock JSON content: %v", err)
	}
	_ = tempFile.Close()

	_, aliases, err := LoadContainerDump(tempFile.Name())
	if err != nil {
		t.Fatalf("Unexpected parsing error: %v", err)
	}

	if len(aliases) != 3 {
		t.Fatalf("Expected 3 aliases, got %d", len(aliases))
	}
	if aliases["App\\Contract\\MailerInterface"] != "App\\Service\\MailService" {
		t.Errorf("Expected MailerInterface -> MailService, got %q", aliases["App\\Contract\\MailerInterface"])
	}
	if aliases["App\\Contract\\LoggerInterface"] != "App\\Service\\Logger" {
		t.Errorf("Expected LoggerInterface -> Logger, got %q", aliases["App\\Contract\\LoggerInterface"])
	}
	if aliases["App\\Contract\\FactoryInterface"] != "app.factory" {
		t.Errorf("Expected FactoryInterface -> app.factory, got %q", aliases["App\\Contract\\FactoryInterface"])
	}
}

// TestLoadContainerDumpMissingFile ensures a missing file surfaces as an error
// rather than silently succeeding.
func TestLoadContainerDumpMissingFile(t *testing.T) {
	_, _, err := LoadContainerDump(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("Expected an error for a missing container dump file, got nil")
	}
}

// TestLoadContainerDumpNormalizesLeadingBackslash ensures FQCNs emitted with a
// leading namespace separator still match the visitor's resolved class name.
func TestLoadContainerDumpNormalizesLeadingBackslash(t *testing.T) {
	tempFile, err := os.CreateTemp("", "container_norm_*.json")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()

	content := `{"services": [{"class": "\\App\\Http\\Stream", "shared": false}]}`
	if _, err := tempFile.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write mock JSON content: %v", err)
	}
	_ = tempFile.Close()

	nonShared, aliases, err := LoadContainerDump(tempFile.Name())
	if err != nil {
		t.Fatalf("Unexpected parsing error: %v", err)
	}

	if !nonShared["App\\Http\\Stream"] {
		t.Error("Stream should be matched after trimming the leading backslash")
	}
	if len(aliases) != 0 {
		t.Errorf("Expected no aliases, got %d", len(aliases))
	}
}

// countErrors walks a snippet of PHP and returns how many ERROR findings the
// visitor produces, optionally seeded with a non-shared service map.
func countErrors(t *testing.T, code string, nonShared NonSharedServiceMap) int {
	t.Helper()

	content := []byte(code)
	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	if err := p.SetLanguage(lang); err != nil {
		t.Fatalf("failed to set language: %v", err)
	}
	tree := p.Parse(content, nil)
	defer tree.Close()

	v := NewVisitor(content, &mockEngine{})
	v.SetNonSharedServices(nonShared)
	v.Walk(tree.RootNode())

	errors := 0
	for _, f := range v.Findings() {
		if f.Severity == "ERROR" {
			errors++
		}
	}
	return errors
}

// TestNonSharedServiceBypassSuppressesMutation reproduces the report's
// categories 3 & 4: a per-request value object whose legitimate mutator is
// flagged as KO. When the class is declared non-shared via the container dump,
// the mutation must be skipped.
func TestNonSharedServiceBypassSuppressesMutation(t *testing.T) {
	code := `<?php
namespace App\Http;

final class Uri
{
    private string $scheme = '';
    public function set(string $v): void
    {
        $this->scheme = $v;
    }
}`

	// Control: without the bridge signal, the mutation is reported as KO.
	if got := countErrors(t, code, nil); got == 0 {
		t.Fatal("expected a KO finding for the mutation without the container dump")
	}

	// With the dump declaring App\Http\Uri as non-shared, it is skipped.
	nonShared := NonSharedServiceMap{"App\\Http\\Uri": true}
	if got := countErrors(t, code, nonShared); got != 0 {
		t.Errorf("expected 0 findings for a non-shared transient class, got %d", got)
	}
}

// TestNonSharedServiceBypassStillFlagsSharedClass ensures the bypass is scoped:
// a class that is NOT in the non-shared map keeps being audited normally.
func TestNonSharedServiceBypassStillFlagsSharedClass(t *testing.T) {
	code := `<?php
namespace App\Security;

final class Security
{
    private string $token = '';
    public function set(string $v): void
    {
        $this->token = $v;
    }
}`

	// Map contains a different class; Security must still be flagged.
	nonShared := NonSharedServiceMap{"App\\Http\\Uri": true}
	if got := countErrors(t, code, nonShared); got == 0 {
		t.Error("expected a KO finding for a shared class absent from the non-shared map")
	}
}

// TestNonSharedServiceBypassIsNoOpWhenUnset proves the feature is fully
// optional: whether the container dump is never configured (the map field stays
// nil), explicitly nil, or an empty non-nil map, the visitor audits mutations
// identically to how it behaved before the feature existed — and reading a nil
// map must never panic.
func TestNonSharedServiceBypassIsNoOpWhenUnset(t *testing.T) {
	code := `<?php
namespace App\Service;

final class StatefulService
{
    private string $token = '';
    public function set(string $v): void
    {
        $this->token = $v;
    }
}`

	// Baseline: a visitor whose SetNonSharedServices is never called, so the
	// nonSharedServices field keeps its zero value (nil map). This is exactly the
	// "no --container-dump configured" production path.
	content := []byte(code)
	p := sitter.NewParser()
	if err := p.SetLanguage(sitter.NewLanguage(php.LanguagePHP())); err != nil {
		t.Fatalf("failed to set language: %v", err)
	}
	tree := p.Parse(content, nil)
	defer tree.Close()

	v := NewVisitor(content, &mockEngine{}) // SetNonSharedServices intentionally NOT called
	v.Walk(tree.RootNode())                 // must not panic reading the nil map

	unset := 0
	for _, f := range v.Findings() {
		if f.Severity == "ERROR" {
			unset++
		}
	}
	if unset == 0 {
		t.Fatal("expected the mutation to be flagged when no container dump is configured (nil map must be a no-op)")
	}

	// An explicit nil map and an empty (non-nil) map must yield the identical
	// result — the bridge can only ever suppress findings for classes it lists.
	if got := countErrors(t, code, nil); got != unset {
		t.Errorf("explicit nil map changed results: got %d, want %d (no-op expected)", got, unset)
	}
	if got := countErrors(t, code, NonSharedServiceMap{}); got != unset {
		t.Errorf("empty map changed results: got %d, want %d (no-op expected)", got, unset)
	}
}
