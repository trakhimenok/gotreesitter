package gotreesitter

func normalizeCollapsedTextToken(n *Node, source []byte, lang *Language, accept func(string) bool) bool {
	if n == nil || lang == nil || n.startByte > n.endByte || n.endByte > uint32(len(source)) {
		return false
	}
	text := string(source[n.startByte:n.endByte])
	if accept != nil && !accept(text) {
		return false
	}
	sym, ok := lang.symbolByNameAndNamed(text, false)
	if !ok {
		sym, ok = symbolByName(lang, text)
		if !ok {
			return false
		}
	}
	named := false
	if idx := int(sym); idx >= 0 && idx < len(lang.SymbolMetadata) {
		named = lang.SymbolMetadata[idx].Named
	}
	child := newLeafNodeInArena(n.ownerArena, sym, named, n.startByte, n.endByte, n.startPoint, n.endPoint)
	child.parent = n
	child.childIndex = 0
	n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
	n.fieldIDs = cloneFieldIDSliceInArena(n.ownerArena, []FieldID{0})
	n.fieldSources = nil
	return true
}
