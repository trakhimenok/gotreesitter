package gotreesitter

func normalizeCSharpQuotedStringContentIdentifiers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c_sharp" || len(source) == 0 {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		for _, child := range n.children {
			walk(child)
		}
		if n.Type(lang) != "identifier" || len(n.children) != 0 || n.startByte == 0 || int(n.endByte) >= len(source) {
			return
		}
		if source[n.startByte-1] != '"' || source[n.endByte] != '"' {
			return
		}
		replacement, ok := csharpBuildStringLiteralNode(n.ownerArena, source, lang, n.startByte-1, n.endByte+1)
		if !ok {
			return
		}
		csharpReplaceNodeContents(n, replacement)
	}
	walk(root)
}
