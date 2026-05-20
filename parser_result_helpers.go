package gotreesitter

func cloneNodeSliceInArena(arena *nodeArena, nodes []*Node) []*Node {
	if len(nodes) == 0 {
		return nil
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(nodes))
		copy(buf, nodes)
		return buf
	}
	buf := make([]*Node, len(nodes))
	copy(buf, nodes)
	return buf
}

func resultChildCount(n *Node) int {
	return nodeChildCountNoMaterialize(n)
}

func resultChildAt(n *Node, i int) *Node {
	return nodeChildAtForReason(n, i, materializeForNormalization)
}

func resultDenseChildrenForMutation(n *Node) []*Node {
	return nodeChildrenForReason(n, materializeForNormalization)
}

func symbolByName(lang *Language, name string) (Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	for i, symName := range lang.SymbolNames {
		if symName == name {
			return Symbol(i), true
		}
	}
	return 0, false
}

func extendNodeEndTo(n *Node, end uint32, source []byte) {
	if n == nil || end <= n.endByte || end > uint32(len(source)) {
		return
	}
	gap := source[n.endByte:end]
	n.endByte = end
	n.endPoint = advancePointByBytes(n.endPoint, gap)
}

func setNodeEndTo(n *Node, end uint32, source []byte) {
	if n == nil || end > uint32(len(source)) || end < n.startByte || end == n.endByte {
		return
	}
	if end > n.endByte {
		extendNodeEndTo(n, end, source)
		return
	}
	n.endByte = end
	n.endPoint = advancePointByBytes(Point{}, source[:end])
}

func advancePointByBytes(start Point, b []byte) Point {
	p := start
	for _, c := range b {
		if c == '\n' {
			p.Row++
			p.Column = 0
			continue
		}
		p.Column++
	}
	return p
}
