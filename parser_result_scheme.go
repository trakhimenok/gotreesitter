package gotreesitter

// normalizeSchemeCompatibility realigns gotreesitter's Scheme output with C
// tree-sitter for a recovery case that gotreesitter currently drops silently.
//
// When a quote-family datum ('x, `x, ,x, ,@x, #'x, #`x, #,x, #,@x) wraps a
// datum but a lone, unterminated "|" appears between the prefix token and the
// quoted datum, C tree-sitter cannot lex the "|" as the start of a piped
// |...| symbol, so its GLR recovery emits a one-byte (ERROR) node spanning
// just the "|" as a child of the quote, ahead of the datum. gotreesitter's
// recovery instead discards the stray "|" entirely, leaving the quote with no
// ERROR child and clearing the tree's error flag.
//
// This rewrite reinserts that single (ERROR) leaf, exactly as C places it, and
// propagates the resulting error flag up to the root. It is deliberately
// narrow: it only fires for a quote-family node whose only gap between the
// prefix token and the quoted datum is whitespace plus exactly one "|".
func normalizeSchemeCompatibility(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "scheme" {
		return
	}

	// walk returns true when the subtree rooted at n gained (or already carried)
	// an error after rewriting, so callers can propagate the error flag upward
	// without relying on parent links being wired at this stage.
	var walk func(n *Node) bool
	walk = func(n *Node) bool {
		if n == nil {
			return false
		}
		childError := false
		for i := 0; i < resultChildCount(n); i++ {
			if walk(resultChildAt(n, i)) {
				childError = true
			}
		}
		if schemeInsertLonePipeError(n, source, lang) {
			childError = true
		}
		if childError && !n.hasError() {
			n.setHasError(true)
		}
		return n.hasError()
	}
	walk(root)
}

// schemeIsQuoteFamily reports whether t is one of the prefix-wrapping datum
// rules whose single quoted datum can be preceded by a stray, unterminated "|".
func schemeIsQuoteFamily(t string) bool {
	switch t {
	case "quote", "quasiquote", "syntax", "quasisyntax",
		"unquote", "unquote_splicing", "unsyntax", "unsyntax_splicing":
		return true
	}
	return false
}

// schemeInsertLonePipeError inspects a single quote-family node and, if it
// matches C's "lone |" recovery shape, inserts the missing (ERROR) leaf. It
// reports whether the rewrite was applied.
func schemeInsertLonePipeError(n *Node, source []byte, lang *Language) bool {
	if n == nil || !schemeIsQuoteFamily(n.Type(lang)) {
		return false
	}
	children := resultChildSliceForMutation(n)
	if len(children) != 2 {
		return false
	}
	prefix := children[0]
	datum := children[1]
	if prefix == nil || datum == nil {
		return false
	}
	// The prefix must be the anonymous wrapper token and the datum a named node;
	// neither may already be (or contain) an ERROR.
	if prefix.isNamed() || !datum.isNamed() {
		return false
	}
	if prefix.symbol == errorSymbol || datum.symbol == errorSymbol {
		return false
	}
	if prefix.hasError() || datum.hasError() {
		return false
	}

	gapStart := prefix.endByte
	gapEnd := datum.startByte
	if gapStart >= gapEnd || int(gapEnd) > len(source) {
		return false
	}
	gap := source[gapStart:gapEnd]

	pipeOffset := -1
	for i := 0; i < len(gap); i++ {
		switch gap[i] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			// whitespace, ignore
		case '|':
			if pipeOffset >= 0 {
				// more than one "|" — not the shape C recovers as a single byte.
				return false
			}
			pipeOffset = i
		default:
			// any non-whitespace, non-pipe byte means this is some other
			// recovery situation; leave it untouched.
			return false
		}
	}
	if pipeOffset < 0 {
		return false
	}

	pipeStart := gapStart + uint32(pipeOffset)
	pipeEnd := pipeStart + 1
	startPoint := advancePointByBytes(prefix.endPoint, source[gapStart:pipeStart])
	endPoint := advancePointByBytes(startPoint, source[pipeStart:pipeEnd])

	errNode := newLeafNodeInArena(n.ownerArena, errorSymbol, true, pipeStart, pipeEnd, startPoint, endPoint)
	errNode.setHasError(true)

	newChildren := []*Node{prefix, errNode, datum}
	if n.ownerArena != nil {
		newChildren = cloneNodeSliceIfArena(n.ownerArena, newChildren)
	}
	replaceNodeChildrenUnfielded(n, newChildren)
	n.setHasError(true)
	return true
}
