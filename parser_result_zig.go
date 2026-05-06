package gotreesitter

func normalizeZigEmptyInitListFields(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "zig" {
		return
	}
	fieldConstantID, ok := lang.FieldByName("field_constant")
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if len(n.fieldIDs) == len(n.children) {
			for i, child := range n.children {
				if child == nil || n.fieldIDs[i] != fieldConstantID || child.Type(lang) != "InitList" {
					continue
				}
				if n.Type(lang) != "SuffixExpr" || len(n.children) != 2 || i != 1 || n.children[0] == nil || n.children[0].Type(lang) != "." {
					continue
				}
				n.fieldIDs[i] = 0
				if len(n.fieldSources) == len(n.children) {
					n.fieldSources[i] = fieldSourceNone
				}
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}
