package gotreesitter

import (
	"os"
	"unsafe"
)

// GSS-FOREST REWRITE (perf/glr-gss-forest) — the only safe cut at the #1
// machinery gap vs tree-sitter C: deep stack-merge node-equivalence is ~46% of
// a fork-heavy parse, because we materialize one tree per stack and must
// deep-compare to dedup. tree-sitter C never compares: its graph-structured
// stack coalesces versions by (state, position) and keeps subtree alternatives
// as forest LINKS (lib/src/stack.c ts_stack_can_merge = 4 scalars + add_link),
// collapsing the forest at finalization by dynamic_precedence/error_cost.
//
// We already have (state, position) merge keys (mergeKeyForStack) and the
// disambiguator (stackCompareMerge: accepted > error-rank > score > shifted).
// The missing piece is a multi-link GSS node. This file builds it behind a flag
// so the default (table-driven dedup) path is untouched until the forest path
// reaches byte-for-byte parity.
//
// STAGED PLAN (see project_glr_merge_design memory + the gss-forest-rewrite
// spore). Stages 1 and 2 are coupled — coalesce produces alternatives that only
// parse correctly once reduce traverses all of them, so parity is expected only
// when BOTH land:
//
//	Stage 0  DONE — instrument. dedup fires 0.2%, fan-out bounded 12-20, so the
//	         forest is narrow (cheap) and the 46% is genuinely wasted compares.
//	Stage 1  DAG node + coalesce-by-(state,position) on push (this file).
//	Stage 2  reduce-over-DAG: enumerate all length-N paths through the links
//	         (C ts_stack_pop_count). The crux; needs error_cost/version bounding.
//	Stage 3  forest finalization: pick per node by score, matching tree-sitter's
//	         dynamic_precedence-then-first-match selection for byte-identical out.

// glrForestEnabled gates the experimental GSS-forest reduce path. Off by default
// (read once from GOT_GLR_FOREST=1) so production keeps the proven dedup path.
var glrForestEnabled = os.Getenv("GOT_GLR_FOREST") == "1"

// SetGLRForestEnabled toggles the GSS-forest path at runtime (tests/benchmarks).
func SetGLRForestEnabled(on bool) { glrForestEnabled = on }

// gssLink is one alternative predecessor in the forest DAG: the subtree consumed
// to reach this node, and the prior node it was consumed from. A coalesced node
// (one per (state, position)) carries one link per surviving parse that reached
// it — exactly tree-sitter C's StackNode.links[].
type gssLink struct {
	prev    *gssForestNode
	subtree stackEntry
	// score is the subtree's cumulative dynamic precedence (a reduce's
	// DynamicPrecedence plus its children's scores; 0 for a shifted leaf). The
	// forest defers ambiguity resolution to finalization: among alternatives at
	// one (state, position), the highest-score subtree wins, matching
	// tree-sitter's dynamic_precedence selection.
	score int
}

// gssForestNode is a coalesced graph-structured-stack node: all parses that
// reach (state, byteOffset) share this single node; their differing histories
// are the links. This replaces the singly-linked gssNode{entry, prev} chain in
// the forest path. score carries the best accumulated dynamic precedence among
// the links for finalization tie-breaks.
type gssForestNode struct {
	state      StateID
	byteOffset uint32
	links      []gssLink
	score      int
	errorCost  int
	// dirty advances whenever a link is appended OR a competing link is
	// replaced by a higher-precedence alternative. Because Nodes are built
	// eagerly at reduce time, a late replacement must re-trigger the reductions
	// that consumed this node so parents rebuild from the winning subtree; the
	// worklist reprocesses a node whenever its dirty count moved past what it
	// last processed. Replacements only happen on a strictly higher score, so
	// dirty advances finitely and the loop terminates.
	dirty int
}

// coalesceForest merges a parse reaching (state, byteOffset) with subtree `entry`
// from predecessor `prev` into the forest: if a node already exists for that
// (state, byteOffset) it gains a link (O(1), no deep-compare — the heart of the
// win); otherwise a new node is created. `index` maps (state, byteOffset) to the
// node so coalescing is a map lookup, not a stack scan.
//
// Stage 1 scaffold: builds the DAG. Correct trees require Stage 2 (reduce walks
// every link); until then this is exercised only under the flag + parity gate.
func coalesceForest(index map[gssForestKey]*gssForestNode, state StateID, byteOffset uint32, prev *gssForestNode, entry stackEntry, score, errorCost int) *gssForestNode {
	key := gssForestKey{state: state, byteOffset: byteOffset}
	node := index[key]
	if node == nil {
		node = &gssForestNode{state: state, byteOffset: byteOffset, score: score, errorCost: errorCost}
		index[key] = node
	} else if errorCost < node.errorCost || (errorCost == node.errorCost && score > node.score) {
		node.score, node.errorCost = score, errorCost
	}
	// Dedup competing alternatives: a link from the same predecessor whose
	// subtree has the same symbol and span is the same reduction reached another
	// way — keep the higher dynamic precedence (tree-sitter's resolution) instead
	// of accumulating a duplicate. This bounds the forest (no exponential link
	// blowup on ambiguous grammars) AND performs Stage-3 disambiguation, cheaply,
	// with no deep-equivalence walk. Only materialized subtrees carry a comparable
	// symbol+span, so the dedup applies to node entries only.
	if entry.kind == stackEntryKindNode && entry.node != nil {
		esym, estart, eend := entrySymSpan(entry)
		for i := range node.links {
			l := &node.links[i]
			if l.prev != prev || l.subtree.kind != stackEntryKindNode {
				continue
			}
			lsym, lstart, lend := entrySymSpan(l.subtree)
			if lsym != esym || lstart != estart || lend != eend {
				continue
			}
			// Competing reduction reaching the same (prev, symbol, span): keep the
			// strictly higher dynamic precedence. A replacement marks the node
			// dirty so the reductions that already consumed the losing subtree
			// re-run and rebuild their parents from the winner.
			if score > l.score {
				l.subtree, l.score = entry, score
				node.dirty++
			}
			return node
		}
	}
	node.links = append(node.links, gssLink{prev: prev, subtree: entry, score: score})
	node.dirty++
	return node
}

// entrySymSpan returns a materialized node entry's symbol and byte span for cheap
// alternative-deduplication (no deep structural comparison).
func entrySymSpan(e stackEntry) (Symbol, uint32, uint32) {
	n := (*Node)(e.node)
	return n.symbol, n.startByte, n.endByte
}

// collectForestRootAndExtras walks the winning (bestLink) path down from the
// accept node to locate the start-symbol root and gather the root-level extras
// that surround it: extras stacked above it are trailing, extras below it are
// leading. Each group is returned in source order; foldResultRootExtras splits
// them back into leading/trailing by position.
func collectForestRootAndExtras(accepted *gssForestNode) (*Node, []*Node) {
	if accepted == nil {
		return nil, nil
	}
	var above []*Node // trailing extras, collected latest-first
	var root *Node
	below := (*gssForestNode)(nil)
	for cur := accepted; cur != nil; {
		link := cur.bestLink()
		if link == nil {
			return nil, nil
		}
		n := (*Node)(link.subtree.node)
		if n.isExtra() {
			above = append(above, n)
			cur = link.prev
			continue
		}
		root, below = n, link.prev
		break
	}
	if root == nil {
		return nil, nil
	}
	var belowExtras []*Node // leading extras, collected latest-first
	for cur := below; cur != nil; {
		link := cur.bestLink()
		if link == nil {
			break
		}
		n := (*Node)(link.subtree.node)
		if !n.isExtra() {
			break
		}
		belowExtras = append(belowExtras, n)
		cur = link.prev
	}
	if len(above) == 0 && len(belowExtras) == 0 {
		return root, nil
	}
	// Reverse each group into source order, then concatenate (leading first).
	extras := make([]*Node, 0, len(belowExtras)+len(above))
	for i := len(belowExtras) - 1; i >= 0; i-- {
		extras = append(extras, belowExtras[i])
	}
	for i := len(above) - 1; i >= 0; i-- {
		extras = append(extras, above[i])
	}
	return root, extras
}

// bestLink returns the link whose subtree wins tree-sitter's selection:
// highest score (dynamic precedence), then earliest (production order).
func (n *gssForestNode) bestLink() *gssLink {
	if n == nil || len(n.links) == 0 {
		return nil
	}
	best := &n.links[0]
	for i := 1; i < len(n.links); i++ {
		if n.links[i].score > best.score {
			best = &n.links[i]
		}
	}
	return best
}

type gssForestKey struct {
	state      StateID
	byteOffset uint32
}

// parseForest runs the GSS-forest GLR algorithm end to end: coalesce by
// (state, byteOffset), reduce over the DAG via reduceOverForest, with NO deep
// equivalence walk anywhere — the merge cost that was ~46% of fork-heavy parses
// is structurally gone. Tokens are pulled via nextToken(leadState) (the lexer /
// token-source wiring stays the caller's concern); the accepted root subtree is
// returned, or (nil,false) if the parse dies. This is the forest path the
// GOT_GLR_FOREST flag dispatches into; parity-iteration (extras, recovery,
// external scanners, full GLR-lexing) is layered on this core.
func (p *Parser) parseForest(arena *nodeArena, source []byte) (*Node, bool) {
	lang := p.language
	meta := lang.SymbolMetadata
	named := func(sym Symbol) bool { return int(sym) < len(meta) && meta[sym].Named }

	// Drive the production token source so keyword promotion, lex-mode
	// selection, immediate tokens, external scanners and GLR-lexing all match
	// the production parser. State is set per step from the frontier.
	lexer := NewLexer(lang.LexStates, source)
	ts := acquireDFATokenSource(lexer, lang, p.lookupActionIndex, p.hasKeywordState, p.externalValidByState)

	// tree-sitter convention: state 0 is the error state, state 1 is the start.
	start := &gssForestNode{state: 1, byteOffset: 0}
	frontier := []*gssForestNode{start}
	glrStates := make([]StateID, 0, 16)

	for {
		// GLR-lex over the union of frontier states; lead = the most-advanced.
		glrStates = glrStates[:0]
		for _, n := range frontier {
			glrStates = append(glrStates, n.state)
		}
		ts.SetGLRStates(glrStates)
		ts.SetParserState(frontier[len(frontier)-1].state)
		tok := ts.Next()
		eof := tok.Symbol == 0

		// Reduces coalesce into curIndex (same position, seeded with the
		// frontier so a reduced nonterminal can merge with an existing actor);
		// shifts coalesce into nextIndex (next position).
		curIndex := make(map[gssForestKey]*gssForestNode, len(frontier))
		for _, n := range frontier {
			curIndex[gssForestKey{n.state, n.byteOffset}] = n
		}
		nextIndex := map[gssForestKey]*gssForestNode{}
		var nextFrontier []*gssForestNode
		var accepted *gssForestNode

		work := append([]*gssForestNode(nil), frontier...)
		processed := map[*gssForestNode]int{}
		for len(work) > 0 {
			node := work[len(work)-1]
			work = work[:len(work)-1]
			// Process a node the first time it is seen, and again whenever it has
			// become dirty (a new link, or a link replaced by a higher-precedence
			// alternative) since it was last processed. Re-running its reductions
			// rebuilds any parents that consumed a now-superseded subtree.
			if pv, seen := processed[node]; seen && pv == node.dirty {
				continue
			}
			processed[node] = node.dirty

			for _, act := range p.actionsForParseState(node.state, tok.Symbol, lang.ParseActions) {
				switch act.Type {
				case ParseActionReduce:
					cc := int(act.ChildCount)
					reduceOverForest(node, cc, func(children []stackEntry, childScore int, popTo *gssForestNode) {
						gotoState := p.lookupGoto(popTo.state, act.Symbol)
						if gotoState == 0 {
							return
						}
						// Trailing extras (a comment after a complete construct)
						// are not part of the reduced node — they belong to the
						// surrounding context. Trim them here and re-push them on
						// top of the reduced node so the next (outer) reduce attaches
						// them, mirroring reduceWindowFromGSS + the trailing re-push.
						reducedEnd := reducedEndBeforeTrailingExtras(children)
						kids := append([]stackEntry(nil), children[:reducedEnd]...)
						childNodes, fieldIDs, fieldSources, _ := p.buildReduceChildrenWithPath(kids, 0, len(kids), cc, act.Symbol, act.ProductionID, arena)
						parent := newParentNodeInArenaWithFieldSources(arena, act.Symbol, named(act.Symbol), childNodes, fieldIDs, fieldSources, act.ProductionID)
						// Position the reduced node at the end of its last real
						// child (before any trimmed trailing extras), falling back
						// to the frontier position for empty productions.
						parentEnd := node.byteOffset
						if reducedEnd > 0 {
							parentEnd = (*Node)(kids[reducedEnd-1].node).endByte
						}
						// Subtree score = this production's dynamic precedence +
						// the children's accumulated scores.
						top := coalesceForest(curIndex, gotoState, parentEnd, popTo,
							stackEntry{node: unsafe.Pointer(parent), state: gotoState, kind: stackEntryKindNode},
							int(act.DynamicPrecedence)+childScore, popTo.errorCost)
						for _, ex := range children[reducedEnd:] {
							exEnd := (*Node)(ex.node).endByte
							top = coalesceForest(curIndex, gotoState, exEnd, top,
								stackEntry{node: ex.node, state: gotoState, kind: stackEntryKindNode},
								0, top.errorCost)
						}
						work = append(work, top)
					})
				case ParseActionShift:
					leaf := newLeafNodeInArena(arena, tok.Symbol, named(tok.Symbol), tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
					// An extra (comment/whitespace) shifts without advancing the
					// parse state: it stays transparent to the grammar and is
					// attached to the surrounding node as an extra child at the next
					// reduce. extraShiftTargetState keeps the current state when the
					// action carries no explicit target.
					target := act.State
					if act.Extra {
						leaf.setExtra(true)
						target = extraShiftTargetState(node.state, act)
					}
					before := len(nextIndex)
					sh := coalesceForest(nextIndex, target, tok.EndByte, node,
						stackEntry{node: unsafe.Pointer(leaf), state: target, kind: stackEntryKindNode},
						0, node.errorCost) // a shifted leaf carries no dynamic precedence
					if len(nextIndex) != before {
						nextFrontier = append(nextFrontier, sh)
					}
				case ParseActionAccept:
					accepted = node
				}
			}
		}

		if eof {
			root, extras := collectForestRootAndExtras(accepted)
			if root == nil {
				return nil, false
			}
			// Leading/trailing extras live outside the start-symbol node (above or
			// below it on the accepted stack); fold them into the root the way the
			// production result builder does, splitting by position.
			if len(extras) > 0 {
				foldResultRootExtras(root, extras, arena)
			}
			return root, true
		}
		if len(nextFrontier) == 0 {
			return nil, false
		}
		frontier = nextFrontier
	}
}

// reduceOverForest enumerates every length-childCount path of subtrees ending at
// `node` and invokes visit once per path with the children in left-to-right
// order (children[0] = first/leftmost child) and `popTo` = the predecessor node
// the reduction pops back to. This is Stage 2 — DAG traversal that replaces the
// single-chain reduce so a coalesced node's multiple histories all reduce, with
// no deep-equivalence walk anywhere (the 46% gone). A single-link chain yields
// exactly one path, identical to today's reduce; coalesced nodes yield one path
// per surviving alternative.
//
// `children` is a SHARED buffer reused across paths and across visit calls — the
// visitor must consume or copy it before returning, never retain it. The walk is
// a bounded DFS (depth == childCount); ambiguous grammars are bounded upstream by
// error_cost pruning + the per-(state,position) link cap, mirroring tree-sitter C.
func reduceOverForest(node *gssForestNode, childCount int, visit func(children []stackEntry, childScore int, popTo *gssForestNode)) {
	if node == nil {
		return
	}
	if childCount == 0 {
		visit(nil, 0, node)
		return
	}
	// Walk back to childCount *non-extra* subtrees. Extra leaves (comments,
	// whitespace) encountered between them are included in the child window but
	// do not count toward childCount — mirroring reduceWindowFromGSS so the
	// reduced node carries its interior extras while the production's symbol
	// count stays correct. The path is collected most-recent-first and reversed
	// to left-to-right at emit. A per-branch copy keeps sibling links (the forks)
	// from corrupting each other's window.
	var dfs func(cur *gssForestNode, remaining, score int, path []stackEntry)
	dfs = func(cur *gssForestNode, remaining, score int, path []stackEntry) {
		if cur == nil {
			return
		}
		for i := range cur.links {
			link := cur.links[i]
			next := make([]stackEntry, len(path)+1)
			copy(next, path)
			next[len(path)] = link.subtree
			rem := remaining
			if !stackEntryNodeIsExtra(link.subtree) {
				rem--
			}
			if rem == 0 {
				children := make([]stackEntry, len(next))
				for j := range next {
					children[j] = next[len(next)-1-j]
				}
				visit(children, score+link.score, link.prev)
				continue
			}
			dfs(link.prev, rem, score+link.score, next)
		}
	}
	dfs(node, childCount, 0, nil)
}
