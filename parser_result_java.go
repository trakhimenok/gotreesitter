package gotreesitter

func normalizeJavaCompatibility(root *Node, source []byte, lang *Language) {
	normalizeJavaPrimitiveTypeTokens(root, source, lang)
	normalizeJavaCollapsedLeafChildren(root, source, lang)
}

func normalizeJavaPrimitiveTypeTokens(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "java" {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if len(n.children) == 0 && javaPrimitiveTypeWrapper(n.Type(lang)) {
			normalizeCollapsedTextToken(n, source, lang, javaPrimitiveTypeToken)
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func javaPrimitiveTypeWrapper(name string) bool {
	switch name {
	case "boolean_type", "integral_type", "floating_point_type", "void_type":
		return true
	default:
		return false
	}
}

func javaPrimitiveTypeToken(text string) bool {
	switch text {
	case "boolean", "byte", "short", "int", "long", "char", "float", "double", "void":
		return true
	default:
		return false
	}
}

func normalizeJavaCollapsedLeafChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "java" || len(source) == 0 {
		return
	}
	normalizeCollapsedNamedLeafChildrenBySource(
		root,
		source,
		lang,
		"modifiers",
		"abstract",
		"default",
		"final",
		"native",
		"non-sealed",
		"private",
		"protected",
		"public",
		"sealed",
		"static",
		"strictfp",
		"synchronized",
		"transient",
		"volatile",
	)
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "asterisk", "*")
}
