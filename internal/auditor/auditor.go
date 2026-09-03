// Package main provides the core auditing logic for igor-php.
package auditor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/igor-php/igor-php/internal/analyzer"
	"github.com/igor-php/igor-php/internal/config"
	"github.com/igor-php/igor-php/pkg/symbol"
	sitter "github.com/tree-sitter/go-tree-sitter"
	php "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

// Auditor orchestrates the analysis of PHP files.
type Auditor struct {
	Config               config.Config
	Symfony              *SymfonyBridge
	NonSharedServices    analyzer.NonSharedServiceMap
	InterfaceAliases     analyzer.AliasesMap
	AuditedClasses       map[string]bool
	methodSignatureCache map[string]map[string]string
	callGraph            map[string]map[string]bool
	projectClasses       map[string]bool
	classParents         map[string]string
	declaredMethods      map[string]map[string]bool
	parentClassCache     map[string]string
	mu                   sync.Mutex
}

// NewAuditor creates a new instance of the Auditor.
func NewAuditor(cfg config.Config) *Auditor {
	return &Auditor{
		Config:               cfg,
		AuditedClasses:       make(map[string]bool),
		methodSignatureCache: make(map[string]map[string]string),
		callGraph:            make(map[string]map[string]bool),
		projectClasses:       make(map[string]bool),
		classParents:         make(map[string]string),
		declaredMethods:      make(map[string]map[string]bool),
		parentClassCache:     make(map[string]string),
	}
}

// LoadInterfaceAliases normalizes raw interface-to-target mappings and stores
// them as interface FQCN -> concrete class FQCN. When a Symfony container is
// available, service IDs and alias chains are resolved to their concrete class.
func (a *Auditor) LoadInterfaceAliases(raw analyzer.AliasesMap) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.InterfaceAliases == nil {
		a.InterfaceAliases = make(analyzer.AliasesMap)
	}

	for iface, target := range raw {
		iface = normalizeClassName(iface)
		target = normalizeClassName(target)
		if iface == "" || target == "" {
			continue
		}
		a.InterfaceAliases[iface] = a.resolveAliasToConcrete(target)
	}
}

// LoadSymfonyAliases converts the Symfony container alias map into the unified
// interface alias map used by reachability ranking.
func (a *Auditor) LoadSymfonyAliases() {
	if a.Symfony == nil || a.Symfony.Container == nil {
		return
	}

	raw := make(analyzer.AliasesMap)
	for aliasKey, val := range a.Symfony.Container.Aliases {
		if target := resolveAliasTarget(val); target != "" {
			raw[aliasKey] = target
		}
	}
	a.LoadInterfaceAliases(raw)
}

// resolveInterfaceClass returns the concrete class for an interface FQCN when
// an alias is known. It returns an empty string when no mapping exists.
func (a *Auditor) resolveInterfaceClass(className string) string {
	if concrete, ok := a.InterfaceAliases[normalizeClassName(className)]; ok {
		return concrete
	}
	return ""
}

// resolveAliasToConcrete follows Symfony aliases and definitions until it
// reaches a concrete class. For non-Symfony projects it returns the target as-is.
func (a *Auditor) resolveAliasToConcrete(target string) string {
	if a.Symfony == nil || a.Symfony.Container == nil {
		return target
	}

	possibleIDs := a.resolveAliases(target)
	for _, id := range possibleIDs {
		for defID, def := range a.Symfony.Container.Definitions {
			normDefID := normalizeClassName(defID)
			normDefClass := normalizeClassName(def.Class)
			if id == normDefID || id == normDefClass {
				if def.Class != "" {
					return normDefClass
				}
				return normDefID
			}
		}
	}
	return target
}

type fileEngine struct {
	*Auditor
	isVendor bool
}

func (f *fileEngine) RecordClassAudited(name string) {
	f.Auditor.RecordClassAudited(name)
	if !f.isVendor {
		f.Auditor.recordProjectClass(name)
	}
}

func (a *Auditor) recordProjectClass(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.projectClasses[normalizeClassName(name)] = true
}

func (a *Auditor) RecordMethodCall(callerClass, callerMethod, calleeClass, calleeMethod string) {
	if callerMethod == "" || calleeMethod == "" {
		return
	}
	callerKey := normalizeClassName(callerClass) + "::" + callerMethod
	calleeKey := normalizeClassName(calleeClass) + "::" + calleeMethod

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.callGraph[callerKey] == nil {
		a.callGraph[callerKey] = make(map[string]bool)
	}
	a.callGraph[callerKey][calleeKey] = true
}

// RecordClassParent records a direct `extends` relationship (childClass extends
// parentClass), used to conservatively promote reachability to inherited methods.
func (a *Auditor) RecordClassParent(className, parentClassName string) {
	if className == "" || parentClassName == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.classParents[normalizeClassName(className)] = normalizeClassName(parentClassName)
}

// RecordMethodDeclared records that methodName has its own declaration (body) on
// className, distinguishing an override from an inherited, un-overridden method.
func (a *Auditor) RecordMethodDeclared(className, methodName string) {
	if className == "" || methodName == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	key := normalizeClassName(className)
	if a.declaredMethods[key] == nil {
		a.declaredMethods[key] = make(map[string]bool)
	}
	a.declaredMethods[key][methodName] = true
}

// resolveDeclaringAncestor walks the direct-`extends` parent chain starting at
// class, looking for the nearest ancestor (including class itself) that
// directly declares methodName. It stops as soon as it finds a declaration —
// so an override on a subclass is never skipped in favor of a grandparent's
// declaration — and guards against cycles in a malformed hierarchy. Returns
// "" if no class in the chain declares the method.
func (a *Auditor) resolveDeclaringAncestor(class, method string) string {
	visited := make(map[string]bool)
	for class != "" && !visited[class] {
		if a.declaredMethods[class][method] {
			return class
		}
		visited[class] = true
		class = a.classParents[class]
	}
	return ""
}

func (a *Auditor) MarkReachability(results []symbol.AuditStatus) {
	a.mu.Lock()
	graph := a.callGraph
	projectClasses := a.projectClasses
	a.mu.Unlock()

	reachableNodes := make(map[string]bool)
	var queue []string
	enqueue := func(node string) {
		if !reachableNodes[node] {
			reachableNodes[node] = true
			queue = append(queue, node)
		}
	}

	for callerKey := range graph {
		callerClass, _, found := strings.Cut(callerKey, "::")
		if found && projectClasses[callerClass] {
			enqueue(callerKey)
		}
	}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		for callee := range graph[node] {
			enqueue(callee)
		}

		class, method, found := strings.Cut(node, "::")
		if !found {
			continue
		}
		if concrete := a.resolveInterfaceClass(class); concrete != "" && concrete != class {
			enqueue(concrete + "::" + method)
		}
		if ancestor := a.resolveDeclaringAncestor(class, method); ancestor != "" && ancestor != class {
			enqueue(ancestor + "::" + method)
		}
	}

	for i := range results {
		for j := range results[i].Findings {
			finding := &results[i].Findings[j]
			if finding.ContextClass == "" || finding.ContextMethod == "" {
				continue
			}
			findingKey := normalizeClassName(finding.ContextClass) + "::" + finding.ContextMethod
			if reachableNodes[findingKey] {
				finding.Reachability = "HIGH"
			} else {
				finding.Reachability = "INFO"
			}
		}
	}
}

// Audit analyzes a single PHP file and returns findings.
func (a *Auditor) Audit(path string, dependencies []string) ([]symbol.Finding, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", path, err)
	}

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	if err := p.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("failed to set language for %s: %v", path, err)
	}

	tree := p.Parse(content, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse %s", path)
	}
	defer tree.Close()

	v := analyzer.NewVisitor(content, &fileEngine{
		Auditor:  a,
		isVendor: isVendorPath(a.Config.NormalizePath(path)),
	})
	v.SetDependencies(dependencies)
	v.SetNonSharedServices(a.NonSharedServices)
	v.Walk(tree.RootNode())

	return v.Findings(), nil
}

func isVendorPath(path string) bool {
	p := filepath.ToSlash(path)
	if strings.Contains(p, "/vendor/") || strings.HasPrefix(p, "vendor/") {
		return true
	}
	if abs, err := filepath.Abs(path); err == nil {
		if strings.Contains(filepath.ToSlash(abs), "/vendor/") {
			return true
		}
	}
	return false
}

// ExtractFQCN extracts the full class name from a file.
func (a *Auditor) ExtractFQCN(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)

	tree := p.Parse(content, nil)
	if tree == nil {
		return "", fmt.Errorf("failed to parse %s", path)
	}
	defer tree.Close()

	var namespace, className string
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil {
			return
		}
		switch n.Kind() {
		case "namespace_definition":
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				namespace = string(content[nameNode.StartByte():nameNode.EndByte()])
			}
		case "class_declaration", "trait_declaration":
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				className = string(content[nameNode.StartByte():nameNode.EndByte()])
			}
		}
		if className != "" {
			return
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(tree.RootNode())

	if className == "" {
		return "", nil
	}
	if namespace == "" {
		return className, nil
	}
	return namespace + "\\" + className, nil
}

func (a *Auditor) RecordClassAudited(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.AuditedClasses[name] = true
}

func (a *Auditor) IsSafeNamespace(className string) bool {
	className = strings.TrimPrefix(className, "\\")
	for _, ns := range a.Config.SafeNamespaces {
		if strings.HasPrefix(className, strings.TrimPrefix(ns, "\\")) {
			return true
		}
	}
	return false
}

// IsDataPath returns true if the file path belongs to a directory that usually contains only data (Entity, DTO, etc.)
func (a *Auditor) IsDataPath(path string) bool {
	dataFolders := []string{"Entity", "DTO", "Dto", "ApiResource", "Migrations", "Document", "tests", "Tests"}
	for _, folder := range dataFolders {
		if strings.Contains(path, string(os.PathSeparator)+folder+string(os.PathSeparator)) ||
			strings.HasSuffix(filepath.Dir(path), string(os.PathSeparator)+folder) {
			return true
		}
	}
	return false
}

func normalizeClassName(s string) string {
	s = strings.ReplaceAll(s, "/", "\\")
	s = strings.TrimSpace(s)
	return strings.TrimPrefix(s, "\\")
}

func resolveAliasTarget(val interface{}) string {
	if s, ok := val.(string); ok {
		return s
	}
	if m, ok := val.(map[string]interface{}); ok {
		for _, key := range []string{"service", "id", "target"} {
			if target, ok := m[key].(string); ok {
				return target
			}
		}
	}
	return ""
}

func (a *Auditor) resolveAliases(className string) []string {
	if a.Symfony == nil || a.Symfony.Container == nil {
		return []string{normalizeClassName(className)}
	}

	normClassName := normalizeClassName(className)
	possibleIDs := []string{normClassName}
	visited := map[string]bool{normClassName: true}
	current := normClassName

	for {
		var foundTarget string
		for aliasKey, val := range a.Symfony.Container.Aliases {
			if normalizeClassName(aliasKey) == current {
				target := resolveAliasTarget(val)
				if target != "" {
					foundTarget = normalizeClassName(target)
					break
				}
			}
		}

		if foundTarget != "" {
			current = foundTarget
			if !visited[current] {
				visited[current] = true
				possibleIDs = append(possibleIDs, current)
				continue
			}
		}
		break
	}

	return possibleIDs
}

func (a *Auditor) IsExplicitlyNonShared(className string) bool {
	if a.Symfony == nil || a.Symfony.Container == nil {
		return false
	}
	possibleIDs := a.resolveAliases(className)

	for defID, def := range a.Symfony.Container.Definitions {
		normDefID := normalizeClassName(defID)
		normDefClass := normalizeClassName(def.Class)

		for _, id := range possibleIDs {
			if normDefID == id || normDefClass == id {
				return !def.Shared
			}
		}
	}
	return false
}

func (a *Auditor) IsResettable(className string) bool {
	if a.Symfony == nil || a.Symfony.Container == nil {
		return false
	}
	possibleIDs := a.resolveAliases(className)

	// Check if any of possibleIDs corresponds to a Doctrine Manager
	isDoctrine := false
	for _, id := range possibleIDs {
		lower := strings.ToLower(id)
		if strings.Contains(lower, "doctrine\\orm\\entitymanager") ||
			strings.Contains(lower, "doctrine\\persistence\\objectmanager") ||
			strings.Contains(lower, "doctrine\\odm\\mongodb\\documentmanager") {
			isDoctrine = true
			break
		}
	}

	if isDoctrine {
		if def, exists := a.Symfony.Container.Definitions["doctrine"]; exists && def.IsResettable() {
			return true
		}
		if def, exists := a.Symfony.Container.Definitions["doctrine_mongodb"]; exists && def.IsResettable() {
			return true
		}
	}

	for defID, def := range a.Symfony.Container.Definitions {
		normDefID := normalizeClassName(defID)
		normDefClass := normalizeClassName(def.Class)

		for _, id := range possibleIDs {
			if normDefID == id || normDefClass == id {
				if def.IsResettable() {
					return true
				}
			}
		}
	}
	return false
}

// IsSharedService checks if a class name is a shared service in Symfony.
func (a *Auditor) IsSharedService(className string) bool {
	if a.Symfony == nil {
		return true
	}
	return a.Symfony.IsSharedService(className)
}

// IsDevPackagePath returns true if the file path belongs to a dev package in vendor/.
func (a *Auditor) IsDevPackagePath(path string) bool {
	// Convert to slash for cross-platform comparison
	path = filepath.ToSlash(path)
	for _, pkg := range a.Config.DevPackages {
		vendorPath := "vendor/" + pkg + "/"
		if strings.Contains(path, vendorPath) {
			return true
		}
	}
	return false
}

// GetMethodReturnType returns the declared return type of a method on a class (including inherited methods from parent classes).
func (a *Auditor) GetMethodReturnType(className, methodName string) string {
	return a.getMethodReturnTypeWithDepth(className, methodName, make(map[string]bool))
}

func (a *Auditor) getMethodReturnTypeWithDepth(className, methodName string, visited map[string]bool) string {
	normClass := normalizeClassName(className)
	if visited[normClass] {
		return ""
	}
	visited[normClass] = true

	a.mu.Lock()
	// Check if present in cache
	if classCache, exists := a.methodSignatureCache[normClass]; exists {
		if retType, found := classCache[methodName]; found && retType != "" {
			a.mu.Unlock()
			return retType
		}
		if parentClass, hasParent := a.parentClassCache[normClass]; hasParent && parentClass != "" {
			a.mu.Unlock()
			return a.getMethodReturnTypeWithDepth(parentClass, methodName, visited)
		}
		a.mu.Unlock()
		return ""
	}
	a.mu.Unlock()

	// Not in cache, try to locate file
	if a.Symfony == nil || a.Symfony.ClassToFile == nil {
		return ""
	}

	filePath, exists := a.Symfony.ClassToFile[className]
	if !exists {
		// Try normalized lookup
		for k, v := range a.Symfony.ClassToFile {
			if normalizeClassName(k) == normClass {
				filePath = v
				exists = true
				break
			}
		}
	}

	if !exists {
		return ""
	}

	// Parse class methods
	signatures, parentClass, err := a.parseClassMethodSignatures(className, filePath)

	a.mu.Lock()
	if err != nil {
		// Cache empty map to avoid re-parsing on error
		a.methodSignatureCache[normClass] = make(map[string]string)
		a.mu.Unlock()
		return ""
	}

	a.methodSignatureCache[normClass] = signatures
	if parentClass != "" {
		a.parentClassCache[normClass] = parentClass
	}
	a.mu.Unlock()

	if retType, found := signatures[methodName]; found && retType != "" {
		return retType
	}

	if parentClass != "" {
		return a.getMethodReturnTypeWithDepth(parentClass, methodName, visited)
	}

	return ""
}

func isBuiltinType(t string) bool {
	switch strings.ToLower(t) {
	case "void", "int", "string", "bool", "float", "array", "callable", "object", "mixed", "never", "false", "null":
		return true
	}
	return false
}

func parseMethodDeclaration(node *sitter.Node, content []byte, namespace, className string) (string, string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return "", ""
	}
	mName := string(content[nameNode.StartByte():nameNode.EndByte()])
	retNode := node.ChildByFieldName("return_type")
	var retType string
	if retNode != nil {
		retType = string(content[retNode.StartByte():retNode.EndByte()])
		retType = strings.TrimSpace(retType)
		retType = strings.TrimPrefix(retType, "?")
		retType = strings.TrimPrefix(retType, "\\")

		// Resolve self and static
		if retType == "self" || retType == "static" {
			retType = className
		} else if namespace != "" && !strings.Contains(retType, "\\") && !isBuiltinType(retType) {
			retType = namespace + "\\" + retType
		}
	}
	return mName, retType
}

func (a *Auditor) parseClassMethodSignatures(className, filePath string) (map[string]string, string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", err
	}

	p := sitter.NewParser()
	lang := sitter.NewLanguage(php.LanguagePHP())
	_ = p.SetLanguage(lang)

	tree := p.Parse(content, nil)
	if tree == nil {
		return nil, "", fmt.Errorf("failed to parse %s", filePath)
	}
	defer tree.Close()

	signatures := make(map[string]string)
	var namespace string
	var parentClass string

	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil {
			return
		}
		kind := n.Kind()
		if kind == "namespace_definition" {
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				namespace = string(content[nameNode.StartByte():nameNode.EndByte()])
			}
		}

		if kind == "class_declaration" || kind == "interface_declaration" {
			var declName string
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				declName = string(content[nameNode.StartByte():nameNode.EndByte()])
			}

			fqcn := declName
			if namespace != "" {
				fqcn = namespace + "\\" + declName
			}

			if normalizeClassName(fqcn) == normalizeClassName(className) {
				parentClass = resolveParentClassFromNode(n, content, namespace)

				// Walk children to find method_declarations
				var walkClassBody func(*sitter.Node)
				walkClassBody = func(bodyNode *sitter.Node) {
					if bodyNode == nil {
						return
					}
					if bodyNode.Kind() == "method_declaration" {
						if mName, retType := parseMethodDeclaration(bodyNode, content, namespace, className); mName != "" {
							signatures[mName] = retType
						}
					}
					for i := uint(0); i < bodyNode.ChildCount(); i++ {
						walkClassBody(bodyNode.Child(i))
					}
				}
				walkClassBody(n)
			}
		}

		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(tree.RootNode())

	return signatures, parentClass, nil
}

// resolveParentClassFromNode extracts and resolves the parent class name from a class declaration node.
func resolveParentClassFromNode(n *sitter.Node, content []byte, namespace string) string {
	var parentClass string
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child.Kind() == "base_clause" {
			for j := uint(0); j < child.ChildCount(); j++ {
				grandChild := child.Child(j)
				if grandChild.Kind() == "name" {
					parentClass = string(content[grandChild.StartByte():grandChild.EndByte()])
					break
				}
			}
			break
		}
	}

	if parentClass != "" {
		lines := strings.Split(string(content), "\n")
		return resolveFQCNInFile(parentClass, lines, namespace)
	}
	return ""
}

// resolveFQCNInFile resolves class names against the namespace and "use" imports inside a file's lines.
func resolveFQCNInFile(typeName string, lines []string, namespace string) string {
	if strings.HasPrefix(typeName, "\\") {
		return strings.TrimPrefix(typeName, "\\")
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "use ") && strings.HasSuffix(line, ";") {
			importPart := strings.TrimSuffix(strings.TrimPrefix(line, "use "), ";")
			importPart = strings.TrimSpace(importPart)
			if strings.Contains(importPart, " as ") {
				parts := strings.Split(importPart, " as ")
				if len(parts) == 2 {
					fqcn := strings.TrimSpace(parts[0])
					alias := strings.TrimSpace(parts[1])
					if alias == typeName {
						return fqcn
					}
				}
			} else {
				parts := strings.Split(importPart, "\\")
				lastPart := parts[len(parts)-1]
				if lastPart == typeName {
					return importPart
				}
			}
		}
	}

	if namespace != "" {
		return namespace + "\\" + typeName
	}
	return typeName
}
