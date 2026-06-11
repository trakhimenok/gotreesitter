package gotreesitter

import "testing"

func TestNormalizeCPONDocumentLeadingTriviaStart(t *testing.T) {
	lang := &Language{
		Name:        "cpon",
		SymbolNames: []string{"EOF", "document", "map"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "document", Visible: true, Named: true},
			{Name: "map", Visible: true, Named: true},
		},
	}
	source := []byte("\n<{}")
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, true, 1, 4, Point{Row: 1}, Point{Row: 1, Column: 3})
	root := newParentNodeInArena(arena, 1, true, []*Node{child}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}
	root.endByte = uint32(len(source))
	root.endPoint = Point{Row: 1, Column: 3}

	normalizeCPONCompatibility(root, source, lang)

	if got, want := root.StartByte(), uint32(1); got != want {
		t.Fatalf("root start byte = %d, want %d", got, want)
	}
	if got, want := root.StartPoint(), (Point{Row: 1}); got != want {
		t.Fatalf("root start point = %+v, want %+v", got, want)
	}
}

func TestNormalizeCPONDocumentLeadingTriviaStartRejectsNonTrivia(t *testing.T) {
	lang := &Language{
		Name:        "cpon",
		SymbolNames: []string{"EOF", "document", "map"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "document", Visible: true, Named: true},
			{Name: "map", Visible: true, Named: true},
		},
	}
	source := []byte("#<{}")
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, true, 1, 4, Point{Column: 1}, Point{Column: 4})
	root := newParentNodeInArena(arena, 1, true, []*Node{child}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}

	normalizeCPONCompatibility(root, source, lang)

	if got, want := root.StartByte(), uint32(0); got != want {
		t.Fatalf("root start byte = %d, want %d", got, want)
	}
}

func TestNormalizeCPONBooleanLiteralsRestoreTokenChildren(t *testing.T) {
	lang := &Language{
		Name:        "cpon",
		SymbolNames: []string{"EOF", "document", "boolean", "true", "false"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "document", Visible: true, Named: true},
			{Name: "boolean", Visible: true, Named: true},
			{Name: "true", Visible: true, Named: false},
			{Name: "false", Visible: true, Named: false},
		},
	}
	source := []byte("true false")
	arena := newNodeArena(arenaClassFull)
	trueNode := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	falseNode := newLeafNodeInArena(arena, 2, true, 5, 10, Point{Column: 5}, Point{Column: 10})
	root := newParentNodeInArena(arena, 1, true, []*Node{trueNode, falseNode}, nil, 0)

	normalizeCPONCompatibility(root, source, lang)

	for _, tc := range []struct {
		node      *Node
		childType string
	}{
		{node: trueNode, childType: "true"},
		{node: falseNode, childType: "false"},
	} {
		if got, want := tc.node.ChildCount(), 1; got != want {
			t.Fatalf("%s child count = %d, want %d", tc.childType, got, want)
		}
		child := tc.node.Child(0)
		if got, want := child.Type(lang), tc.childType; got != want {
			t.Fatalf("boolean child type = %q, want %q", got, want)
		}
		if child.IsNamed() {
			t.Fatalf("%s token child should be anonymous", tc.childType)
		}
	}
}

func TestNormalizeCPONNullLiteralCollapsesTokenChild(t *testing.T) {
	lang := &Language{
		Name:        "cpon",
		SymbolNames: []string{"EOF", "document", "null", "null"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "document", Visible: true, Named: true},
			{Name: "null", Visible: true, Named: true},
			{Name: "null", Visible: true, Named: false},
		},
	}
	source := []byte("null")
	arena := newNodeArena(arenaClassFull)
	token := newLeafNodeInArena(arena, 3, false, 0, 4, Point{}, Point{Column: 4})
	nullNode := newParentNodeInArena(arena, 2, true, []*Node{token}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{nullNode}, nil, 0)

	normalizeCPONCompatibility(root, source, lang)

	if got, want := nullNode.ChildCount(), 0; got != want {
		t.Fatalf("null child count = %d, want %d", got, want)
	}
	if got, want := nullNode.Type(lang), "null"; got != want {
		t.Fatalf("null type = %q, want %q", got, want)
	}
}
