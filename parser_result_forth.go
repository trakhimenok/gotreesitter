package gotreesitter

func normalizeForthCompatibility(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "forth" {
		return
	}
	normalizeRootLeadingTriviaStart(root, source)
	normalizeForthUnterminatedDefinitions(root, lang)
}

func normalizeForthUnterminatedDefinitions(root *Node, lang *Language) {
	wordDefSym, ok := lang.symbolByNameAndNamed("word_definition", true)
	if !ok {
		return
	}
	startDefSym, ok := lang.symbolByNameAndNamed("start_definition", true)
	if !ok {
		return
	}
	endDefSym, ok := lang.symbolByNameAndNamed("end_definition", true)
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n == nil || !n.IsError() {
			return
		}
		children := resultChildSliceForMutation(n)
		if len(children) == 0 || children[0] == nil || children[0].symbol != startDefSym {
			return
		}
		last := children[len(children)-1]
		if last == nil || last.symbol == endDefSym || last.endByte != n.endByte {
			return
		}
		end := newLeafNodeInArena(n.ownerArena, endDefSym, true, n.endByte, n.endByte, n.endPoint, n.endPoint)
		end.setMissing(true)
		end.setHasError(true)
		updated := append(cloneNodeSliceInArena(n.ownerArena, children), end)
		n.symbol = wordDefSym
		n.setNamed(true)
		n.setExtra(false)
		n.children = updated
		n.fieldIDs = nil
		n.fieldSources = nil
		n.setHasError(true)
		populateParentNode(n, updated)
	})
}
