package gotreesitter

import "time"

// Parser-result assembly owns the private handoff from GLR/parse-stack nodes to
// the returned Tree. Runtime files named parser_result_*.go stay in this package
// because many compatibility normalizers rewrite private Node, Language, Symbol,
// and nodeArena state directly. Public-API parser-result regressions live in
// parser_result_test, while source fixtures belong under testdata.

type parseMaterializationTiming struct {
	resultSelectionNanos               int64
	transientParentMaterializeNanos    int64
	resultTreeBuildNanos               int64
	transientChildMaterializationNanos int64
	pythonKeywordRepairNanos           int64
	pythonRootRepairNanos              int64
	resultFinalizeRootNanos            int64
	resultExtendTrailingNanos          int64
	resultNormalizeRootStartNanos      int64
	resultCompatibilityNanos           int64
	resultParentLinkNanos              int64
}

func materializationTimingStart(t *parseMaterializationTiming) time.Time {
	if t == nil {
		return time.Time{}
	}
	return time.Now()
}

func (t *parseMaterializationTiming) addPythonKeywordRepair(start time.Time) {
	if t != nil {
		t.pythonKeywordRepairNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addPythonRootRepair(start time.Time) {
	if t != nil {
		t.pythonRootRepairNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultFinalizeRoot(start time.Time) {
	if t != nil {
		t.resultFinalizeRootNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultExtendTrailing(start time.Time) {
	if t != nil {
		t.resultExtendTrailingNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultNormalizeRootStart(start time.Time) {
	if t != nil {
		t.resultNormalizeRootStartNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultCompatibility(start time.Time) {
	if t != nil {
		t.resultCompatibilityNanos += time.Since(start).Nanoseconds()
	}
}

func (t *parseMaterializationTiming) addResultParentLink(start time.Time) {
	if t != nil {
		t.resultParentLinkNanos += time.Since(start).Nanoseconds()
	}
}

func (p *Parser) currentMaterializationTiming() *parseMaterializationTiming {
	if p == nil {
		return nil
	}
	return p.materializationTiming
}

// buildResultFromGLR picks the best stack and constructs the final tree.
// Prefers accepted stacks, then highest score, then most entries. When
// accepted stacks are otherwise tied, prefer the tree that retains an
// alias-target symbol before falling back to branch order.
func (p *Parser) buildResultFromGLR(stacks []glrStack, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node, transientParents *transientParentScratch, transientChildren *transientChildScratch, skipErrorRank bool, materializationTiming *parseMaterializationTiming) *Tree {
	if len(stacks) == 0 {
		arena.Release()
		return parseErrorTree(source, p.language)
	}
	selectionStart := time.Time{}
	if materializationTiming != nil {
		selectionStart = time.Now()
	}
	best := 0
	for i := 1; i < len(stacks); i++ {
		if stackCompareForResultSelection(p, arena, &stacks[i], &stacks[best], skipErrorRank) > 0 {
			best = i
		}
	}
	if materializationTiming != nil {
		materializationTiming.resultSelectionNanos += time.Since(selectionStart).Nanoseconds()
	}

	selected := stacks[best]
	if len(selected.entries) > 0 {
		materializeStart := time.Time{}
		if materializationTiming != nil {
			materializeStart = time.Now()
		}
		materializeTransientParentEntries(selected.entries, arena, transientParents, transientChildren)
		if materializationTiming != nil {
			materializationTiming.transientParentMaterializeNanos += time.Since(materializeStart).Nanoseconds()
		}
		buildStart := time.Time{}
		if materializationTiming != nil {
			buildStart = time.Now()
		}
		tree := p.buildResult(selected.entries, source, arena, oldTree, reuseState, linkScratch)
		if materializationTiming != nil {
			materializationTiming.resultTreeBuildNanos += time.Since(buildStart).Nanoseconds()
		}
		return tree
	}
	if selected.gss.head == nil {
		buildStart := time.Time{}
		if materializationTiming != nil {
			buildStart = time.Now()
		}
		tree := p.buildResult(nil, source, arena, oldTree, reuseState, linkScratch)
		if materializationTiming != nil {
			materializationTiming.resultTreeBuildNanos += time.Since(buildStart).Nanoseconds()
		}
		return tree
	}
	nodes := nodesFromGSSMaterializingCompactFullLeaves(p, selected.gss, arena)
	materializeStart := time.Time{}
	if materializationTiming != nil {
		materializeStart = time.Now()
	}
	materializeTransientParentNodes(nodes, arena, transientParents, transientChildren)
	if materializationTiming != nil {
		materializationTiming.transientParentMaterializeNanos += time.Since(materializeStart).Nanoseconds()
	}
	buildStart := time.Time{}
	if materializationTiming != nil {
		buildStart = time.Now()
	}
	tree := p.buildResultFromNodes(nodes, source, arena, oldTree, reuseState, linkScratch)
	if materializationTiming != nil {
		materializationTiming.resultTreeBuildNanos += time.Since(buildStart).Nanoseconds()
	}
	return tree
}

func materializeTransientParentEntries(entries []stackEntry, arena *nodeArena, transientParents *transientParentScratch, transientChildren *transientChildScratch) {
	if transientParents == nil {
		return
	}
	transientParents.materializeEntries(entries, arena, transientChildren)
}

func materializeTransientParentNodes(nodes []*Node, arena *nodeArena, transientParents *transientParentScratch, transientChildren *transientChildScratch) {
	if transientParents == nil {
		return
	}
	transientParents.materializeNodeSlice(nodes, arena, transientChildren)
}

func (p *Parser) buildNoTreeBenchmarkResult(source []byte, arena *nodeArena, rootEndByte uint32) *Tree {
	if arena == nil {
		return NewTree(nil, source, p.language)
	}
	sym := Symbol(0)
	if p != nil && p.hasRootSymbol {
		sym = p.rootSymbol
	}
	named := true
	if p != nil && p.language != nil {
		if idx := int(sym); idx >= 0 && idx < len(p.language.SymbolMetadata) {
			named = p.language.SymbolMetadata[sym].Named
		}
	}
	root := arena.allocNodeFast()
	root.ownerArena = arena
	arena.noTreePlaceholderNodesConstructed++
	root.symbol = sym
	root.setNamed(named)
	root.startByte = 0
	root.endByte = rootEndByte
	root.childIndex = -1
	nodeInitEquivVersion(root)
	return newTreeWithArenas(root, source, p.language, arena, nil)
}

func stackCompareForResultSelection(p *Parser, arena *nodeArena, a, b *glrStack, skipErrorRank bool) int {
	if a.dead != b.dead {
		if a.dead {
			return -1
		}
		return 1
	}
	if a.accepted != b.accepted {
		if a.accepted {
			return 1
		}
		return -1
	}
	if !skipErrorRank {
		if aErr, bErr := stackResultErrorRank(a, arena), stackResultErrorRank(b, arena); aErr != bErr {
			if aErr < bErr {
				return 1
			}
			return -1
		}
	}
	if cmp := compareAcceptedStackAliasPreference(p, arena, *a, *b); cmp != 0 {
		return cmp
	}
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	if a.shifted != b.shifted {
		if !a.shifted {
			return 1
		}
		return -1
	}
	aDepth := a.depth()
	bDepth := b.depth()
	if aDepth != bDepth {
		if aDepth > bDepth {
			return 1
		}
		return -1
	}
	if a.byteOffset != b.byteOffset {
		if a.byteOffset > b.byteOffset {
			return 1
		}
		return -1
	}
	if a.branchOrder != b.branchOrder {
		if a.branchOrder < b.branchOrder {
			return 1
		}
		return -1
	}
	return 0
}

func stackResultErrorRank(s *glrStack, arena *nodeArena) int {
	if s == nil {
		return 2
	}
	rank := 0
	if len(s.entries) > 0 {
		for i := range s.entries {
			stackEntryResultErrorRank(s.entries[i], arena, &rank)
			if rank == 2 {
				break
			}
		}
		return rank
	}
	for n := s.gss.head; n != nil; n = n.prev {
		stackEntryResultErrorRank(n.entry, arena, &rank)
		if rank == 2 {
			break
		}
	}
	return rank
}

func stackEntryResultErrorRank(entry stackEntry, arena *nodeArena, rank *int) {
	if rank == nil || *rank == 2 || !stackEntryMaterializesForResult(entry) {
		return
	}
	if stackEntryNodeSymbol(entry) == errorSymbol {
		*rank = 2
		return
	}
	if stackEntryNodeHasError(entry) && *rank == 0 {
		*rank = 1
	}
	if n := stackEntryNode(entry); n != nil {
		for _, child := range n.children {
			stackEntryResultErrorRank(newStackEntryNode(child.parseState, child), arena, rank)
			if *rank == 2 {
				return
			}
		}
		return
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		for i := 0; i < parent.childEntryCount(); i++ {
			child := parent.childEntry(arena, i)
			stackEntryResultErrorRank(child, arena, rank)
			if *rank == 2 {
				return
			}
		}
	}
}

func compareAcceptedStackAliasPreference(p *Parser, arena *nodeArena, a, b glrStack) int {
	if p == nil || p.language == nil {
		return 0
	}
	if len(p.aliasTargetSymbol) == 0 {
		return 0
	}
	if len(a.entries) > 0 && len(b.entries) > 0 {
		return compareStackEntryAliasPreferenceSlices(p, arena, a.entries, b.entries)
	}
	aCount := stackMaterializingResultEntryCount(a)
	if aCount == 0 || aCount != stackMaterializingResultEntryCount(b) {
		return 0
	}
	const maxBufferedAliasPreferenceEntries = 8
	if aCount > maxBufferedAliasPreferenceEntries {
		if !stackHasCompactResultPayload(a) && !stackHasCompactResultPayload(b) {
			return compareAcceptedStackNodeAliasPreference(p, a, b)
		}
		return 0
	}
	var aBuf, bBuf [maxBufferedAliasPreferenceEntries]stackEntry
	aEntries, aOK := stackMaterializingResultEntries(a, aBuf[:0], aCount)
	bEntries, bOK := stackMaterializingResultEntries(b, bBuf[:0], aCount)
	if !aOK || !bOK {
		return 0
	}
	for i := 0; i < aCount; i++ {
		if cmp := compareStackEntryAliasPreference(p, arena, aEntries[i], bEntries[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareAcceptedStackNodeAliasPreference(p *Parser, a, b glrStack) int {
	aNodes := resultNodesFromStack(a)
	bNodes := resultNodesFromStack(b)
	if len(aNodes) != len(bNodes) {
		return 0
	}
	for i := range aNodes {
		if cmp := compareNodeAliasPreference(p, aNodes[i], bNodes[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareStackEntryAliasPreferenceSlices(p *Parser, arena *nodeArena, a, b []stackEntry) int {
	aCount := countMaterializingResultEntries(a)
	if aCount == 0 || aCount != countMaterializingResultEntries(b) {
		return 0
	}
	ai, bi := 0, 0
	for compared := 0; compared < aCount; compared++ {
		var aEntry, bEntry stackEntry
		var ok bool
		aEntry, ai, ok = nextMaterializingResultEntry(a, ai)
		if !ok {
			return 0
		}
		bEntry, bi, ok = nextMaterializingResultEntry(b, bi)
		if !ok {
			return 0
		}
		if cmp := compareStackEntryAliasPreference(p, arena, aEntry, bEntry); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func countMaterializingResultEntries(entries []stackEntry) int {
	count := 0
	for i := range entries {
		if stackEntryMaterializesForResult(entries[i]) {
			count++
		}
	}
	return count
}

func nextMaterializingResultEntry(entries []stackEntry, start int) (stackEntry, int, bool) {
	for i := start; i < len(entries); i++ {
		if stackEntryMaterializesForResult(entries[i]) {
			return entries[i], i + 1, true
		}
	}
	return stackEntry{}, len(entries), false
}

func stackEntryMaterializesForResult(entry stackEntry) bool {
	return stackEntryNode(entry) != nil || stackEntryCompactFullLeaf(entry) != nil || stackEntryPendingParent(entry) != nil
}

func stackEntryHasCompactResultPayload(entry stackEntry) bool {
	return stackEntryCompactFullLeaf(entry) != nil || stackEntryPendingParent(entry) != nil
}

func stackHasCompactResultPayload(s glrStack) bool {
	if len(s.entries) > 0 {
		for i := range s.entries {
			if stackEntryHasCompactResultPayload(s.entries[i]) {
				return true
			}
		}
		return false
	}
	for n := s.gss.head; n != nil; n = n.prev {
		if stackEntryHasCompactResultPayload(n.entry) {
			return true
		}
	}
	return false
}

func stackMaterializingResultEntryCount(s glrStack) int {
	if len(s.entries) > 0 {
		return countMaterializingResultEntries(s.entries)
	}
	if s.gss.head == nil {
		return 0
	}
	count := 0
	for n := s.gss.head; n != nil; n = n.prev {
		if stackEntryMaterializesForResult(n.entry) {
			count++
		}
	}
	return count
}

func stackMaterializingResultEntries(s glrStack, dst []stackEntry, materializingCount int) ([]stackEntry, bool) {
	if materializingCount == 0 || cap(dst) < materializingCount {
		return nil, false
	}
	dst = dst[:materializingCount]
	if len(s.entries) > 0 {
		index := 0
		for i := range s.entries {
			if !stackEntryMaterializesForResult(s.entries[i]) {
				continue
			}
			if index >= materializingCount {
				return nil, false
			}
			dst[index] = s.entries[i]
			index++
		}
		return dst, index == materializingCount
	}
	index := materializingCount - 1
	for n := s.gss.head; n != nil; n = n.prev {
		if !stackEntryMaterializesForResult(n.entry) {
			continue
		}
		if index < 0 {
			return nil, false
		}
		dst[index] = n.entry
		index--
	}
	return dst, index == -1
}

func resultNodesFromStack(s glrStack) []*Node {
	if len(s.entries) > 0 {
		count := 0
		for i := range s.entries {
			if stackEntryNode(s.entries[i]) != nil {
				count++
			}
		}
		if count == 0 {
			return nil
		}
		nodes := make([]*Node, 0, count)
		for i := range s.entries {
			if node := stackEntryNode(s.entries[i]); node != nil {
				nodes = append(nodes, node)
			}
		}
		return nodes
	}
	if s.gss.head == nil {
		return nil
	}
	return nodesFromGSS(s.gss)
}

func compareNodeAliasPreference(p *Parser, a, b *Node) int {
	if a == b || a == nil || b == nil {
		return 0
	}
	if a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		a.isExtra() != b.isExtra() ||
		a.isMissing() != b.isMissing() ||
		len(a.children) != len(b.children) {
		return 0
	}
	if a.symbol != b.symbol {
		aType := a.Type(p.language)
		bType := b.Type(p.language)
		if aType == bType {
			for i := range a.children {
				if cmp := compareNodeAliasPreference(p, a.children[i], b.children[i]); cmp != 0 {
					return cmp
				}
			}
			return 0
		}
		aAlias := p.isAliasTargetSymbol(a.symbol)
		bAlias := p.isAliasTargetSymbol(b.symbol)
		if aAlias != bAlias {
			if aAlias {
				return 1
			}
			return -1
		}
		return 0
	}
	for i := range a.children {
		if cmp := compareNodeAliasPreference(p, a.children[i], b.children[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareStackEntryAliasPreference(p *Parser, arena *nodeArena, a, b stackEntry) int {
	if a.node == b.node && a.kind == b.kind {
		return 0
	}
	if !stackEntryMaterializesForResult(a) || !stackEntryMaterializesForResult(b) {
		return 0
	}
	if stackEntryNode(a) != nil && stackEntryNode(b) != nil {
		return compareNodeAliasPreference(p, stackEntryNode(a), stackEntryNode(b))
	}
	if stackEntryNodeStartByte(a) != stackEntryNodeStartByte(b) ||
		stackEntryNodeEndByte(a) != stackEntryNodeEndByte(b) ||
		stackEntryNodeIsExtra(a) != stackEntryNodeIsExtra(b) ||
		stackEntryNodeIsMissing(a) != stackEntryNodeIsMissing(b) ||
		stackEntryNodeChildCount(a) != stackEntryNodeChildCount(b) {
		return 0
	}
	if stackEntryNodeSymbol(a) != stackEntryNodeSymbol(b) {
		aType := stackEntryTypeName(p, a)
		bType := stackEntryTypeName(p, b)
		if aType == bType {
			for i := 0; i < stackEntryNodeChildCount(a); i++ {
				aChild, aOK := stackEntryAliasChild(a, arena, i)
				bChild, bOK := stackEntryAliasChild(b, arena, i)
				if !aOK || !bOK {
					return 0
				}
				if cmp := compareStackEntryAliasPreference(p, arena, aChild, bChild); cmp != 0 {
					return cmp
				}
			}
			return 0
		}
		aAlias := p.isAliasTargetSymbol(stackEntryNodeSymbol(a))
		bAlias := p.isAliasTargetSymbol(stackEntryNodeSymbol(b))
		if aAlias != bAlias {
			if aAlias {
				return 1
			}
			return -1
		}
		return 0
	}
	for i := 0; i < stackEntryNodeChildCount(a); i++ {
		aChild, aOK := stackEntryAliasChild(a, arena, i)
		bChild, bOK := stackEntryAliasChild(b, arena, i)
		if !aOK || !bOK {
			return 0
		}
		if cmp := compareStackEntryAliasPreference(p, arena, aChild, bChild); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func stackEntryAliasChild(entry stackEntry, arena *nodeArena, i int) (stackEntry, bool) {
	if n := stackEntryNode(entry); n != nil {
		if i < 0 || i >= len(n.children) {
			return stackEntry{}, false
		}
		child := n.children[i]
		return newStackEntryNode(child.parseState, child), true
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		if i < 0 || i >= parent.childEntryCount() {
			return stackEntry{}, false
		}
		return parent.childEntry(arena, i), true
	}
	return stackEntry{}, false
}

func stackEntryTypeName(p *Parser, entry stackEntry) string {
	if stackEntryNodeSymbol(entry) == errorSymbol {
		return "ERROR"
	}
	if p == nil || p.language == nil {
		return ""
	}
	sym := stackEntryNodeSymbol(entry)
	if int(sym) >= len(p.language.SymbolNames) {
		return ""
	}
	return unescapePunctuationSymbolName(p.language.SymbolNames[sym])
}

func (p *Parser) isAliasTargetSymbol(sym Symbol) bool {
	if p == nil || int(sym) >= len(p.aliasTargetSymbol) {
		return false
	}
	return p.aliasTargetSymbol[sym]
}

// isNamedSymbol checks whether a symbol is a named symbol.
func (p *Parser) isNamedSymbol(sym Symbol) bool {
	if int(sym) < len(p.language.SymbolMetadata) {
		return p.language.SymbolMetadata[sym].Named
	}
	return false
}

func nodesFromGSS(stack gssStack) []*Node {
	if stack.head == nil {
		return nil
	}
	count := 0
	for n := stack.head; n != nil; n = n.prev {
		if stackEntryNode(n.entry) != nil {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	nodes := make([]*Node, count)
	i := count - 1
	for n := stack.head; n != nil; n = n.prev {
		if node := stackEntryNode(n.entry); node != nil {
			nodes[i] = node
			i--
		}
	}
	return nodes
}

func nodesFromGSSMaterializingCompactFullLeaves(p *Parser, stack gssStack, arena *nodeArena) []*Node {
	if stack.head == nil {
		return nil
	}
	count := 0
	for n := stack.head; n != nil; n = n.prev {
		if stackEntryNode(n.entry) != nil || stackEntryCompactFullLeaf(n.entry) != nil || stackEntryPendingParent(n.entry) != nil {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	nodes := make([]*Node, count)
	i := count - 1
	for n := stack.head; n != nil; n = n.prev {
		if node := materializeStackEntryPayloadWithParser(p, arena, &n.entry, compactFullLeafMaterializeForFinalTree, pendingParentMaterializeForFinalTree); node != nil {
			nodes[i] = node
			i--
		}
	}
	return nodes
}

func filterZeroWidthExtras(nodes []*Node, arena *nodeArena) []*Node {
	if len(nodes) == 0 {
		return nodes
	}
	keep := 0
	for _, n := range nodes {
		if n == nil || !n.isExtra() || n.endByte > n.startByte {
			keep++
		}
	}
	if keep == len(nodes) || keep == 0 {
		return nodes
	}
	filtered := make([]*Node, 0, keep)
	for _, n := range nodes {
		if n != nil && n.isExtra() && n.endByte == n.startByte {
			continue
		}
		filtered = append(filtered, n)
	}
	if arena != nil {
		out := arena.allocNodeSlice(len(filtered))
		copy(out, filtered)
		return out
	}
	return filtered
}

// buildResult constructs the final Tree from a stack of entries.
func (p *Parser) buildResult(stack []stackEntry, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	var nodes []*Node
	for i := range stack {
		if node := materializeStackEntryPayloadWithParser(p, arena, &stack[i], compactFullLeafMaterializeForFinalTree, pendingParentMaterializeForFinalTree); node != nil {
			nodes = append(nodes, node)
		}
	}
	return p.buildResultFromNodes(nodes, source, arena, oldTree, reuseState, linkScratch)
}

func (p *Parser) buildResultFromNodes(nodes []*Node, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	if len(nodes) == 0 {
		arena.Release()
		if isWhitespaceOnlySource(source) {
			return NewTree(nil, source, p.language)
		}
		return parseErrorTree(source, p.language)
	}

	if arena != nil && arena.used == 0 {
		arena.Release()
		arena = nil
	}

	expectedRootSymbol := Symbol(0)
	hasExpectedRoot := false
	shouldWireParentLinks := oldTree == nil
	if p != nil && p.hasRootSymbol {
		expectedRootSymbol = p.rootSymbol
		hasExpectedRoot = true
	}
	if oldTree != nil && oldTree.RootNode() != nil {
		expectedRootSymbol = oldTree.RootNode().symbol
		hasExpectedRoot = true
	}
	if p != nil && p.language != nil && p.language.Name == "python" {
		timing := p.currentMaterializationTiming()
		repairStart := materializationTimingStart(timing)
		nodes, _ = repairPythonKeywordErrorNodes(nodes, source, arena, p.language)
		timing.addPythonKeywordRepair(repairStart)
		nodes = collapsePythonRootFragments(nodes, arena, p.language)
	}
	if hasExpectedRoot && len(nodes) > 1 {
		nodes = flattenRootSelfFragments(nodes, arena, expectedRootSymbol)
	}
	borrowedResolved := false
	var borrowed []*nodeArena
	getBorrowed := func() []*nodeArena {
		if borrowedResolved {
			return borrowed
		}
		borrowed = reuseState.retainBorrowed(arena)
		borrowedResolved = true
		return borrowed
	}

	if len(nodes) == 1 {
		candidate := nodes[0]
		candidate = flattenInvisibleRootChildren(candidate, arena, p.language)
		timing := p.currentMaterializationTiming()
		keywordRepairStart := materializationTimingStart(timing)
		candidate = repairPythonKeywordErrorNode(candidate, source, arena, p.language)
		timing.addPythonKeywordRepair(keywordRepairStart)
		rootRepairStart := materializationTimingStart(timing)
		candidate = repairPythonRootNode(candidate, arena, p.language)
		timing.addPythonRootRepair(rootRepairStart)
		if !hasExpectedRoot || candidate.symbol == expectedRootSymbol {
			p.finalizeResultRoot(candidate, source, linkScratch, shouldWireParentLinks, true)
			return newTreeWithArenas(candidate, source, p.language, arena, getBorrowed())
		}

		// Incremental reuse guard: if the only stacked node doesn't match the
		// previous root symbol, synthesize an expected root wrapper instead of
		// returning a reused child as the new tree root.
		rootChildren := make([]*Node, 1)
		rootChildren[0] = candidate
		if arena != nil {
			rootChildren = arena.allocNodeSlice(1)
			rootChildren[0] = candidate
		}
		root := newParentNodeInArena(arena, expectedRootSymbol, true, rootChildren, nil, 0)
		p.finalizeResultRoot(root, source, linkScratch, shouldWireParentLinks, true)
		return newTreeWithArenas(root, source, p.language, arena, getBorrowed())
	}

	// When multiple nodes remain on the stack, check whether all but one
	// are extras (e.g. leading whitespace/comments). If so, fold the extras
	// into the real root rather than wrapping everything in an error node.
	var realRoot *Node
	var allExtras []*Node
	var extras []*Node
	for _, n := range nodes {
		if n.isExtra() {
			allExtras = append(allExtras, n)
			// Ignore invisible extras and zero-width extras in final-root
			// recovery; they should not force an error wrapper or inflate root
			// child counts.
			if p != nil && p.language != nil &&
				int(n.symbol) < len(p.language.SymbolMetadata) &&
				p.language.SymbolMetadata[n.symbol].Visible &&
				n.endByte > n.startByte {
				extras = append(extras, n)
			}
		} else {
			if realRoot != nil {
				realRoot = nil // more than one non-extra -> genuine error
				break
			}
			realRoot = n
		}
	}
	if realRoot != nil {
		returnRealRoot := !hasExpectedRoot || realRoot.symbol == expectedRootSymbol
		if reuseState != nil && reuseState.reusedAny {
			realRoot = cloneNodeInArena(arena, realRoot)
			realRoot.parent = nil
			realRoot.childIndex = -1
		}
		foldExtras := returnRealRoot && len(extras) > 0
		if foldExtras {
			for _, e := range allExtras {
				if e != nil && (e.IsError() || e.HasError()) {
					foldExtras = false
					break
				}
			}
		}
		if foldExtras {
			// Fold visible extras into the real root as leading/trailing children.
			merged := make([]*Node, 0, len(extras)+len(realRoot.children))
			leadingCount := 0
			for _, e := range extras {
				if e.startByte <= realRoot.startByte {
					merged = append(merged, e)
					leadingCount++
				}
			}
			merged = append(merged, realRoot.children...)
			for _, e := range extras {
				if e.startByte > realRoot.startByte {
					merged = append(merged, e)
				}
			}
			if arena != nil {
				out := arena.allocNodeSlice(len(merged))
				copy(out, merged)
				merged = out
			}
			realRoot.children = merged
			// Keep fieldIDs aligned with children: extras have no field (0).
			if len(realRoot.fieldIDs) > 0 {
				trailingCount := len(extras) - leadingCount
				padded := make([]FieldID, leadingCount+len(realRoot.fieldIDs)+trailingCount)
				copy(padded[leadingCount:], realRoot.fieldIDs)
				realRoot.fieldIDs = padded
				if len(realRoot.fieldSources) > 0 {
					paddedSources := make([]uint8, len(padded))
					copy(paddedSources[leadingCount:], realRoot.fieldSources)
					realRoot.fieldSources = paddedSources
				}
			}
			// Extend root range to cover the extras.
			for _, e := range extras {
				if e.startByte < realRoot.startByte {
					realRoot.startByte = e.startByte
					realRoot.startPoint = e.startPoint
				}
				if e.endByte > realRoot.endByte {
					realRoot.endByte = e.endByte
					realRoot.endPoint = e.endPoint
				}
			}
		}
		// Invisible extras should still contribute to the final root byte/point range.
		if returnRealRoot {
			for _, e := range allExtras {
				if e.startByte < realRoot.startByte {
					realRoot.startByte = e.startByte
					realRoot.startPoint = e.startPoint
				}
				if e.endByte > realRoot.endByte {
					realRoot.endByte = e.endByte
					realRoot.endPoint = e.endPoint
				}
			}
		}
		timing := p.currentMaterializationTiming()
		rootRepairStart := materializationTimingStart(timing)
		realRoot = repairPythonRootNode(realRoot, arena, p.language)
		timing.addPythonRootRepair(rootRepairStart)
		extendTrailing := returnRealRoot || !realRoot.hasError()
		p.finalizeResultRoot(realRoot, source, linkScratch, shouldWireParentLinks && returnRealRoot, extendTrailing)
		if returnRealRoot {
			return newTreeWithArenas(realRoot, source, p.language, arena, getBorrowed())
		}
	}

	rootChildren := filterZeroWidthExtras(nodes, arena)
	timing := p.currentMaterializationTiming()
	keywordRepairStart := materializationTimingStart(timing)
	rootChildren, _ = repairPythonKeywordErrorNodes(rootChildren, source, arena, p.language)
	timing.addPythonKeywordRepair(keywordRepairStart)
	rootSymbol := rootChildren[len(rootChildren)-1].symbol
	rootHasError := false
	for _, n := range rootChildren {
		if n != nil && (n.IsError() || n.HasError()) {
			rootHasError = true
			break
		}
	}
	if hasExpectedRoot {
		if rootHasError {
			if p != nil && p.language != nil && p.language.Name == "dart" && dartProgramChildrenLookComplete(nodes, p.language) {
				rootSymbol = expectedRootSymbol
			} else {
				rootSymbol = errorSymbol
			}
		} else {
			rootSymbol = expectedRootSymbol
		}
	}
	root := newParentNodeInArena(arena, rootSymbol, true, rootChildren, nil, 0)
	if rootHasError && !(p != nil && p.language != nil && p.language.Name == "python" && hasExpectedRoot && pythonModuleChildrenLookComplete(rootChildren, p.language)) {
		root.setHasError(true)
	}
	rootRepairStart := materializationTimingStart(timing)
	root = repairPythonRootNode(root, arena, p.language)
	timing.addPythonRootRepair(rootRepairStart)
	p.finalizeResultRoot(root, source, linkScratch, shouldWireParentLinks, true)
	return newTreeWithArenas(root, source, p.language, arena, getBorrowed())
}

func (p *Parser) finalizeResultRoot(root *Node, source []byte, linkScratch *[]*Node, wireParentLinks, extendTrailing bool) {
	if root == nil {
		return
	}
	timing := p.currentMaterializationTiming()
	finalizeStart := materializationTimingStart(timing)
	defer timing.addResultFinalizeRoot(finalizeStart)
	if extendTrailing {
		start := materializationTimingStart(timing)
		extendNodeToTrailingWhitespace(root, source)
		timing.addResultExtendTrailing(start)
	}
	start := materializationTimingStart(timing)
	p.normalizeRootSourceStart(root, source)
	timing.addResultNormalizeRootStart(start)
	if p == nil || !p.noResultCompatibilityBenchmarkOnly {
		start = materializationTimingStart(timing)
		normalizeResultCompatibility(root, source, p)
		timing.addResultCompatibility(start)
	}
	if wireParentLinks {
		start = materializationTimingStart(timing)
		if p != nil && p.shouldDeferResultParentLinks(root) {
			root.ownerArena.deferParentLinks(root)
		} else {
			wireParentLinksWithScratch(root, linkScratch)
		}
		timing.addResultParentLink(start)
	}
}

func (p *Parser) shouldDeferResultParentLinks(root *Node) bool {
	if p == nil || p.language == nil || root == nil || root.ownerArena == nil {
		return false
	}
	return (p.language.Name == "java" || p.language.Name == "python") && !p.noTreeBenchmarkOnly
}

func (p *Parser) normalizeRootSourceStart(root *Node, source []byte) {
	if root == nil || root.startByte == 0 || len(source) == 0 {
		return
	}
	// Included-range parses intentionally preserve range-local root spans.
	if p != nil && len(p.included) > 0 {
		return
	}
	root.startByte = 0
	root.startPoint = Point{}
}

// maxTreeWalkDepth prevents stack overflow in recursive tree walkers when
// parsing with grammargen-produced grammars that can create pathologically deep
// hidden-node chains (e.g. Scala with >1M levels).
const maxTreeWalkDepth = 5000
