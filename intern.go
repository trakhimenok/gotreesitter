package gotreesitter

import (
	"unsafe"
)

// Phase 1 scaffolding for the GLR node interning initiative
// (initiative.glr-node-interning in hyphae://m31labs/gotreesitter).
//
// Scope of this file:
//   - InternTable type + reset/teardown lifecycle hooks
//   - Hash function for (symbol, child pointers, span, flags, productionID)
//   - Lookup/store API, designed for the reduce path but NOT YET wired in
//   - Counters for hit/miss/store so future phases have a baseline
//
// What this is NOT (yet):
//   - Not integrated into the reduce path. Calling parser.Parse does not
//     populate or query this table. Phase 2 will wire leaf interning;
//     Phase 3 extends to parents; Phase 4 plugs into glr_merge.
//   - Not a replacement for the 2-way set-associative equiv cache in glr.go.
//     Those primitives stay in place until Phase 4 demonstrates the intern
//     table subsumes them.
//
// Concurrency:
//   - One table per parser instance. Not safe for concurrent Parse() calls
//     on the same Parser. (The runtime already isolates parsers via
//     ParserPool for concurrent use.)
//
// Lifecycle:
//   - Phase 1 invariant: the table is created at parse start and dropped
//     before post-parse normalizers run. This sidesteps the mutation
//     barrier — normalizers mutate Node.children freely; the intern table
//     is gone by then.

// internKey is the lookup key for a single node shape. Keep the struct
// tight: every reduce traverses this struct hot.
type internKey struct {
	// symbol identifies the node's grammar symbol. uint16 in the runtime;
	// widened to uint32 here so the struct lays out cleanly without
	// padding holes between symbol and the pointer-equivalent fields.
	symbol uint32
	// productionID disambiguates two reductions for the same symbol
	// that produced different shapes (e.g. different rule alternatives).
	productionID uint16
	// flags captures isNamed/isExtra/hasError/isMissing in the same byte
	// layout as Node.flags. Two nodes with the same shape but different
	// flags MUST hash to different buckets.
	flags uint8
	// childCount is duplicated from len(childrenHash) to allow rejection
	// without indexing the slice.
	childCount uint8
	// startByte and endByte pin the source span. Identical shapes at
	// different file positions are not interchangeable — consumers use
	// startByte for position queries.
	startByte uint32
	endByte   uint32
	// childrenHash is a Bob Jenkins-style mix of the child pointer
	// values. The pointers themselves live in the table's separate
	// children-pointer arena; we compare them on hash collision.
	childrenHash uint64
}

// internEntry holds one intern table entry. The pointer comparison on
// lookup is cheap; the canonical *Node lives in the parser's main arena
// so this struct does not need to own it.
type internEntry struct {
	key  internKey
	node *Node
}

// internTable is the per-parse intern table. Phase 1 uses a flat
// open-addressed hash table; the table is cleared (length zeroed, slots
// not freed) at parse start and discarded post-parse.
type internTable struct {
	// entries is the open-addressed bucket array. Capacity is power-of-2;
	// occupancy is bounded by maxLoadFactor before growing.
	entries []internEntry
	// occupied is the count of non-empty slots, used to trigger growth.
	occupied int
	// lookups/hits/misses/stores are observability counters. Future
	// phases will wire these into runtimeAudit; Phase 1 keeps them
	// local so the file builds standalone.
	lookups uint64
	hits    uint64
	misses  uint64
	stores  uint64
	// growths counts table resizes — a non-zero value during Phase 2
	// real-corpus runs is a signal to bump initial capacity.
	growths uint64
}

// internTableInitialCap is the starting bucket count. 4096 is enough
// for the JS bench (~700k nodes across 3 files, hit rate target 30%+
// means ~200k canonical shapes; one parse iteration covers ~140k of
// those, fits well above 50% load in a 4K table after one growth).
// Tuned with Phase 2 measurements.
const internTableInitialCap = 4096

// internTableMaxLoadFactor governs when to grow. 0.7 trades memory for
// lookup speed; tighter than the 0.75 default to keep probe sequences
// short on a hot inner loop.
const internTableMaxLoadFactor = 0.7

// newInternTable allocates a fresh table with the initial capacity.
// Caller is responsible for calling reset() between parses.
func newInternTable() *internTable {
	return &internTable{
		entries: make([]internEntry, internTableInitialCap),
	}
}

// reset clears entries for reuse on the next parse. Counters are kept;
// callers can read or zero them as needed.
func (t *internTable) reset() {
	if t == nil {
		return
	}
	for i := range t.entries {
		t.entries[i] = internEntry{}
	}
	t.occupied = 0
}

// hashKey returns a 64-bit hash of the key. The mixing constants are
// xxHash-style; this is not cryptographic — only collision quality
// matters. Stable across runs (no per-process salt yet; revisit if a
// future spore proposes salting to harden against adversarial input).
func hashKey(k internKey) uint64 {
	const (
		prime1 = 0x9e3779b185ebca87
		prime2 = 0xc2b2ae3d27d4eb4f
		prime3 = 0x165667b19e3779f9
	)
	h := uint64(k.symbol) * prime1
	h ^= uint64(k.productionID)<<16 | uint64(k.flags)<<8 | uint64(k.childCount)
	h *= prime2
	h ^= uint64(k.startByte)<<32 | uint64(k.endByte)
	h *= prime3
	h ^= k.childrenHash
	// Final avalanche.
	h ^= h >> 33
	h *= prime2
	h ^= h >> 29
	h *= prime3
	h ^= h >> 32
	return h
}

// hashChildren combines a child pointer slice into the childrenHash
// component of internKey. Position-sensitive: (a, b) and (b, a) hash
// differently. Uses pointer addresses, so cross-parse hashes are NOT
// stable (intentional — the intern table is per-parse).
func hashChildren(children []*Node) uint64 {
	if len(children) == 0 {
		return 0
	}
	var h uint64 = 0xcbf29ce484222325
	for _, c := range children {
		p := uintptr(unsafe.Pointer(c))
		h ^= uint64(p)
		h *= 0x100000001b3
	}
	return h
}

// buildKey constructs an internKey from a node's identifying fields.
// Helper for callers; safe to inline at hot sites if profile demands.
func buildKey(symbol Symbol, productionID uint16, flags nodeFlags, startByte, endByte uint32, children []*Node) internKey {
	return internKey{
		symbol:       uint32(symbol),
		productionID: productionID,
		flags:        uint8(flags),
		childCount:   uint8(len(children)),
		startByte:    startByte,
		endByte:      endByte,
		childrenHash: hashChildren(children),
	}
}

// lookup returns the canonical node for the given key, or nil if absent.
// Caller must also pass the child slice for collision verification (two
// distinct child slices could in principle hash equal).
func (t *internTable) lookup(key internKey, children []*Node) *Node {
	if t == nil || len(t.entries) == 0 {
		return nil
	}
	t.lookups++
	mask := uint64(len(t.entries) - 1)
	h := hashKey(key)
	for probe := uint64(0); probe < uint64(len(t.entries)); probe++ {
		idx := (h + probe) & mask
		entry := &t.entries[idx]
		if entry.node == nil {
			t.misses++
			return nil
		}
		if entry.key == key && childrenSliceEq(entry.node, children) {
			t.hits++
			return entry.node
		}
	}
	t.misses++
	return nil
}

// store inserts node under the given key. If a slot is occupied with a
// matching key, the existing node is preserved (first-wins) — callers
// should look up before allocating.
func (t *internTable) store(key internKey, node *Node) {
	if t == nil || node == nil {
		return
	}
	if float64(t.occupied+1)/float64(len(t.entries)) > internTableMaxLoadFactor {
		t.grow()
	}
	mask := uint64(len(t.entries) - 1)
	h := hashKey(key)
	for probe := uint64(0); probe < uint64(len(t.entries)); probe++ {
		idx := (h + probe) & mask
		entry := &t.entries[idx]
		if entry.node == nil {
			entry.key = key
			entry.node = node
			t.occupied++
			t.stores++
			return
		}
		if entry.key == key {
			// First-wins. Phase 2 may revisit to assert the equal-key
			// node is also pointer-equal to the candidate; for now we
			// silently dedup.
			return
		}
	}
}

// grow doubles the table capacity and re-inserts existing entries.
// Called from store() when the load factor exceeds the bound.
func (t *internTable) grow() {
	oldEntries := t.entries
	t.entries = make([]internEntry, len(oldEntries)*2)
	t.occupied = 0
	t.growths++
	for _, e := range oldEntries {
		if e.node == nil {
			continue
		}
		// Re-insert without going through store() (which would re-check
		// the load factor; we already grew). Open-addressed probe.
		mask := uint64(len(t.entries) - 1)
		h := hashKey(e.key)
		for probe := uint64(0); probe < uint64(len(t.entries)); probe++ {
			idx := (h + probe) & mask
			if t.entries[idx].node == nil {
				t.entries[idx] = e
				t.occupied++
				break
			}
		}
	}
}

// childrenSliceEq verifies that node n has exactly the same child
// pointers as the candidate slice. Used to resolve hash collisions on
// lookup. Compares pointer identity, not deep equality — that's the
// whole point of interning.
func childrenSliceEq(n *Node, candidate []*Node) bool {
	if n == nil {
		return len(candidate) == 0
	}
	if len(n.children) != len(candidate) {
		return false
	}
	for i, c := range candidate {
		if n.children[i] != c {
			return false
		}
	}
	return true
}

// internStats is the observability snapshot exported for tests and
// future runtime_audit integration.
type internStats struct {
	Lookups  uint64
	Hits     uint64
	Misses   uint64
	Stores   uint64
	Growths  uint64
	Occupied int
	Capacity int
}

// stats returns the current counters. Safe to call between parses.
func (t *internTable) stats() internStats {
	if t == nil {
		return internStats{}
	}
	return internStats{
		Lookups:  t.lookups,
		Hits:     t.hits,
		Misses:   t.misses,
		Stores:   t.stores,
		Growths:  t.growths,
		Occupied: t.occupied,
		Capacity: len(t.entries),
	}
}
