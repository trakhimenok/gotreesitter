package gotreesitter

import "testing"

func TestTransientChildScratchMaterializesReachableNode(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch transientChildScratch
	first := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, Symbol(2), true, 1, 2, Point{Column: 1}, Point{Column: 2})
	parent := newParentNodeInArenaNoLinksWithFieldSources(arena, Symbol(3), true, []*Node{}, nil, nil, 0, true)

	children := scratch.alloc(2)
	children[0] = first
	children[1] = second
	parent.children = children

	if !scratch.owns(parent.children) {
		t.Fatal("expected parent children to use transient storage before materialization")
	}

	var stack []*Node
	scratch.materializeNode(parent, arena, &stack)

	if scratch.owns(parent.children) {
		t.Fatal("expected parent children to be copied into arena storage")
	}
	if len(parent.children) != 2 || parent.children[0] != first || parent.children[1] != second {
		t.Fatalf("materialized children = %#v, want [%p %p]", parent.children, first, second)
	}

	scratch.reset()
	if len(parent.children) != 2 || parent.children[0] != first || parent.children[1] != second {
		t.Fatal("materialized children were invalidated by transient reset")
	}
}

func TestTransientParentScratchMaterializesReachableParent(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var childScratch transientChildScratch
	var parentScratch transientParentScratch
	first := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, Symbol(2), true, 1, 2, Point{Column: 1}, Point{Column: 2})
	children := childScratch.alloc(2)
	children[0] = first
	children[1] = second
	parent := parentScratch.allocParent(arena, Symbol(3), true, children, 11, true)
	parent.parseState = 7
	parent.preGotoState = 5

	if !parentScratch.owns(parent) {
		t.Fatal("expected parent to use transient parent storage before materialization")
	}
	if !childScratch.owns(parent.children) {
		t.Fatal("expected parent children to use transient child storage before materialization")
	}

	entries := []stackEntry{newStackEntryNode(parent.parseState, parent)}
	parentScratch.materializeEntries(entries, arena, &childScratch)

	got := stackEntryNode(entries[0])
	if got == nil {
		t.Fatal("materialized entry node = nil")
	}
	if parentScratch.owns(got) {
		t.Fatal("entry still points at transient parent after materialization")
	}
	if childScratch.owns(got.children) {
		t.Fatal("materialized parent still owns transient children")
	}
	if len(got.children) != 2 || got.children[0] != first || got.children[1] != second {
		t.Fatalf("materialized children = %#v, want [%p %p]", got.children, first, second)
	}
	if got.parseState != 7 || got.preGotoState != 5 || got.productionID != 11 {
		t.Fatalf("materialized states = (%d,%d,%d), want (7,5,11)", got.parseState, got.preGotoState, got.productionID)
	}
	if got.StartByte() != 0 || got.EndByte() != 2 {
		t.Fatalf("materialized span = [%d,%d], want [0,2]", got.StartByte(), got.EndByte())
	}
	if got := parentScratch.nodesMaterialized; got != 1 {
		t.Fatalf("nodesMaterialized = %d, want 1", got)
	}

	parentScratch.reset()
	childScratch.reset()
	if len(got.children) != 2 || got.children[0] != first || got.children[1] != second {
		t.Fatal("materialized parent was invalidated by scratch reset")
	}
}

func TestTransientChildScratchMaterializesFieldedArenaParent(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var scratch transientChildScratch
	first := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, Symbol(2), true, 1, 2, Point{Column: 1}, Point{Column: 2})
	children := scratch.alloc(2)
	children[0] = first
	children[1] = second
	fieldIDs := arena.allocFieldIDSlice(2)
	fieldSources := arena.allocFieldSourceSlice(2)
	fieldIDs[0] = 7
	fieldSources[0] = fieldSourceDirect
	parent := newParentNodeInArenaNoLinksWithFieldSources(arena, Symbol(3), true, children, fieldIDs, fieldSources, 0, true)

	var stack []*Node
	scratch.materializeNode(parent, arena, &stack)

	if scratch.owns(parent.children) {
		t.Fatal("fielded parent still owns transient children")
	}
	if len(parent.children) != 2 || parent.children[0] != first || parent.children[1] != second {
		t.Fatalf("materialized children = %#v, want [%p %p]", parent.children, first, second)
	}
	if len(parent.fieldIDs) != 2 || parent.fieldIDs[0] != 7 {
		t.Fatalf("field IDs = %#v, want first field 7", parent.fieldIDs)
	}
	if len(parent.fieldSources) != 2 || parent.fieldSources[0] != fieldSourceDirect {
		t.Fatalf("field sources = %#v, want first direct", parent.fieldSources)
	}

	scratch.reset()
	if len(parent.children) != 2 || parent.children[0] != first || parent.children[1] != second {
		t.Fatal("fielded parent children were invalidated by transient reset")
	}
}

func TestTransientParentScratchMaterializesRecoveredNodeSlice(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	var childScratch transientChildScratch
	var parentScratch transientParentScratch
	first := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	second := newLeafNodeInArena(arena, Symbol(2), true, 1, 2, Point{Column: 1}, Point{Column: 2})
	children := childScratch.alloc(2)
	children[0] = first
	children[1] = second
	parent := parentScratch.allocParent(arena, Symbol(3), true, children, 17, true)
	nodes := []*Node{parent}

	materializeTransientParentNodes(nodes, arena, &parentScratch, &childScratch)

	got := nodes[0]
	if got == nil {
		t.Fatal("materialized node = nil")
	}
	if parentScratch.owns(got) {
		t.Fatal("node slice still points at transient parent")
	}
	if childScratch.owns(got.children) {
		t.Fatal("materialized recovered parent still owns transient children")
	}
	if len(got.children) != 2 || got.children[0] != first || got.children[1] != second {
		t.Fatalf("materialized children = %#v, want [%p %p]", got.children, first, second)
	}
	if got.productionID != 17 {
		t.Fatalf("productionID = %d, want 17", got.productionID)
	}

	parentScratch.reset()
	childScratch.reset()
	if len(got.children) != 2 || got.children[0] != first || got.children[1] != second {
		t.Fatal("recovered materialized parent was invalidated by scratch reset")
	}
}
