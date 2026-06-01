package gotreesitter

func normalizeElixirCompatibility(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "elixir" {
		return
	}
	normalizeElixirNestedCallTargetFields(root, lang)
	normalizeElixirCollapsedLiteralChildren(root, source, lang)
	normalizeElixirMapContentKeywordPairs(root, lang)
	normalizeElixirMapContentBinaryOperators(root, lang)
}

func normalizeElixirNestedCallTargetFields(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "elixir" {
		return
	}
	targetID, ok := lang.FieldByName("target")
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "call" && len(n.children) >= 2 {
			first := n.children[0]
			second := n.children[1]
			if first != nil && second != nil &&
				first.Type(lang) == "call" &&
				second.Type(lang) == "arguments" {
				setNodeChildFieldInheritedIfEmpty(n, 0, targetID)
			}
		}
	})
}

func normalizeElixirCollapsedLiteralChildren(root *Node, source []byte, lang *Language) {
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "boolean", "true", "false")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "nil", "nil")
}

func normalizeElixirMapContentKeywordPairs(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "elixir" {
		return
	}
	mapContentSym, ok := symbolByName(lang, "map_content")
	if !ok {
		return
	}
	keywordsSym, ok := symbolByName(lang, "keywords")
	if !ok {
		return
	}
	pairSym, ok := symbolByName(lang, "pair")
	if !ok {
		return
	}
	keywordSym, ok := symbolByName(lang, "keyword")
	if !ok {
		return
	}
	commaSym, hasComma := symbolByName(lang, ",")
	keywordsNamed := symbolIsNamed(lang, keywordsSym)

	walkResultTree(root, func(n *Node) {
		if n == nil || n.symbol != mapContentSym {
			return
		}
		children := resultChildSliceForMutation(n)
		if len(children) == 0 {
			return
		}
		type span struct {
			start int
			end   int
		}
		var spans []span
		for i := 0; i < len(children); i++ {
			if !elixirIsKeywordPair(children[i], pairSym, keywordSym) {
				continue
			}
			start := i
			end := i + 1
			for hasComma && end+1 < len(children) && children[end] != nil && children[end].symbol == commaSym &&
				elixirIsKeywordPair(children[end+1], pairSym, keywordSym) {
				end += 2
			}
			spans = append(spans, span{start: start, end: end})
			i = end - 1
		}
		for i := len(spans) - 1; i >= 0; i-- {
			s := spans[i]
			groupChildren := cloneNodeSliceInArena(n.ownerArena, children[s.start:s.end])
			keywords := newParentNodeInArena(n.ownerArena, keywordsSym, keywordsNamed, groupChildren, nil, 0)
			replaceChildRangeWithSingleNode(n, s.start, s.end, keywords)
		}
	})
}

func elixirIsKeywordPair(n *Node, pairSym, keywordSym Symbol) bool {
	if n == nil || n.symbol != pairSym || resultChildCount(n) == 0 {
		return false
	}
	first := resultChildAt(n, 0)
	return first != nil && first.symbol == keywordSym
}

func normalizeElixirMapContentBinaryOperators(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "elixir" {
		return
	}
	mapContentSym, ok := symbolByName(lang, "map_content")
	if !ok {
		return
	}
	binaryOperatorSym, ok := symbolByName(lang, "binary_operator")
	if !ok {
		return
	}
	barSym, ok := symbolByName(lang, "|")
	if !ok {
		return
	}
	fatArrowSym, hasFatArrow := symbolByName(lang, "=>")
	binaryOperatorNamed := symbolIsNamed(lang, binaryOperatorSym)

	walkResultTree(root, func(n *Node) {
		if n == nil || n.symbol != mapContentSym {
			return
		}
		if resultChildCount(n) != 3 {
			return
		}
		mid := resultChildAt(n, 1)
		if mid == nil || (mid.symbol != barSym && (!hasFatArrow || mid.symbol != fatArrowSym)) {
			return
		}
		originalChildren := resultChildSliceForMutation(n)
		children := cloneNodeSliceInArena(n.ownerArena, originalChildren)
		var fieldIDs []FieldID
		if len(n.fieldIDs) == len(originalChildren) {
			fieldIDs = cloneFieldIDSliceInArena(n.ownerArena, n.fieldIDs)
		}
		wrapper := newParentNodeInArena(n.ownerArena, binaryOperatorSym, binaryOperatorNamed, children, fieldIDs, 0)
		replaceChildRangeWithNodes(n, 0, 3, []*Node{wrapper})
	})
}
