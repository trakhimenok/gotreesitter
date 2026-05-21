package gotreesitter

func normalizeHaskellCompatibility(root *Node, source []byte, lang *Language) {
	normalizeHaskellImportsSpan(root, source, lang)
	normalizeHaskellZeroWidthTokens(root, lang)
	normalizeHaskellRootImportField(root, lang)
	normalizeHaskellDeclarationsSpan(root, source, lang)
	normalizeHaskellLocalBindsStarts(root, source, lang)
	normalizeHaskellQuasiquoteStarts(root, source, lang)
}
func normalizeHaskellImportsSpan(root *Node, source []byte, lang *Language) {
	childCount := resultChildCount(root)
	if root == nil || childCount < 2 || len(source) == 0 || lang == nil || lang.Name != "haskell" {
		return
	}
	for i := 0; i+1 < childCount; i++ {
		left := resultChildAt(root, i)
		right := resultChildAt(root, i+1)
		if left == nil || right == nil {
			continue
		}
		if left.Type(lang) != "imports" {
			continue
		}
		if left.endByte >= right.startByte {
			continue
		}
		if left.endByte > uint32(len(source)) || right.startByte > uint32(len(source)) {
			continue
		}
		gap := source[left.endByte:right.startByte]
		if !bytesAreTrivia(gap) {
			continue
		}
		left.endByte = right.startByte
		left.endPoint = advancePointByBytes(left.endPoint, gap)
	}
}

func normalizeHaskellZeroWidthTokens(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "haskell" || resultChildCount(root) == 0 {
		return
	}
	tokenSym, hasTokenSym := symbolByName(lang, "_token1")
	view := resultMutableChildrenForMutation(root)
	if view.hasFinalChildRefs() && hasTokenSym {
		changed := false
		for i := 0; i < view.Len(); i++ {
			entry, ok := view.Entry(i)
			if ok && stackEntryNodeSymbol(entry) == tokenSym && stackEntryNodeStartByte(entry) == stackEntryNodeEndByte(entry) {
				changed = true
				break
			}
		}
		if changed {
			view.FilterFinalRefs(func(_ int, entry stackEntry) bool {
				return stackEntryNodeSymbol(entry) != tokenSym || stackEntryNodeStartByte(entry) != stackEntryNodeEndByte(entry)
			})
		}
		return
	}
	if !hasTokenSym {
		return
	}
	children := resultChildSliceForMutation(root)
	filtered := children[:0]
	changed := false
	for _, child := range children {
		if child == nil {
			changed = true
			continue
		}
		if child.symbol == tokenSym && child.startByte == child.endByte {
			changed = true
			continue
		}
		filtered = append(filtered, child)
	}
	if !changed {
		return
	}
	root.children = cloneNodeSliceInArena(root.ownerArena, filtered)
	root.fieldIDs = nil
	root.fieldSources = nil
	populateParentNode(root, root.children)
}

func normalizeHaskellRootImportField(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "haskell" || resultChildCount(root) == 0 {
		return
	}
	if len(lang.FieldNames) == 0 {
		return
	}
	view := resultMutableChildrenForMutation(root)
	fieldStorageReady := len(root.fieldIDs) == view.Len() && len(root.fieldSources) == view.Len()
	for i := 0; i < view.Len(); i++ {
		entry, ok := view.Entry(i)
		if !ok {
			continue
		}
		fid, ok := lang.FieldByName(symbolTypeName(lang, stackEntryNodeSymbol(entry)))
		if !ok {
			continue
		}
		if !fieldStorageReady {
			ensureNodeFieldStorage(root, view.Len())
			fieldStorageReady = true
		}
		root.fieldIDs[i] = fid
		root.fieldSources[i] = fieldSourceInherited
	}
}

func normalizeHaskellDeclarationsSpan(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "haskell" || len(source) == 0 {
		return
	}
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child == nil || child.Type(lang) != "declarations" {
			continue
		}
		if child.endByte >= root.endByte || root.endByte > uint32(len(source)) {
			continue
		}
		gap := source[child.endByte:root.endByte]
		if !bytesAreTrivia(gap) {
			continue
		}
		extendNodeEndTo(child, root.endByte, source)
	}
}

func normalizeHaskellLocalBindsStarts(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "haskell" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "let_in" && resultChildCount(n) >= 2 {
			letNode := resultChildAt(n, 0)
			localBinds := resultChildAt(n, 1)
			if letNode != nil && localBinds != nil && letNode.Type(lang) == "let" && localBinds.Type(lang) == "local_binds" && letNode.endByte < localBinds.startByte && localBinds.startByte <= uint32(len(source)) {
				gap := source[letNode.endByte:localBinds.startByte]
				if len(gap) > 0 && bytesAreTrivia(gap) && !bytesContainLineBreak(gap) {
					localBinds.startByte = letNode.endByte
					localBinds.startPoint = letNode.endPoint
				}
			}
		}
	})
}

func normalizeHaskellQuasiquoteStarts(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "haskell" || len(source) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "quasiquote" && n.startByte > 0 {
			start := int(n.startByte)
			if source[start-1] == ' ' && start < len(source) && source[start] == '[' {
				n.startByte--
				if n.startPoint.Column > 0 {
					n.startPoint.Column--
				} else if n.startPoint.Row > 0 {
					n.startPoint = advancePointByBytes(Point{}, source[:n.startByte])
				}
			}
		}
	})
}
