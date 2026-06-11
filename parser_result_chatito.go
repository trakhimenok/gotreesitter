package gotreesitter

func normalizeChatitoCompatibility(root *Node, source []byte, lang *Language) {
	normalizeChatitoTrailingAliasBodyError(root, source, lang)
}

func normalizeChatitoTrailingAliasBodyError(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "chatito" || root.Type(lang) != "source" || len(source) == 0 || len(root.children) < 2 {
		return
	}
	err := root.children[len(root.children)-1]
	prev := root.children[len(root.children)-2]
	if err == nil || prev == nil || err.Type(lang) != "ERROR" || prev.Type(lang) != "alias_def" {
		return
	}
	if !err.IsExtra() || err.startByte != prev.endByte || err.endByte != uint32(len(source)) || err.startByte >= err.endByte {
		return
	}
	if resultChildCount(err) == 0 || err.children[0] == nil {
		return
	}
	wordStart, ok := chatitoTrailingIndentedWordBounds(source, err.startByte, err.endByte)
	if !ok {
		return
	}
	body := chatitoLastChildOfType(prev, lang, "alias_body")
	if body == nil || body.endByte != err.startByte || body.ownerArena == nil {
		return
	}
	wordSym, ok := lang.symbolByNameAndNamed("word", true)
	if !ok {
		return
	}
	spaceSym := err.children[0].symbol
	spaceNamed := err.children[0].IsNamed()
	space := newLeafNodeInArena(body.ownerArena, spaceSym, spaceNamed, err.startByte, wordStart, err.startPoint, advancePointByBytes(err.startPoint, source[err.startByte:wordStart]))
	word := newLeafNodeInArena(body.ownerArena, wordSym, true, wordStart, err.endByte, space.endPoint, err.endPoint)
	body.children = cloneNodeSliceInArena(body.ownerArena, append(resultChildSliceForMutation(body), space, word))
	populateParentNode(body, body.children)
	body.endByte = err.endByte
	body.endPoint = err.endPoint
	prev.endByte = err.endByte
	prev.endPoint = err.endPoint
	root.children = cloneNodeSliceInArena(root.ownerArena, root.children[:len(root.children)-1])
	populateParentNode(root, root.children)
	root.setHasError(chatitoChildrenHaveError(root.children))
}

func chatitoTrailingIndentedWordBounds(source []byte, start, end uint32) (uint32, bool) {
	if start >= end || int(end) > len(source) {
		return 0, false
	}
	i := start
	for i < end && (source[i] == ' ' || source[i] == '\t') {
		i++
	}
	if i == start || i >= end {
		return 0, false
	}
	for j := i; j < end; j++ {
		switch source[j] {
		case ' ', '\t', '\r', '\n':
			return 0, false
		}
	}
	return i, true
}

func chatitoLastChildOfType(node *Node, lang *Language, typ string) *Node {
	if node == nil {
		return nil
	}
	children := resultChildSliceForMutation(node)
	for i := len(children) - 1; i >= 0; i-- {
		if children[i] != nil && children[i].Type(lang) == typ {
			return children[i]
		}
	}
	return nil
}

func chatitoChildrenHaveError(children []*Node) bool {
	for _, child := range children {
		if child != nil && child.HasError() {
			return true
		}
	}
	return false
}
