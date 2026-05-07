package gotreesitter

import "testing"

func TestGLRMergeScratchResetInvalidatesEquivCacheByEpoch(t *testing.T) {
	var scratch glrMergeScratch
	scratch.beginEquivEpoch()

	a := &Node{symbol: 1, startByte: 1, endByte: 2, equivVersion: 1}
	b := &Node{symbol: 1, startByte: 1, endByte: 2, equivVersion: 1}
	storeNodeEquivCache(&scratch, a, b, 0, true)
	if got, ok := lookupNodeEquivCache(&scratch, a, b, 0); !ok || !got {
		t.Fatalf("lookupNodeEquivCache before reset = %v, %v; want true, true", got, ok)
	}

	before := scratch.equivEpoch
	scratch.reset()
	scratch.beginEquivEpoch()
	if scratch.equivEpoch == before {
		t.Fatalf("equiv epoch did not advance after reset: %d", before)
	}
	if _, ok := lookupNodeEquivCache(&scratch, a, b, 0); ok {
		t.Fatal("stale equivalence cache entry remained visible after reset")
	}
}

func TestGLREntryScratchResetClearsReservedWrittenRange(t *testing.T) {
	var scratch glrEntryScratch
	entries := scratch.allocWithCap(1, 8)
	node := &Node{symbol: 1}
	entries = entries[:cap(entries)]
	entries[len(entries)-1] = stackEntry{state: 7, node: node}

	scratch.reset()
	if len(scratch.slabs) == 0 {
		t.Fatal("expected retained entry slab")
	}
	for i, entry := range scratch.slabs[0].data[:8] {
		if entry.node != nil || entry.state != 0 {
			t.Fatalf("entry slab slot %d after reset = %#v, want zero", i, entry)
		}
	}
}

func TestGSSScratchResetClearsWrittenRange(t *testing.T) {
	var scratch gssScratch
	node := &Node{symbol: 1}
	stack := newGSSStack(1, &scratch)
	stack.push(2, node, &scratch)

	scratch.reset()
	if len(scratch.slabs) == 0 {
		t.Fatal("expected retained GSS slab")
	}
	for i, n := range scratch.slabs[0].data[:2] {
		if n.entry.node != nil || n.prev != nil || n.depth != 0 || n.hash != 0 {
			t.Fatalf("GSS slab slot %d after reset = %#v, want zero", i, n)
		}
	}
}
