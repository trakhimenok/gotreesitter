package gotreesitter

func normalizeIniSectionStarts(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "ini" {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "section" {
			for i := 0; i < resultChildCount(n); i++ {
				child := resultChildAt(n, i)
				if child == nil {
					continue
				}
				if n.startByte < child.startByte {
					n.startByte = child.startByte
					n.startPoint = child.startPoint
				}
				break
			}
		}
	})
}
