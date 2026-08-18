package auditor

import (
	"os"
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
		{
			name:           "Inheritance reset check bypasses inherited properties",
			fixture:        "inheritance_reset_test.php",
			expectedErrors: 1,
			contains:       "Property 'localProp' of ConcreteChildAdapterWithLocalLeak is mutated but not reset",
		},
		{
			name:           "Destructor detection magic method",
			fixture:        "destructor_test.php",
			expectedErrors: 1,
			contains:       "Usage of __destruct() magic method is dangerous in Worker mode.",
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
			"App\\Service\\MapServiceAlias":        map[string]interface{}{"service": "App\\Service\\NonSharedService"},
			"App\\Service\\MapIdAlias":             map[string]interface{}{"id": "App\\Service\\NonSharedService"},
			"App\\Service\\MapTargetAlias":         map[string]interface{}{"target": "App\\Service\\NonSharedService"},
			"App\\Service\\MapInvalidAlias":        map[string]interface{}{"invalid": "App\\Service\\NonSharedService"},
			"App\\Service\\InvalidTypeAlias":       123,
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

	t.Run("IsExplicitlyNonShared with map service alias", func(t *testing.T) {
		if !a.IsExplicitlyNonShared("App\\Service\\MapServiceAlias") {
			t.Error("Expected MapServiceAlias to resolve to non-shared service")
		}
	})

	t.Run("IsExplicitlyNonShared with map id alias", func(t *testing.T) {
		if !a.IsExplicitlyNonShared("App\\Service\\MapIdAlias") {
			t.Error("Expected MapIdAlias to resolve to non-shared service")
		}
	})

	t.Run("IsExplicitlyNonShared with map target alias", func(t *testing.T) {
		if !a.IsExplicitlyNonShared("App\\Service\\MapTargetAlias") {
			t.Error("Expected MapTargetAlias to resolve to non-shared service")
		}
	})

	t.Run("IsExplicitlyNonShared with map invalid alias", func(t *testing.T) {
		if a.IsExplicitlyNonShared("App\\Service\\MapInvalidAlias") {
			t.Error("Expected MapInvalidAlias to fail to resolve")
		}
	})

	t.Run("IsExplicitlyNonShared with invalid type alias", func(t *testing.T) {
		if a.IsExplicitlyNonShared("App\\Service\\InvalidTypeAlias") {
			t.Error("Expected InvalidTypeAlias to fail to resolve")
		}
	})

	t.Run("IsExplicitlyNonShared on shared service", func(t *testing.T) {
		if a.IsExplicitlyNonShared("App\\Translator\\MyTranslator") {
			t.Error("Expected shared MyTranslator to not be explicitly non-shared")
		}
	})
}

func TestAuditor_IsResettable_DoctrineManager(t *testing.T) {
	// Case 1: doctrine is resettable
	{
		a := NewAuditor(config.Config{})
		a.Symfony = &SymfonyBridge{
			Container: &symbol.SymfonyContainer{
				Definitions: map[string]symbol.SymfonyService{
					"doctrine": {
						Class:      "Doctrine\\Bundle\\DoctrineBundle\\Registry",
						Resettable: true,
					},
				},
			},
		}

		if !a.IsResettable("Doctrine\\ORM\\EntityManagerInterface") {
			t.Error("Expected EntityManagerInterface to be resettable when 'doctrine' service is resettable")
		}
	}

	// Case 2: doctrine is NOT resettable
	{
		a := NewAuditor(config.Config{})
		a.Symfony = &SymfonyBridge{
			Container: &symbol.SymfonyContainer{
				Definitions: map[string]symbol.SymfonyService{
					"doctrine": {
						Class:      "Doctrine\\Bundle\\DoctrineBundle\\Registry",
						Resettable: false,
					},
				},
			},
		}

		if a.IsResettable("Doctrine\\ORM\\EntityManagerInterface") {
			t.Error("Expected EntityManagerInterface to NOT be resettable when 'doctrine' service is NOT resettable")
		}
	}

	// Case 3: doctrine_mongodb is resettable
	{
		a := NewAuditor(config.Config{})
		a.Symfony = &SymfonyBridge{
			Container: &symbol.SymfonyContainer{
				Definitions: map[string]symbol.SymfonyService{
					"doctrine_mongodb": {
						Class:      "Doctrine\\Bundle\\MongoDBBundle\\ManagerRegistry",
						Resettable: true,
					},
				},
			},
		}

		if !a.IsResettable("Doctrine\\ODM\\MongoDB\\DocumentManager") {
			t.Error("Expected DocumentManager to be resettable when 'doctrine_mongodb' service is resettable")
		}
	}

	// Case 4: No doctrine registry service defined
	{
		a := NewAuditor(config.Config{})
		a.Symfony = &SymfonyBridge{
			Container: &symbol.SymfonyContainer{
				Definitions: map[string]symbol.SymfonyService{},
			},
		}

		if a.IsResettable("Doctrine\\ORM\\EntityManagerInterface") {
			t.Error("Expected EntityManagerInterface to NOT be resettable when no registry is defined")
		}
	}
}

func TestAuditor_GetMethodReturnType(t *testing.T) {
	// Create temporary directory for our test file
	tmpDir, err := os.MkdirTemp("", "igor_auditor_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "TracerInterface.php")
	content := []byte(`<?php
namespace App\Tracing;

interface TracerInterface {
    public function makeSpan(): Span;
    public function getSelf(): self;
    public function getStatic(): static;
    public function nullableSpan(): ?Span;
    public function noReturnType();
}
`)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write mock file: %v", err)
	}

	a := NewAuditor(config.Config{})
	a.Symfony = &SymfonyBridge{
		ClassToFile: map[string]string{
			"App\\Tracing\\TracerInterface": filePath,
		},
	}

	// Test case 1: Normal return type
	retType := a.GetMethodReturnType("App\\Tracing\\TracerInterface", "makeSpan")
	if retType != "App\\Tracing\\Span" {
		t.Errorf("Expected 'App\\Tracing\\Span', got %q", retType)
	}

	// Test case 2: self resolution
	retTypeSelf := a.GetMethodReturnType("App\\Tracing\\TracerInterface", "getSelf")
	if retTypeSelf != "App\\Tracing\\TracerInterface" {
		t.Errorf("Expected 'App\\Tracing\\TracerInterface' for self return type, got %q", retTypeSelf)
	}

	// Test case 3: static resolution
	retTypeStatic := a.GetMethodReturnType("App\\Tracing\\TracerInterface", "getStatic")
	if retTypeStatic != "App\\Tracing\\TracerInterface" {
		t.Errorf("Expected 'App\\Tracing\\TracerInterface' for static return type, got %q", retTypeStatic)
	}

	// Test case 4: nullable return type
	retTypeNullable := a.GetMethodReturnType("App\\Tracing\\TracerInterface", "nullableSpan")
	if retTypeNullable != "App\\Tracing\\Span" {
		t.Errorf("Expected 'App\\Tracing\\Span' for nullableSpan, got %q", retTypeNullable)
	}

	// Test case 5: no return type declared
	retTypeNone := a.GetMethodReturnType("App\\Tracing\\TracerInterface", "noReturnType")
	if retTypeNone != "" {
		t.Errorf("Expected empty string when no return type is declared, got %q", retTypeNone)
	}

	// Test case 6: non-existent class or method
	retTypeNonExistentClass := a.GetMethodReturnType("App\\Tracing\\Unknown", "any")
	if retTypeNonExistentClass != "" {
		t.Errorf("Expected empty string for non-existent class, got %q", retTypeNonExistentClass)
	}

	retTypeNonExistentMethod := a.GetMethodReturnType("App\\Tracing\\TracerInterface", "unknown")
	if retTypeNonExistentMethod != "" {
		t.Errorf("Expected empty string for non-existent method, got %q", retTypeNonExistentMethod)
	}
}

func TestAuditor_HelperMethods(t *testing.T) {
	cfg := config.Config{
		DevPackages:    []string{"phpunit/phpunit", "friendsofphp/php-cs-fixer"},
		SafeNamespaces: []string{"\\Symfony\\Component\\", "App\\Safe\\"},
	}
	a := NewAuditor(cfg)

	// 1. Test IsDevPackagePath
	t.Run("IsDevPackagePath matching cases", func(t *testing.T) {
		if !a.IsDevPackagePath("vendor/phpunit/phpunit/src/Framework/TestCase.php") {
			t.Error("Expected vendor/phpunit/phpunit path to match dev package")
		}
		if a.IsDevPackagePath("vendor/symfony/http-kernel/Kernel.php") {
			t.Error("Expected vendor/symfony/http-kernel path NOT to match dev package")
		}
	})

	// 2. Test IsDataPath
	t.Run("IsDataPath matching cases", func(t *testing.T) {
		sep := "/"
		if !a.IsDataPath("src" + sep + "Entity" + sep + "User.php") {
			t.Error("Expected Entity path to match data path")
		}
		if !a.IsDataPath("src" + sep + "DTO" + sep + "Request.php") {
			t.Error("Expected DTO path to match data path")
		}
		if a.IsDataPath("src" + sep + "Service" + sep + "MyService.php") {
			t.Error("Expected Service path NOT to match data path")
		}
	})

	// 3. Test ExtractFQCN
	t.Run("ExtractFQCN on StatelessService", func(t *testing.T) {
		path := filepath.Join("..", "..", "test", "fixtures", "clean_code.php")
		fqcn, err := a.ExtractFQCN(path)
		if err != nil {
			t.Fatalf("ExtractFQCN failed: %v", err)
		}
		expected := "App\\Service\\StatelessService"
		if fqcn != expected {
			t.Errorf("Expected FQCN to be %s, got %s", expected, fqcn)
		}
	})

	// 4. Test IsSafeNamespace
	t.Run("IsSafeNamespace cases", func(t *testing.T) {
		if !a.IsSafeNamespace("Symfony\\Component\\HttpClient\\CachingHttpClient") {
			t.Error("Expected Symfony\\Component\\HttpClient\\CachingHttpClient to be in a safe namespace")
		}
		if !a.IsSafeNamespace("\\Symfony\\Component\\HttpClient\\CachingHttpClient") {
			t.Error("Expected absolute path namespace to be safe")
		}
		if !a.IsSafeNamespace("App\\Safe\\Helper") {
			t.Error("Expected App\\Safe\\Helper to be safe")
		}
		if a.IsSafeNamespace("App\\Unsafe\\Helper") {
			t.Error("Expected App\\Unsafe\\Helper NOT to be safe")
		}
	})
}

func TestAuditor_TypeTrackingIntegrationFixture(t *testing.T) {
	a := NewAuditor(config.Config{})
	filePath := filepath.Join("..", "..", "test", "fixtures", "type_tracking_test.php")

	a.Symfony = &SymfonyBridge{
		ClassToFile: map[string]string{
			"App\\Service\\TraceTest\\TracerInterface":   filePath,
			"App\\Service\\TraceTest\\Span":              filePath,
			"App\\Service\\TraceTest\\FakeEntityManager": filePath,
			"App\\Service\\TraceTest\\SuperService":      filePath,
			"App\\Service\\TraceTest\\UnsafeService":     filePath,
			"Doctrine\\ORM\\EntityManagerInterface":      filePath,
		},
		Container: &symbol.SymfonyContainer{
			Definitions: map[string]symbol.SymfonyService{
				"doctrine": {
					Class:      "Doctrine\\Bundle\\DoctrineBundle\\Registry",
					Resettable: true,
				},
				"App\\Service\\TraceTest\\TracerInterface": {
					Class:  "App\\Service\\TraceTest\\TracerInterface",
					Shared: true,
				},
				"App\\Service\\TraceTest\\SuperService": {
					Class:  "App\\Service\\TraceTest\\SuperService",
					Shared: true,
				},
				"App\\Service\\TraceTest\\UnsafeService": {
					Class:  "App\\Service\\TraceTest\\UnsafeService",
					Shared: true,
				},
				"App\\Service\\TraceTest\\FakeEntityManager": {
					Class:  "App\\Service\\TraceTest\\FakeEntityManager",
					Shared: true,
				},
			},
		},
	}

	findings, err := a.Audit(filePath, nil)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	// We expect exactly 2 findings:
	// 1 for the chained EM call in testChainedEM() of SuperService
	// 1 for the chained EM call in testUnsafeTracing() of UnsafeService
	if len(findings) != 2 {
		t.Fatalf("Expected exactly 2 findings on type_tracking_test.php, got %d: %v", len(findings), findings)
	}

	for _, f := range findings {
		if !strings.Contains(f.Snippet, "disable") {
			t.Errorf("Expected finding snippet to contain 'disable', got: %s (Message: %s)", f.Snippet, f.Message)
		}
	}
}

func TestAuditor_UnitEdgeCases(t *testing.T) {
	// Test IsSharedService edge cases
	{
		a := NewAuditor(config.Config{})
		// a.Symfony is nil
		if !a.IsSharedService("AnyClass") {
			t.Error("Expected IsSharedService to return true when Symfony is nil")
		}

		a.Symfony = &SymfonyBridge{}
		// a.Symfony.Container is nil
		if !a.IsSharedService("AnyClass") {
			t.Error("Expected IsSharedService to return true when Symfony Container is nil")
		}
	}

	// Test GetMethodReturnType edge cases
	{
		a := NewAuditor(config.Config{})
		// a.Symfony is nil
		if a.GetMethodReturnType("App\\Service", "make") != "" {
			t.Error("Expected GetMethodReturnType to return empty when Symfony is nil")
		}

		a.Symfony = &SymfonyBridge{}
		// a.Symfony.ClassToFile is nil
		if a.GetMethodReturnType("App\\Service", "make") != "" {
			t.Error("Expected GetMethodReturnType to return empty when ClassToFile is nil")
		}

		// Directory path to force os.ReadFile error inside parseClassMethodSignatures
		tmpDir, err := os.MkdirTemp("", "igor_aud_err")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		a.Symfony = &SymfonyBridge{
			ClassToFile: map[string]string{
				"App\\Service\\ErrorClass": tmpDir, // Directory path will fail ReadFile
			},
		}

		ret := a.GetMethodReturnType("App\\Service\\ErrorClass", "make")
		if ret != "" {
			t.Errorf("Expected empty return type on ReadFile error, got %q", ret)
		}
	}

	// Test isBuiltinType exhaustively
	builtins := []string{"void", "int", "string", "bool", "float", "array", "callable", "object", "mixed", "never", "false", "null"}
	for _, b := range builtins {
		if !isBuiltinType(b) {
			t.Errorf("Expected isBuiltinType to return true for %q", b)
		}
	}
	if isBuiltinType("MyCustomClass") {
		t.Error("Expected isBuiltinType to return false for MyCustomClass")
	}
}
