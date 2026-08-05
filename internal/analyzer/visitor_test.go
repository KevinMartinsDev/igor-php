package analyzer

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	php "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

// mockEngine implements the Engine interface for testing.
type mockEngine struct {
	auditedClasses []string
}

func (m *mockEngine) RecordClassAudited(name string) {
	m.auditedClasses = append(m.auditedClasses, name)
}

func (m *mockEngine) IsExplicitlyNonShared(_ string) bool {
	return false
}

func (m *mockEngine) IsSafeNamespace(_ string) bool {
	return false
}

func (m *mockEngine) IsResettable(className string) bool {
	return className == "Xynnn\\GoogleTagManagerBundle\\Service\\GoogleTagManagerInterface" || className == "App\\Service\\ResettableService"
}

func TestPHPVisitor_Mutation(t *testing.T) {
	code := `<?php
class MyService {
    private $prop;
    public function set($v) {
        $this->prop = $v;
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	if len(findings) == 0 {
		t.Fatal("Expected at least one finding for state mutation, got 0")
	}

	found := false
	for _, f := range findings {
		if f.Severity == "ERROR" && f.Message != "" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected an ERROR finding for state mutation")
	}

	if len(engine.auditedClasses) == 0 || engine.auditedClasses[0] != "MyService" {
		t.Errorf("Expected class 'MyService' to be recorded as audited, got %v", engine.auditedClasses)
	}
}

func TestPHPVisitor_ResetInterface(t *testing.T) {
	code := `<?php
class MyService implements Symfony\Contracts\Service\ResetInterface {
    private $prop;
    public function set($v) {
        $this->prop = $v;
    }
    public function reset() {
        // Not resetting $prop
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	found := false
	for _, f := range findings {
		if f.Severity == "WARNING" && (f.Message == "Property 'prop' of MyService is mutated but not reset in reset().") {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected a WARNING finding for missing reset in ResetInterface")
	}
}

func TestPHPVisitor_DetectSingletonMutationRule(t *testing.T) {
	code := `<?php
class MyService {
    private $googleTagManager;
    public function set($v) {
        $this->googleTagManager->addPush([
            'event' => 'userEmailCaptured',
        ]);
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	found := false
	expectedMsg := "Mutation detected on an injected dependency ($this->googleTagManager). Risk of State Leak in a worker."
	for _, f := range findings {
		if f.Severity == "ERROR" && f.Message == expectedMsg {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected an ERROR finding with message: %q, got: %v", expectedMsg, findings)
	}
}

func TestPHPVisitor_DetectClosureStateLeakRule(t *testing.T) {
	code := `<?php
class MyService {
    private $dispatcher;
    public function createGtmCookie($event) {
        $family = 'test';
        $optin = true;
        $this->dispatcher->addListener('response', function ($event) use ($family, $optin) {
            // leak
        });
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	found := false
	expectedMsg := "Potential Memory Leak: Injection of a closure capturing local state into a shared service."
	for _, f := range findings {
		if f.Severity == "ERROR" && f.Message == expectedMsg {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected an ERROR finding with message: %q, got: %v", expectedMsg, findings)
	}
}

func TestPHPVisitor_DetectSingletonMutationRule_BypassedIfResettable(t *testing.T) {
	code := `<?php
namespace App\Listener;

use Xynnn\GoogleTagManagerBundle\Service\GoogleTagManagerInterface;

class MyListener {
    private GoogleTagManagerInterface $googleTagManager;
    
    public function set($v) {
        $this->googleTagManager->addPush([
            'event' => 'userEmailCaptured',
        ]);
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings because the dependency is resettable, got: %v", findings)
	}
}

func TestPHPVisitor_DetectSingletonMutationRule_BypassedIfClassImplementsResetAndResetsProp(t *testing.T) {
	code := `<?php
class MyService implements Symfony\Contracts\Service\ResetInterface {
    private $orExpression;
    
    public function set($v) {
        $this->orExpression->add($v);
    }
    
    public function reset() {
        $this->orExpression = null;
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings because the class implements ResetInterface and resets the property, got: %v", findings)
	}
}

func TestPHPVisitor_DetectSingletonMutationRule_WarnsIfClassImplementsResetButForgetsToResetProp(t *testing.T) {
	code := `<?php
class MyService implements Symfony\Contracts\Service\ResetInterface {
    private $orExpression;
    
    public function set($v) {
        $this->orExpression->add($v);
    }
    
    public function reset() {
        // forgot to reset orExpression
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	found := false
	expectedMsg := "Property 'orExpression' of MyService is mutated but not reset in reset()."
	for _, f := range findings {
		if f.Severity == "WARNING" && f.Message == expectedMsg {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected a WARNING finding with message %q, got: %v", expectedMsg, findings)
	}
}

func TestPHPVisitor_TryFinallyCleanup(t *testing.T) {
	code := `<?php
class AuthorizationChecker {
    private array $tokenStack = [];
    private array $accessDecisionStack = [];

    public function isGranted($attribute, $subject = null): bool
    {
        $this->accessDecisionStack[] = 'decision';

        try {
            return true;
        } finally {
            array_pop($this->accessDecisionStack);
        }
    }

    public function isGrantedForUser($user, $attribute): bool
    {
        $this->tokenStack[] = 'token';

        try {
            return $this->isGranted($attribute);
        } finally {
            array_pop($this->tokenStack);
        }
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings because the state mutations are perfectly cleaned up in finally blocks, got: %v", findings)
	}
}

func TestPHPVisitor_ClassIsResettableFromSymfony_BypassesDirectMutationAndEnforcesResetCheck(t *testing.T) {
	// ResettableService is defined in mockEngine as IsResettable = true
	code := `<?php
namespace App\Service;

class ResettableService {
    private $cache;
    
    public function mutate($v) {
        $this->cache = $v;
    }
    
    public function reset() {
        // forgot to reset $cache
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	found := false
	expectedMsg := "Property 'cache' of ResettableService is mutated but not reset in reset()."
	for _, f := range findings {
		if f.Severity == "WARNING" && f.Message == expectedMsg {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected an IncompleteReset WARNING finding for missing reset because the class is marked resettable by Symfony, got: %v", findings)
	}
}

func TestPHPVisitor_DetectSingletonMutation_NewPrefixes(t *testing.T) {
	methods := []string{"disable", "enable", "clear", "remove"}
	for _, m := range methods {
		code := fmt.Sprintf(`<?php
class MyService {
    private $someInjectedProp;
    public function set($v) {
        $this->someInjectedProp->%s($v);
    }
}`, m)
		content := []byte(code)

		p := sitter.NewParser()
		lang := sitter.NewLanguage(php.LanguagePHP())
		_ = p.SetLanguage(lang)
		tree := p.Parse(content, nil)
		defer tree.Close()

		engine := &mockEngine{}
		v := NewVisitor(content, engine)
		v.Walk(tree.RootNode())

		findings := v.Findings()
		found := false
		expectedMsg := "Mutation detected on an injected dependency ($this->someInjectedProp). Risk of State Leak in a worker."
		for _, f := range findings {
			if f.Severity == "ERROR" && f.Message == expectedMsg {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected an ERROR finding with message: %q for method %s, got: %v", expectedMsg, m, findings)
		}
	}
}

func TestPHPVisitor_DetectSingletonMutation_ChainedCalls(t *testing.T) {
	code := `<?php
class MyService {
    private $injectedRegistry;
    public function update($v) {
        $this->injectedRegistry->getManager()->getFilters()->disable($v);
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	found := false
	expectedMsg := "Mutation detected on an injected dependency ($this->injectedRegistry). Risk of State Leak in a worker."
	for _, f := range findings {
		if f.Severity == "ERROR" && f.Message == expectedMsg {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected an ERROR finding for chained call with message: %q, got: %v", expectedMsg, findings)
	}
}

func TestPHPVisitor_DetectSingletonMutation_ResetInterfaceException(t *testing.T) {
	code := `<?php
class MyService implements Symfony\Contracts\Service\ResetInterface {
    private $injectedRegistry;
    public function update($v) {
        $this->injectedRegistry->getManager()->getFilters()->disable($v);
    }
    public function reset() {
        // We do not reset the injected service reference itself, but we implement ResetInterface
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	// Should be 0 findings because class implements ResetInterface
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings because the class implements ResetInterface, got: %v", findings)
	}
}

func TestPHPVisitor_DetectSingletonMutation_LocalSharedVariableTracking(t *testing.T) {
	code := `<?php
class DisableSoftDeleteableFilter {
    protected function filterProperty() {
        $entityManager = $this->getManagerRegistry()->getManagerForClass();
        $entityManager->getFilters()->disable('softdeleteable');
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	found := false
	expectedMsg := "Mutation detected on a local reference to a shared service ($entityManager). Risk of State Leak in a worker."
	for _, f := range findings {
		if f.Severity == "ERROR" && f.Message == expectedMsg {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected an ERROR finding for local variable tracking with message: %q, got: %v", expectedMsg, findings)
	}
}

func TestPHPVisitor_DetectSingletonMutation_HeuristicsBypass(t *testing.T) {
	code := `<?php
class MyService {
    public function test() {
        // Heuristic 1: new instantiations
        $ticket = new Ticket();
        $ticket->setIsConnectionAllowed(true);

        // Heuristic 2: QueryBuilders and Expressions
        $expr = $this->queryBuilder->expr()->orX();
        $expr->add('some_like_expr');
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	// Should be 0 findings because all mutations are bypassed by our smart heuristics
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings because of smart heuristics bypass, got: %v", findings)
	}
}

func TestPHPVisitor_DetectSingletonMutation_DoctrineQueryBypass(t *testing.T) {
	code := `<?php
class CustomerParameterRepository {
    public function findCustomersConfigValues($configurationKey, $shortname) {
        $entityManager = $this->getEntityManager();
        $query = $entityManager->createQuery("SELECT c FROM App\\Entity\\Customer c");
        $query->setParameter('customerShortName', $shortname);
        $query->setParameter('configName', $configurationKey);
        
        $nativeQuery = $entityManager->createNativeQuery("SELECT * FROM customer", $rsm);
        $nativeQuery->setParameter('customerShortName', $shortname);
        
        $namedQuery = $entityManager->createNamedQuery("findActive");
        $namedQuery->setParameter('status', 1);

        return $query->getResult();
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	// Should be 0 findings because $query is created using a factory method and is not a local shared reference.
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings because Doctrine Query is transient, got: %v", findings)
	}
}

func TestPHPVisitor_DetectSingletonMutation_InfrastructureTaintBreakers(t *testing.T) {
	code := `<?php
class ValidationAndCacheService {
	private $context;
	private $cache;
	private $repository;

	public function testValidator() {
		// Chained buildViolation with addViolation
		$this->context->buildViolation("Error message")
			->setParameter("{{ value }}", "test")
			->addViolation();
	}

	public function testCache() {
		// Cache items
		$item = $this->cache->getItem("some_key");
		$item->set("cached_value");
	}

	public function testRepositoryAndEntity() {
		// Repository find and entity mutation
		$entity = $this->repository->findOneBy(["id" => 123]);
		$entity->setValue("new_value");
	}
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	// Should be 0 findings because the chains are broken by buildViolation, getItem, and findOneBy.
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings because of infrastructure Taint Breakers, got: %v", findings)
	}
}
func TestPHPVisitor_InheritedPropertyReset(t *testing.T) {
	code := `<?php
abstract class AbstractAdapter implements Symfony\Contracts\Service\ResetInterface {
    protected $sale;
    public function reset() {
        unset($this->sale);
    }
}

class ConcreteAdapter implements Symfony\Contracts\Service\ResetInterface {
    public function setSale($s) {
        $this->sale = $s; // Mutation of an inherited property (not locally declared)
    }
}

class ConcreteAdapterWithLocalProps implements Symfony\Contracts\Service\ResetInterface {
    private $localProp; // Locally declared property
    public function setLocalProp($val) {
        $this->localProp = $val; // Mutation of local property
    }
    // Forgets to implement reset() or doesn't reset $localProp
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	// Should only find the warning for 'localProp' in ConcreteAdapterWithLocalProps,
	// and NOT for 'sale' in ConcreteAdapter or AbstractAdapter (since AbstractAdapter resets it).
	foundLocalPropWarning := false
	foundSaleWarning := false

	for _, f := range findings {
		if f.Message == fmt.Sprintf("Property 'sale' of ConcreteAdapter is mutated but not reset in reset().") {
			foundSaleWarning = true
		}
		if f.Message == fmt.Sprintf("Property 'localProp' of ConcreteAdapterWithLocalProps is mutated but not reset in reset().") {
			foundLocalPropWarning = true
		}
	}

	if foundSaleWarning {
		t.Error("Expected no warning for inherited property 'sale' in ConcreteAdapter")
	}
	if !foundLocalPropWarning {
		t.Error("Expected a warning for local property 'localProp' in ConcreteAdapterWithLocalProps")
	}
}
