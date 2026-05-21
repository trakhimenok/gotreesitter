package gotreesitter

import "bytes"

const goSemicolonContainerSymbolCount = 11

type goCompatibilitySymbols struct {
	semicolon         Symbol
	expressionCase    Symbol
	defaultCase       Symbol
	typeCase          Symbol
	communicationCase Symbol
	statementList     Symbol
	statementListTail Symbol
	semiContainers    [goSemicolonContainerSymbolCount]Symbol
	semiContainerLen  int
}

func normalizeGoCompatibility(root *Node, source []byte, lang *Language) {
	normalizeGoCompatibilityInRanges(root, source, lang, nil)
}

func normalizeGoCompatibilityInRanges(root *Node, source []byte, lang *Language, incrementalRanges []Range) {
	if root == nil || lang == nil || lang.Name != "go" || len(source) == 0 {
		return
	}
	syms, ok := goCompatibilitySymbolsForLanguage(lang)
	if !ok {
		return
	}
	normalizeGoCompatibilitySubtree(root, source, syms, incrementalRanges)
}

func goCompatibilitySymbolsForLanguage(lang *Language) (goCompatibilitySymbols, bool) {
	var syms goCompatibilitySymbols
	var ok bool
	if syms.semicolon, ok = symbolByName(lang, ";"); !ok {
		return syms, false
	}
	if syms.expressionCase, ok = symbolByName(lang, "expression_case"); !ok {
		return syms, false
	}
	if syms.defaultCase, ok = symbolByName(lang, "default_case"); !ok {
		return syms, false
	}
	if syms.typeCase, ok = symbolByName(lang, "type_case"); !ok {
		return syms, false
	}
	if syms.communicationCase, ok = symbolByName(lang, "communication_case"); !ok {
		return syms, false
	}
	if syms.statementList, ok = symbolByName(lang, "statement_list"); !ok {
		return syms, false
	}
	if syms.statementListTail, ok = symbolByName(lang, "statement_list_repeat1"); !ok {
		return syms, false
	}
	syms.addSemicolonContainer(lang, "source_file")
	syms.addSemicolonContainer(lang, "statement_list")
	syms.addSemicolonContainer(lang, "statement_list_repeat1")
	syms.addSemicolonContainer(lang, "import_declaration")
	syms.addSemicolonContainer(lang, "var_declaration")
	syms.addSemicolonContainer(lang, "const_declaration")
	syms.addSemicolonContainer(lang, "type_declaration")
	syms.addSemicolonContainer(lang, "import_spec_list")
	syms.addSemicolonContainer(lang, "var_spec_list")
	syms.addSemicolonContainer(lang, "const_spec_list")
	syms.addSemicolonContainer(lang, "field_declaration_list")
	return syms, true
}

func (s *goCompatibilitySymbols) addSemicolonContainer(lang *Language, name string) {
	if s.semiContainerLen >= len(s.semiContainers) {
		return
	}
	sym, ok := symbolByName(lang, name)
	if !ok {
		return
	}
	s.semiContainers[s.semiContainerLen] = sym
	s.semiContainerLen++
}

func (s goCompatibilitySymbols) isSemicolonContainer(sym Symbol) bool {
	for _, candidate := range s.semiContainers[:s.semiContainerLen] {
		if candidate == sym {
			return true
		}
	}
	return false
}

func (s goCompatibilitySymbols) isCase(sym Symbol) bool {
	switch sym {
	case s.expressionCase, s.defaultCase, s.typeCase, s.communicationCase:
		return true
	default:
		return false
	}
}

func (s goCompatibilitySymbols) isStatementList(sym Symbol) bool {
	return sym == s.statementList || sym == s.statementListTail
}

func normalizeGoCompatibilitySubtree(n *Node, source []byte, syms goCompatibilitySymbols, incrementalRanges []Range) {
	if n == nil || !goNodeOverlapsAnyRange(n, incrementalRanges) {
		return
	}
	if resultChildCount(n) > 0 {
		normalizeGoSemicolonContainer(n, source, syms)
		normalizeGoAdjacentSiblingBoundaries(n, source, syms)
	}
	for i := 0; i < resultChildCount(n); i++ {
		normalizeGoCompatibilitySubtree(resultChildAt(n, i), source, syms, incrementalRanges)
	}
}

func goNodeOverlapsAnyRange(n *Node, ranges []Range) bool {
	if n == nil || len(ranges) == 0 {
		return true
	}
	for _, r := range ranges {
		if !(n.endByte < r.StartByte || r.EndByte < n.startByte) {
			return true
		}
	}
	return false
}

func normalizeGoSemicolonContainer(n *Node, source []byte, syms goCompatibilitySymbols) {
	if !syms.isSemicolonContainer(n.symbol) {
		return
	}
	view := resultMutableChildrenForMutation(n)
	if view.hasFinalChildRefs() {
		normalizeGoSemicolonFinalRefs(view, source, syms.semicolon)
		return
	}
	if !goHasDroppableSemicolonChild(n, source, syms.semicolon) {
		return
	}
	children := resultChildSliceForMutation(n)
	kept := make([]*Node, 0, len(children))
	for _, child := range children {
		if goIsDroppableSemicolonNode(child, source, syms.semicolon) {
			continue
		}
		kept = append(kept, child)
	}
	replaceNodeChildrenUnfielded(n, cloneNodeSliceIfArena(n.ownerArena, kept))
}

func normalizeGoSemicolonFinalRefs(view resultMutableChildView, source []byte, semicolon Symbol) {
	if !goHasDroppableSemicolonFinalRef(view, source, semicolon) {
		return
	}
	view.FilterFinalRefs(func(_ int, entry stackEntry) bool {
		return !goIsDroppableSemicolonEntry(entry, source, semicolon)
	})
}

func goHasDroppableSemicolonFinalRef(view resultMutableChildView, source []byte, semicolon Symbol) bool {
	for i := 0; i < view.Len(); i++ {
		entry, ok := view.Entry(i)
		if ok && goIsDroppableSemicolonEntry(entry, source, semicolon) {
			return true
		}
	}
	return false
}

func goHasDroppableSemicolonChild(n *Node, source []byte, semicolon Symbol) bool {
	for i := 0; i < resultChildCount(n); i++ {
		if goIsDroppableSemicolonNode(resultChildAt(n, i), source, semicolon) {
			return true
		}
	}
	return false
}

func goIsDroppableSemicolonNode(n *Node, source []byte, semicolon Symbol) bool {
	return n != nil && n.symbol == semicolon && goShouldDropSemicolonSpan(n.startByte, n.endByte, source)
}

func goIsDroppableSemicolonEntry(entry stackEntry, source []byte, semicolon Symbol) bool {
	return stackEntryHasNode(entry) &&
		stackEntryNodeSymbol(entry) == semicolon &&
		goShouldDropSemicolonSpan(stackEntryNodeStartByte(entry), stackEntryNodeEndByte(entry), source)
}

func goShouldDropSemicolonSpan(startByte, endByte uint32, source []byte) bool {
	if startByte >= endByte || int(endByte) > len(source) {
		return true
	}
	text := source[startByte:endByte]
	if bytes.IndexByte(text, ';') >= 0 {
		return false
	}
	return bytes.IndexByte(text, '\n') >= 0 || bytes.IndexByte(text, '\r') >= 0
}

func normalizeGoAdjacentSiblingBoundaries(n *Node, source []byte, syms goCompatibilitySymbols) {
	childCount := resultChildCount(n)
	for i := 0; i+1 < childCount; i++ {
		curr := resultChildAt(n, i)
		next := resultChildAt(n, i+1)
		if curr == nil || next == nil {
			continue
		}
		normalizeGoStatementListBoundary(curr, next, source, syms)
		normalizeGoCaseSiblingBoundary(curr, next, source, syms)
	}
}

func normalizeGoStatementListBoundary(curr, next *Node, source []byte, syms goCompatibilitySymbols) {
	if !syms.isStatementList(curr.symbol) || curr.endByte >= next.startByte || int(next.startByte) > len(source) {
		return
	}
	gap := source[curr.endByte:next.startByte]
	if !bytesAreTrivia(gap) {
		return
	}
	target := goTrailingNewlineBoundary(curr.endByte, next.startByte, source)
	if target > curr.endByte {
		extendNodeEndTo(curr, target, source)
	}
}

func normalizeGoCaseSiblingBoundary(curr, next *Node, source []byte, syms goCompatibilitySymbols) {
	if !syms.isCase(curr.symbol) || int(next.startByte) > len(source) {
		return
	}
	tail := goTrailingCaseStatementList(curr, syms)
	if tail == nil {
		return
	}
	target, hasNewline := goTrailingTriviaBoundaryBefore(next.startByte, source)
	if hasNewline {
		normalizeGoCaseBoundaryToTrivia(curr, tail, target, source)
		return
	}
	if curr.endByte > next.startByte {
		setNodeEndTo(curr, next.startByte, source)
	}
	if tail.endByte > next.startByte {
		setNodeEndTo(tail, next.startByte, source)
	}
}

func normalizeGoCaseBoundaryToTrivia(curr, tail *Node, target uint32, source []byte) {
	if curr.endByte != target {
		setNodeEndTo(curr, target, source)
	}
	switch {
	case tail.endByte > target:
		setNodeEndTo(tail, target, source)
	case tail.endByte < target && bytesAreTrivia(source[tail.endByte:target]):
		setNodeEndTo(tail, target, source)
	}
}

func goTrailingNewlineBoundary(start, end uint32, source []byte) uint32 {
	if start >= end || int(end) > len(source) || !bytesAreTrivia(source[start:end]) {
		return start
	}
	gap := source[start:end]
	if newline := bytes.LastIndexByte(gap, '\n'); newline >= 0 {
		return start + uint32(newline+1)
	}
	return start
}

func goTrailingTriviaBoundaryBefore(end uint32, source []byte) (uint32, bool) {
	if end == 0 || int(end) > len(source) {
		return end, false
	}
	start := int(end)
	for start > 0 {
		switch source[start-1] {
		case ' ', '\t', '\r', '\n':
			start--
		default:
			goto gapReady
		}
	}
gapReady:
	gap := source[start:int(end)]
	if newline := bytes.LastIndexByte(gap, '\n'); newline >= 0 {
		return uint32(start + newline + 1), true
	}
	return end, false
}

func goTrailingCaseStatementList(n *Node, syms goCompatibilitySymbols) *Node {
	childCount := resultChildCount(n)
	if n == nil || childCount == 0 {
		return nil
	}
	last := resultChildAt(n, childCount-1)
	if last == nil || !syms.isStatementList(last.symbol) {
		return nil
	}
	return last
}
