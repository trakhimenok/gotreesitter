package gotreesitter

import "testing"

func dhallTestLanguage() *Language {
	return &Language{
		Name:        "dhall",
		SymbolNames: []string{"EOF", "expression", "primitive_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "expression", Visible: true, Named: true},
			{Name: "primitive_expression", Visible: true, Named: true},
		},
	}
}

func TestNormalizeDhallExpressionLeadingTriviaStart(t *testing.T) {
	lang := dhallTestLanguage()
	// Mirrors Prelude/DirectoryTree/Access/Mask/none.dhall, which begins
	// with two spaces: C tree-sitter roots the expression at byte 2.
	source := []byte("  { read = False }")
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, true, 2, uint32(len(source)), Point{Column: 2}, Point{Column: uint32(len(source))})
	root := newParentNodeInArena(arena, 1, true, []*Node{child}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}
	root.endByte = uint32(len(source))
	root.endPoint = Point{Column: uint32(len(source))}

	normalizeDhallCompatibility(root, source, lang)

	if got, want := root.StartByte(), uint32(2); got != want {
		t.Fatalf("root start byte = %d, want %d", got, want)
	}
	if got, want := root.StartPoint(), (Point{Column: 2}); got != want {
		t.Fatalf("root start point = %+v, want %+v", got, want)
	}
}

func TestNormalizeDhallExpressionLeadingTriviaStartRejectsNonTrivia(t *testing.T) {
	lang := dhallTestLanguage()
	// A non-whitespace prefix (e.g. bytes covered by a leading comment that
	// is itself the first child) must keep the root start at 0.
	source := []byte("x { read = False }")
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, true, 2, uint32(len(source)), Point{Column: 2}, Point{Column: uint32(len(source))})
	root := newParentNodeInArena(arena, 1, true, []*Node{child}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}

	normalizeDhallCompatibility(root, source, lang)

	if got, want := root.StartByte(), uint32(0); got != want {
		t.Fatalf("root start byte = %d, want %d", got, want)
	}
}

func TestNormalizeDhallExpressionLeadingTriviaStartIgnoresOtherRoots(t *testing.T) {
	lang := dhallTestLanguage()
	source := []byte("  { read = False }")
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 1, true, 2, uint32(len(source)), Point{Column: 2}, Point{Column: uint32(len(source))})
	// Root symbol 2 is "primitive_expression", not the expression root.
	root := newParentNodeInArena(arena, 2, true, []*Node{child}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}

	normalizeDhallCompatibility(root, source, lang)

	if got, want := root.StartByte(), uint32(0); got != want {
		t.Fatalf("root start byte = %d, want %d", got, want)
	}
}
