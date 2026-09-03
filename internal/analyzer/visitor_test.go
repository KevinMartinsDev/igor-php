package analyzer

import (
	"fmt"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	php "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

// mockEngine implements the Engine interface for testing.
type mockEngine struct {
	auditedClasses    []string
	methodReturnTypes map[string]string
	recordedCalls     []string
	recordedParents   map[string]string
	declaredMethods   map[string][]string
}

func (m *mockEngine) RecordMethodCall(callerClass, callerMethod, calleeClass, calleeMethod string) {
	m.recordedCalls = append(m.recordedCalls, callerClass+"::"+callerMethod+" -> "+calleeClass+"::"+calleeMethod)
}

func (m *mockEngine) RecordClassParent(className, parentClassName string) {
	if m.recordedParents == nil {
		m.recordedParents = make(map[string]string)
	}
	m.recordedParents[className] = parentClassName
}

func (m *mockEngine) RecordMethodDeclared(className, methodName string) {
	if m.declaredMethods == nil {
		m.declaredMethods = make(map[string][]string)
	}
	m.declaredMethods[className] = append(m.declaredMethods[className], methodName)
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

func (m *mockEngine) GetMethodReturnType(className, methodName string) string {
	if m.methodReturnTypes != nil {
		key := className + "::" + methodName
		if ret, exists := m.methodReturnTypes[key]; exists {
			return ret
		}
	}
	return ""
}

func (m *mockEngine) IsSharedService(className string) bool {
	lower := strings.ToLower(className)
	if strings.Contains(lower, "span") || strings.Contains(lower, "dto") {
		return false
	}
	return true
}

func (m *mockEngine) IsResettable(className string) bool {
	lower := strings.ToLower(className)
	if strings.Contains(lower, "doctrine\\orm\\entitymanager") ||
		strings.Contains(lower, "doctrine\\persistence\\objectmanager") ||
		strings.Contains(lower, "doctrine\\odm\\mongodb\\documentmanager") {
		return true
	}
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
    private $someProp;
    private $anotherProp;

    public function isGranted($attribute, $subject = null): bool
    {
        $this->accessDecisionStack[] = 'decision';
        $this->someProp = 'someValue';
        $this->anotherProp = 'anotherValue';

        try {
            return true;
        } finally {
            array_pop($this->accessDecisionStack);
            $this->someProp = null;
            unset($this->anotherProp);
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

type mockSafeNamespaceEngine struct {
	mockEngine
	safeNamespace string
}

func (m *mockSafeNamespaceEngine) IsSafeNamespace(className string) bool {
	return className == m.safeNamespace
}

func TestPHPVisitor_IsSafeNamespaceBypass(t *testing.T) {
	code := `<?php
namespace Symfony\Component\HttpClient;

class CachingHttpClient {
    public function doSomething() {
        static $attemptTag = null;
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockSafeNamespaceEngine{
		safeNamespace: "Symfony\\Component\\HttpClient\\CachingHttpClient",
	}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	if len(findings) > 0 {
		t.Errorf("Expected 0 findings inside safe namespace, got %d: %v", len(findings), findings)
	}
}

func TestPHPVisitor_RecordsCallGraphEdgeForSafeNamespaceCaller(t *testing.T) {
	code := `<?php
namespace App;

class SafeCaller {
    private TargetService $target;

    public function run() {
        $this->target->mutate();
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockSafeNamespaceEngine{
		safeNamespace: "App\\SafeCaller",
	}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	if findings := v.Findings(); len(findings) > 0 {
		t.Errorf("expected 0 findings inside safe namespace caller, got %d: %v", len(findings), findings)
	}

	want := "App\\SafeCaller::run -> App\\TargetService::mutate"
	for _, call := range engine.recordedCalls {
		if call == want {
			return
		}
	}
	t.Errorf("expected recordedCalls to contain %q, got %v", want, engine.recordedCalls)
}

func TestPHPVisitor_StaticMutationAndDependencies(t *testing.T) {
	code := `<?php
class MyStaticService {
    private static $cache;
    public function set($v) {
        self::$cache = $v;
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

	// Test SetDependencies
	deps := []string{"App\\SomeDependency"}
	v.SetDependencies(deps)

	v.Walk(tree.RootNode())

	findings := v.Findings()
	for _, f := range findings {
		t.Logf("Found finding: Message=%q, Severity=%q, Deps=%v", f.Message, f.Severity, f.Dependencies)
	}

	if len(findings) == 0 {
		t.Fatal("Expected at least one finding for static state mutation, got 0")
	}

	found := false
	for _, f := range findings {
		// Use strings.Contains to be resilient to exact message format
		if strings.Contains(f.Message, "cache") {
			found = true
			// Verify dependency was attached
			if len(f.Dependencies) != 1 || f.Dependencies[0] != "App\\SomeDependency" {
				t.Errorf("Expected dependency 'App\\SomeDependency' in finding, got: %v", f.Dependencies)
			}
		}
	}

	if !found {
		t.Error("Expected a static state mutation finding for self::$cache")
	}
}

func TestPHPVisitor_ConstructorPromotionAndAttributes(t *testing.T) {
	code := `<?php
class AdvancedService {
    private string $normalProp;

    public function __construct(
        string $normalParam,
        #[WorkerSafe]
        private readonly string $promotedReadonlyProp,
        private string $promotedProp
    ) {
        $this->normalProp = $normalParam;
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

	var classNode *sitter.Node
	for i := uint(0); i < tree.RootNode().ChildCount(); i++ {
		child := tree.RootNode().Child(i)
		if child.Kind() == "class_declaration" {
			classNode = child
			break
		}
	}

	if classNode != nil {
		v.handleClass(classNode)
	}

	// Verify property types and declarations were extracted correctly
	if tType, ok := v.propertyTypes["promotedReadonlyProp"]; !ok || tType != "string" {
		t.Errorf("Expected promotedReadonlyProp to have type 'string', got: %s", tType)
	}
	if tType, ok := v.propertyTypes["promotedProp"]; !ok || tType != "string" {
		t.Errorf("Expected promotedProp to have type 'string', got: %s", tType)
	}
	if !v.declaredProps["promotedProp"] {
		t.Error("Expected promotedProp to be registered as declared")
	}
	if !v.readonlyProps["promotedReadonlyProp"] {
		t.Error("Expected promotedReadonlyProp to be registered as readonly")
	}
	if !v.workerSafeProps["promotedReadonlyProp"] {
		t.Error("Expected promotedReadonlyProp to be registered as WorkerSafe")
	}
}

func TestPHPVisitor_EdgeCases(t *testing.T) {
	// 1. Test resolveFQCN
	t.Run("resolveFQCN cases", func(t *testing.T) {
		v := &PHPVisitor{
			lines: []string{
				"use App\\Service\\FooService as Foo;",
				"use App\\Service\\BarService;",
			},
			namespace: "App\\Controller",
		}

		// Absolute FQCN
		if res := v.resolveFQCN("\\Some\\Absolute\\Class"); res != "Some\\Absolute\\Class" {
			t.Errorf("Expected Some\\Absolute\\Class, got %s", res)
		}

		// Alias import
		if res := v.resolveFQCN("Foo"); res != "App\\Service\\FooService" {
			t.Errorf("Expected App\\Service\\FooService, got %s", res)
		}

		// Direct import
		if res := v.resolveFQCN("BarService"); res != "App\\Service\\BarService" {
			t.Errorf("Expected App\\Service\\BarService, got %s", res)
		}

		// Fallback to namespace
		if res := v.resolveFQCN("MyController"); res != "App\\Controller\\MyController" {
			t.Errorf("Expected App\\Controller\\MyController, got %s", res)
		}

		// Fallback without namespace
		vNoNS := &PHPVisitor{}
		if res := vNoNS.resolveFQCN("MyClass"); res != "MyClass" {
			t.Errorf("Expected MyClass, got %s", res)
		}
	})

	// 2. Test getClassBody nil check
	t.Run("getClassBody with nil node", func(t *testing.T) {
		v := &PHPVisitor{}
		if res := v.getClassBody(nil); res != nil {
			t.Error("Expected nil body for nil class node")
		}
	})

	// 3. Test isSuperglobal for other variables
	t.Run("isSuperglobal other cases", func(t *testing.T) {
		if isSuperglobal("$this") {
			t.Error("Expected $this to not be a superglobal")
		}
		if !isSuperglobal("$_GET") {
			t.Error("Expected $_GET to be a superglobal")
		}
	})

	// 4. Test handleFunctionCall nil safety
	t.Run("handleFunctionCall nil safety", func(t *testing.T) {
		v := &PHPVisitor{}
		// If n has no children, ChildByFieldName returns nil, which should exit early
		code := `<?php ;`
		content := []byte(code)
		p := sitter.NewParser()
		_ = p.SetLanguage(sitter.NewLanguage(php.LanguagePHP()))
		tree := p.Parse(content, nil)
		defer tree.Close()

		// A non-function-call node passed to handleFunctionCall should just return safely
		v.handleFunctionCall(tree.RootNode())
	})
}

func TestPHPVisitor_DoctrineEntityManager_Lifecycle(t *testing.T) {
	code := `<?php
class DoctrineService {
    private \Doctrine\ORM\EntityManagerInterface $em;

    public function __construct(\Doctrine\ORM\EntityManagerInterface $em) {
        $this->em = $em;
    }

    public function testRemove($product) {
        $this->em->remove($product); // Safe, whitelisted direct call
    }

    public function testClear() {
        $this->em->clear(); // Safe, whitelisted direct call
    }

    public function testSetConfiguration($config) {
        $this->em->setConfiguration($config); // Unsafe: direct call but NOT on UnitOfWork whitelist
    }

    public function testChainedDisable() {
        $this->em->getFilters()->disable('softdeleteable'); // Unsafe: chained mutation call
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

	// We expect exactly 2 findings:
	// 1 for setConfiguration
	// 1 for disable ('softdeleteable') called in a chained way
	if len(findings) != 2 {
		t.Fatalf("Expected exactly 2 findings, got %d: %v", len(findings), findings)
	}

	foundSetConfiguration := false
	foundChainedDisable := false

	for _, f := range findings {
		if strings.Contains(f.Snippet, "setConfiguration") {
			foundSetConfiguration = true
		}
		if strings.Contains(f.Snippet, "disable") {
			foundChainedDisable = true
		}
	}

	if !foundSetConfiguration {
		t.Error("Expected a finding for setConfiguration on EntityManager")
	}
	if !foundChainedDisable {
		t.Error("Expected a finding for chained disable call on EntityManager filters")
	}
}

func TestPHPVisitor_SemanticTypeTracking(t *testing.T) {
	code := `<?php
class SuperService {
    private \App\Tracing\TracerInterface $tracer;
    private \App\Service\SomeSharedService $sharedService;

    public function __construct(\App\Tracing\TracerInterface $tracer, \App\Service\SomeSharedService $sharedService) {
        $this->tracer = $tracer;
        $this->sharedService = $sharedService;
    }

    public function testTransientSpan() {
        $span = $this->tracer->makeSpan();
        $span->setAttribute('key', 'value'); // Safe: Span is transient
    }

    public function testSharedServiceMutation() {
        $service = $this->sharedService->getSelf();
        $service->setSomeValue('abc'); // Unsafe: someSharedService is a shared service!
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{
		methodReturnTypes: map[string]string{
			"App\\Tracing\\TracerInterface::makeSpan":  "OpenTelemetry\\API\\Trace\\Span",
			"App\\Service\\SomeSharedService::getSelf": "App\\Service\\SomeSharedService",
		},
	}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()

	// We expect exactly 1 finding: for $service->setSomeValue('abc') since SomeSharedService is shared.
	// We expect 0 findings for $span->setAttribute('key', 'value') since Span is not shared.
	if len(findings) != 1 {
		t.Fatalf("Expected exactly 1 finding, got %d: %v", len(findings), findings)
	}

	if !strings.Contains(findings[0].Snippet, "setSomeValue") {
		t.Errorf("Expected finding to be on setSomeValue, got: %s", findings[0].Snippet)
	}
}

func TestPHPVisitor_ClassInheritance_DirectExtends(t *testing.T) {
	code := `<?php
namespace App\Service;

use Vendor\Lib\BaseService;

class ChildService extends BaseService
{
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

	got := engine.recordedParents["App\\Service\\ChildService"]
	want := "Vendor\\Lib\\BaseService"
	if got != want {
		t.Errorf("expected ChildService parent %q, got %q (all: %v)", want, got, engine.recordedParents)
	}
}

func TestPHPVisitor_ClassInheritance_MultiLevel(t *testing.T) {
	code := `<?php
namespace App\Service;

class GrandParentService
{
}

class ParentService extends GrandParentService
{
}

class ChildService extends ParentService
{
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

	if got := engine.recordedParents["App\\Service\\ChildService"]; got != "App\\Service\\ParentService" {
		t.Errorf("expected ChildService parent App\\Service\\ParentService, got %q", got)
	}
	if got := engine.recordedParents["App\\Service\\ParentService"]; got != "App\\Service\\GrandParentService" {
		t.Errorf("expected ParentService parent App\\Service\\GrandParentService, got %q", got)
	}
	if _, exists := engine.recordedParents["App\\Service\\GrandParentService"]; exists {
		t.Errorf("expected GrandParentService to have no recorded parent, got %v", engine.recordedParents)
	}
}

func TestPHPVisitor_ClassInheritance_NotRecordedForTraitsAnonymousOrImplementsOnly(t *testing.T) {
	code := `<?php
namespace App\Service;

trait MyTrait
{
}

interface MyInterface
{
}

class ImplementsOnly implements MyInterface
{
}

class AnonHost
{
    public function make()
    {
        return new class extends \ArrayObject {
        };
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

	if len(engine.recordedParents) != 0 {
		t.Errorf("expected no recorded parents for trait/anonymous/implements-only classes, got %v", engine.recordedParents)
	}
}

func TestPHPVisitor_MethodDeclared_OnlyClassMethodsNotFreeFunctions(t *testing.T) {
	code := `<?php
namespace App\Service;

class MyService
{
    public function doWork(): void {}
    private function helper(): void {}
}

function freeFunction(): void {}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	methods := engine.declaredMethods["App\\Service\\MyService"]
	if len(methods) != 2 {
		t.Fatalf("expected 2 declared methods for MyService, got %v", methods)
	}
	found := map[string]bool{}
	for _, m := range methods {
		found[m] = true
	}
	if !found["doWork"] || !found["helper"] {
		t.Errorf("expected doWork and helper declared, got %v", methods)
	}

	for class, methods := range engine.declaredMethods {
		for _, m := range methods {
			if m == "freeFunction" {
				t.Errorf("free function must not be recorded as a declared method (class=%q)", class)
			}
		}
	}
}

func TestPHPVisitor_Issue59_Fixes(t *testing.T) {
	code := `<?php
class DoctrineAndChainedDemo {
    private \Doctrine\ORM\EntityManager $em;
    private \App\Service\MyFactoryClass $factory;

    public function __construct(\Doctrine\ORM\EntityManager $em, \App\Service\MyFactoryClass $factory) {
        $this->em = $em;
        $this->factory = $factory;
    }

    public function testResettableLocalVariable() {
        $manager = $this->em;
        $manager->remove($product); // Safe: $manager has type EntityManager (resettable) and remove is a UnitOfWork lifecycle method
    }

    public function testResettableLocalVariableFromMethod() {
        $manager = $this->getEntityManager();
        $manager->remove($product); // Safe: $manager type resolved from getEntityManager() return type and is resettable
    }

    public function getEntityManager(): \Doctrine\ORM\EntityManager {
        return $this->em;
    }

    public function testChainedCallsOnNonSharedService() {
        // Safe: MyFactoryClass::wrap() returns MyWrapper (which contains "dto" and is not a shared service)
        $this->factory->wrap()->setAttribute('k', 'v'); 
    }

    public function testChainedCallsOnSharedService() {
        // Unsafe: MyFactoryClass::getSharedReceiver() returns AnotherSharedService (shared service!)
        $this->factory->getSharedReceiver()->setAttribute('k', 'v'); 
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockEngine{
		methodReturnTypes: map[string]string{
			"DoctrineAndChainedDemo::getEntityManager":        "Doctrine\\ORM\\EntityManager",
			"App\\Service\\MyFactoryClass::wrap":              "App\\ValueObject\\MyWrapperDto",
			"App\\Service\\MyFactoryClass::getSharedReceiver": "App\\Service\\AnotherSharedService",
		},
	}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()

	// We expect exactly 1 finding: for the chained call on the shared service.
	// The other three cases (resettable local variable, resettable from method call, and non-shared chained call) should pass!
	if len(findings) != 1 {
		t.Fatalf("Expected exactly 1 finding, got %d: %v", len(findings), findings)
	}

	if !strings.Contains(findings[0].Snippet, "getSharedReceiver()->setAttribute") {
		t.Errorf("Expected finding to be on getSharedReceiver()->setAttribute, got: %s", findings[0].Snippet)
	}
}

func TestPHPVisitor_DestructorDetection(t *testing.T) {
	code := `<?php
class SharedService {
    public function __destruct() {
        // Unsafe: shared service destructors are not invoked in persistent worker mode
    }
}

class TransientService {
    public function __destruct() {
        // Safe: transient/non-shared service destructors are invoked when they go out of scope
    }
}

#[WorkerSafe]
class SafeService {
    public function __destruct() {
        // Safe: marked as WorkerSafe
    }
}

class SafeMethodService {
    #[WorkerSafe]
    public function __destruct() {
        // Safe: marked as WorkerSafe
    }
}
`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	// Engine configuration where TransientService is explicitly non-shared
	engine := &mockEngine{}
	v := NewVisitor(content, engine)

	// Set TransientService as non-shared
	nonShared := make(NonSharedServiceMap)
	nonShared["TransientService"] = true
	v.SetNonSharedServices(nonShared)

	v.Walk(tree.RootNode())

	findings := v.Findings()

	// We expect exactly 1 finding: on SharedService::__destruct()
	if len(findings) != 1 {
		t.Fatalf("Expected exactly 1 finding, got %d: %v", len(findings), findings)
	}

	finding := findings[0]
	if !strings.Contains(finding.Message, "__destruct()") {
		t.Errorf("Expected message to mention __destruct(), got: %s", finding.Message)
	}
	if finding.Severity != "ERROR" {
		t.Errorf("Expected severity to be ERROR, got: %s", finding.Severity)
	}
	if !strings.Contains(finding.Remediation, "Destructors") {
		t.Errorf("Expected remediation to mention Destructors, got: %s", finding.Remediation)
	}
}

func TestPHPVisitor_DependencyInSafeNamespace_BypassesMutation(t *testing.T) {
	code := `<?php
class ServiceWithSafeDependency {
    private \Symfony\Component\HttpClient\CachingHttpClient $client;

    public function __construct(\Symfony\Component\HttpClient\CachingHttpClient $client) {
        $this->client = $client;
    }

    public function testMutate() {
        $this->client->setSomeOption('val'); // Safe because CachingHttpClient is in a safe namespace
    }
}`
	content := []byte(code)

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)
	tree := p.Parse(content, nil)
	defer tree.Close()

	engine := &mockSafeNamespaceEngine{
		safeNamespace: "Symfony\\Component\\HttpClient\\CachingHttpClient",
	}
	v := NewVisitor(content, engine)
	v.Walk(tree.RootNode())

	findings := v.Findings()
	if len(findings) > 0 {
		t.Errorf("Expected 0 findings when dependency is in a safe namespace, got %d: %v", len(findings), findings)
	}
}
