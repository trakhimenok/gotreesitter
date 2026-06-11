package gotreesitter

// normalizeSquirrelCompatibility aligns gotreesitter's Squirrel parse tree with
// C tree-sitter for known shape gaps.
//
// When the source begins with leading whitespace, C tree-sitter starts the
// `script` root node at the first non-whitespace byte rather than at byte 0.
// The generic root normalization (normalizeRootSourceStart) forces the root
// start back to 0, so this compatibility pass — which runs afterwards — restores
// the C-aligned start offset.
func normalizeSquirrelCompatibility(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "squirrel" || len(source) == 0 {
		return
	}
	if root.Type(lang) != "script" {
		return
	}
	start := firstNonTriviaByteStart(source)
	if start == 0 || start <= root.startByte || start > root.endByte {
		return
	}
	root.startByte = start
	root.startPoint = advancePointByBytes(Point{}, source[:start])
}

// firstNonTriviaByteStart returns the byte offset of the first non-whitespace
// byte in source, or 0 when source is entirely whitespace.
func firstNonTriviaByteStart(source []byte) uint32 {
	start := 0
	if len(source) >= len(utf8BOM) &&
		source[0] == utf8BOM[0] && source[1] == utf8BOM[1] && source[2] == utf8BOM[2] {
		start = len(utf8BOM)
	}
	for i := start; i < len(source); i++ {
		switch source[i] {
		case ' ', '\t', '\n', '\r', '\f':
			continue
		default:
			return uint32(i)
		}
	}
	return 0
}
