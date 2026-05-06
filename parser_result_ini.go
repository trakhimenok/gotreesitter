package gotreesitter

func normalizeIniSectionStarts(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "ini" {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "section" {
			for _, child := range n.children {
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
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}
