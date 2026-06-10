package gotreesitter

func normalizeCPONCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCPONDocumentLeadingTriviaStart(root, source, lang)
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
