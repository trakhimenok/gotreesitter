package gotreesitter

func normalizeHCLConfigFileRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "hcl" || root.Type(lang) != "config_file" {
		return
	}
	normalizeHCLCollapsedNamedLeafChildren(root, source, lang)
	childCount := resultChildCount(root)
	if childCount == 0 {
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

type hclCollapsedNamedLeafRule struct {
	parentName string
	childNames []string
}

var hclCollapsedNamedLeafRules = []hclCollapsedNamedLeafRule{
	{parentName: "block_start", childNames: []string{"{"}},
	{parentName: "block_end", childNames: []string{"}"}},
	{parentName: "bool_lit", childNames: []string{"true", "false"}},
	{parentName: "tuple_start", childNames: []string{"["}},
	{parentName: "tuple_end", childNames: []string{"]"}},
	{parentName: "object_start", childNames: []string{"{"}},
	{parentName: "object_end", childNames: []string{"}"}},
}

type hclCollapsedNamedLeafChild struct {
	sym   Symbol
	named bool
}

func normalizeHCLCollapsedNamedLeafChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 {
		return
	}
	rules := make(map[Symbol]map[string]hclCollapsedNamedLeafChild, len(hclCollapsedNamedLeafRules))
	for _, rule := range hclCollapsedNamedLeafRules {
		parentSym, ok := lang.symbolByNameAndNamed(rule.parentName, true)
		if !ok {
			parentSym, ok = symbolByName(lang, rule.parentName)
		}
		if !ok {
			continue
		}
		children := rules[parentSym]
		if children == nil {
			children = make(map[string]hclCollapsedNamedLeafChild, len(rule.childNames))
			rules[parentSym] = children
		}
		for _, childName := range rule.childNames {
			childSym, ok := lang.symbolByNameAndNamed(childName, false)
			if !ok {
				childSym, ok = symbolByName(lang, childName)
			}
			if !ok {
				continue
			}
			children[childName] = hclCollapsedNamedLeafChild{sym: childSym, named: symbolIsNamed(lang, childSym)}
		}
	}
	if len(rules) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		children, ok := rules[n.symbol]
		if !ok || resultChildCount(n) != 0 || int(n.startByte) > len(source) || int(n.endByte) > len(source) || n.startByte > n.endByte {
			return
		}
		childRule, ok := children[string(source[n.startByte:n.endByte])]
		if !ok {
			return
		}
		child := newLeafNodeInArena(n.ownerArena, childRule.sym, childRule.named, n.startByte, n.endByte, n.startPoint, n.endPoint)
		child.parent = n
		child.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
	})
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
