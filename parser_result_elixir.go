package gotreesitter

func normalizeElixirNestedCallTargetFields(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "elixir" {
		return
	}
	targetID, ok := lang.FieldByName("target")
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "call" && len(n.children) >= 2 {
			first := n.children[0]
			second := n.children[1]
			if first != nil && second != nil &&
				first.Type(lang) == "call" &&
				second.Type(lang) == "arguments" &&
				(len(n.fieldIDs) == 0 || n.fieldIDs[0] == 0) {
				if len(n.fieldIDs) < len(n.children) {
					fieldIDs := make([]FieldID, len(n.children))
					copy(fieldIDs, n.fieldIDs)
					n.fieldIDs = fieldIDs
				}
				n.fieldIDs[0] = targetID
				if len(n.fieldSources) < len(n.children) {
					fieldSources := make([]uint8, len(n.children))
					copy(fieldSources, n.fieldSources)
					n.fieldSources = fieldSources
				}
				n.fieldSources[0] = fieldSourceInherited
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}
