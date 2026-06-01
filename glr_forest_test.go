package gotreesitter

import (
	"fmt"
	"testing"
)

// pathsOf reduces childCount children over node and returns each visited path as
// "child-states|popToState" so order and fan-out are easy to assert.
func pathsOf(node *gssForestNode, childCount int) []string {
	var out []string
	reduceOverForest(node, childCount, func(children []stackEntry, _ int, popTo *gssForestNode) {
		states := make([]uint32, len(children))
		for i, c := range children {
			states[i] = uint32(c.state)
		}
		out = append(out, fmt.Sprintf("%v|%d", states, popTo.state))
	})
	return out
}

func TestReduceOverForestLinearChain(t *testing.T) {
	// n0 <-(a:10)- n1 <-(b:11)- n2 <-(c:12)- n3
	n0 := &gssForestNode{state: 0, byteOffset: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{{prev: n0, subtree: stackEntry{state: 10}}}}
	n2 := &gssForestNode{state: 2, links: []gssLink{{prev: n1, subtree: stackEntry{state: 11}}}}
	n3 := &gssForestNode{state: 3, links: []gssLink{{prev: n2, subtree: stackEntry{state: 12}}}}

	// reduce 2 children over n3 → [b,c] = [11 12], pop back to n1.
	got := pathsOf(n3, 2)
	want := []string{"[11 12]|1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("childCount=2: got %v want %v", got, want)
	}
	// reduce 0 children → empty, pop to n3 itself.
	if got := pathsOf(n3, 0); fmt.Sprint(got) != "[[]|3]" {
		t.Fatalf("childCount=0: got %v", got)
	}
	// reduce all 3 → [a b c] = [10 11 12], pop to n0.
	if got := pathsOf(n3, 3); fmt.Sprint(got) != "[[10 11 12]|0]" {
		t.Fatalf("childCount=3: got %v", got)
	}
}

func TestReduceOverForestLinearChainWithExtra(t *testing.T) {
	// n0 <-(a:10)- n1 <-(b:11)- n2 <-(extra:90)- n3 <-(c:12)- n4
	extra := &Node{}
	extra.setExtra(true)
	n0 := &gssForestNode{state: 0, byteOffset: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{{prev: n0, subtree: stackEntry{state: 10}}}}
	n2 := &gssForestNode{state: 2, links: []gssLink{{prev: n1, subtree: stackEntry{state: 11}}}}
	n3 := &gssForestNode{state: 3, links: []gssLink{{prev: n2, subtree: newStackEntryNode(90, extra)}}}
	n4 := &gssForestNode{state: 4, links: []gssLink{{prev: n3, subtree: stackEntry{state: 12}}}}

	// Extras are included in the reduce window but do not count toward childCount.
	got := pathsOf(n4, 2)
	want := []string{"[11 90 12]|1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("childCount=2 with extra: got %v want %v", got, want)
	}
}

func TestReduceOverForestForkedNode(t *testing.T) {
	// Shared base n0 <-(a:10)- n1, then two alternatives reaching a coalesced n3:
	//   path A: n1 <-(b:11)- n2  <-(c:12)- n3
	//   path B: n1 <-(x:21)- n2a <-(y:22)- n3
	n0 := &gssForestNode{state: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{{prev: n0, subtree: stackEntry{state: 10}}}}
	n2 := &gssForestNode{state: 2, links: []gssLink{{prev: n1, subtree: stackEntry{state: 11}}}}
	n2a := &gssForestNode{state: 20, links: []gssLink{{prev: n1, subtree: stackEntry{state: 21}}}}
	n3 := &gssForestNode{state: 3, links: []gssLink{
		{prev: n2, subtree: stackEntry{state: 12}},
		{prev: n2a, subtree: stackEntry{state: 22}},
	}}

	// reduce 2 children over the coalesced n3 → BOTH alternatives, each popping to n1.
	got := pathsOf(n3, 2)
	want := []string{"[11 12]|1", "[21 22]|1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("forked childCount=2: got %v want %v", got, want)
	}
	if got, want := pathsOf(n3, 1), []string{"[12]|2", "[22]|20"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("forked childCount=1: got %v want %v", got, want)
	}
}

// TestCoalesceForestSharesNode proves coalesceForest dedups by (state,byteOffset)
// into one node with multiple links — the O(1), no-deep-compare mechanism.
func TestCoalesceForestSharesNode(t *testing.T) {
	idx := newGSSForestIndex(0)
	slab := &gssForestNodeSlab{}
	base := &gssForestNode{state: 0}
	// Two distinct parses reach (state=5, byteOffset=42).
	a := coalesceForest(&idx, slab, 5, 42, base, stackEntry{state: 100}, 3, 0)
	b := coalesceForest(&idx, slab, 5, 42, base, stackEntry{state: 101}, 7, 0)
	if a != b {
		t.Fatal("coalesceForest created two nodes for the same (state,byteOffset)")
	}
	if len(a.links) != 2 {
		t.Fatalf("want 2 links on the coalesced node, got %d", len(a.links))
	}
	// Higher score wins the node-level disambiguator (lower error cost first).
	if a.score != 7 {
		t.Fatalf("want node score 7 (the better of 3/7), got %d", a.score)
	}
	// A different (state,byteOffset) is a separate node.
	c := coalesceForest(&idx, slab, 6, 42, base, stackEntry{state: 102}, 1, 0)
	if c == a {
		t.Fatal("distinct (state,byteOffset) coalesced into the same node")
	}
}

func TestCoalesceForestMarksDirtyWhenPredecessorChanges(t *testing.T) {
	idx := newGSSForestIndex(0)
	slab := &gssForestNodeSlab{}
	prev := &gssForestNode{state: 1, dirty: 1}
	entry := newStackEntryNode(2, &Node{symbol: 7, startByte: 10, endByte: 20})

	top := coalesceForest(&idx, slab, 5, 20, prev, entry, 0, 0)
	initialDirty := top.dirty
	initialLinks := len(top.links)

	prev.dirty++
	again := coalesceForest(&idx, slab, 5, 20, prev, entry, 0, 0)
	if again != top {
		t.Fatal("same link reached a different coalesced node")
	}
	if len(top.links) != initialLinks {
		t.Fatalf("duplicate link appended: got %d links, want %d", len(top.links), initialLinks)
	}
	if top.dirty <= initialDirty {
		t.Fatalf("coalesced node dirty=%d, want > %d after predecessor changed", top.dirty, initialDirty)
	}
}

func TestGSSForestNodeSlabReleaseClearsPointers(t *testing.T) {
	slab := &gssForestNodeSlab{}
	base := slab.alloc(1, 0, 0, 0)
	node := slab.alloc(2, 1, 0, 0)
	nodeLinkStart := slab.linkIdx - forestMaxLinksPerNode
	node.links = append(node.links, gssLink{
		prev:    base,
		subtree: stackEntry{state: 3},
	})

	if len(slab.nodeBatches) == 0 || len(slab.linkBatches) == 0 {
		t.Fatal("expected slab batches to be allocated")
	}
	if got := slab.linkBatches[0][nodeLinkStart].prev; got != base {
		t.Fatalf("test setup failed: link batch prev = %p, want %p", got, base)
	}
	slab.resetForRelease()
	if got := slab.nodeBatches[0][0].links; got != nil {
		t.Fatalf("node batch retained stale links slice: %v", got)
	}
	if got := slab.linkBatches[0][nodeLinkStart].prev; got != nil {
		t.Fatalf("link batch retained stale prev pointer: %p", got)
	}
}
