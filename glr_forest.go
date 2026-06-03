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

// nodeCachedHeight returns the subtree height (root = 1), memoized on the node
// (n.subtreeHeight, 0 = uncomputed). Nodes are immutable after build and arena
// slots are zeroed on alloc, so the cache is valid within a parse and never stale
// across parses. Keeps the coalesce dedup tie-break O(1) amortized instead of an
// O(subtree) walk on every score tie (which 7x'd merge-heavy parses).
func nodeCachedHeight(n *Node) int {
	if n == nil {
		return 0
	}
	if n.subtreeHeight != 0 {
		return int(n.subtreeHeight)
	}
	best := 0
	for _, c := range n.children {
		if h := nodeCachedHeight(c); h > best {
			best = h
		}
	}
	h := best + 1
	if h > 255 {
		h = 255
	}
	n.subtreeHeight = uint8(h)
	return h
}

func stackEntrySubtreeHeight(e stackEntry) int {
	if e.kind != stackEntryKindNode || e.node == nil {
		return 0
	}
	return nodeCachedHeight((*Node)(e.node))
}

// forestDedupTieReplace reports whether, on a coalesce dedup score tie, the new
// entry should replace the existing link — true only when the new subtree is
// taller, mirroring tree-sitter C / production stackCompareMerge's post-score
// depth tie-break (go type_instantiation_expression over index_expression under
// the shared `_expression` supertype on `m.T[r.s][r.t]`).
func forestDedupTieReplace(entry, existing stackEntry) bool {
	return stackEntrySubtreeHeight(entry) > stackEntrySubtreeHeight(existing)
}

// ParseForestExperimental parses source with the experimental GSS-forest GLR
// path and returns a releasable tree (or nil,false if the parse dies — the
// forest path has no error recovery yet). Exported so out-of-tree benchmarks
// and validation in packages that attach external scanners (e.g. grammars) can
// drive it; not part of the stable API.
func (p *Parser) ParseForestExperimental(source []byte) (*Tree, bool) {
	arena := acquireNodeArena(arenaClassFull)
	root, ok := p.parseForest(arena, source)
	if !ok || root == nil {
		arena.Release()
		return nil, false
	}
	p.finalizeForestRoot(root, source)
	return newTreeWithArenas(root, source, p.language, arena, nil), true
}

// languageWantsForest reports whether a language dispatches to the GSS-forest
// GLR fast path by default. Restricted to languages whose production GLR parse
// suffers the super-linear deep-stack-equivalence blowup AND that are verified
// byte-identical to production on their real corpus by TestForestCorpusParity
// (which compares full node BYTE RANGES, not just s-expressions — an s-expr-only
// gate hid systematic span bugs). Measured byte-range-clean production-vs-forest
// speedups on the real corpus: bash 803x, erlang 664x, cmake 166x, awk 202x,
// javascript 36x, css 5x, scss 3x, c_sharp 3x. GraphQL is clean against
// production here too, but stays out until the production tree is C-oracle-clean
// on the ring matrix. The forest has no error recovery, so tryForestFastPath
// falls back to production on any decline (failure / error / truncation); that
// fallback means a language can never regress the cases it declines, but does
// NOT catch a clean-but-different tree — so a language joins this list only
// once its byte-range gate is green.
//
// Verified NOT forest-amenable (2026-06-02 sweep — do NOT re-add as "divergent",
// the older note was stale): python is forest byte-CLEAN (diverged=0) but ~0.8x
// because it has no merge blowup for the forest to amortize the GSS overhead
// against; rust forest TRUNCATES (incomplete) and fails safe to production; dart
// declines every file. ruby is unverified. haskell is NOT forest-amenable: its
// production parse is so pathologically slow (the O(n^2) deep-merge blowup) that
// the forest-vs-production gate times out, and the forest-vs-C oracle gate
// (TestForestVsCOracleParity) shows the forest RELOCATES the blowup — its reduce
// DFS times out on every haskell corpus file.
//
// php is now forest byte-clean vs C — the zero-width recovery ";" missing-flag
// fix (commit e5cf641a) made its production tree C-oracle-clean and the forest
// matches it, so correctness no longer blocks it — but it stays OUT on PERF
// grounds: only ~1/3 of its real corpus dispatches, and the GOT_GLR_FOREST
// on/off A/B is a net-wall LOSS (forest ~1.40ms vs production ~1.21ms over the
// corpus) because the failed forest attempts on the ~2/3 fallback files cost
// more than the dispatched third saves. Re-promote only if the dispatch rate
// rises (e.g. the forest learns the constructs it currently parse_fails on).
//
// go joined 2026-06-03. forest-vs-production on the curated corpus is diverged=0
// at 1.5x; forest-vs-C oracle is diverged=0 at ~0.88x C (near C-tier); and a
// 250-file repo sweep is forest>=production on EVERY file (BETTER 25, equal 224,
// WORSE 0) — the forest is in fact MORE correct than production on real .go (it
// fixes a for-range composite-literal-vs-block mis-parse and parses ~6% of valid
// files production error-recovers). Required three forest fixes:
//   - keyword-leaf collapse (false/nil -> leaf), commit 03031c9b
//   - generics `T[X]` dedup tie-break (taller subtree wins a coalesce score tie,
//     under the shared `_expression` supertype), with cached node height; 880124ca
//   - blank_identifier `import _` C-oracle-seeded gap collapse; adb1a0fd
// One pre-existing gotreesitter-vs-C span quirk (off-by-3 on a few files) is
// SHARED with production (not a forest regression), so it does not block.
func languageWantsForest(name string) bool {
	switch name {
	case "bash", "erlang", "cmake", "css", "scss", "awk", "javascript", "c_sharp", "go":
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
	p.finalizeForestRoot(root, source)
	tree := newTreeWithArenas(root, source, p.language, arena, nil)
	tree.setParseRuntime(forestAcceptedRuntime(root, source))
	tree.forestFastPath = true
	if !languageAllowsForestIncrementalPath(p.language.Name) {
		tree.incrementalReuseDisabled = true
	}
	p.normalizeReturnedTree(rawRootOrNil(tree), source)
	return tree
}

func (p *Parser) finalizeForestRoot(root *Node, source []byte) {
	p.finalizeResultRoot(root, source, nil, false, false)
}

func forestAcceptedRuntime(root *Node, source []byte) ParseRuntime {
	if root == nil {
		return ParseRuntime{StopReason: ParseStopNone}
	}
	sourceLen := uint32(len(source))
	return ParseRuntime{
		StopReason:       ParseStopAccepted,
		SourceLen:        sourceLen,
		ExpectedEOFByte:  sourceLen,
		RootEndByte:      root.EndByte(),
		LastTokenEndByte: sourceLen,
		LastTokenSymbol:  0,
		LastTokenWasEOF:  true,
	}
}

// languageAllowsForestIncrementalPath reports forest-default languages whose
// forest-built trees are safe to feed into the normal incremental parser path.
// Some languages still report subtree reuse as unsupported there, but entering
// that path can be much faster than forcing a fresh forest full parse. Languages
// stay disabled until the edited real-corpus matrix proves the path is correct
// and faster than fresh-parse fallback.
func languageAllowsForestIncrementalPath(name string) bool {
	switch name {
	case "erlang", "cmake", "css", "scss", "javascript":
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
	prev *gssForestNode
	// prevDirty is the predecessor dirty version this link last observed. A
	// link can be structurally identical while its predecessor gained a new
	// alternative; downstream reductions must re-run in that case.
	prevDirty int32
	subtree   stackEntry
	// score is the subtree's cumulative dynamic precedence (a reduce's
	// DynamicPrecedence plus its children's scores; 0 for a shifted leaf). The
	// forest defers ambiguity resolution to finalization: among alternatives at
	// one (state, position), the highest-score subtree wins, matching
	// tree-sitter's dynamic_precedence selection.
	score int
}

func forestNodeDirty(node *gssForestNode) int32 {
	if node == nil {
		return 0
	}
	return node.dirty
}

func forestLinkNoExtraDepth(prev *gssForestNode, entry stackEntry) uint8 {
	if forestStackEntryIsExtra(entry) {
		return 0
	}
	if prev == nil {
		return 1
	}
	if prev.noExtraDepth == ^uint8(0) {
		return prev.noExtraDepth
	}
	return prev.noExtraDepth + 1
}

func forestRecordNoExtraDepth(node *gssForestNode, first bool, depth uint8) {
	if node == nil {
		return
	}
	if first || depth < node.noExtraDepth {
		node.noExtraDepth = depth
	}
}

func forestRecordMinLinkScore(node *gssForestNode, first bool, score int) {
	if node == nil {
		return
	}
	if first || score < node.minLinkScore {
		node.minLinkScore = score
	}
}

func forestRefreshMinLinkScore(node *gssForestNode) {
	if node == nil || len(node.links) == 0 {
		return
	}
	minScore := node.links[0].score
	for i := 1; i < len(node.links); i++ {
		if node.links[i].score < minScore {
			minScore = node.links[i].score
		}
	}
	node.minLinkScore = minScore
}

// gssForestNode is a coalesced graph-structured-stack node: all parses that
// reach (state, byteOffset) share this single node; their differing histories
// are the links. This replaces the singly-linked gssNode{entry, prev} chain in
// the forest path. Link scores carry dynamic-precedence tie-breaks for final
// selection; minLinkScore caches the weakest retained link for cap pruning.
type gssForestNode struct {
	state        StateID
	byteOffset   uint32
	links        []gssLink
	errorCost    int
	minLinkScore int
	// dirty advances whenever a link is appended OR a competing link is
	// replaced by a higher-precedence alternative. Because Nodes are built
	// eagerly at reduce time, a late replacement must re-trigger the reductions
	// that consumed this node so parents rebuild from the winning subtree; the
	// worklist reprocesses a node whenever its dirty count moved past what it
	// last processed. Replacements only happen on a strictly higher score, so
	// dirty advances finitely and the loop terminates.
	dirty          int32
	processedEpoch int32
	processedDirty int32
	noExtraDepth   uint8
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
	linkNoExtraDepth := forestLinkNoExtraDepth(prev, entry)
	if node == nil {
		node = slab.alloc(state, byteOffset, score, errorCost)
		index.set(key, node)
		if perfCountersEnabled {
			perfRecordForestCoalesceNewNode()
		}
	} else if errorCost < node.errorCost {
		node.errorCost = errorCost
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
				oldScore := l.score
				l.subtree, l.score = entry, score
				if oldScore == node.minLinkScore {
					forestRefreshMinLinkScore(node)
				}
				node.dirty++
				replaced = true
			} else if score == l.score && forestDedupTieReplace(entry, l.subtree) {
				l.subtree = entry
				node.dirty++
				replaced = true
			}
			if l.prevDirty != forestNodeDirty(prev) {
				l.prevDirty = forestNodeDirty(prev)
				node.dirty++
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
			node.links[worst] = gssLink{prev: prev, prevDirty: forestNodeDirty(prev), subtree: entry, score: score}
			forestRefreshMinLinkScore(node)
			forestRecordNoExtraDepth(node, false, linkNoExtraDepth)
			node.dirty++
			if perfCountersEnabled {
				perfRecordForestCoalesceCap(true)
			}
		} else if perfCountersEnabled {
			perfRecordForestCoalesceCap(false)
		}
		return node
	}
	firstLink := len(node.links) == 0
	node.links = append(node.links, gssLink{prev: prev, prevDirty: forestNodeDirty(prev), subtree: entry, score: score})
	forestRecordMinLinkScore(node, firstLink, score)
	forestRecordNoExtraDepth(node, firstLink, linkNoExtraDepth)
	if perfCountersEnabled {
		perfRecordForestCoalesceLinkAppend()
	}
	node.dirty++
	return node
}

const forestGotoCacheSize = 8

type forestGotoCache struct {
	states  [forestGotoCacheSize]StateID
	targets [forestGotoCacheSize]StateID
	used    uint8
}

func (c *forestGotoCache) lookup(p *Parser, state StateID, sym Symbol) StateID {
	for i := 0; i < int(c.used); i++ {
		if c.states[i] == state {
			return c.targets[i]
		}
	}
	target := p.lookupGoto(state, sym)
	if c.used < forestGotoCacheSize {
		c.states[c.used] = state
		c.targets[c.used] = target
		c.used++
	}
	return target
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
	return score <= node.minLinkScore
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
// forestCollapsibleNamedKeywordLeaf returns the collapsed LEAF for a unary reduce
// `NamedSym -> single anonymous keyword token` whose token name equals the rule
// name (a keyword-as-named-node: go `false`/`nil`/`true`/`iota`). tree-sitter C
// inlines these to named leaves (ChildCount 0); the production reduce collapses
// them too. Two gates make it forest-safe where the production predicate is not:
//
//   - sameSymbolName only (NOT the broader isSingleTokenWrapperSymbol path that
//     collapsibleRawUnarySelfReduction also takes): production gates that path on
//     child.parent != nil, but the forest connects nodes via gssLink and never
//     sets node.parent, so it would over-collapse rules C keeps as cc=1 (css
//     universal_selector `*`).
//   - KeywordCaptureToken != 0: only languages with word-token keyword extraction
//     inline a `Named -> 'kw'` rule; languages without it keep the token child
//     even when names match (css `to`/`from`), so the same-name test alone is not
//     enough.
//
// aliasedNodeInArena clones, so the shared child is never mutated. Returns nil
// when not applicable.
// forestGapCollapseSymbols lists, per forest language, the named single-token
// rules tree-sitter C collapses to a LEAF that the sameSymbolName test misses
// (the rule name != the token, so it is not a same-name keyword like false/nil:
// go `blank_identifier` -> '_'). C-ORACLE-SEEDED: the collapse_extract dev tool
// (parse each forest lang + go vs the C oracle, diff forest-keeps-vs-C-collapses
// on single anonymous children) found this is the ONLY such gap across all forest
// languages + go — the 8 allowlisted langs are gap-free. Re-run that tool when
// adding a language. This whitelist is the only safe way to collapse these: they
// are statically indistinguishable from single-token rules C KEEPS (awk
// `pattern`), which production tells apart via child.parent, a contextual signal
// the forest's link-based DAG lacks.
var forestGapCollapseSymbols = map[string]map[string]bool{
	"go": {"blank_identifier": true},
}

func forestGapCollapse(lang *Language, sym Symbol) bool {
	if lang == nil {
		return false
	}
	set := forestGapCollapseSymbols[lang.Name]
	if set == nil {
		return false
	}
	if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
		return false
	}
	return set[lang.SymbolNames[sym]]
}

func (p *Parser) forestCollapsibleNamedKeywordLeaf(act ParseAction, tok Token, arena *nodeArena, entries []stackEntry, start, reducedEnd int) *Node {
	if p == nil || arena == nil || tok.NoLookahead {
		return nil
	}
	if p.language == nil || p.language.KeywordCaptureToken == 0 {
		return nil
	}
	if reducedEnd-start != 1 || start < 0 || reducedEnd > len(entries) {
		return nil
	}
	if p.reduceProductionHasEffectiveFields(int(act.ChildCount), act.ProductionID, arena) || len(p.reduceAliasSequence(act.ProductionID)) != 0 {
		return nil
	}
	child := stackEntryNode(entries[start])
	if child == nil || child.ownerArena != arena || child.parent != nil {
		return nil
	}
	if child.symbol == act.Symbol || child.ChildCount() != 0 {
		return nil
	}
	if !p.canCollapseNamedLeafWrapper(act.Symbol, child.symbol) {
		return nil
	}
	if p.shouldPreserveVisibleUnaryTokenWrapper(act.Symbol) {
		return nil
	}
	if !p.sameSymbolName(act.Symbol, child.symbol) && !forestGapCollapse(p.language, act.Symbol) {
		return nil
	}
	return aliasedNodeInArena(arena, p.language, child, act.Symbol)
}

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

const gssForestLookupCacheSize = 16

type gssForestLookupCacheEntry struct {
	key   gssForestKey
	node  *gssForestNode
	valid bool
}

type gssForestIndex struct {
	nodes       map[gssForestKey]*gssForestNode
	keys        []gssForestKey
	lookupCache [gssForestLookupCacheSize]gssForestLookupCacheEntry
	lastKey     gssForestKey
	lastNode    *gssForestNode
	lastValid   bool
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
	for i := range idx.lookupCache {
		idx.lookupCache[i] = gssForestLookupCacheEntry{}
	}
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
	slot := forestLookupCacheSlot(key)
	cached := &idx.lookupCache[slot]
	if cached.valid && cached.key == key {
		idx.lastKey = key
		idx.lastNode = cached.node
		idx.lastValid = true
		return cached.node
	}
	node := idx.nodes[key]
	*cached = gssForestLookupCacheEntry{key: key, node: node, valid: true}
	idx.lastKey = key
	idx.lastNode = node
	idx.lastValid = true
	return node
}

func (idx *gssForestIndex) set(key gssForestKey, node *gssForestNode) {
	if idx.nodes == nil {
		idx.nodes = make(map[gssForestKey]*gssForestNode, 16)
	}
	idx.nodes[key] = node
	idx.keys = append(idx.keys, key)
	idx.lookupCache[forestLookupCacheSlot(key)] = gssForestLookupCacheEntry{key: key, node: node, valid: true}
	idx.lastKey = key
	idx.lastNode = node
	idx.lastValid = true
}

func forestLookupCacheSlot(key gssForestKey) uint32 {
	return (uint32(key.state)*16777619 ^ key.byteOffset ^ (key.byteOffset >> 8)) & (gssForestLookupCacheSize - 1)
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

func (s *gssForestNodeSlab) alloc(state StateID, byteOffset uint32, _ int, errorCost int) *gssForestNode {
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
	n.errorCost = errorCost
	n.minLinkScore = 0
	n.links = s.linkSlice()
	n.dirty = 0
	n.processedEpoch = 0
	n.processedDirty = 0
	n.noExtraDepth = 0
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
	var work, nextFrontier []*gssForestNode
	processEpoch := int32(0)

	for {
		processEpoch++
		if reducer.capped {
			// A forest reduce exceeded forestReduceStepCap (high-ambiguity
			// blowup); decline so the caller falls back to the production parser.
			return nil, false
		}
		// GLR-lex over the union of frontier states; lead = the most-advanced.
		glrStates = glrStates[:0]
		for _, n := range frontier {
			glrStates = append(glrStates, n.state)
		}
		ts.SetGLRStates(glrStates)
		ts.SetParserState(frontier[len(frontier)-1].state)
		tok := ts.Next()
		p.updateCurrentExternalTokenCheckpoint(ts, tok)
		eof := tok.Symbol == 0

		// Reduces coalesce into curIndex (same position, seeded with the
		// frontier so a reduced nonterminal can merge with an existing actor);
		// shifts coalesce into nextIndex (next position).
		curIndex.reset()
		for _, n := range frontier {
			curIndex.set(gssForestKey{n.state, n.byteOffset}, n)
		}
		nextIndex.reset()
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
			if node.processedEpoch == processEpoch && node.processedDirty == node.dirty {
				continue
			}
			node.processedEpoch = processEpoch
			node.processedDirty = node.dirty

			for _, act := range p.actionsForParseState(node.state, tok.Symbol, lang.ParseActions) {
				switch act.Type {
				case ParseActionReduce:
					cc := int(act.ChildCount)
					var gotoCache forestGotoCache
					reducer.reduce(node, cc, func(children []stackEntry, childScore int, popTo *gssForestNode, noExtras bool) {
						gotoState := gotoCache.lookup(p, popTo.state, act.Symbol)
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
						reducedEnd := len(children)
						if !noExtras {
							reducedEnd = reducedEndBeforeTrailingExtras(children)
						}
						score := int(act.DynamicPrecedence) + childScore
						// If the target forest node is already at its fan-out cap and
						// this reduction cannot displace an existing alternative, avoid
						// building the reduced children and parent node just to drop it.
						parentEnd := node.byteOffset
						if reducedEnd < len(children) && reducedEnd > 0 {
							parentEnd = stackEntryNodeEndByte(children[reducedEnd-1])
						}
						if forestCoalesceWouldDropForCap(&curIndex, gotoState, parentEnd, score, popTo.errorCost) {
							if perfCountersEnabled {
								perfRecordForestCoalescePreCapDrop()
							}
							return
						}
						// A collapsible named-keyword-leaf reduce (e.g. go `false`->'false',
						// `nil`, `iota`): the named node absorbs its single anonymous keyword
						// token to a LEAF, matching the production reduce (applyReduceAction)
						// and tree-sitter C (ChildCount 0, not 1). aliasedNodeInArena clones,
						// so the shared forest child is never mutated; skip the child build
						// entirely so the child's parent link is untouched and the collapsed
						// leaf keeps the child's span.
						var parent *Node
						if collapsed := p.forestCollapsibleNamedKeywordLeaf(act, tok, arena, children, 0, reducedEnd); collapsed != nil {
							parent = collapsed
						} else {
							childNodes, fieldIDs, fieldSources, childPath := p.buildReduceChildrenWithPath(children, 0, reducedEnd, cc, act.Symbol, act.ProductionID, arena)
							parent = newParentNodeInArenaWithFieldSources(arena, act.Symbol, named(act.Symbol), childNodes, fieldIDs, fieldSources, act.ProductionID)
							// Recover the reduced node's byte span from the full window,
							// mirroring the production reduce. newParentNode spans only the
							// VISIBLE children, so anonymous/invisible tokens that
							// buildReduceChildren drops (e.g. the digits of a css
							// integer_value, or a node with zero visible children) would
							// otherwise leave the span wrong or empty ([0:0]).
							rawSpanApplied := false
							if shouldUseRawSpanForReduction(act.Symbol, childNodes, lang.SymbolMetadata, p.forceRawSpanAll, p.forceRawSpanTable) && reducedEnd > 0 {
								span := computeReduceRawSpan(children, 0, reducedEnd)
								parent.startByte, parent.endByte = span.startByte, span.endByte
								parent.startPoint, parent.endPoint = span.startPoint, span.endPoint
								rawSpanApplied = true
							}
							if !rawSpanApplied && reduceChildPathMayDropSpan(childPath) {
								extendParentSpanToWindow(parent, children, 0, reducedEnd, lang.SymbolMetadata, p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols)
							}
						}
						// Coalescing tracks parser input position, not necessarily the
						// visible node span. JavaScript blocks can end before dropped
						// anonymous delimiters in the tree, but the stack has still
						// consumed through node.byteOffset. If trailing extras were
						// trimmed, key the parent before those extras so they can be
						// re-pushed on top.
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
					p.recordCurrentExternalLeafCheckpoint(leaf, tok)
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
	(&forestReducer{}).reduce(node, childCount, func(children []stackEntry, childScore int, popTo *gssForestNode, _ bool) {
		visit(children, childScore, popTo)
	})
}

type forestReduceVisitor func(children []stackEntry, childScore int, popTo *gssForestNode, noExtras bool)

// forestReducer holds the two scratch slices the reduce DFS reuses across every
// call within one parse, so the hot path allocates nothing: `path` collects the
// current branch most-recent-first (append on descend, truncate on backtrack),
// and `rev` is the left-to-right view handed to visit. The visitor must consume
// children before returning (it copies), and must not re-enter reduce.
// forestReduceStepCap bounds a SINGLE forest reduce's path enumeration. The
// reduce DFS (forestReducer.dfs / .dfsNoExtras) walks every reduce path through
// the GSS forest; on a high-ambiguity grammar (haskell) the path count is
// exponential and the DFS runs effectively unbounded — it times out the
// forest-vs-C oracle gate (TestForestVsCOracleParity). The counter resets per
// reduce() call and counts link iterations (each is a recursion step or a
// coalescing visit, where the real cost lives); when it crosses this cap the
// reducer sets `capped` (sticky for the rest of the parse) and parseForest
// declines, so a pathological input falls back to the production parser instead
// of hanging. A normal reduce enumerates a handful of paths, orders of magnitude
// under this cap, so it never fires for the allowlisted forest languages.
var forestReduceStepCap = 1 << 16

type forestReducer struct {
	path   []stackEntry
	rev    []stackEntry
	steps  int
	capped bool
}

// reduce walks back to childCount non-extra subtrees ending at node, including
// any interior extras in the window (they do not count toward childCount —
// mirroring reduceWindowFromGSS), and calls visit once per surviving path with
// the children left-to-right and popTo = the predecessor the reduction pops to.
func (fr *forestReducer) reduce(node *gssForestNode, childCount int, visit forestReduceVisitor) {
	if node == nil {
		return
	}
	fr.steps = 0 // per-reduce budget; fr.capped stays sticky for the whole parse
	if perfCountersEnabled {
		perfRecordForestReduceCall(childCount)
	}
	if childCount == 0 {
		if perfCountersEnabled {
			perfRecordForestReduceZero()
		}
		visit(nil, 0, node, true)
		return
	}
	if childCount == 1 && fr.reduceOneNoExtras(node, visit) {
		return
	}
	if fr.reduceLinearNoExtras(node, childCount, visit) {
		if perfCountersEnabled {
			perfRecordForestReduceLinearNoExtras(childCount)
		}
		return
	}
	if fr.reduceForkedLinearNoExtras(node, childCount, visit) {
		if perfCountersEnabled {
			perfRecordForestReduceLinearNoExtras(childCount)
		}
		return
	}
	if fr.reduceForkedLinearSinglePath(node, childCount, visit) {
		return
	}
	if fr.reduceLinearForkedSinglePath(node, childCount, visit) {
		return
	}
	if fr.reduceLinearSinglePath(node, childCount, visit) {
		return
	}
	if fr.reduceNoExtrasDFS(node, childCount, visit) {
		return
	}
	if perfCountersEnabled {
		perfRecordForestReduceDFS()
	}
	fr.path = fr.path[:0]
	fr.dfs(node, childCount, 0, visit)
}

func (fr *forestReducer) reduceOneNoExtras(node *gssForestNode, visit forestReduceVisitor) bool {
	if node == nil {
		return true
	}
	if cap(fr.rev) < 1 {
		fr.rev = make([]stackEntry, 1)
	} else {
		fr.rev = fr.rev[:1]
	}
	for i := range node.links {
		if forestStackEntryIsExtra(node.links[i].subtree) {
			return false
		}
	}
	links := node.links
	for i := range links {
		link := &links[i]
		fr.rev[0] = link.subtree
		visit(fr.rev, link.score, link.prev, true)
	}
	return true
}

func (fr *forestReducer) reduceLinearNoExtras(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
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
		link := &cur.links[0]
		if forestStackEntryIsExtra(link.subtree) {
			return false
		}
		fr.rev[i] = link.subtree
		score += link.score
		cur = link.prev
	}
	visit(fr.rev, score, cur, true)
	return true
}

func (fr *forestReducer) reduceForkedLinearNoExtras(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 1 || node == nil || len(node.links) <= 1 {
		return false
	}
	links := node.links
	for i := range links {
		link := &links[i]
		if forestStackEntryIsExtra(link.subtree) {
			return false
		}
		cur := link.prev
		for child := childCount - 2; child >= 0; child-- {
			if cur == nil || len(cur.links) != 1 {
				return false
			}
			next := &cur.links[0]
			if forestStackEntryIsExtra(next.subtree) {
				return false
			}
			cur = next.prev
		}
	}
	if cap(fr.rev) < childCount {
		fr.rev = make([]stackEntry, childCount)
	} else {
		fr.rev = fr.rev[:childCount]
	}
	for i := range links {
		link := &links[i]
		score := link.score
		fr.rev[childCount-1] = link.subtree
		cur := link.prev
		for child := childCount - 2; child >= 0; child-- {
			next := &cur.links[0]
			fr.rev[child] = next.subtree
			score += next.score
			cur = next.prev
		}
		visit(fr.rev, score, cur, true)
	}
	return true
}

func (fr *forestReducer) reduceForkedLinearSinglePath(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 0 || node == nil || len(node.links) <= 1 {
		return false
	}
	maxPathLen := 0
	links := node.links
	for i := range links {
		pathLen, ok := validateLinearReducePathFromLink(&links[i], childCount)
		if !ok {
			return false
		}
		if pathLen > maxPathLen {
			maxPathLen = pathLen
		}
	}
	if cap(fr.path) < maxPathLen {
		fr.path = make([]stackEntry, 0, maxPathLen)
	}
	if cap(fr.rev) < maxPathLen {
		fr.rev = make([]stackEntry, maxPathLen)
	}
	for i := range links {
		fr.emitLinearReducePathFromLink(&links[i], childCount, visit)
	}
	return true
}

func validateLinearReducePathFromLink(link *gssLink, childCount int) (int, bool) {
	remaining := childCount
	pathLen := 0
	for {
		pathLen++
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				return pathLen, true
			}
		}
		cur := link.prev
		if cur == nil || len(cur.links) != 1 {
			return 0, false
		}
		link = &cur.links[0]
	}
}

func (fr *forestReducer) emitLinearReducePathFromLink(link *gssLink, childCount int, visit forestReduceVisitor) {
	fr.path = fr.path[:0]
	remaining := childCount
	score := 0
	for {
		fr.path = append(fr.path, link.subtree)
		score += link.score
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				fr.rev = fr.rev[:len(fr.path)]
				for i := range fr.path {
					fr.rev[len(fr.path)-1-i] = fr.path[i]
				}
				visit(fr.rev, score, link.prev, false)
				return
			}
		}
		link = &link.prev.links[0]
	}
}

func (fr *forestReducer) reduceLinearForkedSinglePath(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 0 || node == nil || len(node.links) != 1 {
		return false
	}
	fr.path = fr.path[:0]
	cur := node
	remaining := childCount
	prefixScore := 0
	for cur != nil && len(cur.links) == 1 {
		link := &cur.links[0]
		fr.path = append(fr.path, link.subtree)
		prefixScore += link.score
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				return false
			}
		}
		cur = link.prev
	}
	if cur == nil || len(cur.links) <= 1 {
		return false
	}
	prefixLen := len(fr.path)
	maxPathLen := prefixLen
	var branchLens [forestMaxLinksPerNode]int
	links := cur.links
	for i := range links {
		pathLen, ok := validateLinearReducePathFromLink(&links[i], remaining)
		if !ok {
			return false
		}
		branchLens[i] = pathLen
		if prefixLen+pathLen > maxPathLen {
			maxPathLen = prefixLen + pathLen
		}
	}
	if cap(fr.rev) < maxPathLen {
		fr.rev = make([]stackEntry, maxPathLen)
	}
	for i := range links {
		fr.emitLinearReducePathFromLinkWithPrefix(&links[i], remaining, branchLens[i], prefixLen, prefixScore, visit)
	}
	return true
}

func (fr *forestReducer) emitLinearReducePathFromLinkWithPrefix(link *gssLink, childCount int, branchPathLen, prefixLen int, score int, visit forestReduceVisitor) {
	totalLen := prefixLen + branchPathLen
	fr.rev = fr.rev[:totalLen]
	for i := 0; i < prefixLen; i++ {
		fr.rev[totalLen-1-i] = fr.path[i]
	}
	remaining := childCount
	branchOut := branchPathLen - 1
	for {
		fr.rev[branchOut] = link.subtree
		branchOut--
		score += link.score
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				visit(fr.rev, score, link.prev, false)
				return
			}
		}
		link = &link.prev.links[0]
	}
}

func (fr *forestReducer) reduceLinearSinglePath(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 0 {
		return false
	}
	fr.path = fr.path[:0]
	cur := node
	remaining := childCount
	score := 0
	for cur != nil {
		if len(cur.links) != 1 {
			return false
		}
		link := &cur.links[0]
		fr.path = append(fr.path, link.subtree)
		score += link.score
		if !forestStackEntryIsExtra(link.subtree) {
			remaining--
			if remaining == 0 {
				if cap(fr.rev) < len(fr.path) {
					fr.rev = make([]stackEntry, len(fr.path))
				} else {
					fr.rev = fr.rev[:len(fr.path)]
				}
				for i := range fr.path {
					fr.rev[len(fr.path)-1-i] = fr.path[i]
				}
				visit(fr.rev, score, link.prev, false)
				return true
			}
		}
		cur = link.prev
	}
	return false
}

func (fr *forestReducer) reduceNoExtrasDFS(node *gssForestNode, childCount int, visit forestReduceVisitor) bool {
	if childCount <= 0 || node == nil || int(node.noExtraDepth) < childCount {
		return false
	}
	if cap(fr.rev) < childCount {
		fr.rev = make([]stackEntry, childCount)
	} else {
		fr.rev = fr.rev[:childCount]
	}
	switch childCount {
	case 2:
		fr.dfsNoExtras2(node, 0, visit)
	case 3:
		fr.dfsNoExtras3(node, 0, visit)
	case 4:
		fr.dfsNoExtras4(node, 0, visit)
	default:
		fr.dfsNoExtras(node, childCount, 0, visit)
	}
	return true
}

func (fr *forestReducer) dfsNoExtras2(cur *gssForestNode, score int, visit forestReduceVisitor) {
	links0 := cur.links
	for i := range links0 {
		l0 := &links0[i]
		fr.rev[1] = l0.subtree
		score0 := score + l0.score
		n1 := l0.prev
		links1 := n1.links
		for j := range links1 {
			l1 := &links1[j]
			fr.rev[0] = l1.subtree
			visit(fr.rev, score0+l1.score, l1.prev, true)
		}
	}
}

func (fr *forestReducer) dfsNoExtras3(cur *gssForestNode, score int, visit forestReduceVisitor) {
	links0 := cur.links
	for i := range links0 {
		l0 := &links0[i]
		fr.rev[2] = l0.subtree
		score0 := score + l0.score
		n1 := l0.prev
		links1 := n1.links
		for j := range links1 {
			l1 := &links1[j]
			fr.rev[1] = l1.subtree
			score1 := score0 + l1.score
			n2 := l1.prev
			links2 := n2.links
			for k := range links2 {
				l2 := &links2[k]
				fr.rev[0] = l2.subtree
				visit(fr.rev, score1+l2.score, l2.prev, true)
			}
		}
	}
}

func (fr *forestReducer) dfsNoExtras4(cur *gssForestNode, score int, visit forestReduceVisitor) {
	links0 := cur.links
	for i := range links0 {
		l0 := &links0[i]
		fr.rev[3] = l0.subtree
		score0 := score + l0.score
		n1 := l0.prev
		links1 := n1.links
		for j := range links1 {
			l1 := &links1[j]
			fr.rev[2] = l1.subtree
			score1 := score0 + l1.score
			n2 := l1.prev
			links2 := n2.links
			for k := range links2 {
				l2 := &links2[k]
				fr.rev[1] = l2.subtree
				score2 := score1 + l2.score
				n3 := l2.prev
				links3 := n3.links
				for m := range links3 {
					l3 := &links3[m]
					fr.rev[0] = l3.subtree
					visit(fr.rev, score2+l3.score, l3.prev, true)
				}
			}
		}
	}
}

func (fr *forestReducer) dfsNoExtras(cur *gssForestNode, remaining, score int, visit forestReduceVisitor) {
	out := remaining - 1
	links := cur.links
	for i := range links {
		if fr.capped {
			break
		}
		if fr.steps++; fr.steps > forestReduceStepCap {
			fr.capped = true
			break
		}
		link := &links[i]
		fr.rev[out] = link.subtree
		nextScore := score + link.score
		if remaining == 1 {
			visit(fr.rev, nextScore, link.prev, true)
			continue
		}
		fr.dfsNoExtras(link.prev, remaining-1, nextScore, visit)
	}
}

func (fr *forestReducer) dfs(cur *gssForestNode, remaining, score int, visit forestReduceVisitor) {
	if cur == nil {
		return
	}
	mark := len(fr.path)
	links := cur.links
	for i := range links {
		if fr.capped {
			break
		}
		if fr.steps++; fr.steps > forestReduceStepCap {
			fr.capped = true
			break
		}
		link := &links[i]
		extra := forestStackEntryIsExtra(link.subtree)
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
			if cap(fr.rev) < len(fr.path) {
				fr.rev = make([]stackEntry, len(fr.path))
			} else {
				fr.rev = fr.rev[:len(fr.path)]
			}
			for j := range fr.path {
				fr.rev[len(fr.path)-1-j] = fr.path[j]
			}
			visit(fr.rev, score+link.score, link.prev, false)
			continue
		}
		fr.dfs(link.prev, rem, score+link.score, visit)
	}
	fr.path = fr.path[:mark]
}

func forestStackEntryIsExtra(e stackEntry) bool {
	return e.kind == stackEntryKindNode && e.node != nil && (*Node)(e.node).isExtra()
}
