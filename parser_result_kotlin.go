package gotreesitter

import "strings"

func normalizeKotlinCompatibility(root *Node, source []byte, lang *Language) {
	normalizeKotlinRecoveredSourceFileRoot(root, source, lang)
	normalizeKotlinBindingPatternKindTokens(root, source, lang)
	normalizeKotlinCollapsedModifierChildren(root, source, lang)
	normalizeKotlinCollapsedSimpleIdentifierChildren(root, source, lang)
	normalizeKotlinCollapsedLiteralChildren(root, source, lang)
	normalizeKotlinCollapsedExpressionChildren(root, source, lang)
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
