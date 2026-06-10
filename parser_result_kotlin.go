package gotreesitter

import "strings"

func normalizeKotlinCompatibility(root *Node, source []byte, lang *Language) {
	normalizeKotlinRecoveredSourceFileRoot(root, source, lang)
	normalizeKotlinBindingPatternKindTokens(root, source, lang)
	normalizeKotlinCollapsedModifierChildren(root, source, lang)
	normalizeKotlinCollapsedSimpleIdentifierChildren(root, source, lang)
	normalizeKotlinCollapsedLiteralChildren(root, source, lang)
	normalizeKotlinCollapsedExpressionChildren(root, source, lang)
	normalizeKotlinCollapsedIdentifierChildren(root, source, lang)
	normalizeKotlinCallableReferenceNavigations(root, source, lang)
	normalizeKotlinReceiverFunctionNames(root, source, lang)
	normalizeKotlinSourceFileLeadingTriviaStart(root, source, lang)
}

// normalizeKotlinReceiverFunctionNames splits the function name back out of a
// receiver type that swallowed it. For `fun Channel.Factory.range(...)`, C
// tree-sitter parses receiver_type `Channel.Factory`, "." and the
// simple_identifier name `range` as separate function_declaration children;
// gotreesitter's GLR instead folds the whole dotted path into receiver_type's
// user_type and leaves a zero-width simple_identifier where the name should
// be. Detect that exact shape and rebuild the C one: shrink the user_type by
// its trailing "." + type_identifier pair, and splice those two tokens into
// the function_declaration as the "." separator and the (retagged)
// simple_identifier name, replacing the zero-width placeholder.
func normalizeKotlinReceiverFunctionNames(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	fnSym, ok := lang.symbolByNameAndNamed("function_declaration", true)
	if !ok {
		return
	}
	recvSym, ok := lang.symbolByNameAndNamed("receiver_type", true)
	if !ok {
		return
	}
	userSym, ok := lang.symbolByNameAndNamed("user_type", true)
	if !ok {
		return
	}
	simpleSym, ok := lang.symbolByNameAndNamed("simple_identifier", true)
	if !ok {
		return
	}
	typeIdSym, ok := lang.symbolByNameAndNamed("type_identifier", true)
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.symbol != fnSym {
			return
		}
		cc := resultChildCount(n)
		idx := -1
		for i := 0; i+1 < cc; i++ {
			c0, c1 := n.children[i], n.children[i+1]
			if c0 != nil && c1 != nil && c0.symbol == recvSym && c1.symbol == simpleSym && c1.startByte == c1.endByte {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		recv := n.children[idx]
		if resultChildCount(recv) != 1 {
			return
		}
		user := recv.children[0]
		if user == nil || user.symbol != userSym {
			return
		}
		ucc := resultChildCount(user)
		if ucc < 3 {
			return
		}
		dotTok, nameTok := user.children[ucc-2], user.children[ucc-1]
		if dotTok == nil || nameTok == nil || nameTok.symbol != typeIdSym {
			return
		}
		if dotTok.IsNamed() || dotTok.startByte+1 != dotTok.endByte ||
			int(dotTok.endByte) > len(source) || source[dotTok.startByte] != '.' {
			return
		}
		// Shrink user_type (and the receiver_type wrapping it) to end before
		// the trailing "." + name pair.
		user.children = cloneNodeSliceInArena(user.ownerArena, user.children[:ucc-2])
		lastKept := user.children[len(user.children)-1]
		user.endByte = lastKept.endByte
		user.endPoint = lastKept.endPoint
		populateParentNode(user, user.children)
		recv.endByte = user.endByte
		recv.endPoint = user.endPoint
		// The swallowed trailing type_identifier is the function name.
		nameTok.symbol = simpleSym
		children := make([]*Node, 0, cc+1)
		children = append(children, n.children[:idx+1]...)
		children = append(children, dotTok, nameTok)
		children = append(children, n.children[idx+2:]...)
		if len(n.fieldIDs) == cc {
			fieldIDs := make([]FieldID, 0, cc+1)
			fieldIDs = append(fieldIDs, n.fieldIDs[:idx+1]...)
			fieldIDs = append(fieldIDs, 0, 0)
			fieldIDs = append(fieldIDs, n.fieldIDs[idx+2:]...)
			n.fieldIDs = cloneFieldIDSliceInArena(n.ownerArena, fieldIDs)
			n.fieldSources = defaultFieldSourcesInArena(n.ownerArena, n.fieldIDs)
		}
		n.children = cloneNodeSliceInArena(n.ownerArena, children)
		populateParentNode(n, n.children)
	})
}

// normalizeKotlinCallableReferenceNavigations rewrites `Name::target`
// navigation_expression parses into the callable_reference shape C
// tree-sitter selects. The grammar makes `Foo::bar` ambiguous between
// navigation_expression(simple_identifier, navigation_suffix("::" target))
// and callable_reference(type_identifier "::" target); C's GLR resolves the
// conflict to callable_reference, gotreesitter's keeps the navigation parse.
// Only the bare single-identifier base form is rewritten — chained bases
// (`a.b::c`) stay navigation_expressions in C too — and only when the suffix
// carries no type_arguments, which callable_reference cannot hold.
func normalizeKotlinCallableReferenceNavigations(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	navSym, ok := lang.symbolByNameAndNamed("navigation_expression", true)
	if !ok {
		return
	}
	suffixSym, ok := lang.symbolByNameAndNamed("navigation_suffix", true)
	if !ok {
		return
	}
	simpleSym, ok := lang.symbolByNameAndNamed("simple_identifier", true)
	if !ok {
		return
	}
	crSym, ok := lang.symbolByNameAndNamed("callable_reference", true)
	if !ok {
		return
	}
	typeIdSym, ok := lang.symbolByNameAndNamed("type_identifier", true)
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.symbol != navSym || resultChildCount(n) != 2 {
			return
		}
		base, suffix := n.children[0], n.children[1]
		if base == nil || suffix == nil || base.symbol != simpleSym || suffix.symbol != suffixSym {
			return
		}
		if resultChildCount(suffix) != 2 {
			return
		}
		op, target := suffix.children[0], suffix.children[1]
		if op == nil || target == nil || op.startByte+2 != op.endByte ||
			int(op.endByte) > len(source) || string(source[op.startByte:op.endByte]) != "::" {
			return
		}
		if target.symbol != simpleSym {
			if int(target.endByte) > len(source) || string(source[target.startByte:target.endByte]) != "class" || target.IsNamed() {
				return
			}
		}
		base.symbol = typeIdSym
		n.symbol = crSym
		n.setNamed(true)
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{base, op, target})
		n.fieldIDs = nil
		n.fieldSources = nil
		n.productionID = 0
		populateParentNode(n, n.children)
	})
}

// normalizeKotlinCollapsedIdentifierChildren restores the simple_identifier
// child of a single-element `identifier` node (identifier is sep1 of
// simple_identifier by "."): C tree-sitter always materializes the child
// (e.g. in `import benchmarks.*` the identifier wraps one simple_identifier),
// while the Go collapse logic strips lone children.
func normalizeKotlinCollapsedIdentifierChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	normalizeCollapsedNamedLeafChildren(root, lang, "identifier", "simple_identifier")
}

// normalizeKotlinSourceFileLeadingTriviaStart restores the C-aligned root
// start offset for sources that begin with whitespace: C tree-sitter treats
// whitespace as token padding (never a node), so the source_file root starts
// at the first non-whitespace byte. The generic root normalization forces the
// root start back to 0; this pass — which runs afterwards — snaps the root
// start back to its first child when only whitespace precedes it.
func normalizeKotlinSourceFileLeadingTriviaStart(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || root.Type(lang) != "source_file" || len(source) == 0 || len(root.children) == 0 {
		return
	}
	first := root.children[0]
	if first == nil || first.startByte == 0 || first.startByte > uint32(len(source)) {
		return
	}
	if !kotlinLeadingTriviaOnly(source[:first.startByte]) {
		return
	}
	root.startByte = first.startByte
	root.startPoint = first.startPoint
}

func kotlinLeadingTriviaOnly(prefix []byte) bool {
	for _, b := range prefix {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return false
		}
	}
	return true
}

func normalizeKotlinRecoveredSourceFileRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || root.Type(lang) != "ERROR" {
		return
	}
	if !kotlinRootLooksRecoverableSourceFile(root, lang) {
		return
	}
	normalizeKotlinTopLevelFunctionFragments(root, source, lang)
	sym, ok := symbolByName(lang, "source_file")
	if !ok {
		return
	}
	retagResultRootAndRefreshError(root, sym, symbolIsNamed(lang, sym))
}

func kotlinRootLooksRecoverableSourceFile(root *Node, lang *Language) bool {
	if root == nil || lang == nil || resultChildCount(root) == 0 {
		return false
	}
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "package_header",
			"import_list",
			"class_declaration",
			"function_declaration",
			"object_declaration",
			"property_declaration",
			"typealias_declaration",
			"multiline_comment",
			"line_comment":
			return true
		}
	}
	return false
}

func normalizeKotlinTopLevelFunctionFragments(root *Node, source []byte, lang *Language) {
	fnSym, ok := symbolByName(lang, "function_declaration")
	if !ok {
		return
	}
	funSym, ok := symbolByName(lang, "fun")
	if !ok {
		return
	}
	children := resultChildSliceForMutation(root)
	if len(children) < 3 {
		return
	}
	arena := root.ownerArena
	if arena == nil {
		return
	}
	rebuilt := make([]*Node, 0, len(children))
	changed := false
	for i := 0; i < len(children); i++ {
		if fn, ok := kotlinRecoveredTopLevelFunction(arena, children, i, source, lang, fnSym, funSym); ok {
			rebuilt = append(rebuilt, fn)
			i += 2
			changed = true
			continue
		}
		rebuilt = append(rebuilt, children[i])
	}
	if !changed {
		return
	}
	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(arena, rebuilt))
}

func kotlinRecoveredTopLevelFunction(arena *nodeArena, children []*Node, idx int, source []byte, lang *Language, fnSym, funSym Symbol) (*Node, bool) {
	if idx+2 >= len(children) {
		return nil, false
	}
	funKeyword := children[idx]
	name := children[idx+1]
	params := children[idx+2]
	if funKeyword == nil || name == nil || params == nil {
		return nil, false
	}
	if funKeyword.Type(lang) != "ERROR" || strings.TrimSpace(funKeyword.Text(source)) != "fun" {
		return nil, false
	}
	if name.Type(lang) != "simple_identifier" || params.Type(lang) != "function_value_parameters" {
		return nil, false
	}
	retagResultRoot(funKeyword, funSym, symbolIsNamed(lang, funSym))
	funKeyword.setHasError(false)
	fnChildren := cloneNodeSliceInArena(funKeyword.ownerArena, []*Node{funKeyword, name, params})
	fn := newParentNodeInArena(funKeyword.ownerArena, fnSym, symbolIsNamed(lang, fnSym), fnChildren, nil, 0)
	fn.setHasError(true)
	return fn, true
}

func normalizeKotlinBindingPatternKindTokens(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" {
		return
	}

	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "binding_pattern_kind" && len(n.children) == 0 {
			normalizeKotlinBindingPatternKindToken(n, source, lang)
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeKotlinBindingPatternKindToken(n *Node, source []byte, lang *Language) {
	if n == nil || n.startByte > n.endByte || n.endByte > uint32(len(source)) {
		return
	}
	text := string(source[n.startByte:n.endByte])
	if text != "val" && text != "var" {
		return
	}
	normalizeCollapsedTextToken(n, source, lang, func(text string) bool {
		return text == "val" || text == "var"
	})
}

type kotlinCollapsedModifierRule struct {
	parent   string
	children []string
}

type kotlinCollapsedModifierChild struct {
	symbol Symbol
	named  bool
}

var kotlinCollapsedModifierRules = []kotlinCollapsedModifierRule{
	{parent: "class_modifier", children: []string{"sealed", "annotation", "data", "inner", "value"}},
	{parent: "member_modifier", children: []string{"override", "lateinit"}},
	{parent: "visibility_modifier", children: []string{"public", "private", "internal", "protected"}},
	{parent: "variance_modifier", children: []string{"in", "out"}},
	{parent: "function_modifier", children: []string{"tailrec", "operator", "infix", "inline", "external", "suspend"}},
	{parent: "property_modifier", children: []string{"const"}},
	{parent: "inheritance_modifier", children: []string{"abstract", "final", "open"}},
	{parent: "parameter_modifier", children: []string{"vararg", "noinline", "crossinline"}},
	{parent: "reification_modifier", children: []string{"reified"}},
	{parent: "platform_modifier", children: []string{"expect", "actual"}},
}

func normalizeKotlinCollapsedModifierChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	rules := kotlinCollapsedModifierSymbolRules(lang)
	if len(rules) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n == nil || resultChildCount(n) != 0 || int(n.startByte) > len(source) || int(n.endByte) > len(source) || n.startByte > n.endByte {
			return
		}
		children, ok := rules[n.symbol]
		if !ok {
			return
		}
		child, ok := children[string(source[n.startByte:n.endByte])]
		if !ok {
			return
		}
		leaf := newLeafNodeInArena(n.ownerArena, child.symbol, child.named, n.startByte, n.endByte, n.startPoint, n.endPoint)
		leaf.parent = n
		leaf.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{leaf})
		n.fieldIDs = nil
		n.fieldSources = nil
	})
}

func kotlinCollapsedModifierSymbolRules(lang *Language) map[Symbol]map[string]kotlinCollapsedModifierChild {
	out := make(map[Symbol]map[string]kotlinCollapsedModifierChild, len(kotlinCollapsedModifierRules))
	for _, rule := range kotlinCollapsedModifierRules {
		parentSym, ok := lang.symbolByNameAndNamed(rule.parent, true)
		if !ok {
			parentSym, ok = symbolByName(lang, rule.parent)
		}
		if !ok {
			continue
		}
		childSyms := make(map[string]kotlinCollapsedModifierChild, len(rule.children))
		for _, childName := range rule.children {
			childSym, ok := lang.symbolByNameAndNamed(childName, false)
			if !ok {
				childSym, ok = symbolByName(lang, childName)
			}
			if !ok {
				continue
			}
			childSyms[childName] = kotlinCollapsedModifierChild{
				symbol: childSym,
				named:  symbolIsNamed(lang, childSym),
			}
		}
		if len(childSyms) != 0 {
			out[parentSym] = childSyms
		}
	}
	return out
}

var kotlinSimpleIdentifierKeywordChildren = []string{
	"expect",
	"data",
	"inner",
	"value",
	"actual",
	"set",
	"get",
	"override",
	"suspend",
	"annotation",
	"sealed",
	"lateinit",
	"tailrec",
	"operator",
	"infix",
	"inline",
	"external",
	"public",
	"private",
	"internal",
	"protected",
	"abstract",
	"final",
	"open",
	"const",
	"vararg",
	"noinline",
	"crossinline",
	"reified",
	"field",
	"property",
	"receiver",
	"param",
	"setparam",
	"delegate",
	"companion",
	"constructor",
	"init",
	"dynamic",
	"where",
	"catch",
	"finally",
	"enum",
}

func normalizeKotlinCollapsedSimpleIdentifierChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "simple_identifier", kotlinSimpleIdentifierKeywordChildren...)
}

func normalizeKotlinCollapsedLiteralChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "boolean_literal", "true", "false")
}

func normalizeKotlinCollapsedExpressionChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "this_expression", "this")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "super_expression", "super")
}
