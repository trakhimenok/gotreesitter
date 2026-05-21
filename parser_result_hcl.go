package gotreesitter

func normalizeHCLConfigFileRoot(root *Node, lang *Language) {
	childCount := resultChildCount(root)
	if root == nil || lang == nil || lang.Name != "hcl" || root.Type(lang) != "config_file" || childCount == 0 {
		return
	}
	whitespaceSym, hasWhitespaceSym := symbolByName(lang, "_whitespace")
	view := resultMutableChildrenForMutation(root)
	filteredChanged := false
	if view.hasFinalChildRefs() && hasWhitespaceSym {
		for i := 0; i < view.Len(); i++ {
			entry, ok := view.Entry(i)
			if ok && stackEntryNodeSymbol(entry) == whitespaceSym {
				filteredChanged = true
				break
			}
		}
		if filteredChanged {
			view.FilterFinalRefs(func(_ int, entry stackEntry) bool {
				return stackEntryNodeSymbol(entry) != whitespaceSym
			})
		}
	} else {
		for i := 0; i < childCount; i++ {
			child := resultChildAt(root, i)
			if child != nil && child.Type(lang) == "_whitespace" {
				filteredChanged = true
				break
			}
		}
		if filteredChanged {
			children := resultChildSliceForMutation(root)
			filtered := make([]*Node, 0, len(children))
			for _, child := range children {
				if child == nil || child.Type(lang) == "_whitespace" {
					continue
				}
				filtered = append(filtered, child)
			}
			replaceNodeChildrenUnfielded(root, cloneNodeSliceIfArena(root.ownerArena, filtered))
		}
	}
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child == nil || child.Type(lang) != "body" {
			continue
		}
		snapHCLBodyBounds(child)
	}
}

func snapHCLBodyBounds(body *Node) {
	if body == nil || resultChildCount(body) == 0 {
		return
	}
	var first, last *Node
	for i := 0; i < resultChildCount(body); i++ {
		child := resultChildAt(body, i)
		if child == nil {
			continue
		}
		if first == nil {
			first = child
		}
		last = child
	}
	if first == nil || last == nil {
		return
	}
	body.startByte = first.startByte
	body.startPoint = first.startPoint
	body.endByte = last.endByte
	body.endPoint = last.endPoint
}
