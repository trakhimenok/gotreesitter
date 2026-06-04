package gotreesitter

func bytesAreTrivia(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return false
		}
	}
	return true
}

// bytesAreInterTokenTrivia is bytesAreTrivia plus line-continuation backslashes
// (`\` immediately before a newline). The forest's reduce-coverage rejection
// uses it to decide whether a hole BETWEEN two reduced children is real
// (a dropped statement/token → invalid) or just inter-token whitespace the
// lexer skips. A bare `\<newline>` only appears between tokens where the
// language treats it as a continuation (bash/python/C/…), so accepting it here
// is safe; a real `\` token is never immediately followed by a newline.
func bytesAreInterTokenTrivia(b []byte) bool {
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case ' ', '\t', '\n', '\r', '\f':
			continue
		case '\\':
			if i+1 < len(b) && (b[i+1] == '\n' || b[i+1] == '\r') {
				continue // line-continuation backslash; the newline is whitespace
			}
			return false
		default:
			return false
		}
	}
	return true
}

func lastNonTriviaByteEnd(source []byte) uint32 {
	for i := len(source); i > 0; i-- {
		switch source[i-1] {
		case ' ', '\t', '\n', '\r', '\f':
			continue
		default:
			return uint32(i)
		}
	}
	return 0
}

func trimTrailingExtraTriviaRoot(root *Node, source []byte) {
	view := resultMutableChildrenForMutation(root)
	childCount := view.Len()
	if root == nil || childCount == 0 || len(source) == 0 {
		return
	}
	if view.hasFinalChildRefs() {
		last, ok := view.Entry(childCount - 1)
		if !ok || !stackEntryNodeIsExtra(last) || stackEntryNodeChildCount(last) != 0 {
			return
		}
		start := stackEntryNodeStartByte(last)
		end := stackEntryNodeEndByte(last)
		if start >= end || end != root.endByte || int(end) > len(source) {
			return
		}
		if !bytesAreTrivia(source[start:end]) {
			return
		}
		view.FilterFinalRefs(func(i int, entry stackEntry) bool {
			return i+1 < childCount
		})
		return
	}
	last := resultChildAt(root, childCount-1)
	if last == nil || !last.IsExtra() || resultChildCount(last) != 0 {
		return
	}
	if last.startByte >= last.endByte || last.endByte != root.endByte || int(last.endByte) > len(source) {
		return
	}
	if !bytesAreTrivia(source[last.startByte:last.endByte]) {
		return
	}
	children := resultChildSliceForMutation(root)
	root.children = cloneNodeSliceInArena(root.ownerArena, children[:len(children)-1])
	if len(root.fieldIDs) > len(root.children) {
		root.fieldIDs = root.fieldIDs[:len(root.children)]
	}
	if len(root.fieldSources) > len(root.children) {
		root.fieldSources = root.fieldSources[:len(root.children)]
	}
	populateParentNode(root, root.children)
}
