package gotreesitter

func normalizeAwkCompatibility(root *Node, source []byte, lang *Language) {
	normalizeAwkBareBackslashProgramSpan(root, source, lang)
}

func normalizeAwkBareBackslashProgramSpan(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "awk" || root.Type(lang) != "program" {
		return
	}
	if resultChildCount(root) != 0 || len(source) != 1 || source[0] != '\\' {
		return
	}
	end := uint32(len(source))
	point := advancePointByBytes(Point{}, source)
	root.startByte = end
	root.endByte = end
	root.startPoint = point
	root.endPoint = point
}
