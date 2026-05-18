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
