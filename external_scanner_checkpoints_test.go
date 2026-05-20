package gotreesitter

import (
	"bytes"
	"testing"
)

func TestExternalScannerCheckpointPrimaryLookup(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	node := arena.allocNode()
	node.ownerArena = arena

	start := []byte{1, 2, 3}
	end := []byte{4, 5, 6}
	arena.recordExternalScannerLeafCheckpoint(node, start, end)

	start[0] = 9
	end[0] = 8

	got, ok := externalScannerCheckpointForNode(node)
	if !ok {
		t.Fatal("missing checkpoint for primary arena node")
	}
	if !bytes.Equal(got.start, []byte{1, 2, 3}) {
		t.Fatalf("primary start checkpoint = %v, want [1 2 3]", got.start)
	}
	if !bytes.Equal(got.end, []byte{4, 5, 6}) {
		t.Fatalf("primary end checkpoint = %v, want [4 5 6]", got.end)
	}
}

func TestExternalScannerCheckpointOverflowLookup(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	arena.nodes = make([]Node, 1)
	arena.recomputeAllocatedBytes()

	primary := arena.allocNode()
	primary.ownerArena = arena
	overflow := arena.allocNode()
	overflow.ownerArena = arena

	arena.recordExternalScannerLeafCheckpoint(overflow, []byte{7, 8}, []byte{9, 10})

	got, ok := externalScannerCheckpointForNode(overflow)
	if !ok {
		t.Fatal("missing checkpoint for overflow slab node")
	}
	if !bytes.Equal(got.start, []byte{7, 8}) {
		t.Fatalf("overflow start checkpoint = %v, want [7 8]", got.start)
	}
	if !bytes.Equal(got.end, []byte{9, 10}) {
		t.Fatalf("overflow end checkpoint = %v, want [9 10]", got.end)
	}
}

func TestExternalScannerCheckpointResetClearsSlot(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	node := arena.allocNode()
	node.ownerArena = arena
	arena.recordExternalScannerLeafCheckpoint(node, []byte{1}, []byte{2})

	arena.reset()

	reused := arena.allocNode()
	reused.ownerArena = arena
	if _, ok := externalScannerCheckpointForNode(reused); ok {
		t.Fatal("stale checkpoint remained visible after arena reset")
	}
}

func TestExternalScannerCheckpointStats(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	node := arena.allocNode()
	node.ownerArena = arena
	arena.recordExternalScannerLeafCheckpoint(node, []byte{1, 2, 3}, []byte{4, 5})

	if got, want := arena.externalScannerCheckpointRecords, uint64(1); got != want {
		t.Fatalf("checkpoint records = %d, want %d", got, want)
	}
	if got, want := arena.externalScannerSnapshotPayloadBytes, uint64(5); got != want {
		t.Fatalf("snapshot bytes = %d, want %d", got, want)
	}
	if got := arena.externalScannerCheckpointSlotsAllocated(); got == 0 {
		t.Fatal("checkpoint slots allocated = 0, want non-zero")
	}
	if got := arena.externalScannerCheckpointBytesAllocated(); got == 0 {
		t.Fatal("checkpoint bytes allocated = 0, want non-zero")
	}

	arena.reset()
	if got := arena.externalScannerCheckpointRecords; got != 0 {
		t.Fatalf("checkpoint records after reset = %d, want 0", got)
	}
	if got := arena.externalScannerSnapshotPayloadBytes; got != 0 {
		t.Fatalf("snapshot bytes after reset = %d, want 0", got)
	}
}

func TestExternalScannerCheckpointPreallocatesSparseSet(t *testing.T) {
	arena := newNodeArena(arenaClassFull)

	const checkpointSlots = 8
	before := arena.allocatedBytes
	arena.ensureExternalScannerCheckpointCapacity(checkpointSlots)
	if got := arena.externalScannerCheckpointSlotsAllocated(); got != checkpointSlots {
		t.Fatalf("checkpoint slots = %d, want %d", got, checkpointSlots)
	}
	if got := arena.allocatedBytes - before; got != arena.externalScannerCheckpointBytesAllocated() {
		t.Fatalf("allocated byte delta = %d, want checkpoint bytes %d", got, arena.externalScannerCheckpointBytesAllocated())
	}

	node := arena.allocNode()
	node.ownerArena = arena
	recordCheckpointBytesBefore := arena.externalScannerCheckpointBytesAllocated()
	arena.recordExternalScannerLeafCheckpoint(node, []byte{1}, []byte{2})
	if got := arena.externalScannerCheckpointBytesAllocated() - recordCheckpointBytesBefore; got != 0 {
		t.Fatalf("checkpoint sparse-set bytes grew by %d, want 0 with preallocated slots", got)
	}
}

func TestExternalScannerCheckpointReusesEqualStartEndSnapshot(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	node := arena.allocNode()
	node.ownerArena = arena
	arena.recordExternalScannerLeafCheckpoint(node, []byte{1, 2, 3}, []byte{1, 2, 3})

	if got, want := arena.externalScannerSnapshotPayloadBytes, uint64(3); got != want {
		t.Fatalf("snapshot bytes = %d, want %d", got, want)
	}
	got, ok := externalScannerCheckpointForNode(node)
	if !ok {
		t.Fatal("missing checkpoint")
	}
	if !bytes.Equal(got.start, []byte{1, 2, 3}) || !bytes.Equal(got.end, []byte{1, 2, 3}) {
		t.Fatalf("checkpoint = (%v, %v), want ([1 2 3], [1 2 3])", got.start, got.end)
	}
}

func TestExternalScannerCheckpointReusesConsecutiveSnapshots(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	first := arena.allocNode()
	first.ownerArena = arena
	second := arena.allocNode()
	second.ownerArena = arena
	snapshot := []byte{1, 2, 3}

	arena.recordExternalScannerLeafCheckpoint(first, snapshot, snapshot)
	arena.recordExternalScannerLeafCheckpoint(second, snapshot, snapshot)

	if got, want := arena.externalScannerSnapshotPayloadBytes, uint64(3); got != want {
		t.Fatalf("snapshot bytes = %d, want %d", got, want)
	}
	if got, ok := externalScannerCheckpointForNode(second); !ok || !bytes.Equal(got.start, snapshot) || !bytes.Equal(got.end, snapshot) {
		t.Fatalf("second checkpoint = (%v, %v, %v), want snapshot", got.start, got.end, ok)
	}
}

func TestExternalScannerCheckpointSparseLookupHandlesOutOfOrderWrites(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	first := arena.allocNode()
	first.ownerArena = arena
	second := arena.allocNode()
	second.ownerArena = arena

	arena.recordExternalScannerLeafCheckpoint(second, []byte{2}, []byte{3})
	arena.recordExternalScannerLeafCheckpoint(first, []byte{1}, []byte{4})

	gotFirst, ok := externalScannerCheckpointForNode(first)
	if !ok || !bytes.Equal(gotFirst.start, []byte{1}) || !bytes.Equal(gotFirst.end, []byte{4}) {
		t.Fatalf("first checkpoint = (%v, %v, %v), want ([1], [4], true)", gotFirst.start, gotFirst.end, ok)
	}
	gotSecond, ok := externalScannerCheckpointForNode(second)
	if !ok || !bytes.Equal(gotSecond.start, []byte{2}) || !bytes.Equal(gotSecond.end, []byte{3}) {
		t.Fatalf("second checkpoint = (%v, %v, %v), want ([2], [3], true)", gotSecond.start, gotSecond.end, ok)
	}
}

func TestRebuildExternalScannerCheckpointsUsesLazyFinalChildRefs(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	leftCP := arena.recordExternalScannerCompactCheckpoint([]byte{1}, []byte{2})
	rightCP := arena.recordExternalScannerCompactCheckpoint([]byte{3}, []byte{4})
	left := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	left.parseState = 11
	left.checkpoint = leftCP
	left.hasCheckpoint = true
	right := newCompactFullLeafInArena(arena, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	right.parseState = 12
	right.checkpoint = rightCP
	right.hasCheckpoint = true
	inner := newPendingParentInArena(arena, 2, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(left.parseState, left),
		newStackEntryCompactFullLeaf(right.parseState, right),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	inner.parseState = 13
	outer := newPendingParentInArena(arena, 3, true, 5, []stackEntry{
		newStackEntryPendingParent(inner.parseState, inner),
	}, 0, 2, Point{}, Point{Column: 2}, false)
	outer.parseState = 14

	entry := newStackEntryPendingParent(outer.parseState, outer)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)
	if root == nil {
		t.Fatal("root = nil")
	}
	rebuildExternalScannerCheckpoints(root, &Language{Name: "python", ExternalScanner: parserTestSafeExternalScanner{}})

	got, ok := externalScannerCheckpointForNode(root)
	if !ok {
		t.Fatal("missing rebuilt checkpoint for lazy final-child parent")
	}
	if !bytes.Equal(got.start, []byte{1}) || !bytes.Equal(got.end, []byte{4}) {
		t.Fatalf("checkpoint = (%v, %v), want ([1], [4])", got.start, got.end)
	}
	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children materialized = %d, want 0", got)
	}
}

func TestEditMaterializedPendingParentRebuildsExternalScannerCheckpoint(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	cp := arena.recordExternalScannerCompactCheckpoint([]byte{5}, []byte{6})
	leaf := newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	leaf.parseState = 11
	leaf.checkpoint = cp
	leaf.hasCheckpoint = true
	parent := newPendingParentInArena(arena, 2, true, 4, []stackEntry{
		newStackEntryCompactFullLeaf(leaf.parseState, leaf),
	}, 0, 1, Point{}, Point{Column: 1}, false)
	parent.parseState = 12

	entry := newStackEntryPendingParent(parent.parseState, parent)
	node := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForEdit)
	if node == nil {
		t.Fatal("node = nil")
	}
	got, ok := externalScannerCheckpointForNode(node)
	if !ok {
		t.Fatal("missing checkpoint for edit-materialized pending parent")
	}
	if !bytes.Equal(got.start, []byte{5}) || !bytes.Equal(got.end, []byte{6}) {
		t.Fatalf("checkpoint = (%v, %v), want ([5], [6])", got.start, got.end)
	}
}
