package gotreesitter

// normalizeCollapsedNamedLeafChildren restores collapsed single-anonymous-child
// nodes. When a named node (parentName) wraps a single anonymous token
// (childName) and the collapse logic strips the child, this function
// reconstructs the child so the tree matches C tree-sitter output.
func normalizeCollapsedNamedLeafChildren(root *Node, lang *Language, parentName, childName string) {
	normalizeCollapsedNamedLeafChildrenWithStats(root, lang, parentName, childName)
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
	{languageName: "tsx", parentName: "existential_type", childName: "*"},
	{languageName: "typescript", parentName: "existential_type", childName: "*"},
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
