package gotreesitter

func normalizeHTTPCompatibility(root *Node, source []byte, lang *Language) {
	normalizeHTTPDocumentSections(root, source, lang)
}

func normalizeHTTPDocumentSections(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "http" || root.Type(lang) != "document" {
		return
	}
	childCount := resultChildCount(root)
	if childCount < 2 {
		return
	}

	children := resultChildSliceForMutation(root)
	out := make([]*Node, 0, len(children))
	changed := false
	for _, child := range children {
		if !httpCanFoldSectionIntoPrevious(child, source, lang) {
			out = append(out, child)
			continue
		}
		if len(out) == 0 {
			out = append(out, child)
			continue
		}
		prev := out[len(out)-1]
		if prev == nil || prev.Type(lang) != "section" || prev.endByte != child.startByte {
			out = append(out, child)
			continue
		}
		mergeHTTPSectionIntoPrevious(prev, child, source)
		changed = true
	}
	if !changed {
		return
	}
	replaceNodeChildrenUnfielded(root, cloneNodeSliceIfArena(root.ownerArena, out))
	if root.endByte < uint32(len(source)) {
		extendNodeEndTo(root, uint32(len(source)), source)
	}
}

func httpCanFoldSectionIntoPrevious(n *Node, source []byte, lang *Language) bool {
	if n == nil || n.Type(lang) != "section" || n.hasError() {
		return false
	}
	if n.startByte >= n.endByte || n.endByte > uint32(len(source)) {
		return false
	}
	childCount := resultChildCount(n)
	if childCount == 0 {
		return httpSectionSpanIsBlankLine(n, source)
	}
	first := resultChildAt(n, 0)
	return first == nil || first.Type(lang) != "request_separator"
}

func httpSectionSpanIsBlankLine(n *Node, source []byte) bool {
	if n == nil || n.startByte >= n.endByte || n.endByte > uint32(len(source)) {
		return false
	}
	span := source[n.startByte:n.endByte]
	return bytesAreTrivia(span) && bytesContainLineBreak(span)
}

func mergeHTTPSectionIntoPrevious(prev, cur *Node, source []byte) {
	if prev == nil || cur == nil {
		return
	}
	curChildren := resultChildSliceForMutation(cur)
	if len(curChildren) == 0 {
		extendNodeEndTo(prev, cur.endByte, source)
		return
	}

	prevChildren := resultChildSliceForMutation(prev)
	prevLen := len(prevChildren)
	curLen := len(curChildren)
	merged := make([]*Node, 0, prevLen+curLen)
	merged = append(merged, prevChildren...)
	merged = append(merged, curChildren...)

	fieldIDs, fieldSources := mergeHTTPSectionFieldMetadata(prev, cur, prevLen, curLen)
	prev.children = cloneNodeSliceIfArena(prev.ownerArena, merged)
	prev.fieldIDs = cloneFieldIDSliceInArena(prev.ownerArena, fieldIDs)
	prev.fieldSources = cloneHTTPFieldSourceSliceInArena(prev.ownerArena, fieldSources)
	if prev.ownerArena != nil {
		prev.ownerArena.clearFinalChildRefs(prev)
	}
	populateParentNode(prev, prev.children)
}

func mergeHTTPSectionFieldMetadata(prev, cur *Node, prevLen, curLen int) ([]FieldID, []uint8) {
	needFields := len(prev.fieldIDs) == prevLen || len(cur.fieldIDs) == curLen
	needSources := len(prev.fieldSources) == prevLen || len(cur.fieldSources) == curLen
	total := prevLen + curLen
	var fieldIDs []FieldID
	if needFields {
		fieldIDs = make([]FieldID, total)
		for i := 0; i < prevLen; i++ {
			fieldIDs[i] = nodeFieldIDAt(prev, i)
		}
		for i := 0; i < curLen; i++ {
			fieldIDs[prevLen+i] = nodeFieldIDAt(cur, i)
		}
	}
	var fieldSources []uint8
	if needSources {
		fieldSources = make([]uint8, total)
		for i := 0; i < prevLen; i++ {
			fieldSources[i] = fieldSourceAt(prev.fieldSources, i)
		}
		for i := 0; i < curLen; i++ {
			fieldSources[prevLen+i] = fieldSourceAt(cur.fieldSources, i)
		}
	}
	return fieldIDs, fieldSources
}

func cloneHTTPFieldSourceSliceInArena(arena *nodeArena, fieldSources []uint8) []uint8 {
	if len(fieldSources) == 0 {
		return nil
	}
	if arena != nil {
		out := arena.allocFieldSourceSlice(len(fieldSources))
		copy(out, fieldSources)
		return out
	}
	out := make([]uint8, len(fieldSources))
	copy(out, fieldSources)
	return out
}
