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
