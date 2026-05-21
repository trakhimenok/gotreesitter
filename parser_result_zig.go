package gotreesitter

func normalizeZigEmptyInitListFields(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "zig" {
		return
	}
	fieldConstantID, ok := lang.FieldByName("field_constant")
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if len(n.fieldIDs) == len(n.children) {
			for i, child := range n.children {
				if child == nil || n.fieldIDs[i] != fieldConstantID || child.Type(lang) != "InitList" {
					continue
				}
				if n.Type(lang) != "SuffixExpr" || len(n.children) != 2 || i != 1 || n.children[0] == nil || n.children[0].Type(lang) != "." {
					continue
				}
				clearNodeChildField(n, i)
			}
		}
	})
}
