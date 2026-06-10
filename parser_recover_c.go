package gotreesitter

import "fmt"

// parser_recover_c.go is the stage-1 faithful port of tree-sitter C's error
// recovery loop into the pure-Go GLR engine, gated per grammar via
// errorCostCompetitionLanguage (currently: requirements only).
//
// THE C CODE IS THE SPEC (tree-sitter v0.25 lib/src):
//   - parser.c  ts_parser__handle_error / ts_parser__recover /
//     ts_parser__do_all_potential_reductions / ts_parser__recover_to_state /
//     ts_parser__compare_versions / ts_parser__version_status /
//     ts_parser__better_version_exists / ts_parser__condense_stack
//   - stack.c   pause/resume/summary/error-cost bookkeeping
//   - subtree.c ts_subtree_error_cost / summarize_children
//   - error_costs.h cost constants
//
// Mapping notes (Go GLR engine vs C stack versions):
//   - C keeps one Stack with multi-link versions; this engine keeps separate
//     glrStack copies. C's handle_error merges all do_all_potential_reductions
//     versions into ONE multi-path version before recording the stack summary;
//     here each forked version becomes its own absorbing stack with its own
//     summary. The union of per-stack summaries covers the same recovery
//     candidates; the C cost competition (ported below) prunes the set.
//   - C marks an erroring version "paused", lets other versions advance, and
//     resumes the best paused version in condense_stack. This engine processes
//     stacks in lockstep per token, so handle_error runs immediately at the
//     no-action point; cCondenseStacks applies the same cost competition after
//     each dispatch pass.
//   - C's ERROR_STATE (state 0) is real in the generated Go tables too (the
//     recover row). An absorbing stack pushes a node-less stackEntry{state: 0}
//     as the C "NULL subtree discontinuity"; with state 0 on top, the DFA token
//     source lexes with LexModes[0] — exactly C's error-mode lexing.
//   - C's error_repeat chain is flattened: the open error region is a single
//     ERROR node whose children are the absorbed tokens.
//   - C memoizes error cost per subtree; stage 1 recomputes it by walking the
//     stack (gated grammars parse small files). Stage 2 should make it
//     incremental if wider grammars need it.

// C error_costs.h. NOTE: ERROR_COST_PER_SKIPPED_LINE is 30 in the C header
// (the recovery-cost-competition.md table said 2; the header wins).
const (
	cErrCostPerRecovery    = 500
	cErrCostPerMissingTree = 110
	cErrCostPerSkippedTree = 100
	cErrCostPerSkippedLine = 30
	cErrCostPerSkippedChar = 1
)

const (
	// C parser.c MAX_VERSION_COUNT / MAX_SUMMARY_DEPTH / MAX_COST_DIFFERENCE.
	cRecoverMaxVersionCount   = 6
	cRecoverMaxSummaryDepth   = 16
	cRecoverMaxCostDifference = 18 * cErrCostPerSkippedTree
	// cErrorState is the C ERROR_STATE: the generated tables' recover row.
	cErrorState = StateID(0)
)

// errorCostCompetitionLanguage reports whether the faithful C error-recovery
// port is enabled for the active grammar. Enabled one grammar at a time, each
// verified to net-improve its full corpus against the C oracle with zero
// clean-grammar regressions (recovery-cost-competition.md requirement 4).
func errorCostCompetitionLanguage(lang *Language) bool {
	if lang == nil {
		return false
	}
	switch lang.Name {
	case "requirements":
		return true
	}
	return false
}

func (p *Parser) errorCostCompetitionEnabled() bool {
	return p != nil && p.errorCostCompetition && !p.noTreeBenchmarkOnly && !p.noTreeCheckpointBenchmarkOnly
}

// cStackSummaryEntry mirrors C StackSummaryEntry (stack.h): a (depth, state)
// pair with the stack position at that depth, recorded when entering the
// error state and consulted by ts_parser__recover strategy 1.
type cStackSummaryEntry struct {
	depth    int
	state    StateID
	posBytes uint32
	posRow   uint32
}

// cRecoverState marks a glrStack as being in the C error state (head at
// ERROR_STATE absorbing skipped tokens). nil == not in error.
type cRecoverState struct {
	summary []cStackSummaryEntry
	// openErr is the open error region node on the stack top (the C
	// error_repeat being accumulated). nil right after entering the error
	// state — the C "ERROR_STATE head with NULL subtree" shape, which costs an
	// extra ERROR_COST_PER_RECOVERY in ts_stack_error_cost.
	openErr *Node
}

func (r *cRecoverState) clone() *cRecoverState {
	if r == nil {
		return nil
	}
	cp := &cRecoverState{openErr: r.openErr}
	if len(r.summary) > 0 {
		cp.summary = append([]cStackSummaryEntry(nil), r.summary...)
	}
	return cp
}

// cRecoverOutcome describes what the gated recovery did with the current
// stack for the current token.
type cRecoverOutcome int

const (
	// cRecFallthrough: not handled — caller continues normal dispatch.
	cRecFallthrough cRecoverOutcome = iota
	// cRecConsumed: token absorbed into the error region (or recover_eof
	// accepted); the stack is done with this token.
	cRecConsumed
	// cRecHalted: the stack was halted (clearly worse than another version).
	cRecHalted
)

// ---------------------------------------------------------------------------
// Error cost (subtree.c ts_subtree_error_cost / summarize_children port)
// ---------------------------------------------------------------------------

func cSymbolVisibleLang(lang *Language, sym Symbol) bool {
	if sym == errorSymbol {
		return true
	}
	if lang == nil || int(sym) >= len(lang.SymbolMetadata) {
		return false
	}
	return lang.SymbolMetadata[sym].Visible
}

func (p *Parser) cSymbolVisible(sym Symbol) bool {
	if p == nil {
		return false
	}
	return cSymbolVisibleLang(p.language, sym)
}

// cNodeVisibleChildCount mirrors SubtreeHeapData.visible_child_count:
// direct children that are visible, plus the visible children of invisible
// internal children.
func cNodeVisibleChildCountLang(lang *Language, n *Node) int {
	if n == nil {
		return 0
	}
	count := 0
	for _, c := range n.children {
		if c == nil {
			continue
		}
		if cSymbolVisibleLang(lang, c.symbol) {
			count++
		} else if len(c.children) > 0 {
			count += cNodeVisibleChildCountLang(lang, c)
		}
	}
	return count
}

// cNodeVisibleSubtreeCount mirrors stack.c stack__subtree_node_count: the
// visible descendant count (plus the node itself when visible), used for
// node-count-since-error bookkeeping.
func (p *Parser) cNodeVisibleSubtreeCount(n *Node) int {
	if n == nil {
		return 0
	}
	count := 0
	if p.cSymbolVisible(n.symbol) {
		count++
	}
	for _, c := range n.children {
		count += p.cNodeVisibleSubtreeCount(c)
	}
	return count
}

// cNodeErrorCost ports ts_subtree_error_cost + the error-cost part of
// ts_subtree_summarize_children. Go has no error_repeat chain: ERROR nodes
// hold absorbed children directly, so the per-ERROR recovery cost is charged
// once per ERROR node. Go nodes carry no padding either; an ERROR node's span
// already starts at its first real token, matching the C "size excludes
// padding" rule for the common case.
func cNodeErrorCostLang(lang *Language, n *Node) uint32 {
	if n == nil {
		return 0
	}
	if n.isMissing() && len(n.children) == 0 {
		return cErrCostPerMissingTree + cErrCostPerRecovery
	}
	var cost uint32
	for _, c := range n.children {
		cost += cNodeErrorCostLang(lang, c)
	}
	if n.symbol == errorSymbol {
		for _, c := range n.children {
			if c == nil || c.isExtra() {
				continue
			}
			if c.symbol == errorSymbol && len(c.children) == 0 {
				continue
			}
			if cSymbolVisibleLang(lang, c.symbol) {
				cost += cErrCostPerSkippedTree
			} else if len(c.children) > 0 {
				cost += cErrCostPerSkippedTree * uint32(cNodeVisibleChildCountLang(lang, c))
			}
		}
		bytes := uint32(0)
		rows := uint32(0)
		if n.endByte > n.startByte {
			bytes = n.endByte - n.startByte
		}
		if n.endPoint.Row > n.startPoint.Row {
			rows = n.endPoint.Row - n.startPoint.Row
		}
		cost += cErrCostPerRecovery + cErrCostPerSkippedChar*bytes + cErrCostPerSkippedLine*rows
	}
	return cost
}

func (p *Parser) cNodeErrorCost(n *Node) uint32 {
	if p == nil {
		return 0
	}
	return cNodeErrorCostLang(p.language, n)
}

// cStackErrorCost ports ts_stack_error_cost: the accumulated error cost of
// every subtree on the stack, plus one open recovery when the version just
// entered the error state and has not absorbed anything yet (the C
// "ERROR_STATE head with NULL subtree" case).
func (p *Parser) cStackErrorCost(s *glrStack) uint32 {
	if s == nil {
		return 0
	}
	var cost uint32
	walk := func(n *Node) {
		if n != nil {
			cost += p.cNodeErrorCost(n)
		}
	}
	if len(s.entries) > 0 {
		for i := range s.entries {
			walk(stackEntryNode(s.entries[i]))
		}
	} else {
		for gn := s.gss.head; gn != nil; gn = gn.prev {
			walk(stackEntryNode(gn.entry))
		}
	}
	if s.cRec != nil && s.cRec.openErr == nil {
		cost += cErrCostPerRecovery
	}
	return cost
}

// cNodeCountSinceError approximates ts_stack_node_count_since_error: for
// absorbing stacks, the visible-subtree count pushed above the error
// discontinuity; for clean stacks, the whole stack's count (C resets
// node_count_at_last_error only when an error begins).
func (p *Parser) cNodeCountSinceError(s *glrStack) int {
	if s == nil {
		return 0
	}
	count := 0
	stopAtDiscontinuity := s.cRec != nil
	counted := func(e stackEntry) bool {
		if stopAtDiscontinuity && !stackEntryHasNode(e) && e.state == cErrorState {
			return false
		}
		if n := stackEntryNode(e); n != nil {
			count += p.cNodeVisibleSubtreeCount(n)
		}
		return true
	}
	if len(s.entries) > 0 {
		for i := len(s.entries) - 1; i >= 0; i-- {
			if !counted(s.entries[i]) {
				return count
			}
		}
		return count
	}
	for gn := s.gss.head; gn != nil; gn = gn.prev {
		if !counted(gn.entry) {
			return count
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Version status + comparison (parser.c ErrorStatus / ErrorComparison port)
// ---------------------------------------------------------------------------

type cErrorStatus struct {
	cost      uint32
	nodeCount int
	dynPrec   int
	isInError bool
}

type cErrorComparison int

const (
	cErrorComparisonTakeLeft cErrorComparison = iota
	cErrorComparisonPreferLeft
	cErrorComparisonNone
	cErrorComparisonPreferRight
	cErrorComparisonTakeRight
)

func (p *Parser) cVersionStatus(s *glrStack) cErrorStatus {
	return cErrorStatus{
		cost:      p.cStackErrorCost(s),
		nodeCount: p.cNodeCountSinceError(s),
		dynPrec:   s.score,
		isInError: s.cRec != nil,
	}
}

// cCompareVersions is a literal port of ts_parser__compare_versions.
func cCompareVersions(a, b cErrorStatus) cErrorComparison {
	if !a.isInError && b.isInError {
		if a.cost < b.cost {
			return cErrorComparisonTakeLeft
		}
		return cErrorComparisonPreferLeft
	}
	if a.isInError && !b.isInError {
		if b.cost < a.cost {
			return cErrorComparisonTakeRight
		}
		return cErrorComparisonPreferRight
	}
	if a.cost < b.cost {
		if (b.cost-a.cost)*uint32(1+a.nodeCount) > cRecoverMaxCostDifference {
			return cErrorComparisonTakeLeft
		}
		return cErrorComparisonPreferLeft
	}
	if b.cost < a.cost {
		if (a.cost-b.cost)*uint32(1+b.nodeCount) > cRecoverMaxCostDifference {
			return cErrorComparisonTakeRight
		}
		return cErrorComparisonPreferRight
	}
	if a.dynPrec > b.dynPrec {
		return cErrorComparisonPreferLeft
	}
	if b.dynPrec > a.dynPrec {
		return cErrorComparisonPreferRight
	}
	return cErrorComparisonNone
}

// cBetterVersionExists ports ts_parser__better_version_exists: would the
// candidate (self with hypothetical cost) clearly lose to an existing live
// stack at the same or later position?
func (p *Parser) cBetterVersionExists(stacks []glrStack, self int, isInError bool, cost uint32) bool {
	pos := stacks[self].byteOffset
	status := cErrorStatus{
		cost:      cost,
		isInError: isInError,
		dynPrec:   stacks[self].score,
		nodeCount: p.cNodeCountSinceError(&stacks[self]),
	}
	for i := range stacks {
		if i == self || stacks[i].dead || stacks[i].byteOffset < pos {
			continue
		}
		st := p.cVersionStatus(&stacks[i])
		switch cCompareVersions(status, st) {
		case cErrorComparisonTakeRight:
			return true
		case cErrorComparisonPreferRight:
			// C: only when the two versions could merge. Header equivalence
			// (top state + position) is the ts_stack_can_merge precondition.
			if stacksHeaderEquivalent(stacks[self], stacks[i]) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Stack walking helpers
// ---------------------------------------------------------------------------

// cStackEntriesTopFirst materializes the stack spine top-first.
func cStackEntriesTopFirst(s *glrStack, gssScratch *gssScratch) []stackEntry {
	s.ensureGSS(gssScratch)
	depth := s.depth()
	if depth == 0 {
		return nil
	}
	entries := make([]stackEntry, 0, depth)
	for n := s.gss.head; n != nil; n = n.prev {
		entries = append(entries, n.entry)
	}
	return entries
}

// cEntryCountsTowardDepth mirrors stack__iter subtree counting: non-extra
// subtrees count, NULL discontinuities count, extras do not.
func cEntryCountsTowardDepth(e stackEntry) bool {
	if !stackEntryHasNode(e) {
		// Node-less entries: the stack base does not count (C's base node has
		// no link); the error discontinuity (state 0) counts like C's NULL
		// subtree link. Both are node-less; distinguish by state — only the
		// discontinuity carries cErrorState above a non-empty prefix, and the
		// base is never crossed by bounded pops anyway.
		return e.state == cErrorState
	}
	return !stackEntryNodeIsExtra(e)
}

// cRecordSummary ports ts_stack_record_summary over this engine's linear
// spine: entries top-first, depth = crossings of depth-counting links,
// deduped on (depth, state), bounded by MAX_SUMMARY_DEPTH.
func (p *Parser) cRecordSummary(entries []stackEntry) []cStackSummaryEntry {
	summary := make([]cStackSummaryEntry, 0, 8)
	depth := 0
	// Position of the node at-or-below each entry: C node positions are the
	// cumulative input position at that stack node.
	posBytesAt := make([]uint32, len(entries))
	posRowAt := make([]uint32, len(entries))
	var pb uint32
	var pr uint32
	for i := len(entries) - 1; i >= 0; i-- {
		if n := stackEntryNode(entries[i]); n != nil {
			pb = n.endByte
			pr = n.endPoint.Row
		}
		posBytesAt[i] = pb
		posRowAt[i] = pr
	}
	record := func(d int, st StateID, posBytes, posRow uint32) {
		for j := len(summary) - 1; j >= 0; j-- {
			if summary[j].depth < d {
				break
			}
			if summary[j].depth == d && summary[j].state == st {
				return
			}
		}
		summary = append(summary, cStackSummaryEntry{depth: d, state: st, posBytes: posBytes, posRow: posRow})
	}
	for i := 0; i < len(entries); i++ {
		record(depth, entries[i].state, posBytesAt[i], posRowAt[i])
		if cEntryCountsTowardDepth(entries[i]) {
			depth++
			if depth > cRecoverMaxSummaryDepth {
				break
			}
		}
	}
	return summary
}

// ---------------------------------------------------------------------------
// do_all_potential_reductions port
// ---------------------------------------------------------------------------

type cReduceActionKey struct {
	symbol Symbol
	count  uint8
}

// cCollectPotentialReductions gathers the deduped reduce-action set for the
// state over the symbol range, and whether any non-extra shift exists
// (parser.c lines 1121-1157).
func (p *Parser) cCollectPotentialReductions(state StateID, lookaheadSym Symbol, reduces *[]ParseAction) bool {
	*reduces = (*reduces)[:0]
	hasShift := false
	seen := make(map[cReduceActionKey]bool, 4)
	scan := func(sym Symbol) {
		idx := p.lookupActionIndex(state, sym)
		if idx == 0 || int(idx) >= len(p.language.ParseActions) {
			return
		}
		for _, act := range p.language.ParseActions[idx].Actions {
			switch act.Type {
			case ParseActionShift, ParseActionRecover:
				if !act.Extra && !act.Repetition {
					hasShift = true
				}
			case ParseActionReduce:
				if act.ChildCount > 0 {
					key := cReduceActionKey{symbol: act.Symbol, count: act.ChildCount}
					if !seen[key] {
						seen[key] = true
						*reduces = append(*reduces, act)
					}
				}
			}
		}
	}
	if lookaheadSym != 0 {
		scan(lookaheadSym)
	} else {
		tokenCount := Symbol(p.language.TokenCount)
		for sym := Symbol(1); sym < tokenCount; sym++ {
			scan(sym)
		}
	}
	return hasShift
}

// cDoAllPotentialReductions ports ts_parser__do_all_potential_reductions for
// one starting stack. It returns the resulting version set (what the starting
// version became, plus surviving forks) and whether some version can shift
// the lookahead. With lookaheadSym == 0 the reductions reachable on ANY
// symbol are applied (the "close in-progress productions" step); versions
// that dead-end keep their pre-reduction shape (C leaves them in place).
// With lookaheadSym != 0 dead-end versions are dropped (C removes them).
func (p *Parser) cDoAllPotentialReductions(start glrStack, lookaheadSym Symbol, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) ([]glrStack, bool) {
	versions := []glrStack{start}
	canShift := false
	var reduces []ParseAction
	v := 0
	for iter := 0; ; iter++ {
		if v >= len(versions) {
			break
		}
		// Merge check against earlier versions created in this call.
		merged := false
		for j := 1; j < v; j++ {
			if stacksHeaderEquivalent(versions[j], versions[v]) {
				versions = append(versions[:v], versions[v+1:]...)
				merged = true
				break
			}
		}
		if merged {
			continue
		}
		state := versions[v].top().state
		hasShift := p.cCollectPotentialReductions(state, lookaheadSym, &reduces)
		firstReduction := -1
		for _, act := range reduces {
			fork := versions[v].cloneWithScratch(gssScratch)
			fork.cRec = versions[v].cRec.clone()
			var dummy bool
			p.applyAction(&fork, act, tok, &dummy, nodeCount, arena, entryScratch, gssScratch, nil, false, trackChildErrors)
			if fork.dead {
				continue
			}
			versions = append(versions, fork)
			if firstReduction < 0 {
				firstReduction = len(versions) - 1
			}
		}
		if hasShift {
			canShift = true
		} else if firstReduction >= 0 && iter < cRecoverMaxVersionCount {
			// C renumbers the reduction version onto the current version and
			// reprocesses it in place.
			versions[v] = versions[firstReduction]
			versions = append(versions[:firstReduction], versions[firstReduction+1:]...)
			continue
		} else if lookaheadSym != 0 {
			versions = append(versions[:v], versions[v+1:]...)
			continue
		}
		if v == 0 {
			v = 1
		} else {
			v++
		}
		if len(versions) > cRecoverMaxVersionCount+1 {
			break
		}
	}
	return versions, canShift
}

// ---------------------------------------------------------------------------
// handle_error port
// ---------------------------------------------------------------------------

// cTerminalNextState ports ts_language_next_state for terminals: the shift
// target of the last action (extra shifts keep the state).
func (p *Parser) cTerminalNextState(state StateID, sym Symbol) (StateID, ParseAction, bool) {
	idx := p.lookupActionIndex(state, sym)
	if idx == 0 || int(idx) >= len(p.language.ParseActions) {
		return 0, ParseAction{}, false
	}
	actions := p.language.ParseActions[idx].Actions
	if len(actions) == 0 {
		return 0, ParseAction{}, false
	}
	act := actions[len(actions)-1]
	if act.Type != ParseActionShift {
		return 0, ParseAction{}, false
	}
	if act.Extra {
		return state, act, true
	}
	return act.State, act, true
}

// cHandleError ports ts_parser__handle_error for the stack at index si: run
// do_all_potential_reductions on ANY symbol, attempt one missing-token
// insertion across the version set, push the error discontinuity on every
// version, record summaries, then run ts_parser__recover for the current
// lookahead on each absorbing version. New versions are appended to *stacks.
//
// Returns the outcome for the original stack (which stacks[si] now reflects)
// and whether any new version still needs to act on the current token (the
// missing-token version and strategy-1 recoveries) — the caller must force a
// re-dispatch pass for the same token.
func (p *Parser) cHandleError(stacks *[]glrStack, si int, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (cRecoverOutcome, bool) {
	s := &(*stacks)[si]

	// 1. Close in-progress productions: reductions reachable on any symbol.
	versions, _ := p.cDoAllPotentialReductions(s.clone(), 0, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)

	// 2. Missing-token insertion (once across the version set, in order).
	var missingVersion *glrStack
	for vi := range versions {
		state := versions[vi].top().state
		tokenCount := Symbol(p.language.TokenCount)
		for ms := Symbol(1); ms < tokenCount; ms++ {
			nextState, shiftAct, ok := p.cTerminalNextState(state, ms)
			if !ok || nextState == 0 || nextState == state {
				continue
			}
			if !p.stateHasLeadingReduceAction(nextState, tok.Symbol) {
				continue
			}
			cand := versions[vi].cloneWithScratch(gssScratch)
			cand.cRec = nil
			missingTok := Token{
				Symbol:     ms,
				StartByte:  tok.StartByte,
				EndByte:    tok.StartByte,
				StartPoint: tok.StartPoint,
				EndPoint:   tok.StartPoint,
				Missing:    true,
			}
			if top := cand.top(); stackEntryHasNode(top) && stackEntryNodeEndByte(top) <= tok.StartByte {
				missingTok.StartByte = stackEntryNodeEndByte(top)
				missingTok.EndByte = stackEntryNodeEndByte(top)
				missingTok.StartPoint = stackEntryNodeEndPoint(top)
				missingTok.EndPoint = stackEntryNodeEndPoint(top)
			}
			var dummy bool
			p.applyAction(&cand, shiftAct, missingTok, &dummy, nodeCount, arena, entryScratch, gssScratch, nil, false, trackChildErrors)
			cand.shifted = false
			if cand.dead {
				continue
			}
			reduced, canShift := p.cDoAllPotentialReductions(cand, tok.Symbol, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			if !canShift || len(reduced) == 0 {
				continue
			}
			missingVersion = &reduced[0]
			break
		}
		if missingVersion != nil {
			break
		}
	}

	// 3. Enter the error state on every version: push the discontinuity
	// (C NULL subtree at ERROR_STATE) and record the stack summary.
	for vi := range versions {
		v := &versions[vi]
		v.pushEntry(stackEntry{state: cErrorState}, entryScratch, gssScratch)
		entries := cStackEntriesTopFirst(v, gssScratch)
		v.cRec = &cRecoverState{summary: p.cRecordSummary(entries)}
		v.shifted = false
	}

	// The original stack becomes the first absorbing version.
	*s = versions[0]

	// 4. Run recover for the current lookahead on each absorbing version.
	// Recover may fork strategy-1 candidates (which must act on this token),
	// absorb the token, or halt the version.
	needsRedispatch := false
	outcome := cRecoverOutcome(cRecConsumed)
	for vi := range versions {
		var v *glrStack
		if vi == 0 {
			v = &(*stacks)[si]
		} else {
			*stacks = append(*stacks, versions[vi])
			v = &(*stacks)[len(*stacks)-1]
		}
		res, forked := p.cRecover(stacks, v, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
		if forked {
			needsRedispatch = true
		}
		if vi == 0 {
			outcome = res
		} else if res == cRecHalted {
			v.dead = true
		}
	}

	if missingVersion != nil {
		missingVersion.branchOrder = (*stacks)[si].branchOrder
		*stacks = append(*stacks, *missingVersion)
		needsRedispatch = true
	}
	return outcome, needsRedispatch
}

// ---------------------------------------------------------------------------
// recover port
// ---------------------------------------------------------------------------

// cRecover ports ts_parser__recover for one absorbing version. It may append
// a strategy-1 recovered fork to *stacks (returned forked=true: the fork must
// act on the current token), absorb the token into the open error region
// (cRecConsumed), accept at EOF (cRecConsumed), or halt the version
// (cRecHalted).
func (p *Parser) cRecover(stacks *[]glrStack, v *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (cRecoverOutcome, bool) {
	didRecover := false
	forked := false
	rec := v.cRec
	if rec == nil {
		return cRecFallthrough, false
	}
	vIndex := -1
	for i := range *stacks {
		if &(*stacks)[i] == v {
			vIndex = i
			break
		}
	}

	// Strategy 1: recover to a previous state from the summary in which the
	// lookahead is valid.
	if len(rec.summary) > 0 && tok.Symbol != errorSymbol && tok.Symbol != 0 {
		curCost := p.cStackErrorCost(v)
		for _, entry := range rec.summary {
			if entry.state == cErrorState {
				continue
			}
			if entry.posBytes == v.byteOffset {
				continue
			}
			depth := entry.depth
			if rec.openErr != nil {
				// C: node_count_since_error > 0 — the open error region
				// occupies one extra (non-extra) slot above the summary.
				depth++
			}
			// Do not recover in ways that create redundant stack versions.
			wouldMerge := false
			for i := range *stacks {
				if i == vIndex || (*stacks)[i].dead {
					continue
				}
				if (*stacks)[i].top().state == entry.state && (*stacks)[i].byteOffset == v.byteOffset {
					wouldMerge = true
					break
				}
			}
			if wouldMerge {
				continue
			}
			curRow := cStackPosRow(v)
			newCost := curCost +
				uint32(entry.depth)*cErrCostPerSkippedTree +
				(v.byteOffset-entry.posBytes)*cErrCostPerSkippedChar +
				(curRow-entry.posRow)*cErrCostPerSkippedLine
			if vIndex >= 0 && p.cBetterVersionExists(*stacks, vIndex, false, newCost) {
				break
			}
			if p.lookupActionIndex(entry.state, tok.Symbol) == 0 {
				continue
			}
			if fork, ok := p.cRecoverToState(v, depth, entry.state, arena, entryScratch, gssScratch, trackChildErrors); ok {
				fork.branchOrder = v.branchOrder
				*stacks = append(*stacks, fork)
				if nodeCount != nil {
					*nodeCount = *nodeCount + 1
				}
				didRecover = true
				forked = true
				if p.glrTrace {
					traceCRecoverToState(entry.state, depth)
				}
				break
			}
		}
	}

	// Re-resolve v: the append above may have reallocated the stacks slice.
	if vIndex >= 0 {
		v = &(*stacks)[vIndex]
		rec = v.cRec
	}

	// C: if strategy 1 succeeded and there are already too many versions,
	// drop the absorbing version.
	if didRecover && len(*stacks) > cRecoverMaxVersionCount {
		v.dead = true
		return cRecHalted, forked
	}

	// EOF: wrap everything and accept (ts_parser__recover recover_eof).
	if tok.Symbol == 0 && tok.StartByte == tok.EndByte {
		v.accepted = true
		v.shifted = true
		return cRecConsumed, forked
	}

	// Do not skip the token if doing so would clearly be worse than some
	// existing version.
	tokBytes := uint32(0)
	if tok.EndByte > tok.StartByte {
		tokBytes = tok.EndByte - tok.StartByte
	}
	tokRows := uint32(0)
	if tok.EndPoint.Row > tok.StartPoint.Row {
		tokRows = tok.EndPoint.Row - tok.StartPoint.Row
	}
	newCost := p.cStackErrorCost(v) + cErrCostPerSkippedTree +
		tokBytes*cErrCostPerSkippedChar + tokRows*cErrCostPerSkippedLine
	if vIndex >= 0 && p.cBetterVersionExists(*stacks, vIndex, false, newCost) {
		v.dead = true
		return cRecHalted, forked
	}

	// Wrap the lookahead into the open error region (strategy 2).
	p.cAbsorbTokenIntoError(v, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
	v.shifted = true
	return cRecConsumed, forked
}

func cStackPosRow(s *glrStack) uint32 {
	if s == nil {
		return 0
	}
	if len(s.entries) > 0 {
		for i := len(s.entries) - 1; i >= 0; i-- {
			if n := stackEntryNode(s.entries[i]); n != nil {
				return n.endPoint.Row
			}
		}
		return 0
	}
	for gn := s.gss.head; gn != nil; gn = gn.prev {
		if n := stackEntryNode(gn.entry); n != nil {
			return n.endPoint.Row
		}
	}
	return 0
}

// cRecoverToState ports ts_parser__recover_to_state: pop `depth`
// depth-counting links off a copy of v, splice in any open error region
// children, wrap the popped subtrees (minus trailing extras) into an extra
// ERROR node pushed at the goal state, and re-push the trailing extras.
func (p *Parser) cRecoverToState(v *glrStack, depth int, goal StateID, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (glrStack, bool) {
	entries := cStackEntriesTopFirst(v, gssScratch)
	if len(entries) == 0 {
		return glrStack{}, false
	}
	// Find the cut index: cross `depth` depth-counting links from the top.
	crossed := 0
	cut := -1
	for i := 0; i < len(entries); i++ {
		if crossed == depth {
			cut = i
			break
		}
		if cEntryCountsTowardDepth(entries[i]) {
			crossed++
		}
	}
	if cut < 0 {
		if crossed == depth {
			cut = len(entries)
		} else {
			return glrStack{}, false
		}
	}
	if cut >= len(entries) || entries[cut].state != goal {
		return glrStack{}, false
	}

	// Materialize popped payloads in stack order (base-most first).
	popped := entries[:cut]
	nodes := make([]*Node, 0, len(popped))
	for i := len(popped) - 1; i >= 0; i-- {
		if !stackEntryHasNode(popped[i]) {
			continue // the error discontinuity
		}
		n, _ := materializeStackEntryPayloadEntryWithParser(p, arena, popped[i], materializeForRecovery, materializeForRecovery)
		if n == nil {
			return glrStack{}, false
		}
		nodes = append(nodes, n)
	}

	// Split trailing extras (re-pushed after the ERROR per C).
	end := len(nodes)
	for end > 0 && nodes[end-1].isExtra() {
		end--
	}
	wrapped := nodes[:end]
	trailing := nodes[end:]

	// Flatten the open error region (C splices popped error subtrees /
	// keeps error_repeat chains invisible; the Go equivalent is splicing the
	// open ERROR node's children).
	children := make([]*Node, 0, len(wrapped)+2)
	openErr := (*cRecoverState)(nil)
	if v.cRec != nil {
		openErr = v.cRec
	}
	for _, n := range wrapped {
		if openErr != nil && n == openErr.openErr {
			children = append(children, n.children...)
			continue
		}
		children = append(children, n)
	}

	fork := v.cloneWithScratch(gssScratch)
	fork.cRec = nil
	fork.dead = false
	fork.shifted = false
	keepDepth := len(entries) - cut
	if !fork.truncate(keepDepth) {
		return glrStack{}, false
	}
	// C also pops a directly-preceding closed ERROR subtree and splices its
	// children in front (ts_stack_pop_error).
	if top := stackEntryNode(fork.top()); top != nil && top.symbol == errorSymbol && !top.isMissing() && fork.depth() > 1 {
		prev := top
		if fork.truncate(fork.depth() - 1) {
			children = append(append(make([]*Node, 0, len(prev.children)+len(children)), prev.children...), children...)
		}
	}

	if len(children) > 0 {
		errNode := newParentNodeInArena(arena, errorSymbol, true, children, nil, 0)
		errNode.setHasError(true)
		errNode.setExtra(true)
		errNode.preGotoState = goal
		errNode.parseState = goal
		nodeBumpEquivVersion(errNode)
		if perfCountersEnabled {
			perfRecordErrorNode()
		}
		if trackChildErrors != nil {
			*trackChildErrors = true
		}
		p.pushStackNode(&fork, goal, errNode, entryScratch, gssScratch)
	}
	for _, ex := range trailing {
		p.pushStackNode(&fork, goal, ex, entryScratch, gssScratch)
	}
	return fork, true
}

// cAbsorbTokenIntoError ports the strategy-2 tail of ts_parser__recover:
// mark extra-shiftable tokens extra (excluded from error cost), then fold the
// token into the open error region at ERROR_STATE.
func (p *Parser) cAbsorbTokenIntoError(v *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
	leaf := newLeafNodeInArena(arena, tok.Symbol, p.isNamedSymbol(tok.Symbol),
		tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
	leaf.setHasError(true)
	// C: if the token shifts as extra in state 1, mark it extra so it is not
	// counted in error cost calculations.
	if idx := p.lookupActionIndex(1, tok.Symbol); idx != 0 && int(idx) < len(p.language.ParseActions) {
		if actions := p.language.ParseActions[idx].Actions; len(actions) > 0 {
			if last := actions[len(actions)-1]; last.Type == ParseActionShift && last.Extra {
				leaf.setExtra(true)
			}
		}
	}
	if trackChildErrors != nil {
		*trackChildErrors = true
	}

	rec := v.cRec
	if rec != nil && rec.openErr != nil {
		top := stackEntryNode(v.top())
		if top == rec.openErr {
			rec.openErr.children = append(rec.openErr.children, leaf)
			rec.openErr.endByte = tok.EndByte
			rec.openErr.endPoint = tok.EndPoint
			nodeBumpEquivVersion(rec.openErr)
			if v.byteOffset < tok.EndByte {
				v.byteOffset = tok.EndByte
			}
			if nodeCount != nil {
				*nodeCount = *nodeCount + 1
			}
			return
		}
		// Extras were pushed above the open error region (C pops the previous
		// error_repeat plus trailing extras and re-wraps them together).
		entries := cStackEntriesTopFirst(v, gssScratch)
		above := 0
		found := false
		for i := 0; i < len(entries); i++ {
			if stackEntryNode(entries[i]) == rec.openErr {
				above = i
				found = true
				break
			}
		}
		if found {
			extras := make([]*Node, 0, above)
			for i := above - 1; i >= 0; i-- {
				n, _ := materializeStackEntryPayloadEntryWithParser(p, arena, entries[i], materializeForRecovery, materializeForRecovery)
				if n == nil {
					found = false
					break
				}
				extras = append(extras, n)
			}
			if found && v.truncate(len(entries)-above) {
				rec.openErr.children = append(rec.openErr.children, extras...)
				rec.openErr.children = append(rec.openErr.children, leaf)
				rec.openErr.endByte = tok.EndByte
				rec.openErr.endPoint = tok.EndPoint
				nodeBumpEquivVersion(rec.openErr)
				if v.byteOffset < tok.EndByte {
					v.byteOffset = tok.EndByte
				}
				if nodeCount != nil {
					*nodeCount = *nodeCount + 1
				}
				return
			}
		}
	}

	errNode := newParentNodeInArena(arena, errorSymbol, true, []*Node{leaf}, nil, 0)
	errNode.setHasError(true)
	errNode.parseState = cErrorState
	nodeBumpEquivVersion(errNode)
	if perfCountersEnabled {
		perfRecordErrorNode()
	}
	p.pushStackNode(v, cErrorState, errNode, entryScratch, gssScratch)
	if rec != nil {
		rec.openErr = errNode
	}
	if nodeCount != nil {
		*nodeCount = *nodeCount + 2
	}
}

// ---------------------------------------------------------------------------
// Dispatch hooks
// ---------------------------------------------------------------------------

// cRecoverDispatchInError intercepts dispatch for a stack already in the
// error state (C: the ERROR_STATE table row). Extra-shiftable tokens fall
// through to the normal dispatch (C shifts extras in ERROR_STATE without
// extending the error); everything else goes through ts_parser__recover.
func (p *Parser) cRecoverDispatchInError(stacks *[]glrStack, si int, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (cRecoverOutcome, bool) {
	s := &(*stacks)[si]
	if tok.Symbol != 0 {
		if idx := p.lookupActionIndex(cErrorState, tok.Symbol); idx != 0 && int(idx) < len(p.language.ParseActions) {
			if actions := p.language.ParseActions[idx].Actions; len(actions) > 0 &&
				actions[0].Type == ParseActionShift && actions[0].Extra {
				return cRecFallthrough, false
			}
		}
		// Zero-width non-EOF tokens are skipped (C's error-mode lexer never
		// returns empty internal tokens; the Go DFA source can).
		if tok.StartByte == tok.EndByte {
			s.shifted = true
			return cRecConsumed, false
		}
	}
	return p.cRecover(stacks, s, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
}

// cCondenseStacks ports the comparison/prune part of ts_parser__condense_stack
// for the gated grammar: remove versions that clearly lose the error-cost
// competition, order survivors most-promising-first, and enforce
// MAX_VERSION_COUNT. Merging identical stacks remains the job of the regular
// mergeStacks pass. Only runs when some stack is in the error state, so clean
// parses keep today's behavior exactly.
func (p *Parser) cCondenseStacks(stacks []glrStack) []glrStack {
	anyRec := false
	for i := range stacks {
		if stacks[i].cRec != nil {
			anyRec = true
			break
		}
	}
	if !anyRec {
		return stacks
	}
	// Drop dead versions first (C removes halted versions in condense).
	alive := stacks[:0]
	for i := range stacks {
		if !stacks[i].dead {
			alive = append(alive, stacks[i])
		}
	}
	stacks = alive
	for i := 0; i < len(stacks); i++ {
		statusI := p.cVersionStatus(&stacks[i])
		for j := 0; j < i; j++ {
			statusJ := p.cVersionStatus(&stacks[j])
			switch cCompareVersions(statusJ, statusI) {
			case cErrorComparisonTakeLeft:
				stacks = append(stacks[:i], stacks[i+1:]...)
				i--
				j = i
			case cErrorComparisonPreferRight:
				stacks[i], stacks[j] = stacks[j], stacks[i]
				statusI = p.cVersionStatus(&stacks[i])
			case cErrorComparisonTakeRight:
				stacks = append(stacks[:j], stacks[j+1:]...)
				i--
				j--
				statusI = p.cVersionStatus(&stacks[i])
			}
			if i < 0 {
				break
			}
		}
	}
	if len(stacks) > cRecoverMaxVersionCount {
		stacks = stacks[:cRecoverMaxVersionCount]
	}
	return stacks
}

// cStackResultErrorCost is the result-selection cost: the error cost of the
// stack's would-be tree (requirement 4 of the spec: fold error cost into
// stackCompareForResultSelection).
func (p *Parser) cStackResultErrorCost(s *glrStack) uint32 {
	return p.cStackErrorCost(s)
}

// cTreeErrorCost computes the C error cost over a finished tree, for
// retry-selection integration (preferRetryTree).
func (p *Parser) cTreeErrorCost(t *Tree) uint32 {
	if t == nil || t.root == nil {
		return 0
	}
	return p.cNodeErrorCost(t.root)
}

func traceCRecoverToState(state StateID, depth int) {
	fmt.Printf("      -> C-RECOVER-TO-STATE state=%d depth=%d\n", state, depth)
}
