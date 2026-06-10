package gotreesitter

import "testing"

func TestNormalizeHackAsyncModifierRestoresAsyncTokenChild(t *testing.T) {
	lang := &Language{
		Name:        "hack",
		SymbolNames: []string{"EOF", "script", "async_modifier", "async"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "script", Visible: true, Named: true},
			{Name: "async_modifier", Visible: true, Named: true},
			{Name: "async", Visible: true, Named: false},
		},
	}
	source := []byte("async function f(): Awaitable<int> {}")
	arena := newNodeArena(arenaClassFull)
	modifier := newLeafNodeInArena(arena, 2, true, 0, 5, Point{}, Point{Column: 5})
	root := newParentNodeInArena(arena, 1, true, []*Node{modifier}, nil, 0)

	normalizeHackCompatibility(root, source, lang)

	if got, want := modifier.ChildCount(), 1; got != want {
		t.Fatalf("async_modifier child count = %d, want %d", got, want)
	}
	if got, want := modifier.Child(0).Type(lang), "async"; got != want {
		t.Fatalf("async_modifier child type = %q, want %q", got, want)
	}
}

func TestNormalizeHackBooleanLiteralsRestoreTokenChildren(t *testing.T) {
	lang := &Language{
		Name:        "hack",
		SymbolNames: []string{"EOF", "script", "true", "false", "true", "false"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "script", Visible: true, Named: true},
			{Name: "true", Visible: true, Named: true},
			{Name: "false", Visible: true, Named: true},
			{Name: "true", Visible: true, Named: false},
			{Name: "false", Visible: true, Named: false},
		},
	}
	source := []byte("true false")
	arena := newNodeArena(arenaClassFull)
	trueNode := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	falseNode := newLeafNodeInArena(arena, 3, true, 5, 10, Point{Column: 5}, Point{Column: 10})
	root := newParentNodeInArena(arena, 1, true, []*Node{trueNode, falseNode}, nil, 0)

	normalizeHackCompatibility(root, source, lang)

	if got, want := trueNode.ChildCount(), 1; got != want {
		t.Fatalf("true child count = %d, want %d", got, want)
	}
	if got, want := trueNode.Child(0).Type(lang), "true"; got != want {
		t.Fatalf("true child type = %q, want %q", got, want)
	}
	if trueNode.Child(0).IsNamed() {
		t.Fatal("true token child should be anonymous")
	}
	if got, want := falseNode.ChildCount(), 1; got != want {
		t.Fatalf("false child count = %d, want %d", got, want)
	}
	if got, want := falseNode.Child(0).Type(lang), "false"; got != want {
		t.Fatalf("false child type = %q, want %q", got, want)
	}
	if falseNode.Child(0).IsNamed() {
		t.Fatal("false token child should be anonymous")
	}
}

func TestNormalizeHackClassLeafWrappersRestoreTokenChildren(t *testing.T) {
	lang := &Language{
		Name: "hack",
		SymbolNames: []string{
			"EOF",
			"script",
			"abstract_modifier",
			"static_modifier",
			"visibility_modifier",
			"null",
			"scope_identifier",
			"abstract",
			"static",
			"public",
			"protected",
			"private",
			"null",
			"parent",
			"self",
			"variadic_modifier",
			"...",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "script", Visible: true, Named: true},
			{Name: "abstract_modifier", Visible: true, Named: true},
			{Name: "static_modifier", Visible: true, Named: true},
			{Name: "visibility_modifier", Visible: true, Named: true},
			{Name: "null", Visible: true, Named: true},
			{Name: "scope_identifier", Visible: true, Named: true},
			{Name: "abstract", Visible: true, Named: false},
			{Name: "static", Visible: true, Named: false},
			{Name: "public", Visible: true, Named: false},
			{Name: "protected", Visible: true, Named: false},
			{Name: "private", Visible: true, Named: false},
			{Name: "null", Visible: true, Named: false},
			{Name: "parent", Visible: true, Named: false},
			{Name: "self", Visible: true, Named: false},
			{Name: "variadic_modifier", Visible: true, Named: true},
			{Name: "...", Visible: true, Named: false},
		},
	}
	source := []byte("abstract static public protected private null parent self static ...")
	arena := newNodeArena(arenaClassFull)
	nodes := []*Node{
		newLeafNodeInArena(arena, 2, true, 0, 8, Point{}, Point{Column: 8}),
		newLeafNodeInArena(arena, 3, true, 9, 15, Point{Column: 9}, Point{Column: 15}),
		newLeafNodeInArena(arena, 4, true, 16, 22, Point{Column: 16}, Point{Column: 22}),
		newLeafNodeInArena(arena, 4, true, 23, 32, Point{Column: 23}, Point{Column: 32}),
		newLeafNodeInArena(arena, 4, true, 33, 40, Point{Column: 33}, Point{Column: 40}),
		newLeafNodeInArena(arena, 5, true, 41, 45, Point{Column: 41}, Point{Column: 45}),
		newLeafNodeInArena(arena, 6, true, 46, 52, Point{Column: 46}, Point{Column: 52}),
		newLeafNodeInArena(arena, 6, true, 53, 57, Point{Column: 53}, Point{Column: 57}),
		newLeafNodeInArena(arena, 6, true, 58, 64, Point{Column: 58}, Point{Column: 64}),
		newLeafNodeInArena(arena, 15, true, 65, 68, Point{Column: 65}, Point{Column: 68}),
	}
	root := newParentNodeInArena(arena, 1, true, nodes, nil, 0)

	normalizeHackCompatibility(root, source, lang)

	for i, tc := range []struct {
		name      string
		childType string
	}{
		{name: "abstract_modifier", childType: "abstract"},
		{name: "static_modifier", childType: "static"},
		{name: "visibility_modifier", childType: "public"},
		{name: "visibility_modifier", childType: "protected"},
		{name: "visibility_modifier", childType: "private"},
		{name: "null", childType: "null"},
		{name: "scope_identifier", childType: "parent"},
		{name: "scope_identifier", childType: "self"},
		{name: "scope_identifier", childType: "static"},
		{name: "variadic_modifier", childType: "..."},
	} {
		node := nodes[i]
		if got, want := node.Type(lang), tc.name; got != want {
			t.Fatalf("node %d type = %q, want %q", i, got, want)
		}
		if got, want := node.ChildCount(), 1; got != want {
			t.Fatalf("%s child count = %d, want %d", tc.name, got, want)
		}
		child := node.Child(0)
		if got, want := child.Type(lang), tc.childType; got != want {
			t.Fatalf("%s child type = %q, want %q", tc.name, got, want)
		}
		if child.IsNamed() {
			t.Fatalf("%s token child should be anonymous", tc.name)
		}
	}
}
