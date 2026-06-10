package gotreesitter

// normalizeGitcommitCompatibility keeps gitcommit trees aligned with C
// tree-sitter output.
func normalizeGitcommitCompatibility(root *Node, source []byte, lang *Language) {
	normalizeGitcommitCollapsedMessageLines(root, source, lang)
}

// normalizeGitcommitCollapsedMessageLines restores the message_line child of a
// gitcommit message whose aliased repeat collapsed into a leaf.
//
// The grammar defines `message` as alias(repeat($._body_line), $.message) with
// hidden NEWLINE body lines. When the repeat matches exactly one message_line
// (a single-line body, e.g. "subject\n\nline\n<EOF>"), the production parser
// collapses the wrapper onto the line token — yielding a childless message
// that also stops before the trailing newline(s) — while the C oracle keeps
// the message_line child visible and extends message over the hidden NEWLINE
// body lines that follow it. Rebuild the child and re-extend the span.
func normalizeGitcommitCollapsedMessageLines(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "gitcommit" || len(source) == 0 {
		return
	}
	msgSym, ok := lang.symbolByNameAndNamed("message", true)
	if !ok {
		return
	}
	lineSym, ok := lang.symbolByNameAndNamed("message_line", true)
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.symbol != msgSym || resultChildCount(n) != 0 {
			return
		}
		if n.startByte >= n.endByte || int(n.endByte) > len(source) {
			return
		}
		// A collapsed message is a single line; a childless message spanning
		// line breaks is a legitimate all-NEWLINE body — leave it alone.
		for _, b := range source[n.startByte:n.endByte] {
			if b == '\n' || b == '\r' {
				return
			}
		}
		child := newLeafNodeInArena(n.ownerArena, lineSym, true, n.startByte, n.endByte, n.startPoint, n.endPoint)
		child.parent = n
		child.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
		// C keeps the hidden NEWLINE body lines following the message_line
		// inside message; consume the trailing run of line breaks.
		end := n.endByte
		row := n.endPoint.Row
		for int(end) < len(source) {
			if source[end] == '\n' {
				end++
				row++
			} else if source[end] == '\r' && int(end)+1 < len(source) && source[end+1] == '\n' {
				end += 2
				row++
			} else {
				break
			}
		}
		if end != n.endByte {
			n.endByte = end
			n.endPoint = Point{Row: row, Column: 0}
		}
	})
}
