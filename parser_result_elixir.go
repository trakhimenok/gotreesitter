package gotreesitter

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
