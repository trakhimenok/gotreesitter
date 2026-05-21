package gotreesitter

import "bytes"

type reuseFrame struct {
	node       *Node
	underDirty bool
}

// reuseCursor incrementally walks reusable nodes from an old tree in
// pre-order, caching candidates for the current token start byte.
type reuseCursor struct {
	sourceLen uint32
	oldSource []byte
	newSource []byte
	minEditAt uint32
	hasEdits  bool

	stack []reuseFrame
	next  *Node

	topLevelParent *Node
	topLevelIndex  int
	topLevelEnd    int

	cachedStart      uint32
	cachedStartValid bool
	cached           []*Node

	rejectDirty                   uint64
	rejectAncestorDirtyBeforeEdit uint64
	rejectHasError                uint64
	rejectInvalidSpan             uint64
	rejectOutOfBounds             uint64
	rejectRootNonLeafChanged      uint64
	rejectLargeNonLeaf            uint64
}

// reuseScratch holds reusable buffers for incremental reuse traversal.
type reuseScratch struct {
	stack []reuseFrame
	cache []*Node
}

func (c *reuseCursor) reset(oldTree *Tree, source []byte, scratch *reuseScratch) *reuseCursor {
	if oldTree == nil || oldTree.RootNode() == nil {
		return nil
	}
	if scratch == nil {
		scratch = &reuseScratch{}
	}

	c.sourceLen = uint32(len(source))
	c.oldSource = oldTree.source
	c.newSource = source
	c.minEditAt = 0
	c.hasEdits = len(oldTree.edits) > 0
	if c.hasEdits {
		c.minEditAt = oldTree.edits[0].StartByte
		for i := 1; i < len(oldTree.edits); i++ {
			if oldTree.edits[i].StartByte < c.minEditAt {
				c.minEditAt = oldTree.edits[i].StartByte
			}
		}
	}
	c.stack = scratch.stack[:0]
	c.next = nil
	c.cachedStart = 0
	c.cachedStartValid = false
	c.cached = scratch.cache[:0]
	c.rejectDirty = 0
	c.rejectAncestorDirtyBeforeEdit = 0
	c.rejectHasError = 0
	c.rejectInvalidSpan = 0
	c.rejectOutOfBounds = 0
	c.rejectRootNonLeafChanged = 0
	c.rejectLargeNonLeaf = 0

	root := oldTree.RootNode()
	c.stack = append(c.stack, reuseFrame{node: root})
	c.topLevelParent = nil
	c.topLevelIndex = 0
	c.topLevelEnd = 0
	childCount := nodeChildCountNoMaterialize(root)
	if c.hasEdits && root != nil && childCount > 0 {
		firstAffected := -1
		for i := 0; i < childCount; i++ {
			entry, ok := nodeChildEntryAtNoMaterialize(root, i)
			if !ok {
				continue
			}
			if stackEntryNodeEndByte(entry) > c.minEditAt {
				firstAffected = i
				break
			}
		}
		if firstAffected >= 0 && firstAffected+1 < childCount {
			c.topLevelParent = root
			c.topLevelIndex = firstAffected + 1
			c.topLevelEnd = childCount
		}
	}
	return c
}

func (c *reuseCursor) commitScratch(scratch *reuseScratch) {
	if scratch == nil {
		return
	}
	scratch.stack = c.stack[:0]
	scratch.cache = c.cached[:0]
}

// releaseNodeRefs nils all *Node pointers so the GC can collect the arenas
// they reference. Call before returning a Parser to a pool to prevent arena
// retention via reuseCursor holding nodes from the last incremental parse.
// Backing arrays (stack, cached) are kept to avoid re-allocation next parse.
func (c *reuseCursor) releaseNodeRefs() {
	c.next = nil
	c.topLevelParent = nil
	c.oldSource = nil
	c.newSource = nil
	if cap(c.stack) > 0 {
		clear(c.stack[:cap(c.stack)])
		c.stack = c.stack[:0]
	}
	if cap(c.cached) > 0 {
		clear(c.cached[:cap(c.cached)])
		c.cached = c.cached[:0]
	}
}

// releaseNodeRefs nils *Node pointers in the scratch buffers.
func (s *reuseScratch) releaseNodeRefs() {
	if cap(s.stack) > 0 {
		clear(s.stack[:cap(s.stack)])
		s.stack = s.stack[:0]
	}
	if cap(s.cache) > 0 {
		clear(s.cache[:cap(s.cache)])
		s.cache = s.cache[:0]
	}
}

func (c *reuseCursor) candidates(start uint32) []*Node {
	if c == nil {
		return nil
	}
	if c.cachedStartValid {
		if start == c.cachedStart {
			return c.cached
		}
		if start < c.cachedStart {
			return nil
		}
	}

	c.cached = c.cached[:0]
	c.cachedStart = start
	c.cachedStartValid = true
	if c.collectTopLevelCandidates(start) {
		return c.cached
	}

	for {
		n := c.peek()
		if n == nil {
			return c.cached
		}

		if n.startByte < start {
			c.pop()
			continue
		}
		if n.startByte > start {
			return c.cached
		}

		for {
			n = c.peek()
			if n == nil || n.startByte != start {
				return c.cached
			}
			c.cached = append(c.cached, c.pop())
		}
	}
}

func (c *reuseCursor) collectTopLevelCandidates(start uint32) bool {
	if c == nil || c.topLevelParent == nil || c.topLevelIndex >= c.topLevelEnd {
		return false
	}
	for c.topLevelIndex < c.topLevelEnd {
		entry, ok := nodeChildEntryAtNoMaterialize(c.topLevelParent, c.topLevelIndex)
		if !ok || !stackEntryHasNode(entry) {
			c.topLevelIndex++
			continue
		}
		childStart := stackEntryNodeStartByte(entry)
		if childStart < start {
			c.topLevelIndex++
			continue
		}
		if childStart > start {
			return false
		}
		for c.topLevelIndex < c.topLevelEnd {
			entry, ok = nodeChildEntryAtNoMaterialize(c.topLevelParent, c.topLevelIndex)
			if !ok || !stackEntryHasNode(entry) {
				c.topLevelIndex++
				continue
			}
			if stackEntryNodeStartByte(entry) != start {
				return true
			}
			c.topLevelIndex++
			if !c.reusableIndexedEntry(entry) {
				continue
			}
			n := nodeChildAtForReason(c.topLevelParent, c.topLevelIndex-1, materializeForEdit)
			if n != nil {
				c.cached = append(c.cached, n)
			}
		}
		return true
	}
	return false
}

func (c *reuseCursor) reusableIndexedEntry(entry stackEntry) bool {
	if !stackEntryHasNode(entry) {
		return false
	}
	start := stackEntryNodeStartByte(entry)
	end := stackEntryNodeEndByte(entry)
	if c.hasEdits && !nodeBytesEqual(start, end, c.oldSource, c.newSource) {
		c.rejectDirty++
		return false
	}
	dirtyHere := stackEntryNodeDirty(entry)
	if dirtyHere && nodeBytesEqual(start, end, c.oldSource, c.newSource) {
		setStackEntryDirty(entry, false)
		dirtyHere = false
	}
	if stackEntryNodeHasError(entry) {
		c.rejectHasError++
		return false
	}
	if end <= start {
		c.rejectInvalidSpan++
		return false
	}
	if end > c.sourceLen {
		c.rejectOutOfBounds++
		return false
	}
	if dirtyHere {
		c.rejectDirty++
		return false
	}
	return true
}

func (c *reuseCursor) peek() *Node {
	if c.next != nil {
		return c.next
	}
	c.next = c.advance()
	return c.next
}

func (c *reuseCursor) pop() *Node {
	n := c.peek()
	if n != nil && perfCountersEnabled {
		perfRecordReusePopped()
	}
	c.next = nil
	return n
}

func (c *reuseCursor) advance() *Node {
	for len(c.stack) > 0 {
		last := len(c.stack) - 1
		frame := c.stack[last]
		c.stack = c.stack[:last]
		cur := frame.node
		if cur == nil {
			continue
		}
		if perfCountersEnabled {
			perfRecordReuseVisited()
		}

		dirtyHere := cur.dirty()
		if dirtyHere {
			if nodeBytesEqual(cur.startByte, cur.endByte, c.oldSource, c.newSource) {
				// Undo edit path: unchanged bytes can be reused safely.
				cur.setDirty(false)
				dirtyHere = false
			}
		}

		childUnderDirty := frame.underDirty || dirtyHere

		childCount := nodeChildCountNoMaterialize(cur)
		if perfCountersEnabled {
			perfRecordReusePushed(childCount)
		}
		for i := childCount - 1; i >= 0; i-- {
			if entry, ok := nodeChildEntryAtNoMaterialize(cur, i); ok &&
				c.cachedStartValid &&
				stackEntryNodeEndByte(entry) <= c.cachedStart {
				continue
			}
			child := nodeChildAtForReason(cur, i, materializeForEdit)
			if child == nil {
				continue
			}
			c.stack = append(c.stack, reuseFrame{
				node:       child,
				underDirty: childUnderDirty,
			})
		}

		if frame.underDirty && c.hasEdits &&
			!nodeBytesEqual(cur.startByte, cur.endByte, c.oldSource, c.newSource) {
			c.rejectAncestorDirtyBeforeEdit++
			continue
		}
		if cur.hasError() {
			c.rejectHasError++
			continue
		}
		if cur.endByte <= cur.startByte {
			c.rejectInvalidSpan++
			continue
		}
		if cur.endByte > c.sourceLen {
			c.rejectOutOfBounds++
			continue
		}
		if dirtyHere {
			c.rejectDirty++
			continue
		}
		return cur
	}
	return nil
}

func nodeBytesEqual(start, end uint32, oldSource, newSource []byte) bool {
	if end < start {
		return false
	}
	if end > uint32(len(oldSource)) || end > uint32(len(newSource)) {
		return false
	}
	return bytes.Equal(oldSource[start:end], newSource[start:end])
}

// tryReuseSubtree attempts to reuse an old subtree at the current lookahead.
// On success it appends the reused node to the stack and returns the first
// lookahead token that begins at or after the node's end byte.
func (p *Parser) tryReuseSubtree(s *glrStack, lookahead Token, ts TokenSource, idx *reuseCursor, entryScratch *glrEntryScratch, gssScratch *gssScratch) (Token, uint32, bool) {
	candidates := idx.candidates(lookahead.StartByte)
	if perfCountersEnabled {
		perfRecordReuseCandidates(len(candidates))
	}
	if len(candidates) == 0 {
		return lookahead, 0, false
	}

	state := s.top().state
	for _, n := range candidates {
		if n.ChildCount() > 0 {
			// Preserve full-root reuse on undo when bytes are identical.
			if !(n.startByte == 0 &&
				n.endByte == idx.sourceLen &&
				nodeBytesEqual(n.startByte, n.endByte, idx.oldSource, idx.newSource)) {
				idx.rejectRootNonLeafChanged++
				continue
			}
		}
		nextState, ok := p.reuseTargetState(state, n, lookahead)
		if !ok {
			continue
		}
		cp, ok := canReuseNodeWithExternalScannerCheckpoint(ts, state, n)
		if !ok {
			continue
		}
		return reuseNode(p, s, n, nextState, state, lookahead, ts, idx, entryScratch, gssScratch, cp)
	}

	// Conservative fallback: try small non-root non-leaf nodes. This increases
	// reuse surface without jumping to large ancestor nodes that can trigger
	// expensive recovery behavior.
	const maxNonLeafReuseSpan = 2048
	for _, n := range candidates {
		if n == nil || n.ChildCount() == 0 || n.parent == nil {
			continue
		}
		span := n.EndByte() - n.StartByte()
		if span == 0 || span > maxNonLeafReuseSpan {
			if span > maxNonLeafReuseSpan {
				idx.rejectLargeNonLeaf++
			}
			continue
		}
		nextState, truncateDepth, ok := p.reuseNonLeafTargetStateOnStack(s, n, lookahead.StartByte, entryScratch)
		if !ok {
			continue
		}
		if truncateDepth > 0 && truncateDepth < s.depth() {
			if !s.truncate(truncateDepth) {
				continue
			}
		}
		startState := s.top().state
		cp, ok := canReuseNodeWithExternalScannerCheckpoint(ts, startState, n)
		if !ok {
			continue
		}
		return reuseNode(p, s, n, nextState, startState, lookahead, ts, idx, entryScratch, gssScratch, cp)
	}

	return lookahead, 0, false
}

func reuseNode(p *Parser, s *glrStack, n *Node, nextState StateID, startState StateID, lookahead Token, ts TokenSource, idx *reuseCursor, entryScratch *glrEntryScratch, gssScratch *gssScratch, checkpoint externalScannerCheckpointRef) (Token, uint32, bool) {
	if perfCountersEnabled {
		perfRecordReuseSuccess()
		if n.ChildCount() == 0 {
			perfRecordReuseLeafSuccess()
		} else {
			perfRecordReuseNonLeafSuccess(n.EndByte() - n.StartByte())
		}
	}
	p.pushStackNode(s, nextState, n, entryScratch, gssScratch)
	reusedBytes := n.EndByte() - n.StartByte()

	// If the reused node reaches EOF, we can synthesize EOF directly
	// instead of consuming every trailing token.
	if n.EndByte() == idx.sourceLen {
		pt := n.EndPoint()
		return Token{
			Symbol:     0,
			StartByte:  idx.sourceLen,
			EndByte:    idx.sourceLen,
			StartPoint: pt,
			EndPoint:   pt,
		}, reusedBytes, true
	}

	// dfaTokenSource fast skip does not preserve external-scanner state.
	// For checkpointed scanner languages, only reuse nodes when the start
	// parser/scanner state matches exactly, then restore the recorded end
	// snapshot before skipping to the node end.
	if dts := underlyingDFATokenSource(ts); dts != nil && dts.language != nil && dts.language.ExternalScanner != nil {
		if languageUsesExternalScannerCheckpoints(dts.language) {
			if stateful, ok := ts.(parserStateTokenSource); ok {
				stateful.SetParserState(nextState)
				stateful.SetGLRStates(nil)
			}
			if startState != n.PreGotoState() {
				return lookahead, 0, false
			}
			if tok, ok := fastForwardWithExternalScannerCheckpoint(ts, n, checkpoint); ok {
				return tok, reusedBytes, true
			}
		}
		return advanceTokenSourceTo(ts, lookahead, n.EndByte()), reusedBytes, true
	}

	if skipper, ok := ts.(PointSkippableTokenSource); ok {
		if stateful, ok := ts.(parserStateTokenSource); ok {
			stateful.SetParserState(nextState)
			stateful.SetGLRStates(nil)
		}
		return skipper.SkipToByteWithPoint(n.EndByte(), n.EndPoint()), reusedBytes, true
	}
	if skipper, ok := ts.(ByteSkippableTokenSource); ok {
		if stateful, ok := ts.(parserStateTokenSource); ok {
			stateful.SetParserState(nextState)
			stateful.SetGLRStates(nil)
		}
		return skipper.SkipToByte(n.EndByte()), reusedBytes, true
	}

	return advanceTokenSourceTo(ts, lookahead, n.EndByte()), reusedBytes, true
}

func advanceTokenSourceTo(ts TokenSource, lookahead Token, endByte uint32) Token {
	tok := lookahead
	for tok.Symbol != 0 && tok.EndByte <= endByte {
		next := ts.Next()
		// Defensive break for non-advancing token sources.
		if next.StartByte == tok.StartByte && next.EndByte == tok.EndByte {
			return next
		}
		tok = next
	}
	return tok
}

func (p *Parser) reuseTargetState(state StateID, n *Node, lookahead Token) (StateID, bool) {
	// Leaf reuse must match the current lookahead token symbol.
	if n.ChildCount() == 0 {
		if n.Symbol() != lookahead.Symbol {
			return 0, false
		}

		action := p.lookupAction(state, n.Symbol())
		if action == nil || len(action.Actions) == 0 {
			return 0, false
		}
		var uniqueShiftState StateID
		shiftCount := 0
		for _, act := range action.Actions {
			if act.Type != ParseActionShift {
				continue
			}
			targetState := act.State
			// Extra-token shifts keep the parser state unchanged.
			if act.Extra {
				targetState = state
			}
			if targetState == n.parseState {
				return targetState, true
			}
			if shiftCount == 0 {
				uniqueShiftState = targetState
			}
			shiftCount++
		}
		if shiftCount == 1 {
			return uniqueShiftState, true
		}
		return 0, false
	}

	if perfCountersEnabled {
		perfRecordReuseNonLeafCheck()
	}
	gotoState := p.lookupGoto(state, n.Symbol())
	if gotoState == 0 {
		if perfCountersEnabled {
			perfRecordReuseNonLeafNoGoto()
			if p.language != nil && int(n.Symbol()) < int(p.language.TokenCount) {
				perfRecordReuseNonLeafNoGotoTerminal()
			} else {
				perfRecordReuseNonLeafNoGotoNonTerminal()
			}
		}
		return 0, false
	}
	if n.parseState == 0 {
		if perfCountersEnabled {
			perfRecordReuseNonLeafStateZero()
		}
	}
	return gotoState, true
}

func reuseStackDepthForPreGoto(entries []stackEntry, startByte uint32, preGoto StateID) int {
	if len(entries) == 0 {
		return 0
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].state != preGoto {
			continue
		}
		frontier := uint32(0)
		if n := stackEntryNode(entries[i]); n != nil {
			frontier = n.endByte
		}
		if frontier <= startByte {
			return i + 1
		}
	}
	return 0
}

func (p *Parser) reuseNonLeafTargetStateOnStack(s *glrStack, n *Node, startByte uint32, entryScratch *glrEntryScratch) (StateID, int, bool) {
	if s == nil || n == nil || n.ChildCount() == 0 {
		return 0, 0, false
	}
	if perfCountersEnabled {
		perfRecordReuseNonLeafCheck()
	}

	preGoto := n.PreGotoState()

	gotoState := p.lookupGoto(preGoto, n.Symbol())
	if gotoState == 0 {
		if perfCountersEnabled {
			perfRecordReuseNonLeafNoGoto()
			if p.language != nil && int(n.Symbol()) < int(p.language.TokenCount) {
				perfRecordReuseNonLeafNoGotoTerminal()
			} else {
				perfRecordReuseNonLeafNoGotoNonTerminal()
			}
		}
		return 0, 0, false
	}
	if n.parseState != 0 && gotoState != n.parseState {
		if perfCountersEnabled {
			perfRecordReuseNonLeafStateMiss()
		}
		return 0, 0, false
	}

	entries := s.ensureEntries(entryScratch)
	depth := reuseStackDepthForPreGoto(entries, startByte, preGoto)
	if depth == 0 {
		if perfCountersEnabled {
			perfRecordReuseNonLeafStateMiss()
		}
		return 0, 0, false
	}

	return gotoState, depth, true
}
