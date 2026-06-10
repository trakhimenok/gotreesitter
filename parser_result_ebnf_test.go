package gotreesitter

import "testing"

func TestNormalizeEBNFRecoveredRootEndExtendsOneByteShortErrorRoot(t *testing.T) {
	lang := &Language{
		Name:        "ebnf",
		SymbolNames: []string{"EOF", "ERROR"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
		},
	}
	source := []byte("rule ::= 'x'")
	arena := newNodeArena(arenaClassFull)
	root := newLeafNodeInArena(arena, errorSymbol, true, 0, uint32(len(source)-1), Point{}, Point{Column: uint32(len(source) - 1)})

	normalizeEBNFCompatibility(root, source, lang)

	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("ERROR root EndByte = %d, want %d", got, want)
	}
	if got, want := root.EndPoint(), (Point{Column: uint32(len(source))}); got != want {
		t.Fatalf("ERROR root EndPoint = %+v, want %+v", got, want)
	}
}

func TestNormalizeEBNFRecoveredRootEndRejectsNonErrorRoot(t *testing.T) {
	lang := &Language{
		Name:        "ebnf",
		SymbolNames: []string{"EOF", "source_file"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
		},
	}
	source := []byte("rule ::= 'x'")
	arena := newNodeArena(arenaClassFull)
	root := newLeafNodeInArena(arena, 1, true, 0, uint32(len(source)-1), Point{}, Point{Column: uint32(len(source) - 1)})

	normalizeEBNFCompatibility(root, source, lang)

	if got, want := root.EndByte(), uint32(len(source)-1); got != want {
		t.Fatalf("source_file root EndByte = %d, want %d", got, want)
	}
}

func TestNormalizeEBNFRecoveredRootEndRejectsLargerGap(t *testing.T) {
	lang := &Language{
		Name:        "ebnf",
		SymbolNames: []string{"EOF", "ERROR"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "ERROR", Visible: true, Named: true},
		},
	}
	source := []byte("rule ::= 'x'")
	arena := newNodeArena(arenaClassFull)
	root := newLeafNodeInArena(arena, errorSymbol, true, 0, uint32(len(source)-2), Point{}, Point{Column: uint32(len(source) - 2)})

	normalizeEBNFCompatibility(root, source, lang)

	if got, want := root.EndByte(), uint32(len(source)-2); got != want {
		t.Fatalf("ERROR root EndByte = %d, want %d", got, want)
	}
}
