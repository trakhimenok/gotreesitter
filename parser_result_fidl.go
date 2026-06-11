package gotreesitter

func normalizeFIDLCompatibility(root *Node, source []byte, lang *Language) {
	normalizeFIDLVersionedLayoutModifiers(root, source, lang)
}

func normalizeFIDLVersionedLayoutModifiers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "fidl" || len(source) == 0 {
		return
	}
	changed := false
	walkResultTree(root, func(n *Node) {
		if normalizeFIDLVersionedLayoutDeclaration(n, source, lang) {
			changed = true
		}
	})
	if changed {
		propagateFIDLHasError(root)
	}
}

func normalizeFIDLVersionedLayoutDeclaration(n *Node, source []byte, lang *Language) bool {
	if n == nil || n.Type(lang) != "layout_declaration" || resultChildCount(n) != 4 {
		return false
	}
	typeNode := resultChildAt(n, 0)
	nameNode := resultChildAt(n, 1)
	eqNode := resultChildAt(n, 2)
	inline := resultChildAt(n, 3)
	if typeNode == nil || nameNode == nil || eqNode == nil || inline == nil ||
		eqNode.Type(lang) != "=" || inline.Type(lang) != "inline_layout" || resultChildCount(inline) < 4 {
		return false
	}
	firstMod := resultChildAt(inline, 0)
	secondMod := resultChildAt(inline, 1)
	layoutKind := resultChildAt(inline, 2)
	layoutBody := resultChildAt(inline, 3)
	if !fidlIsDeclarationModifier(firstMod, lang) || !fidlIsDeclarationModifier(secondMod, lang) ||
		layoutKind == nil || layoutKind.Type(lang) != "layout_kind" || layoutBody == nil || layoutBody.Type(lang) != "layout_body" {
		return false
	}
	firstArgs, ok := fidlModifierArgs(source, firstMod.endByte, secondMod.startByte)
	if !ok {
		return false
	}
	secondArgs, ok := fidlModifierArgs(source, secondMod.endByte, layoutKind.startByte)
	if !ok {
		return false
	}
	eqSym, okEq := symbolByName(lang, "=")
	lparenSym, okLParen := symbolByName(lang, "(")
	rparenSym, okRParen := symbolByName(lang, ")")
	if !okEq || !okLParen || !okRParen || n.ownerArena == nil {
		return false
	}
	arena := n.ownerArena
	firstOpen := fidlLeaf(arena, lparenSym, false, source, firstArgs.open, firstArgs.open+1)
	firstNameErr := fidlErrorLeaf(arena, source, firstArgs.nameStart, firstArgs.nameEnd)
	firstArgEq := fidlLeaf(arena, eqSym, false, source, firstArgs.eq, firstArgs.eq+1)
	firstTailErr := fidlModifierValueTailError(arena, source, firstArgs, rparenSym)

	secondOpen := fidlLeaf(arena, lparenSym, false, source, secondArgs.open, secondArgs.open+1)
	secondNameErr := fidlErrorLeaf(arena, source, secondArgs.nameStart, secondArgs.nameEnd)
	secondArgEq := fidlLeaf(arena, eqSym, false, source, secondArgs.eq, secondArgs.eq+1)
	secondTailErr := fidlModifierValueTailError(arena, source, secondArgs, rparenSym)

	outerChildren := cloneNodeSliceInArena(arena, []*Node{
		eqNode,
		firstMod,
		firstOpen,
		firstNameErr,
		firstArgEq,
		firstTailErr,
		secondMod,
		secondOpen,
		secondNameErr,
	})
	outerErr := newParentNodeInArena(arena, errorSymbol, true, outerChildren, nil, 0)
	outerErr.setExtra(true)
	outerErr.setHasError(true)
	outerErr.startByte = eqNode.startByte
	outerErr.startPoint = eqNode.startPoint
	outerErr.endByte = secondArgs.eq
	outerErr.endPoint = fidlPointAt(source, secondArgs.eq)

	replaceNodeChildrenUnfielded(inline, cloneNodeSliceInArena(arena, []*Node{layoutKind, layoutBody}))
	inline.startByte = layoutKind.startByte
	inline.startPoint = layoutKind.startPoint
	inline.setHasError(false)

	replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(arena, []*Node{
		typeNode,
		nameNode,
		outerErr,
		secondArgEq,
		secondTailErr,
		inline,
	}))
	n.setHasError(true)
	return true
}

type fidlModifierArgSpan struct {
	open       uint32
	nameStart  uint32
	nameEnd    uint32
	eq         uint32
	valueStart uint32
	valueEnd   uint32
	close      uint32
}

func fidlIsDeclarationModifier(n *Node, lang *Language) bool {
	return n != nil && n.Type(lang) == "declaration_modifiers" && resultChildCount(n) == 1
}

func fidlModifierArgs(source []byte, afterKeyword, beforeNext uint32) (fidlModifierArgSpan, bool) {
	if afterKeyword >= beforeNext || int(beforeNext) > len(source) {
		return fidlModifierArgSpan{}, false
	}
	i := fidlSkipSpaces(source, afterKeyword, beforeNext)
	if i >= beforeNext || source[i] != '(' {
		return fidlModifierArgSpan{}, false
	}
	nameStart := fidlSkipSpaces(source, i+1, beforeNext)
	eq := nameStart
	for eq < beforeNext && source[eq] != '=' && source[eq] != ')' && source[eq] != '\n' && source[eq] != '\r' {
		eq++
	}
	nameEnd := fidlTrimRightSpaces(source, nameStart, eq)
	if eq >= beforeNext || source[eq] != '=' || nameEnd <= nameStart {
		return fidlModifierArgSpan{}, false
	}
	valueStart := fidlSkipSpaces(source, eq+1, beforeNext)
	close := valueStart
	for close < beforeNext && source[close] != ')' && source[close] != '\n' && source[close] != '\r' {
		close++
	}
	valueEnd := fidlTrimRightSpaces(source, valueStart, close)
	if close >= beforeNext || source[close] != ')' || valueEnd <= valueStart {
		return fidlModifierArgSpan{}, false
	}
	return fidlModifierArgSpan{
		open:       i,
		nameStart:  nameStart,
		nameEnd:    nameEnd,
		eq:         eq,
		valueStart: valueStart,
		valueEnd:   valueEnd,
		close:      close,
	}, true
}

func fidlSkipSpaces(source []byte, start, end uint32) uint32 {
	for start < end {
		switch source[start] {
		case ' ', '\t':
			start++
		default:
			return start
		}
	}
	return start
}

func fidlTrimRightSpaces(source []byte, start, end uint32) uint32 {
	for end > start {
		switch source[end-1] {
		case ' ', '\t':
			end--
		default:
			return end
		}
	}
	return end
}

func fidlModifierValueTailError(arena *nodeArena, source []byte, span fidlModifierArgSpan, rparenSym Symbol) *Node {
	children := []*Node{}
	if fidlSourceLooksIdentifier(source[span.valueStart:span.valueEnd]) {
		children = append(children, fidlErrorLeaf(arena, source, span.valueStart, span.valueEnd))
	}
	children = append(children, fidlLeaf(arena, rparenSym, false, source, span.close, span.close+1))
	err := newParentNodeInArena(arena, errorSymbol, true, cloneNodeSliceInArena(arena, children), nil, 0)
	err.setExtra(true)
	err.setHasError(true)
	err.startByte = span.valueStart
	err.startPoint = fidlPointAt(source, span.valueStart)
	err.endByte = span.close + 1
	err.endPoint = fidlPointAt(source, span.close+1)
	return err
}

func fidlSourceLooksIdentifier(src []byte) bool {
	if len(src) == 0 {
		return false
	}
	for i, b := range src {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' || (i > 0 && b >= '0' && b <= '9') {
			continue
		}
		return false
	}
	return true
}

func fidlLeaf(arena *nodeArena, sym Symbol, named bool, source []byte, start, end uint32) *Node {
	return newLeafNodeInArena(arena, sym, named, start, end, fidlPointAt(source, start), fidlPointAt(source, end))
}

func fidlErrorLeaf(arena *nodeArena, source []byte, start, end uint32) *Node {
	return newLeafNodeInArena(arena, errorSymbol, true, start, end, fidlPointAt(source, start), fidlPointAt(source, end))
}

func fidlPointAt(source []byte, pos uint32) Point {
	if int(pos) > len(source) {
		pos = uint32(len(source))
	}
	return advancePointByBytes(Point{}, source[:pos])
}

func propagateFIDLHasError(n *Node) bool {
	if n == nil {
		return false
	}
	hasErr := n.HasError()
	for i := 0; i < resultChildCount(n); i++ {
		if propagateFIDLHasError(resultChildAt(n, i)) {
			hasErr = true
		}
	}
	if hasErr {
		n.setHasError(true)
	}
	return hasErr
}
