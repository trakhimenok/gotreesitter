package gotreesitter

func normalizeJavaCompatibility(root *Node, source []byte, lang *Language) {
	normalizeJavaCollapsedLeafChildren(root, source, lang)
}

func normalizeJavaCollapsedLeafChildren(root *Node, source []byte, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || lang.Name != "java" || len(source) == 0 {
		return counters
	}
	modifiersSym, ok := symbolByName(lang, "modifiers")
	if !ok {
		return counters
	}
	asteriskSym, ok := symbolByName(lang, "asterisk")
	if !ok {
		return counters
	}

	childSymsByParent := map[Symbol]map[string]Symbol{}
	childNamed := map[Symbol]bool{}
	addChild := func(parent Symbol, name string) {
		childSym, ok := symbolByName(lang, name)
		if !ok {
			return
		}
		children := childSymsByParent[parent]
		if children == nil {
			children = map[string]Symbol{}
			childSymsByParent[parent] = children
		}
		children[name] = childSym
		if int(childSym) < len(lang.SymbolMetadata) {
			childNamed[childSym] = lang.SymbolMetadata[childSym].Named
		}
	}
	for _, keyword := range []string{
		"abstract",
		"default",
		"final",
		"native",
		"non-sealed",
		"private",
		"protected",
		"public",
		"sealed",
		"static",
		"strictfp",
		"synchronized",
		"transient",
		"volatile",
	} {
		addChild(modifiersSym, keyword)
	}
	addChild(asteriskSym, "*")
	if len(childSymsByParent) == 0 {
		return counters
	}

	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if childCount == 0 && int(n.startByte) <= len(source) && int(n.endByte) <= len(source) && n.startByte <= n.endByte {
			if childSyms := childSymsByParent[n.symbol]; len(childSyms) > 0 {
				if childSym, ok := childSyms[string(source[n.startByte:n.endByte])]; ok {
					child := newLeafNodeInArena(n.ownerArena, childSym, childNamed[childSym], n.startByte, n.endByte, n.startPoint, n.endPoint)
					n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
					counters.nodesRewritten++
					childCount = 1
				}
			}
		}
		for i := 0; i < childCount; i++ {
			walk(resultChildAt(n, i))
		}
	}
	walk(root)
	return counters
}
