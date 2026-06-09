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
	for i := 0; i < len(source); i++ {
		switch source[i] {
		case ' ', '\t', '\n', '\r', '\f':
			continue
		default:
			return uint32(i)
		}
	}
	return 0
}
