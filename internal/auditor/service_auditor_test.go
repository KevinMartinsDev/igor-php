package auditor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
)

func TestAuditFixtures(t *testing.T) {
	cfg := config.Config{}
	auditor := NewAuditor(cfg)

	tests := []struct {
		name           string
		fixture        string
		expectedErrors int
		contains       string
	}{
		{
			name:           "Clean code should have 0 errors",
			fixture:        "clean_code.php",
			expectedErrors: 0,
		},
		{
			name:           "Simple state mutation",
			fixture:        "state_mutation.php",
			expectedErrors: 5,
			contains:       "Mutation of state",
		},
		{
			name:           "Array state mutation",
			fixture:        "array_mutation.php",
			expectedErrors: 4,
			contains:       "Mutation of state",
		},
		{
			name:           "Execution terminators",
			fixture:        "terminators.php",
			expectedErrors: 2,
			contains:       "forbidden",
		},
		{
			name:           "ResetInterface partial cleanup",
			fixture:        "reset_check.php",
			expectedErrors: 1, // Only IncompleteResetService should fail/warn
			contains:       "not reset in reset()",
		},
		{
			name:           "ResetInterface partial cleanup (3 props, 2 reset)",
			fixture:        "reset_incomplete.php",
			expectedErrors: 1,
			contains:       "Property 'prop3' of IncompleteService is mutated but not reset",
		},
		{
			name:           "Security risks (superglobals & static vars)",
			fixture:        "security_risks.php",
			expectedErrors: 9, // 8 superglobals + 1 static var
			contains:       "$request->query",
		}, {
			name:           "Complex mutations (nested & dynamic)",
			fixture:        "complex_mutations.php",
			expectedErrors: 2, // Nested + Dynamic (Reference is harder to detect without data flow)
			contains:       "Mutation of state",
		},
		{
			name:           "Ignore annotation (@igor-ignore)",
			fixture:        "ignore_annotation.php",
			expectedErrors: 1,
			contains:       "Mutation of state",
		},
		{
			name:           "Readonly support (PHP 8.1+)",
			fixture:        "readonly_test.php",
			expectedErrors: 1, // Only the regular mutation on 'counter'
			contains:       "Mutation of state 'counter'",
		},
		{
			name:           "PHP 8 Attribute exclusions (WorkerSafe)",
			fixture:        "attribute_exclusion.php",
			expectedErrors: 3,
			contains:       "Mutation of state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join("..", "..", "test", "fixtures", tt.fixture)
			findings, err := auditor.Audit(path, nil)
			if err != nil {
				t.Fatalf("Failed to audit %s: %v", path, err)
			}

			if len(findings) != tt.expectedErrors {
				t.Errorf("Expected %d findings, got %d", tt.expectedErrors, len(findings))
				for _, f := range findings {
					t.Logf("- %s (Line %d)", f.Message, f.Line)
				}
			}

			if tt.contains != "" && len(findings) > 0 {
				found := false
				for _, f := range findings {
					if strings.Contains(f.Message, tt.contains) || strings.Contains(f.Remediation, tt.contains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected findings (Message or Remediation) to contain %q", tt.contains)
				}
			}
		})
	}
}

func TestAuditor_IsResettable_And_IsExplicitlyNonShared(t *testing.T) {
	// Create Auditor
	cfg := config.Config{}
	a := NewAuditor(cfg)

	// Mock SymfonyBridge and Container
	container := &symbol.SymfonyContainer{
		Definitions: map[string]symbol.SymfonyService{
			".abstract.instanceof.App\\Translator\\MyTranslator": {
				Class:      "App\\Translator\\MyTranslator",
				Public:     false,
				Shared:     true,
				Resettable: true,
			},
			"App\\Service\\NonSharedService": {
				Class:      "App\\Service\\NonSharedService",
				Public:     true,
				Shared:     false,
				Resettable: false,
			},
		},
		Aliases: map[string]interface{}{
			"App\\Translator\\TranslatorInterface": ".abstract.instanceof.App\\Translator\\MyTranslator",
			"App\\Service\\NonSharedInterface":     "App\\Service\\NonSharedService",
		},
	}

	a.Symfony = &SymfonyBridge{
		Container: container,
	}

	// Test IsResettable
	t.Run("IsResettable with exact class matching", func(t *testing.T) {
		if !a.IsResettable("App\\Translator\\MyTranslator") {
			t.Error("Expected MyTranslator class to be resettable")
		}
	})

	t.Run("IsResettable with slashes normalized", func(t *testing.T) {
		if !a.IsResettable("App/Translator/MyTranslator") {
			t.Error("Expected slash-normalized class to be resettable")
		}
	})

	t.Run("IsResettable with alias resolution", func(t *testing.T) {
		if !a.IsResettable("App\\Translator\\TranslatorInterface") {
			t.Error("Expected TranslatorInterface alias to resolve to resettable concrete service")
		}
	})

	t.Run("IsResettable with unknown class", func(t *testing.T) {
		if a.IsResettable("App\\UnknownService") {
			t.Error("Expected unknown class to not be resettable")
		}
	})

	// Test IsExplicitlyNonShared
	t.Run("IsExplicitlyNonShared with exact class matching", func(t *testing.T) {
		if !a.IsExplicitlyNonShared("App\\Service\\NonSharedService") {
			t.Error("Expected NonSharedService to be non-shared")
		}
	})

	t.Run("IsExplicitlyNonShared with alias resolution", func(t *testing.T) {
		if !a.IsExplicitlyNonShared("App\\Service\\NonSharedInterface") {
			t.Error("Expected NonSharedInterface alias to resolve to non-shared concrete service")
		}
	})

	t.Run("IsExplicitlyNonShared on shared service", func(t *testing.T) {
		if a.IsExplicitlyNonShared("App\\Translator\\MyTranslator") {
			t.Error("Expected shared MyTranslator to not be explicitly non-shared")
		}
	})
}
