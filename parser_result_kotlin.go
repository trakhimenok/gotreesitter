package gotreesitter

func normalizeKotlinCompatibility(root *Node, source []byte, lang *Language) {
	normalizeKotlinBindingPatternKindTokens(root, source, lang)
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
