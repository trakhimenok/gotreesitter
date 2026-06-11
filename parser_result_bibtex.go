package gotreesitter

func normalizeBibtexCompatibility(root *Node, source []byte, lang *Language) {
	normalizeRootLeadingTriviaStart(root, source)
	if root == nil || lang == nil || lang.Name != "bibtex" {
		return
	}
	walkResultTree(root, func(n *Node) {
		normalizeBibtexMissingEntryKey(n, lang)
	})
}

func normalizeBibtexMissingEntryKey(entry *Node, lang *Language) {
	if entry == nil || lang == nil || entry.Type(lang) != "entry" {
		return
	}
	children := resultChildSliceForMutation(entry)
	if len(children) < 5 || children[0] == nil || children[0].Type(lang) != "entry_type" {
		return
	}
	for openIdx := 1; openIdx+1 < len(children); openIdx++ {
		open := children[openIdx]
		if open == nil || open.Type(lang) != "{" {
			continue
		}
		keyIdx := -1
		errIdx := openIdx + 1
		if errIdx < len(children) && children[errIdx] != nil && children[errIdx].Type(lang) == "key_brace" {
			keyIdx = errIdx
			errIdx++
		}
		if errIdx >= len(children) {
			return
		}
		errNode := children[errIdx]
		if errNode == nil || errNode.Type(lang) != "ERROR" || !errNode.IsExtra() {
			continue
		}
		errChildren := resultChildSliceForMutation(errNode)
		valueOpenIdx := bibtexRecoveredValueOpenIndex(errChildren, lang)
		if valueOpenIdx < 0 {
			continue
		}

		newErrChildren := make([]*Node, 0, 1+len(errChildren[:valueOpenIdx]))
		newErrChildren = append(newErrChildren, open)
		if keyIdx >= 0 {
			newErrChildren = append(newErrChildren, children[keyIdx])
		}
		newErrChildren = append(newErrChildren, errChildren[:valueOpenIdx]...)
		newErrChildren = cloneNodeSliceInArena(errNode.ownerArena, newErrChildren)
		replaceNodeChildrenUnfielded(errNode, newErrChildren)
		errNode.setNamed(true)
		errNode.setExtra(true)
		errNode.setHasError(true)
		for _, child := range newErrChildren {
			if child != nil && child.Type(lang) == "ERROR" {
				child.setNamed(true)
			}
		}
		errNode.startByte = open.startByte
		errNode.startPoint = open.startPoint
		lastErrChild := newErrChildren[len(newErrChildren)-1]
		errNode.endByte = lastErrChild.endByte
		errNode.endPoint = lastErrChild.endPoint

		newEntryChildren := make([]*Node, 0, len(children))
		newEntryChildren = append(newEntryChildren, children[:openIdx]...)
		newEntryChildren = append(newEntryChildren, errNode)
		newEntryChildren = append(newEntryChildren, errChildren[valueOpenIdx:]...)
		newEntryChildren = append(newEntryChildren, children[errIdx+1:]...)
		replaceNodeChildrenUnfielded(entry, cloneNodeSliceInArena(entry.ownerArena, newEntryChildren))
		return
	}
}

func bibtexRecoveredValueOpenIndex(children []*Node, lang *Language) int {
	for i := 0; i+1 < len(children); i++ {
		if children[i] == nil || children[i+1] == nil {
			continue
		}
		if children[i].Type(lang) == "=" && children[i+1].Type(lang) == "{" {
			return i + 1
		}
	}
	return -1
}
