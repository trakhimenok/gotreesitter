package gotreesitter

func normalizeSvelteTrailingExtraTrivia(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "svelte" || root.Type(lang) != "document" || len(root.children) == 0 || len(source) == 0 {
		return
	}
	normalizeSvelteRecoveredRawTextErrors(root, source, lang)

	last := root.children[len(root.children)-1]
	if last == nil || last.IsNamed() || !last.IsExtra() || len(last.children) != 0 {
		return
	}
	if last.Type(lang) != "_tag_value_token1" {
		return
	}
	if last.startByte >= last.endByte || last.endByte != root.endByte || int(last.endByte) > len(source) {
		return
	}
	if !bytesAreTrivia(source[last.startByte:last.endByte]) {
		return
	}
	root.children = root.children[:len(root.children)-1]
	if len(root.fieldIDs) > len(root.children) {
		root.fieldIDs = root.fieldIDs[:len(root.children)]
	}
	if len(root.fieldSources) > len(root.children) {
		root.fieldSources = root.fieldSources[:len(root.children)]
	}
}

func normalizeSvelteRecoveredRawTextErrors(root *Node, source []byte, lang *Language) {
	walkResultTree(root, func(n *Node) {
		if n == nil || n.symbol != errorSymbol {
			return
		}
		children := resultChildSliceForMutation(n)
		if len(children) == 0 {
			return
		}
		changed := false
		out := make([]*Node, 0, len(children)+3)
		for i, child := range children {
			if child == nil {
				out = append(out, child)
				continue
			}
			if child.symbol == errorSymbol {
				if !child.IsNamed() {
					changed = true
				}
				child.setNamed(true)
				out = append(out, child)
				continue
			}
			if child.Type(lang) != "svelte_raw_text" {
				out = append(out, child)
				continue
			}
			if split, ok := svelteRecoveredConstAssignmentTail(children, i, child, source, lang); ok {
				out = append(out, split...)
				changed = true
				continue
			}
			child.symbol = errorSymbol
			child.setNamed(true)
			child.setHasError(true)
			out = append(out, child)
			changed = true
		}
		if changed {
			replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(n.ownerArena, out))
			n.setHasError(true)
		}
	})
}

func svelteRecoveredConstAssignmentTail(siblings []*Node, idx int, raw *Node, source []byte, lang *Language) ([]*Node, bool) {
	if idx < 2 || raw == nil || int(raw.endByte) > len(source) || raw.startByte >= raw.endByte {
		return nil, false
	}
	if siblings[1] == nil || siblings[1].Type(lang) != "expression_tag" || string(source[siblings[1].startByte:siblings[1].endByte]) != "@const" {
		return nil, false
	}
	start := int(raw.startByte)
	end := int(raw.endByte)
	eq := start
	for eq < end && svelteIsASCIISpace(source[eq]) {
		eq++
	}
	if eq >= end || source[eq] != '=' {
		return nil, false
	}
	quote := eq + 1
	for quote < end && svelteIsASCIISpace(source[quote]) {
		quote++
	}
	if quote >= end || (source[quote] != '\'' && source[quote] != '"') {
		return nil, false
	}
	closeQuote := end - 1
	for closeQuote > quote && svelteIsASCIISpace(source[closeQuote]) {
		closeQuote--
	}
	if closeQuote <= quote || source[closeQuote] != source[quote] || quote+1 >= closeQuote {
		return nil, false
	}
	for i := quote + 1; i < closeQuote; i++ {
		if source[i] == source[quote] || source[i] == '\n' || source[i] == '\r' {
			return nil, false
		}
	}
	eqSym, ok := firstSvelteTokenSymbol(lang, "=")
	if !ok {
		return nil, false
	}
	quoteSym, ok := firstSvelteTokenSymbol(lang, string(source[quote]))
	if !ok {
		return nil, false
	}
	arena := raw.ownerArena
	eqNode := svelteLeaf(arena, lang, source, eqSym, uint32(eq), uint32(eq+1))
	openQuote := svelteLeaf(arena, lang, source, quoteSym, uint32(quote), uint32(quote+1))
	err := newLeafNodeInArena(arena, errorSymbol, true, uint32(quote+1), uint32(closeQuote), sveltePointAt(source, uint32(quote+1)), sveltePointAt(source, uint32(closeQuote)))
	err.setHasError(true)
	closeQuoteNode := svelteLeaf(arena, lang, source, quoteSym, uint32(closeQuote), uint32(closeQuote+1))
	return []*Node{eqNode, openQuote, err, closeQuoteNode}, true
}

func firstSvelteTokenSymbol(lang *Language, name string) (Symbol, bool) {
	syms := lang.TokenSymbolsByName(name)
	if len(syms) == 0 {
		return 0, false
	}
	return syms[0], true
}

func svelteLeaf(arena *nodeArena, lang *Language, source []byte, sym Symbol, start, end uint32) *Node {
	return newLeafNodeInArena(arena, sym, symbolIsNamed(lang, sym), start, end, sveltePointAt(source, start), sveltePointAt(source, end))
}

func sveltePointAt(source []byte, off uint32) Point {
	if int(off) > len(source) {
		off = uint32(len(source))
	}
	return advancePointByBytes(Point{}, source[:off])
}

func svelteIsASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}
