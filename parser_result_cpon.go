package gotreesitter

func normalizeCPONCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCPONDocumentLeadingTriviaStart(root, source, lang)
	normalizeCPONNullLeafChildren(root, source, lang)
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "boolean", "true", "false")
}

func normalizeCPONDocumentLeadingTriviaStart(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "cpon" || root.Type(lang) != "document" || len(source) == 0 || len(root.children) == 0 {
		return
	}
	first := root.children[0]
	if first == nil || first.startByte == 0 || first.startByte > uint32(len(source)) {
		return
	}
	if !cponLeadingTriviaOnly(source[:first.startByte]) {
		return
	}
	root.startByte = first.startByte
	root.startPoint = first.startPoint
}

func cponLeadingTriviaOnly(prefix []byte) bool {
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

func normalizeCPONNullLeafChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "cpon" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n == nil || n.Type(lang) != "null" || resultChildCount(n) != 1 {
			return
		}
		if n.startByte > n.endByte || int(n.endByte) > len(source) || string(source[n.startByte:n.endByte]) != "null" {
			return
		}
		child := resultChildAt(n, 0)
		if child == nil || child.Type(lang) != "null" || child.startByte != n.startByte || child.endByte != n.endByte {
			return
		}
		n.children = nil
		n.fieldIDs = nil
		n.fieldSources = nil
		if n.ownerArena != nil {
			n.ownerArena.clearFinalChildRefs(n)
		}
	})
}
