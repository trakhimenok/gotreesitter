package gotreesitter

func normalizeEBNFCompatibility(root *Node, source []byte, lang *Language) {
	normalizeEBNFRecoveredRootEnd(root, source, lang)
}

func normalizeEBNFRecoveredRootEnd(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "ebnf" || len(source) == 0 || root.Type(lang) != "ERROR" {
		return
	}
	if root.endByte >= uint32(len(source)) || int(root.endByte) > len(source) {
		return
	}
	if uint32(len(source))-root.endByte != 1 {
		return
	}
	extendNodeEndTo(root, uint32(len(source)), source)
}
