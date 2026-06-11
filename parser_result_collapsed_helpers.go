package gotreesitter

// normalizeCollapsedNamedLeafChildren restores collapsed single-anonymous-child
// nodes. When a named node (parentName) wraps a single anonymous token
// (childName) and the collapse logic strips the child, this function
// reconstructs the child so the tree matches C tree-sitter output.
func normalizeCollapsedNamedLeafChildren(root *Node, lang *Language, parentName, childName string) {
	normalizeCollapsedNamedLeafChildrenWithStats(root, lang, parentName, childName)
}

func normalizeCollapsedNamedLeafChildrenBySource(root *Node, source []byte, lang *Language, parentName string, childNames ...string) {
	normalizeCollapsedNamedLeafChildrenBySourceWithStats(root, source, lang, parentName, childNames...)
}

type collapsedNamedLeafRule struct {
	languageName string
	parentName   string
	childName    string
}

var resultCollapsedNamedLeafRules = []collapsedNamedLeafRule{
	{languageName: "c_sharp", parentName: "implicit_type", childName: "var"},
	{languageName: "cobol", parentName: "period", childName: "."},
	{languageName: "COBOL", parentName: "period", childName: "."},
	// Ruby collapses bare_string/bare_symbol -> string_content unary wrappers
	// into a leaf; C keeps the single named string_content child over the same
	// span. Only fires when Go's node has zero children (i.e. no escapes /
	// interpolation, which would already materialize children in both trees).
	{languageName: "ruby", parentName: "bare_string", childName: "string_content"},
	{languageName: "ruby", parentName: "bare_symbol", childName: "string_content"},
	// Apex wraps its contextual / DML / trigger-event keywords as a named node
	// over the anonymous keyword token (`keyword -> 'keyword'`). Go collapses
	// these single-token wrappers to a leaf; C keeps the anonymous token child.
	// Verified exhaustively against the apex corpus: every collapsed instance's
	// C child is the same-named anonymous token over the identical span.
	{languageName: "apex", parentName: "after_delete", childName: "after_delete"},
	{languageName: "apex", parentName: "after_insert", childName: "after_insert"},
	{languageName: "apex", parentName: "after_undelete", childName: "after_undelete"},
	{languageName: "apex", parentName: "after_update", childName: "after_update"},
	{languageName: "apex", parentName: "before_delete", childName: "before_delete"},
	{languageName: "apex", parentName: "before_insert", childName: "before_insert"},
	{languageName: "apex", parentName: "before_update", childName: "before_update"},
	{languageName: "apex", parentName: "delete", childName: "delete"},
	{languageName: "apex", parentName: "insert", childName: "insert"},
	{languageName: "apex", parentName: "super", childName: "super"},
	{languageName: "apex", parentName: "system", childName: "system"},
	{languageName: "apex", parentName: "undelete", childName: "undelete"},
	{languageName: "apex", parentName: "upsert", childName: "upsert"},
	{languageName: "apex", parentName: "user", childName: "user"},
}

func normalizeResultCollapsedNamedLeafChildren(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	for _, rule := range resultCollapsedNamedLeafRules {
		if rule.languageName != lang.Name {
			continue
		}
		normalizeCollapsedNamedLeafChildren(root, lang, rule.parentName, rule.childName)
	}
}

func normalizeCollapsedNamedLeafChildrenWithStats(root *Node, lang *Language, parentName, childName string) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil {
		return counters
	}
	parentSym, ok := lang.symbolByNameAndNamed(parentName, true)
	if !ok {
		parentSym, ok = symbolByName(lang, parentName)
	}
	if !ok {
		return counters
	}
	childSym, childOk := lang.symbolByNameAndNamed(childName, false)
	if !childOk {
		childSym, childOk = symbolByName(lang, childName)
		if !childOk {
			return counters
		}
	}
	childNamed := symbolIsNamed(lang, childSym)
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if n.symbol == parentSym && childCount == 0 {
			child := newLeafNodeInArena(n.ownerArena, childSym, childNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
			child.parent = n
			child.childIndex = 0
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
			counters.nodesRewritten++
		}
	})
	return counters
}

func normalizeCollapsedNamedLeafChildrenBySourceWithStats(root *Node, source []byte, lang *Language, parentName string, childNames ...string) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || len(source) == 0 || len(childNames) == 0 {
		return counters
	}
	parentSym, ok := lang.symbolByNameAndNamed(parentName, true)
	if !ok {
		parentSym, ok = symbolByName(lang, parentName)
	}
	if !ok {
		return counters
	}
	childSyms := make(map[string]Symbol, len(childNames))
	childNamed := make(map[Symbol]bool, len(childNames))
	for _, childName := range childNames {
		childSym, ok := lang.symbolByNameAndNamed(childName, false)
		if !ok {
			childSym, ok = symbolByName(lang, childName)
			if !ok {
				continue
			}
		}
		childSyms[childName] = childSym
		childNamed[childSym] = symbolIsNamed(lang, childSym)
	}
	if len(childSyms) == 0 {
		return counters
	}
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if n.symbol != parentSym || childCount != 0 || int(n.startByte) > len(source) || int(n.endByte) > len(source) || n.startByte > n.endByte {
			return
		}
		childSym, ok := childSyms[string(source[n.startByte:n.endByte])]
		if !ok {
			return
		}
		child := newLeafNodeInArena(n.ownerArena, childSym, childNamed[childSym], n.startByte, n.endByte, n.startPoint, n.endPoint)
		child.parent = n
		child.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
		counters.nodesRewritten++
	})
	return counters
}
