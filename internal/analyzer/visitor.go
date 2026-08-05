package analyzer

import (
	"fmt"
	"strings"

	"github.com/igor-php/igor-php/pkg/symbol"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Engine defines the required interface for the auditor to interact with the visitor.
type Engine interface {
	RecordClassAudited(name string)
	IsExplicitlyNonShared(className string) bool
	IsSafeNamespace(className string) bool
	IsResettable(className string) bool
}

type mutationInfo struct {
	line       int
	code       string
	snippet    string
	astDetails string
}

// PHPVisitor analyzes a single PHP file using tree-sitter.
type PHPVisitor struct {
	content            []byte
	lines              []string
	findings           []symbol.Finding
	curClass           string
	namespace          string
	curMethod          string
	isReset            bool
	isReadonlyClass    bool
	isWorkerSafeClass  bool
	isWorkerSafeMethod bool
	readonlyProps      map[string]bool
	workerSafeProps    map[string]bool
	propertyTypes      map[string]string
	declaredProps      map[string]bool
	finallyCleaned     map[string]bool
	mutated            map[string]mutationInfo
	resetted           map[string]bool
	engine             Engine
	dependencies       []string
	nonSharedServices  NonSharedServiceMap
	localSharedVars    map[string]bool
}

// NewVisitor creates a new instance of the PHPVisitor.
func NewVisitor(content []byte, engine Engine) *PHPVisitor {
	return &PHPVisitor{
		content:         content,
		lines:           strings.Split(string(content), "\n"),
		mutated:         make(map[string]mutationInfo),
		resetted:        make(map[string]bool),
		readonlyProps:   make(map[string]bool),
		workerSafeProps: make(map[string]bool),
		propertyTypes:   make(map[string]string),
		declaredProps:   make(map[string]bool),
		finallyCleaned:  make(map[string]bool),
		localSharedVars: make(map[string]bool),
		engine:          engine,
	}
}

func (v *PHPVisitor) SetDependencies(deps []string) {
	v.dependencies = deps
}

// SetNonSharedServices supplies the generic container-dump lookup of transient
// (shared: false) classes. Mutations inside such classes are skipped, mirroring
// the Symfony bridge's IsExplicitlyNonShared() signal for non-Symfony frameworks.
func (v *PHPVisitor) SetNonSharedServices(m NonSharedServiceMap) {
	v.nonSharedServices = m
}

func (v *PHPVisitor) Walk(n *sitter.Node) {
	v.walk(n)
}

func (v *PHPVisitor) Findings() []symbol.Finding {
	return v.findings
}

func (v *PHPVisitor) walk(n *sitter.Node) {
	if n == nil {
		return
	}
	nodeType := n.Kind()

	oldClass, oldMethod, oldIsRes, oldIsReadonly, oldReadonlyProps, oldDeclaredProps, oldIsWorkerSafeClass, oldIsWorkerSafeMethod, oldFinallyCleaned := v.curClass, v.curMethod, v.isReset, v.isReadonlyClass, v.readonlyProps, v.declaredProps, v.isWorkerSafeClass, v.isWorkerSafeMethod, v.finallyCleaned

	switch nodeType {
	case "namespace_definition":
		v.handleNamespace(n)
	case "class_declaration", "trait_declaration", "anonymous_class":
		v.handleClass(n)
	case "method_declaration", "function_definition":
		if nameNode := n.ChildByFieldName("name"); nameNode != nil {
			v.curMethod = v.getContent(nameNode)
		}
		v.isWorkerSafeMethod = v.hasAttribute(n, "WorkerSafe")
		v.finallyCleaned = make(map[string]bool)
		v.localSharedVars = make(map[string]bool)
		v.preScanFinallyCleanups(n)
	case "assignment_expression", "augmented_assignment_expression":
		v.handleAssignment(n)
	case "update_expression":
		v.handleMutation(n)
	case "unset_statement":
		v.handleUnset(n)
	case "exit_statement", "exit":
		v.addFinding(n, "Usage of exit/die is forbidden in Worker mode.", "Use Symfony response or exceptions instead.", "ERROR")
	case "function_call_expression":
		v.handleFunctionCall(n)
	case "member_call_expression":
		v.handleMethodCall(n)
	case "variable_name":
		v.handleVariable(n)
	case "static_variable_declaration":
		v.addFinding(n, "Usage of local static variable is dangerous in Worker mode.", "Static variables persist across requests.", "ERROR")
	}

	for i := uint(0); i < n.ChildCount(); i++ {
		v.walk(n.Child(i))
	}

	switch nodeType {
	case "class_declaration", "trait_declaration", "anonymous_class":
		if v.isReset {
			v.performResetCheck()
		}
		v.curClass, v.isReset, v.isReadonlyClass, v.readonlyProps, v.declaredProps, v.isWorkerSafeClass = oldClass, oldIsRes, oldIsReadonly, oldReadonlyProps, oldDeclaredProps, oldIsWorkerSafeClass
	case "method_declaration", "function_definition":
		v.curMethod, v.isWorkerSafeMethod, v.finallyCleaned = oldMethod, oldIsWorkerSafeMethod, oldFinallyCleaned
	}
}

func (v *PHPVisitor) handleNamespace(n *sitter.Node) {
	if nameNode := n.ChildByFieldName("name"); nameNode != nil {
		v.namespace = v.getContent(nameNode)
	}
}

func (v *PHPVisitor) handleClass(n *sitter.Node) {
	if nameNode := n.ChildByFieldName("name"); nameNode != nil {
		v.curClass = v.getContent(nameNode)
	} else {
		v.curClass = "AnonymousClass"
	}

	fullName := v.curClass
	if v.namespace != "" {
		fullName = v.namespace + "\\" + v.curClass
	}

	if v.engine != nil {
		v.engine.RecordClassAudited(fullName)
	}

	classText := strings.ToLower(string(v.content[n.StartByte():n.EndByte()]))
	headerEnd := strings.Index(classText, "{")
	if headerEnd == -1 {
		headerEnd = len(classText)
	}
	classHeader := classText[:headerEnd]

	v.isReset = strings.Contains(classHeader, "resetinterface") || strings.Contains(classHeader, "resettableinterface")
	if !v.isReset && v.engine != nil {
		if v.engine.IsResettable(fullName) {
			v.isReset = true
		}
	}
	v.isReadonlyClass = strings.Contains(classHeader, "readonly")

	v.mutated = make(map[string]mutationInfo)
	v.resetted = make(map[string]bool)
	v.readonlyProps = make(map[string]bool)
	v.workerSafeProps = make(map[string]bool)
	v.propertyTypes = make(map[string]string)
	v.declaredProps = make(map[string]bool)
	v.isWorkerSafeClass = v.hasAttribute(n, "WorkerSafe")

	v.scanReadonlyProps(n)
	v.scanPropertyTypes(n)
}

func (v *PHPVisitor) scanReadonlyProps(classNode *sitter.Node) {
	body := classNode.ChildByFieldName("body")
	if body == nil {
		return
	}

	for i := uint(0); i < body.ChildCount(); i++ {
		member := body.Child(i)
		// 1. Regular property declarations
		if member.Kind() == "property_declaration" {
			v.scanPropertyNode(member)
		}
		// 2. Constructor promotion
		if member.Kind() == "method_declaration" {
			nameNode := member.ChildByFieldName("name")
			if nameNode != nil && strings.ToLower(v.getContent(nameNode)) == "__construct" {
				params := member.ChildByFieldName("parameters")
				if params != nil {
					for j := uint(0); j < params.ChildCount(); j++ {
						param := params.Child(j)
						if param.Kind() == "parameter_declaration" || param.Kind() == "property_promotion_parameter" {
							v.scanPropertyNode(param)
						}
					}
				}
			}
		}
	}
}

func (v *PHPVisitor) scanPropertyNode(n *sitter.Node) {
	isReadonly := false
	// Check for readonly modifier
	for j := uint(0); j < n.ChildCount(); j++ {
		child := n.Child(j)
		if (child.Kind() == "modifier" || child.Kind() == "readonly_modifier") && strings.Contains(v.getContent(child), "readonly") {
			isReadonly = true
			break
		}
	}

	isWorkerSafe := v.hasAttribute(n, "WorkerSafe")

	if isReadonly || isWorkerSafe {
		// For property_declaration, properties are in property_element
		// For parameter_declaration/property_promotion_parameter, look for variable_name child
		if n.Kind() == "property_declaration" {
			for j := uint(0); j < n.ChildCount(); j++ {
				child := n.Child(j)
				if child.Kind() == "property_element" {
					nameNode := child.ChildByFieldName("name")
					if nameNode != nil {
						propName := strings.TrimPrefix(v.getContent(nameNode), "$")
						if isReadonly {
							v.readonlyProps[propName] = true
						}
						if isWorkerSafe {
							v.workerSafeProps[propName] = true
						}
					}
				}
			}
		} else {
			// Find the variable_name child
			for j := uint(0); j < n.ChildCount(); j++ {
				child := n.Child(j)
				if child.Kind() == "variable_name" {
					propName := strings.TrimPrefix(v.getContent(child), "$")
					if isReadonly {
						v.readonlyProps[propName] = true
					}
					if isWorkerSafe {
						v.workerSafeProps[propName] = true
					}
					break
				}
			}
		}
	}
}

func (v *PHPVisitor) handleUnset(n *sitter.Node) {
	if v.curMethod != "reset" {
		return
	}

	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child.Kind() == "member_access_expression" {
			obj := child.ChildByFieldName("object")
			if obj != nil && strings.Contains(v.getContent(obj), "$this") {
				nameNode := child.ChildByFieldName("name")
				if nameNode != nil {
					prop := v.getContent(nameNode)
					v.resetted[prop] = true
				}
			}
		}
	}
}

func (v *PHPVisitor) handleFunctionCall(n *sitter.Node) {
	nameNode := n.ChildByFieldName("function")
	if nameNode == nil {
		nameNode = n.ChildByFieldName("name")
	}

	if nameNode == nil {
		return
	}

	name := strings.ToLower(v.getContent(nameNode))
	switch name {
	case "date_default_timezone_set", "ini_set", "setlocale", "error_reporting", "putenv":
		msg := fmt.Sprintf("Function '%s' modifies the global PHP process state.", name)
		hint := "This change will persist across requests in Worker mode and might affect other users."
		v.addFinding(n, msg, hint, "WARNING")
	}
}

func (v *PHPVisitor) handleVariable(n *sitter.Node) {
	name := v.getContent(n)
	if isSuperglobal(name) {
		v.addFinding(n, fmt.Sprintf("Usage of PHP Superglobal %s is discouraged in Worker mode.", name), "Use the Symfony Request object ($request->query, $request->request, etc.) instead.", "WARNING")
	}
}

func isSuperglobal(name string) bool {
	switch name {
	case "$_GET", "$_POST", "$_SESSION", "$_SERVER", "$_FILES", "$_COOKIE", "$_REQUEST", "$_ENV":
		return true
	}
	return false
}

func (v *PHPVisitor) handleMutation(n *sitter.Node) {
	if n == nil || v.isReadonlyClass || v.curMethod == "__construct" || (v.curMethod == "" && v.curClass != "AnonymousClass" && v.curClass != "") {
		return
	}

	fullName := v.curClass
	if v.namespace != "" {
		fullName = v.namespace + "\\" + v.curClass
	}

	// Generic container-dump bypass: the class is registered as a transient
	// (shared: false) service, so its mutations never outlive a request.
	if v.nonSharedServices[strings.TrimPrefix(fullName, "\\")] {
		return
	}

	if v.engine != nil && (v.engine.IsExplicitlyNonShared(fullName) || v.engine.IsSafeNamespace(fullName)) {
		return
	}

	switch n.Kind() {
	case "member_access_expression":
		v.handleMemberAccess(n)
	case "subscript_expression":
		if n.ChildCount() > 0 {
			v.handleMutation(n.Child(0))
		}
	case "scoped_property_access_expression":
		v.handleScopedAccess(n)
	case "update_expression":
		for i := uint(0); i < n.ChildCount(); i++ {
			c := n.Child(i)
			if c.Kind() != "++" && c.Kind() != "--" {
				v.handleMutation(c)
			}
		}
	}
}

func (v *PHPVisitor) handleMemberAccess(n *sitter.Node) {
	obj := n.ChildByFieldName("object")
	if obj == nil {
		return
	}

	objContent := v.getContent(obj)
	switch {
	case strings.Contains(objContent, "$this"):
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			v.logMutation(n, v.getContent(nameNode), false)
		}
	case obj.Kind() == "member_access_expression" || obj.Kind() == "subscript_expression":
		v.handleMutation(obj)
	}
}

// handleAssignment handles variable assignments and tracks variables holding references to shared dependencies/services.
func (v *PHPVisitor) handleAssignment(n *sitter.Node) {
	if n == nil {
		return
	}

	left := n.ChildByFieldName("left")
	right := n.ChildByFieldName("right")

	// Standard mutation detection on left side
	if left != nil {
		v.handleMutation(left)
	}

	// Dynamic local shared variable tracking
	if left != nil && right != nil {
		leftContent := v.getContent(left)
		rightContent := v.getContent(right)

		// If the left side is a simple variable (e.g. $entityManager)
		if strings.HasPrefix(leftContent, "$") && !strings.Contains(leftContent, "->") && !strings.Contains(leftContent, "[") {
			leftLower := strings.ToLower(leftContent)
			rightLower := strings.ToLower(rightContent)

			if v.isTransientOrEphemeral(leftLower, rightLower, rightContent, right) {
				return
			}

			if v.isRightSideShared(rightContent) {
				v.localSharedVars[leftContent] = true
			}
		}
	}
}

func (v *PHPVisitor) isTransientOrEphemeral(leftLower, rightLower, rightContent string, right *sitter.Node) bool {
	// Heuristic 1: Instantiations using the 'new' keyword are always fresh/transient objects, never shared
	if right.Kind() == "new_expression" || strings.HasPrefix(strings.TrimSpace(rightContent), "new ") {
		return true
	}

	// Heuristic 2: Ephemeral database QueryBuilders and Expressions are query-scoped, never shared across requests
	if strings.Contains(rightLower, "querybuilder") ||
		strings.Contains(rightLower, "->expr(") ||
		strings.Contains(rightLower, "->orx(") ||
		strings.Contains(rightLower, "->andx(") ||
		strings.Contains(leftLower, "querybuilder") ||
		strings.Contains(leftLower, "expr") {
		return true
	}

	// Heuristic 3: Known factory, builder and safe transient infrastructure methods
	return strings.Contains(rightLower, "->find") ||
		strings.Contains(rightLower, "->create") ||
		strings.Contains(rightLower, "->build") ||
		strings.Contains(rightLower, "->getitem")
}

func (v *PHPVisitor) isRightSideShared(rightContent string) bool {
	// Scenario 1: RHS contains $this or self/static (which represents properties, methods or dependencies of the shared service class)
	if strings.Contains(rightContent, "$this") || strings.Contains(strings.ToLower(rightContent), "self::") || strings.Contains(strings.ToLower(rightContent), "static::") {
		return true
	}

	// Scenario 2: RHS contains another already tracked local shared variable
	for trackedVar := range v.localSharedVars {
		if strings.Contains(rightContent, trackedVar) {
			return true
		}
	}

	return false
}

func (v *PHPVisitor) handleScopedAccess(n *sitter.Node) {
	scope := n.ChildByFieldName("scope")
	if scope != nil {
		s := strings.ToLower(v.getContent(scope))
		if s == "self" || s == "static" {
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				v.logMutation(n, v.getContent(nameNode), true)
			}
		}
	}
}

func (v *PHPVisitor) logMutation(n *sitter.Node, prop string, static bool) {
	key := prop
	if static {
		key = "static::" + prop
	}

	// Skip if property is readonly, WorkerSafe, or cleaned up in a finally block
	if !static && (v.readonlyProps[prop] || v.workerSafeProps[prop] || v.finallyCleaned[prop]) {
		return
	}
	if static && v.workerSafeProps[prop] {
		return
	}

	switch {
	case v.curMethod == "reset":
		v.resetted[key] = true
	case v.isReset:
		v.mutated[key] = mutationInfo{
			line:       int(n.StartPosition().Row) + 1,
			code:       v.lines[n.StartPosition().Row],
			snippet:    v.getContent(n),
			astDetails: n.ToSexp(),
		}
	case v.curClass != "" || static:
		msg := fmt.Sprintf("Mutation of state '%s' in %s::%s()", key, v.curClass, v.curMethod)
		v.addFinding(n, msg, "State mutations persist across requests in Worker mode.", "ERROR")
	}
}

func (v *PHPVisitor) performResetCheck() {
	for prop, info := range v.mutated {
		if v.workerSafeProps[prop] {
			continue
		}
		propName := prop
		if strings.HasPrefix(prop, "static::") {
			propName = strings.TrimPrefix(prop, "static::")
		}
		if !v.declaredProps[propName] {
			continue
		}
		if !v.resetted[prop] {
			v.findings = append(v.findings, symbol.Finding{
				Message:      fmt.Sprintf("Property '%s' of %s is mutated but not reset in reset().", prop, v.curClass),
				Severity:     "WARNING",
				Line:         info.line,
				Code:         info.code,
				Snippet:      info.snippet,
				ASTDetails:   info.astDetails,
				Dependencies: v.dependencies,
				Remediation:  fmt.Sprintf("Add '$this->%s = ...' in the reset() method.", prop),
			})
		}
	}
}

func (v *PHPVisitor) addFinding(n *sitter.Node, msg, hint, severity string) {
	if v.isWorkerSafeClass || v.isWorkerSafeMethod {
		return
	}

	row := int(n.StartPosition().Row)
	lineContent := v.lines[row]

	// Check if the current line or the previous line contains @igor-ignore
	if strings.Contains(lineContent, "@igor-ignore") {
		return
	}
	if row > 0 && strings.Contains(v.lines[row-1], "@igor-ignore") {
		return
	}

	v.findings = append(v.findings, symbol.Finding{
		Message:      msg,
		Line:         row + 1,
		Code:         v.lines[row],
		Snippet:      v.getContent(n),
		ASTDetails:   n.ToSexp(),
		Dependencies: v.dependencies,
		Remediation:  hint,
		Severity:     severity,
	})
}

func (v *PHPVisitor) getContent(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	return string(v.content[n.StartByte():n.EndByte()])
}

// hasAttribute checks if the given node has an attribute ending with the target name (e.g. "WorkerSafe").
func (v *PHPVisitor) hasAttribute(n *sitter.Node, target string) bool {
	if n == nil {
		return false
	}
	attributesNode := n.ChildByFieldName("attributes")
	if attributesNode == nil {
		return false
	}

	for i := uint(0); i < attributesNode.ChildCount(); i++ {
		group := attributesNode.Child(i)
		if group.Kind() == "attribute_group" {
			for j := uint(0); j < group.ChildCount(); j++ {
				attr := group.Child(j)
				if attr.Kind() == "attribute" {
					for k := uint(0); k < attr.ChildCount(); k++ {
						nameNode := attr.Child(k)
						if nameNode.Kind() == "name" || nameNode.Kind() == "qualified_name" || nameNode.Kind() == "fully_qualified_name" {
							nameContent := v.getContent(nameNode)
							if nameContent == target || strings.HasSuffix(nameContent, "\\"+target) {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

// handleMethodCall intercepts and analyzes method calls (e.g., $this->dispatcher->addListener() or $this->googleTagManager->addPush()).
func (v *PHPVisitor) handleMethodCall(n *sitter.Node) {
	if n == nil {
		return
	}

	fullName := v.curClass
	if v.namespace != "" {
		fullName = v.namespace + "\\" + v.curClass
	}

	// If the service is explicitly transient (non-shared), mutations are accepted
	if v.nonSharedServices[strings.TrimPrefix(fullName, "\\")] {
		return
	}
	if v.engine != nil && (v.engine.IsExplicitlyNonShared(fullName) || v.engine.IsSafeNamespace(fullName)) {
		return
	}

	// 1. Retrieve the calling receiver object and the method name
	obj := n.ChildByFieldName("object")
	if obj == nil {
		return
	}

	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := v.getContent(nameNode)

	// 2. Check if the object/chain starts with a property of the current class ($this->propertyName)
	propName, isProp := v.resolveRootProperty(obj)
	if !isProp {
		return
	}

	_, isDirectProp := v.isPropertyFetchOnThis(obj)

	// Run Rule 1: DetectSingletonMutationRule
	v.detectSingletonMutation(n, propName, methodName, isDirectProp)

	// Run Rule 2: DetectClosureStateLeakRule
	v.detectClosureStateLeak(n)
}

// detectSingletonMutation flags runtime mutations on injected dependencies unless they are resettable.
func (v *PHPVisitor) detectSingletonMutation(n *sitter.Node, propName, methodName string, isDirectProp bool) {
	if !hasMutationPrefix(methodName) {
		return
	}

	// If the current class implements ResetInterface, we treat this as a standard property mutation.
	// It is allowed as long as the property itself is cleared/reset in the reset() method.
	if v.isReset {
		if isDirectProp {
			v.mutated[propName] = mutationInfo{
				line:       int(n.StartPosition().Row) + 1,
				code:       v.lines[n.StartPosition().Row],
				snippet:    v.getContent(n),
				astDetails: n.ToSexp(),
			}
		}
		return
	}

	// Otherwise, check if the injected type implements ResetInterface in the Symfony container
	isResettable := false
	if typeName, ok := v.propertyTypes[propName]; ok {
		fqcn := v.resolveFQCN(typeName)
		if v.engine != nil && v.engine.IsResettable(fqcn) {
			isResettable = true
		}
	}

	if !isResettable {
		var msg string
		if v.localSharedVars["$"+propName] {
			msg = fmt.Sprintf("Mutation detected on a local reference to a shared service ($%s). Risk of State Leak in a worker.", propName)
		} else {
			msg = fmt.Sprintf("Mutation detected on an injected dependency ($this->%s). Risk of State Leak in a worker.", propName)
		}
		v.addFinding(n, msg, "Avoid modifying injected dependencies at runtime, or use a ResetInterface.", "ERROR")
	}
}

// detectClosureStateLeak flags anonymous functions capturing local scope via use() passed as arguments to singleton dependencies.
func (v *PHPVisitor) detectClosureStateLeak(n *sitter.Node) {
	argsNode := n.ChildByFieldName("arguments")
	if argsNode == nil {
		return
	}

	var findClosures func(*sitter.Node)
	findClosures = func(node *sitter.Node) {
		if node == nil {
			return
		}
		// If the node is an anonymous function (closure)
		if node.Kind() == "anonymous_function" {
			// And it has a "use" capture clause (use ($var))
			if hasUseClause(node) {
				msg := "Potential Memory Leak: Injection of a closure capturing local state into a shared service."
				v.addFinding(node, msg, "Avoid injecting closures that capture local state via use() into shared services.", "ERROR")
			}
		}
		// Recursive walk of argument node children (e.g., in a nested array)
		for i := uint(0); i < node.ChildCount(); i++ {
			findClosures(node.Child(i))
		}
	}
	findClosures(argsNode)
}

// isPropertyFetchOnThis checks if a node is a property of the current class via $this (e.g., $this->propertyName)
func (v *PHPVisitor) isPropertyFetchOnThis(n *sitter.Node) (string, bool) {
	if n == nil {
		return "", false
	}
	if n.Kind() != "member_access_expression" {
		return "", false
	}
	obj := n.ChildByFieldName("object")
	if obj == nil {
		return "", false
	}
	objContent := v.getContent(obj)
	// Check if the access is made on the $this object
	if !strings.Contains(objContent, "$this") {
		return "", false
	}
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return "", false
	}
	return v.getContent(nameNode), true
}

// resolveRootProperty traverses down a chain of member calls, member accesses, and subscripts
// to find if the root is a property fetch on $this (or a tracked local shared variable).
func (v *PHPVisitor) resolveRootProperty(n *sitter.Node) (string, bool) {
	curr := n
	for curr != nil {
		kind := curr.Kind()
		switch kind {
		case "member_call_expression", "nullsafe_member_call_expression":
			nameNode := curr.ChildByFieldName("name")
			if nameNode != nil {
				methodName := v.getContent(nameNode)
				if isTaintBreakerMethod(methodName) {
					return "", false
				}
			}
			curr = curr.ChildByFieldName("object")
		case "member_access_expression", "nullsafe_member_access_expression":
			obj := curr.ChildByFieldName("object")
			if obj != nil {
				// If the object's kind is NOT member_access_expression or member_call_expression,
				// and it contains $this, then curr is the root property access on $this.
				objKind := obj.Kind()
				if objKind != "member_access_expression" && objKind != "nullsafe_member_access_expression" &&
					objKind != "member_call_expression" && objKind != "nullsafe_member_call_expression" {
					objContent := v.getContent(obj)
					if strings.Contains(objContent, "$this") {
						nameNode := curr.ChildByFieldName("name")
						if nameNode != nil {
							return v.getContent(nameNode), true
						}
						return "", false
					}
				}
			}
			curr = obj
		case "subscript_expression":
			if curr.ChildCount() > 0 {
				curr = curr.Child(0)
			} else {
				curr = nil
			}
		default:
			// If we reached a variable node, check if it's a tracked local shared variable
			content := v.getContent(curr)
			if strings.HasPrefix(content, "$") && v.localSharedVars[content] {
				return strings.TrimPrefix(content, "$"), true
			}
			curr = nil
		}
	}
	// Fallback check on final node
	if curr != nil {
		content := v.getContent(curr)
		if strings.HasPrefix(content, "$") && v.localSharedVars[content] {
			return strings.TrimPrefix(content, "$"), true
		}
	}
	return "", false
}

// isTaintBreakerMethod checks if the method is a factory, builder, or query-scoped method that breaks the taint chain
func isTaintBreakerMethod(methodName string) bool {
	m := strings.ToLower(methodName)
	return strings.HasPrefix(m, "find") ||
		strings.HasPrefix(m, "create") ||
		strings.HasPrefix(m, "build") ||
		m == "getitem" ||
		m == "getitems" ||
		m == "expr"
}

// hasMutationPrefix checks if the method name starts with a standard mutation prefix
func hasMutationPrefix(methodName string) bool {
	prefixes := []string{"add", "set", "push", "register", "append", "disable", "enable", "clear", "remove"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(methodName, prefix) {
			// Ensure the prefix is properly bounded (e.g., camelCase/PascalCase) to avoid false positives like "settle" or "address"
			if len(methodName) == len(prefix) {
				return true
			}
			nextChar := methodName[len(prefix)]
			if nextChar >= 'A' && nextChar <= 'Z' {
				return true
			}
			if nextChar == '_' {
				return true
			}
		}
	}
	return false
}

// hasUseClause checks if an anonymous function has a "use (...)" capture clause
func hasUseClause(node *sitter.Node) bool {
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "anonymous_function_use_clause" {
			return true
		}
	}
	return false
}

// scanPropertyTypes recursively extracts type-hints of class properties and promoted constructor parameters.
func (v *PHPVisitor) scanPropertyTypes(classNode *sitter.Node) {
	body := classNode.ChildByFieldName("body")
	if body == nil {
		return
	}

	for i := uint(0); i < body.ChildCount(); i++ {
		member := body.Child(i)
		if member.Kind() == "property_declaration" {
			v.handlePropertyDeclaration(member)
		}
		if member.Kind() == "method_declaration" {
			v.handleConstructorDeclaration(member)
		}
	}
}

func (v *PHPVisitor) handlePropertyDeclaration(member *sitter.Node) {
	typeNode := member.ChildByFieldName("type")
	var typeStr string
	if typeNode != nil {
		typeStr = v.getContent(typeNode)
	}
	for j := uint(0); j < member.ChildCount(); j++ {
		child := member.Child(j)
		if child.Kind() == "property_element" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				propName := strings.TrimPrefix(v.getContent(nameNode), "$")
				if typeStr != "" {
					v.propertyTypes[propName] = typeStr
				}
				v.declaredProps[propName] = true
			}
		}
	}
}

func (v *PHPVisitor) handleConstructorDeclaration(member *sitter.Node) {
	nameNode := member.ChildByFieldName("name")
	if nameNode == nil || strings.ToLower(v.getContent(nameNode)) != "__construct" {
		return
	}
	params := member.ChildByFieldName("parameters")
	if params == nil {
		return
	}
	for j := uint(0); j < params.ChildCount(); j++ {
		param := params.Child(j)
		if param.Kind() == "parameter_declaration" || param.Kind() == "property_promotion_parameter" {
			typeNode := param.ChildByFieldName("type")
			var typeStr string
			if typeNode != nil {
				typeStr = v.getContent(typeNode)
			}
			for k := uint(0); k < param.ChildCount(); k++ {
				child := param.Child(k)
				if child.Kind() == "variable_name" {
					propName := strings.TrimPrefix(v.getContent(child), "$")
					if typeStr != "" {
						v.propertyTypes[propName] = typeStr
					}
					if param.Kind() == "property_promotion_parameter" {
						v.declaredProps[propName] = true
					}
					break
				}
			}
		}
	}
}

// resolveFQCN resolves the Fully Qualified Class Name (FQCN) of a type-hint by scanning the "use" imports of the file.
func (v *PHPVisitor) resolveFQCN(typeName string) string {
	if strings.HasPrefix(typeName, "\\") {
		return strings.TrimPrefix(typeName, "\\")
	}

	// 1. Search among the "use" imports of the current file
	for _, line := range v.lines {
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

	// 2. Fallback to the current namespace
	if v.namespace != "" {
		return v.namespace + "\\" + typeName
	}
	return typeName
}

// getFinallyCleanedProperties scans a finally_clause AST node to extract properties of $this that are cleaned up.
func (v *PHPVisitor) getFinallyCleanedProperties(finallyNode *sitter.Node) map[string]bool {
	cleanups := make(map[string]bool)
	if finallyNode == nil {
		return cleanups
	}

	var walkFinally func(*sitter.Node)
	walkFinally = func(node *sitter.Node) {
		if node == nil {
			return
		}

		switch node.Kind() {
		case "function_call_expression":
			v.checkFinallyFunctionCall(node, cleanups)
		case "assignment_expression":
			v.checkFinallyAssignment(node, cleanups)
		case "unset_statement":
			v.checkFinallyUnset(node, cleanups)
		}

		for i := uint(0); i < node.ChildCount(); i++ {
			walkFinally(node.Child(i))
		}
	}

	walkFinally(finallyNode)
	return cleanups
}

// checkFinallyFunctionCall detects cleanup function calls like array_pop($this->propertyName) or array_shift($this->propertyName).
func (v *PHPVisitor) checkFinallyFunctionCall(node *sitter.Node, cleanups map[string]bool) {
	fnNameNode := node.ChildByFieldName("function")
	if fnNameNode == nil {
		fnNameNode = node.ChildByFieldName("name")
	}
	if fnNameNode == nil {
		return
	}
	fnName := strings.ToLower(v.getContent(fnNameNode))
	if fnName != "array_pop" && fnName != "array_shift" {
		return
	}
	argsNode := node.ChildByFieldName("arguments")
	if argsNode == nil {
		return
	}
	for i := uint(0); i < argsNode.ChildCount(); i++ {
		arg := argsNode.Child(i)
		if arg.Kind() == "argument" && arg.NamedChildCount() > 0 {
			expr := arg.NamedChild(0)
			if expr != nil && expr.Kind() == "member_access_expression" {
				prop, isProp := v.isPropertyFetchOnThis(expr)
				if isProp {
					cleanups[prop] = true
				}
			}
		}
	}
}

// checkFinallyAssignment detects cleanup assignments like $this->propertyName = null or $this->propertyName = [].
func (v *PHPVisitor) checkFinallyAssignment(node *sitter.Node, cleanups map[string]bool) {
	left := node.ChildByFieldName("left")
	if left != nil && left.Kind() == "member_access_expression" {
		prop, isProp := v.isPropertyFetchOnThis(left)
		if isProp {
			cleanups[prop] = true
		}
	}
}

// checkFinallyUnset detects cleanup unset statements like unset($this->propertyName).
func (v *PHPVisitor) checkFinallyUnset(node *sitter.Node, cleanups map[string]bool) {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "member_access_expression" {
			prop, isProp := v.isPropertyFetchOnThis(child)
			if isProp {
				cleanups[prop] = true
			}
		}
	}
}

// preScanFinallyCleanups recursively scans a method or function body to extract properties cleaned up in finally blocks.
func (v *PHPVisitor) preScanFinallyCleanups(n *sitter.Node) {
	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	var preScanFinally func(*sitter.Node)
	preScanFinally = func(node *sitter.Node) {
		if node == nil {
			return
		}
		if node.Kind() == "try_statement" {
			for i := uint(0); i < node.ChildCount(); i++ {
				child := node.Child(i)
				if child.Kind() == "finally_clause" {
					cleanups := v.getFinallyCleanedProperties(child)
					for prop := range cleanups {
						v.finallyCleaned[prop] = true
					}
				}
			}
		}
		for i := uint(0); i < node.ChildCount(); i++ {
			preScanFinally(node.Child(i))
		}
	}
	preScanFinally(body)
}
