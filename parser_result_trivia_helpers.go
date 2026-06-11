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

func normalizeRootLeadingTriviaStart(root *Node, source []byte) {
	if root == nil || len(source) == 0 || resultChildCount(root) == 0 {
		return
	}
	first := resultChildAt(root, 0)
	if first == nil || root.startByte >= first.startByte || int(first.startByte) > len(source) {
		return
	}
	if !bytesAreTrivia(source[root.startByte:first.startByte]) {
		return
	}
	root.startByte = first.startByte
	root.startPoint = first.startPoint
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
