package gotreesitter

// normalizeDhallCompatibility aligns gotreesitter's dhall parse tree with C
// tree-sitter for known shape gaps.
func normalizeDhallCompatibility(root *Node, source []byte, lang *Language) {
	normalizeDhallExpressionLeadingTriviaStart(root, source, lang)
}

// normalizeDhallExpressionLeadingTriviaStart restores the C-aligned root
// start offset for sources that begin with whitespace.
//
// In C tree-sitter, whitespace is token padding (never a node), so the
// `expression` root starts at the first non-whitespace byte rather than at
// byte 0. The generic root normalization (normalizeRootSourceStart) forces
// the root start back to 0; this compatibility pass — which runs afterwards —
// snaps the root start back to its first child when only whitespace precedes
// it, matching the C oracle.
func normalizeDhallExpressionLeadingTriviaStart(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "dhall" || root.Type(lang) != "expression" || len(source) == 0 || len(root.children) == 0 {
		return
	}
	first := root.children[0]
	if first == nil || first.startByte == 0 || first.startByte > uint32(len(source)) {
		return
	}
	if !dhallLeadingTriviaOnly(source[:first.startByte]) {
		return
	}
	root.startByte = first.startByte
	root.startPoint = first.startPoint
}

func dhallLeadingTriviaOnly(prefix []byte) bool {
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
