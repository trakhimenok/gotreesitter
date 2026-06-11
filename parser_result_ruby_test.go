package gotreesitter

import "testing"

// rubyThenLang builds a minimal ruby-named Language whose symbol table contains
// just the node kinds the then-start normalizer inspects.
func rubyThenLang() *Language {
	return &Language{
		Name:        "ruby",
		SymbolNames: []string{"EOF", "rescue", "exception_variable", "then", "call"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "rescue", Visible: true, Named: true},
			{Name: "exception_variable", Visible: true, Named: true},
			{Name: "then", Visible: true, Named: true},
			{Name: "call", Visible: true, Named: true},
		},
	}
}

// TestNormalizeRubyThenStartsExtendsRescueBody mirrors the C oracle: inside a
// rescue clause the `then` body node starts at the previous sibling's end byte
// (covering the leading indentation extras), not at its first child.
func TestNormalizeRubyThenStartsExtendsRescueBody(t *testing.T) {
	lang := rubyThenLang()
	arena := newNodeArena(arenaClassFull)

	// rescue <exception_variable [1423:1427]> <then [1438:1565]>
	excVar := newLeafNodeInArena(arena, 2, true, 1423, 1427, Point{Row: 10, Column: 6}, Point{Row: 10, Column: 10})
	bodyCall := newLeafNodeInArena(arena, 4, true, 1438, 1502, Point{Row: 11, Column: 10}, Point{Row: 11, Column: 74})
	thenNode := newParentNodeInArena(arena, 3, true, []*Node{bodyCall}, nil, 0)
	thenNode.startByte = 1438
	thenNode.endByte = 1565
	thenNode.startPoint = Point{Row: 11, Column: 10}
	rescueNode := newParentNodeInArena(arena, 1, true, []*Node{excVar, thenNode}, nil, 0)

	normalizeRubyThenStarts(rescueNode, lang)

	if got, want := thenNode.StartByte(), uint32(1427); got != want {
		t.Fatalf("then.StartByte() = %d, want %d (prev exception_variable end)", got, want)
	}
	if got, want := thenNode.startPoint, excVar.endPoint; got != want {
		t.Fatalf("then.startPoint = %v, want %v", got, want)
	}
	// End byte and children must be untouched.
	if got, want := thenNode.EndByte(), uint32(1565); got != want {
		t.Fatalf("then.EndByte() = %d, want %d", got, want)
	}
	if got, want := resultChildCount(thenNode), 1; got != want {
		t.Fatalf("then child count = %d, want %d", got, want)
	}
}

// TestNormalizeRubyThenStartsLeavesNonAdjacentBodies is a guard: when the then
// node already starts at or before the previous sibling's end, nothing moves.
func TestNormalizeRubyThenStartsLeavesAdjacent(t *testing.T) {
	lang := rubyThenLang()
	arena := newNodeArena(arenaClassFull)

	excVar := newLeafNodeInArena(arena, 2, true, 1423, 1427, Point{}, Point{Row: 0, Column: 1427})
	bodyCall := newLeafNodeInArena(arena, 4, true, 1427, 1502, Point{}, Point{})
	thenNode := newParentNodeInArena(arena, 3, true, []*Node{bodyCall}, nil, 0)
	thenNode.startByte = 1427
	thenNode.endByte = 1565
	rescueNode := newParentNodeInArena(arena, 1, true, []*Node{excVar, thenNode}, nil, 0)

	normalizeRubyThenStarts(rescueNode, lang)

	if got, want := thenNode.StartByte(), uint32(1427); got != want {
		t.Fatalf("then.StartByte() = %d, want %d (already adjacent, unchanged)", got, want)
	}
}
