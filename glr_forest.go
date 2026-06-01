package gotreesitter

import (
	"os"
	"sync"
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

// glrForestEnabled is the master switch for the GSS-forest fast path. ON by
// default: the byte-range-verified languages in languageWantsForest dispatch to
// the forest automatically (with production fallback). Set GOT_GLR_FOREST=0 to
// disable globally; tests/benchmarks toggle via SetGLRForestEnabled. Languages
// NOT in the allowlist always use production regardless of this switch.
var glrForestEnabled = os.Getenv("GOT_GLR_FOREST") != "0"

// SetGLRForestEnabled toggles the GSS-forest path at runtime (tests/benchmarks).
func SetGLRForestEnabled(on bool) { glrForestEnabled = on }

// ParseForestExperimental parses source with the experimental GSS-forest GLR
// path and returns the root node (or nil,false if the parse dies — the forest
// path has no error recovery yet). Exported so out-of-tree benchmarks and
// validation in packages that attach external scanners (e.g. grammars) can
// drive it; not part of the stable API.
func (p *Parser) ParseForestExperimental(source []byte) (*Node, bool) {
	return p.parseForest(newNodeArena(arenaClassFull), source)
}

// languageWantsForest reports whether a language dispatches to the GSS-forest
// GLR fast path by default. Restricted to languages whose production GLR parse
// suffers the super-linear deep-stack-equivalence blowup AND that are verified
// byte-identical to production on their real corpus by TestForestCorpusParity
// (which compares full node BYTE RANGES, not just s-expressions — an s-expr-only
// gate hid systematic span bugs). Measured byte-range-clean production-vs-forest
// speedups on the real corpus: erlang 664x, cmake 166x, awk 202x, css 5x, scss
// 3x. GraphQL is clean against production here too, but stays out until the
// production tree is C-oracle-clean on the ring matrix. The
// forest has no error recovery, so tryForestFastPath falls back to production on
// any decline (failure / error / truncation); that fallback means a language can
// never regress the cases it declines, but does NOT catch a clean-but-different
// tree — so a language joins this list only once its byte-range gate is green.
// EXCLUDED: bash (a single-token-GLR-lexing / stateful-scanner divergence),
// python + ruby (still diverge — genuine structural bugs).
func languageWantsForest(name string) bool {
	switch name {
	case "erlang", "cmake", "css", "scss", "awk":
		return true
	default:
		return false
	}
}

// tryForestFastPath attempts a full parse via the GSS-forest path and returns a
// Tree on success, or nil to tell the caller to fall back to the production
// parser. It declines (nil) whenever the forest cannot produce a clean,
// complete tree — it has no error recovery, so any failure, error node, or
// truncation routes to production. Gated by glrForestEnabled (GOT_GLR_FOREST);
// off by default so the production path is unchanged until per-language corpus
// parity is verified and the gate is lifted.
func (p *Parser) tryForestFastPath(source []byte) *Tree {
	if !glrForestEnabled || p == nil || p.language == nil || !languageWantsForest(p.language.Name) {
		return nil
	}
	arena := acquireNodeArena(arenaClassFull)
	root, ok := p.parseForest(arena, source)
	if !ok || root == nil || root.HasError() {
		arena.Release()
		return nil // production fallback handles failures and error recovery
	}
	// Guard against an early-EOF token source: the root must reach the last
	// non-whitespace byte. Trailing whitespace/newlines are extras and may sit
	// outside the root span, so they are excluded from the bound.
	end := len(source)
	for end > 0 {
		switch source[end-1] {
		case ' ', '\t', '\r', '\n':
			end--
			continue
		}
		break
	}
	if root.EndByte() < uint32(end) {
		arena.Release()
		return nil // did not consume the whole input; let production recover it
	}
	tree := newTreeWithArenas(root, source, p.language, arena, nil)
	tree.forestFastPath = true
	if !languageAllowsForestIncrementalPath(p.language.Name) {
		tree.incrementalReuseDisabled = true
	}
	p.normalizeReturnedTree(rawRootOrNil(tree), source)
	return tree
}

// languageAllowsForestIncrementalPath reports forest-default languages whose
// forest-built trees are safe to feed into the normal incremental parser path.
// Some languages still report subtree reuse as unsupported there, but entering
// that path can be much faster than forcing a fresh forest full parse. Languages
// stay disabled until the edited real-corpus matrix proves the path is correct
// and faster than fresh-parse fallback.
func languageAllowsForestIncrementalPath(name string) bool {
	switch name {
	case "erlang", "scss":
		return true
	default:
		return false
	}
}

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
func coalesceForest(index *gssForestIndex, slab *gssForestNodeSlab, state StateID, byteOffset uint32, prev *gssForestNode, entry stackEntry, score, errorCost int) *gssForestNode {
	if perfCountersEnabled {
		perfRecordForestCoalesceCall()
	}
	key := gssForestKey{state: state, byteOffset: byteOffset}
	node := index.lookup(key)
	if node == nil {
		node = slab.alloc(state, byteOffset, score, errorCost)
		index.set(key, node)
		if perfCountersEnabled {
			perfRecordForestCoalesceNewNode()
		}
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
			replaced := false
			if score > l.score {
				l.subtree, l.score = entry, score
				node.dirty++
				replaced = true
			}
			if perfCountersEnabled {
				perfRecordForestCoalesceDedupHit(replaced)
			}
			return node
		}
	}
	// Bound the link fan-out per node (tree-sitter caps active versions). Without
	// a cap, a repeated/ambiguous structure accumulates O(n) links on one node and
	// reduceOverForest enumerates O(n^childCount) paths. Keep the best-score links;
	// replace the weakest when full.
	if len(node.links) >= forestMaxLinksPerNode {
		worst := 0
		for i := 1; i < len(node.links); i++ {
			if node.links[i].score < node.links[worst].score {
				worst = i
			}
		}
		if score > node.links[worst].score {
			node.links[worst] = gssLink{prev: prev, subtree: entry, score: score}
			node.dirty++
			if perfCountersEnabled {
				perfRecordForestCoalesceCap(true)
			}
		} else if perfCountersEnabled {
			perfRecordForestCoalesceCap(false)
		}
		return node
	}
	node.links = append(node.links, gssLink{prev: prev, subtree: entry, score: score})
	if perfCountersEnabled {
		perfRecordForestCoalesceLinkAppend()
	}
	node.dirty++
	return node
}

func forestCoalesceWouldDropForCap(index *gssForestIndex, state StateID, byteOffset uint32, score, errorCost int) bool {
	if index == nil {
		return false
	}
	node := index.lookup(gssForestKey{state: state, byteOffset: byteOffset})
	if node == nil || len(node.links) < forestMaxLinksPerNode {
		return false
	}
	if errorCost < node.errorCost {
		return false
	}
	worstScore := node.links[0].score
	for i := 1; i < len(node.links); i++ {
		if node.links[i].score < worstScore {
			worstScore = node.links[i].score
		}
	}
	return score <= worstScore
}

// forestMaxLinksPerNode caps the alternative fan-out coalesced at one
// (state, byteOffset) node, bounding reduceOverForest's path enumeration.
const forestMaxLinksPerNode = 8

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

type gssForestIndex struct {
	nodes     map[gssForestKey]*gssForestNode
	keys      []gssForestKey
	lastKey   gssForestKey
	lastNode  *gssForestNode
	lastValid bool
}

func newGSSForestIndex(capacity int) gssForestIndex {
	return gssForestIndex{
		nodes: make(map[gssForestKey]*gssForestNode, capacity),
		keys:  make([]gssForestKey, 0, capacity),
	}
}

func (idx *gssForestIndex) reset() {
	for _, key := range idx.keys {
		delete(idx.nodes, key)
	}
	idx.keys = idx.keys[:0]
	idx.lastValid = false
	idx.lastNode = nil
}

func (idx *gssForestIndex) len() int {
	if idx == nil {
		return 0
	}
	return len(idx.nodes)
}

func (idx *gssForestIndex) lookup(key gssForestKey) *gssForestNode {
	if idx.lastValid && idx.lastKey == key {
		return idx.lastNode
	}
	node := idx.nodes[key]
	if node != nil {
		idx.lastKey = key
		idx.lastNode = node
		idx.lastValid = true
	}
	return node
}

func (idx *gssForestIndex) set(key gssForestKey, node *gssForestNode) {
	if idx.nodes == nil {
		idx.nodes = make(map[gssForestKey]*gssForestNode, 16)
	}
	idx.nodes[key] = node
	idx.keys = append(idx.keys, key)
	idx.lastKey = key
	idx.lastNode = node
	idx.lastValid = true
}

const (
	gssForestNodeBatchCap            = 4096
	gssForestLinkBatchCap            = 8192
	maxRetainedGSSForestScratchBytes = 32 * 1024 * 1024
)

var gssForestNodeSlabPool = sync.Pool{
	New: func() any {
		return &gssForestNodeSlab{}
	},
}

// gssForestNodeSlab batch-allocates gssForestNodes so the forest doesn't pay one
// heap allocation per coalesced (state, byteOffset) node — the C GSS pools its
// stack nodes the same way. Nodes must outlive the whole parse (the DAG
// references them via links), so batches stay live until parseForest returns,
// then the scratch is cleared and pooled.
type gssForestNodeSlab struct {
	nodeBatches [][]gssForestNode
	nodeBatch   int
	nodeIdx     int
	linkBatches [][]gssLink
	linkBatch   int
	linkIdx     int
}

func acquireGSSForestNodeSlab() *gssForestNodeSlab {
	s := gssForestNodeSlabPool.Get().(*gssForestNodeSlab)
	s.nodeBatch = 0
	s.nodeIdx = 0
	s.linkBatch = 0
	s.linkIdx = 0
	return s
}

func releaseGSSForestNodeSlab(s *gssForestNodeSlab) {
	if s == nil {
		return
	}
	s.resetForRelease()
	if s.retainedBytes() > maxRetainedGSSForestScratchBytes {
		s.nodeBatches = nil
		s.linkBatches = nil
	}
	gssForestNodeSlabPool.Put(s)
}

func (s *gssForestNodeSlab) alloc(state StateID, byteOffset uint32, score, errorCost int) *gssForestNode {
	if len(s.nodeBatches) == 0 {
		s.nodeBatches = append(s.nodeBatches, make([]gssForestNode, gssForestNodeBatchCap))
	} else if s.nodeIdx >= len(s.nodeBatches[s.nodeBatch]) {
		s.nodeBatch++
		s.nodeIdx = 0
		if s.nodeBatch >= len(s.nodeBatches) {
			s.nodeBatches = append(s.nodeBatches, make([]gssForestNode, gssForestNodeBatchCap))
		}
	}
	n := &s.nodeBatches[s.nodeBatch][s.nodeIdx]
	s.nodeIdx++
	n.state = state
	n.byteOffset = byteOffset
	n.score = score
	n.errorCost = errorCost
	n.links = s.linkSlice()
	n.dirty = 0
	return n
}

// linkSlice hands out a zero-length slice backed by the shared link buffer with
// enough capacity for the capped forest fan-out. The pooled slab makes this a
// retained scratch cost and avoids per-node append growth on ambiguous states.
func (s *gssForestNodeSlab) linkSlice() []gssLink {
	const initCap = forestMaxLinksPerNode
	if len(s.linkBatches) == 0 {
		s.linkBatches = append(s.linkBatches, make([]gssLink, gssForestLinkBatchCap))
	} else if s.linkIdx+initCap > len(s.linkBatches[s.linkBatch]) {
		s.linkBatch++
		s.linkIdx = 0
		if s.linkBatch >= len(s.linkBatches) {
			s.linkBatches = append(s.linkBatches, make([]gssLink, gssForestLinkBatchCap))
		}
	}
	buf := s.linkBatches[s.linkBatch]
	sl := buf[s.linkIdx : s.linkIdx : s.linkIdx+initCap]
	s.linkIdx += initCap
	return sl
}

func (s *gssForestNodeSlab) resetForRelease() {
	for i := 0; i <= s.nodeBatch && i < len(s.nodeBatches); i++ {
		used := len(s.nodeBatches[i])
		if i == s.nodeBatch {
			used = s.nodeIdx
		}
		clear(s.nodeBatches[i][:used])
	}
	for i := 0; i <= s.linkBatch && i < len(s.linkBatches); i++ {
		used := len(s.linkBatches[i])
		if i == s.linkBatch {
			used = s.linkIdx
		}
		clear(s.linkBatches[i][:used])
	}
	s.nodeBatch = 0
	s.nodeIdx = 0
	s.linkBatch = 0
	s.linkIdx = 0
}

func (s *gssForestNodeSlab) retainedBytes() int {
	total := 0
	nodeSize := int(unsafe.Sizeof(gssForestNode{}))
	linkSize := int(unsafe.Sizeof(gssLink{}))
	for _, batch := range s.nodeBatches {
		total += cap(batch) * nodeSize
	}
	for _, batch := range s.linkBatches {
		total += cap(batch) * linkSize
	}
	return total
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

	// Reuse ONE child-builder scratch for every reduce in this parse (like the
	// production loop). buildReduceChildrenWithPath calls newReduceBuildScratch,
	// which reuses p.reduceScratch when set, else allocates a fresh scratch +
	// growing node slice PER REDUCE — the dominant forest allocation. One reused
	// scratch turns that into a single up-front allocation.
	prevReduceScratch := p.reduceScratch
	var forestReduceScratch reduceBuildScratch
	p.reduceScratch = &forestReduceScratch
	defer func() { p.reduceScratch = prevReduceScratch }()

	// Drive the production token source so keyword promotion, lex-mode
	// selection, immediate tokens, external scanners and GLR-lexing all match
	// the production parser. State is set per step from the frontier.
	lexer := NewLexer(lang.LexStates, source)
	ts := acquireDFATokenSource(lexer, lang, p.lookupActionIndex, p.hasKeywordState, p.externalValidByState)

	// tree-sitter convention: state 0 is the error state, state 1 is the start.
	start := &gssForestNode{state: 1, byteOffset: 0}
	frontier := []*gssForestNode{start}
	glrStates := make([]StateID, 0, 16)
	reducer := &forestReducer{}
	slab := acquireGSSForestNodeSlab()
	defer releaseGSSForestNodeSlab(slab)

	// Per-step scratch reused across every token (cleared, not reallocated): the
	// allocation/GC of fresh maps+slices each step dominated the profile.
	curIndex := newGSSForestIndex(16)
	nextIndex := newGSSForestIndex(16)
	processed := make(map[*gssForestNode]int, 16)
	var work, nextFrontier []*gssForestNode

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
		curIndex.reset()
		for _, n := range frontier {
			curIndex.set(gssForestKey{n.state, n.byteOffset}, n)
		}
		nextIndex.reset()
		clear(processed)
		nextFrontier = nextFrontier[:0]
		var accepted *gssForestNode

		work = append(work[:0], frontier...)
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
					reducer.reduce(node, cc, func(children []stackEntry, childScore int, popTo *gssForestNode) {
						gotoState := p.lookupGoto(popTo.state, act.Symbol)
						if gotoState == 0 {
							if perfCountersEnabled {
								perfRecordForestReduceGotoMiss()
							}
							return
						}
						if perfCountersEnabled {
							perfRecordForestReduceGotoHit()
						}
						// Trailing extras (a comment after a complete construct)
						// are not part of the reduced node — they belong to the
						// surrounding context. Trim them here and re-push them on
						// top of the reduced node so the next (outer) reduce attaches
						// them, mirroring reduceWindowFromGSS + the trailing re-push.
						// `children` is the reducer's shared buffer, stable for the
						// duration of this visit (no re-entry until we return), so the
						// node-builder and span helpers read it in place — no per-reduce
						// copy. window = children[0:reducedEnd] (trailing extras trimmed).
						reducedEnd := reducedEndBeforeTrailingExtras(children)
						parentEnd := node.byteOffset
						if reducedEnd > 0 {
							parentEnd = (*Node)(children[reducedEnd-1].node).endByte
						}
						score := int(act.DynamicPrecedence) + childScore
						if forestCoalesceWouldDropForCap(&curIndex, gotoState, parentEnd, score, popTo.errorCost) {
							if perfCountersEnabled {
								perfRecordForestCoalescePreCapDrop()
							}
							return
						}
						childNodes, fieldIDs, fieldSources, childPath := p.buildReduceChildrenWithPath(children, 0, reducedEnd, cc, act.Symbol, act.ProductionID, arena)
						parent := newParentNodeInArenaWithFieldSources(arena, act.Symbol, named(act.Symbol), childNodes, fieldIDs, fieldSources, act.ProductionID)
						// Recover the reduced node's byte span from the full window,
						// mirroring the production reduce. newParentNode spans only the
						// VISIBLE children, so anonymous/invisible tokens that
						// buildReduceChildren drops (e.g. the digits of a css
						// integer_value, or a node with zero visible children) would
						// otherwise leave the span wrong or empty ([0:0]).
						if shouldUseRawSpanForReduction(act.Symbol, childNodes, lang.SymbolMetadata, p.forceRawSpanAll, p.forceRawSpanTable) && reducedEnd > 0 {
							span := computeReduceRawSpan(children, 0, reducedEnd)
							parent.startByte, parent.endByte = span.startByte, span.endByte
							parent.startPoint, parent.endPoint = span.startPoint, span.endPoint
						}
						if reduceChildPathMayDropSpan(childPath) {
							extendParentSpanToWindow(parent, children, 0, reducedEnd, lang.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols)
						}
						// Position the reduced node at the end of its last real
						// child (before any trimmed trailing extras), falling back
						// to the frontier position for empty productions.
						parent.preGotoState = popTo.state
						parent.parseState = gotoState
						// Subtree score = this production's dynamic precedence +
						// the children's accumulated scores.
						top := coalesceForest(&curIndex, slab, gotoState, parentEnd, popTo,
							stackEntry{node: unsafe.Pointer(parent), state: gotoState, kind: stackEntryKindNode},
							score, popTo.errorCost)
						for _, ex := range children[reducedEnd:] {
							extra := (*Node)(ex.node)
							extra.parseState = gotoState
							nodeBumpEquivVersion(extra)
							exEnd := extra.endByte
							top = coalesceForest(&curIndex, slab, gotoState, exEnd, top,
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
					leaf.preGotoState = node.state
					leaf.parseState = target
					before := nextIndex.len()
					sh := coalesceForest(&nextIndex, slab, target, tok.EndByte, node,
						stackEntry{node: unsafe.Pointer(leaf), state: target, kind: stackEntryKindNode},
						0, node.errorCost) // a shifted leaf carries no dynamic precedence
					if nextIndex.len() != before {
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
			// The production root spans the whole input, including trailing
			// trivia; the forest root stops at the last token. Extend to match
			// when the remaining bytes are trivia (whitespace/comments only).
			if int(root.endByte) < len(source) && bytesAreTrivia(source[root.endByte:]) {
				extendNodeEndTo(root, uint32(len(source)), source)
			}
			return root, true
		}
		if len(nextFrontier) == 0 {
			return nil, false
		}
		// Copy (not alias) so the next step can reset nextFrontier in place;
		// frontier is only read at the top of a step, before that reset.
		frontier = append(frontier[:0], nextFrontier...)
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
	(&forestReducer{}).reduce(node, childCount, visit)
}

// forestReducer holds the two scratch slices the reduce DFS reuses across every
// call within one parse, so the hot path allocates nothing: `path` collects the
// current branch most-recent-first (append on descend, truncate on backtrack),
// and `rev` is the left-to-right view handed to visit. The visitor must consume
// children before returning (it copies), and must not re-enter reduce.
type forestReducer struct {
	path []stackEntry
	rev  []stackEntry
}

// reduce walks back to childCount non-extra subtrees ending at node, including
// any interior extras in the window (they do not count toward childCount —
// mirroring reduceWindowFromGSS), and calls visit once per surviving path with
// the children left-to-right and popTo = the predecessor the reduction pops to.
func (fr *forestReducer) reduce(node *gssForestNode, childCount int, visit func(children []stackEntry, childScore int, popTo *gssForestNode)) {
	if node == nil {
		return
	}
	if perfCountersEnabled {
		perfRecordForestReduceCall(childCount)
	}
	if childCount == 0 {
		if perfCountersEnabled {
			perfRecordForestReduceZero()
		}
		visit(nil, 0, node)
		return
	}
	if fr.reduceLinearNoExtras(node, childCount, visit) {
		if perfCountersEnabled {
			perfRecordForestReduceLinearNoExtras(childCount)
		}
		return
	}
	if perfCountersEnabled {
		perfRecordForestReduceDFS()
	}
	fr.path = fr.path[:0]
	fr.dfs(node, childCount, 0, visit)
}

func (fr *forestReducer) reduceLinearNoExtras(node *gssForestNode, childCount int, visit func(children []stackEntry, childScore int, popTo *gssForestNode)) bool {
	if childCount <= 0 {
		return false
	}
	if cap(fr.rev) < childCount {
		fr.rev = make([]stackEntry, childCount)
	} else {
		fr.rev = fr.rev[:childCount]
	}
	cur := node
	score := 0
	for i := childCount - 1; i >= 0; i-- {
		if cur == nil || len(cur.links) != 1 {
			return false
		}
		link := cur.links[0]
		if stackEntryNodeIsExtra(link.subtree) {
			return false
		}
		fr.rev[i] = link.subtree
		score += link.score
		cur = link.prev
	}
	visit(fr.rev, score, cur)
	return true
}

func (fr *forestReducer) dfs(cur *gssForestNode, remaining, score int, visit func(children []stackEntry, childScore int, popTo *gssForestNode)) {
	if cur == nil {
		return
	}
	mark := len(fr.path)
	for i := range cur.links {
		link := cur.links[i]
		extra := stackEntryNodeIsExtra(link.subtree)
		if perfCountersEnabled {
			perfRecordForestReduceDFSStep(len(cur.links), extra)
		}
		fr.path = append(fr.path[:mark], link.subtree)
		rem := remaining
		if !extra {
			rem--
		}
		if rem == 0 {
			if perfCountersEnabled {
				perfRecordForestReduceDFSVisit(len(fr.path))
			}
			fr.rev = fr.rev[:0]
			for j := len(fr.path) - 1; j >= 0; j-- {
				fr.rev = append(fr.rev, fr.path[j])
			}
			visit(fr.rev, score+link.score, link.prev)
			continue
		}
		fr.dfs(link.prev, rem, score+link.score, visit)
	}
	fr.path = fr.path[:mark]
}
