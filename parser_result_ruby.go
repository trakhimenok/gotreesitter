package gotreesitter

func normalizeRubyTopLevelModuleBounds(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "ruby" || root.Type(lang) != "program" || len(source) == 0 {
		return
	}
	end := lastNonTriviaByteEnd(source)
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child == nil || child.IsExtra() || child.Type(lang) != "module" {
			continue
		}
		firstChild := resultChildAt(child, 0)
		if firstChild != nil && child.startByte < firstChild.startByte {
			child.startByte = firstChild.startByte
			child.startPoint = firstChild.startPoint
		}
		if child.endByte == root.endByte && end > child.startByte && end < child.endByte {
			child.endByte = end
			child.endPoint = advancePointByBytes(Point{}, source[:end])
		}
	}
}

func normalizeRubyThenStarts(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "ruby" {
		return
	}
	walkResultTree(root, func(n *Node) {
		switch n.Type(lang) {
		case "elsif", "if", "unless", "when":
			normalizeRubyThenChildStarts(n, lang)
		}
	})
}

func normalizeRubyThenChildStarts(parent *Node, lang *Language) {
	childCount := resultChildCount(parent)
	if parent == nil || lang == nil || childCount < 2 {
		return
	}
	for i := 0; i < childCount; i++ {
		child := resultChildAt(parent, i)
		if child == nil || child.Type(lang) != "then" || i == 0 {
			continue
		}
		prev := (*Node)(nil)
		for j := i - 1; j >= 0; j-- {
			if candidate := resultChildAt(parent, j); candidate != nil {
				prev = candidate
				break
			}
		}
		if prev == nil || prev.endByte >= child.startByte {
			continue
		}
		child.startByte = prev.endByte
		child.startPoint = prev.endPoint
	}
}
