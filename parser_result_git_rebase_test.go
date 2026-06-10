package gotreesitter

import "testing"

func TestNormalizeGitRebaseOptionRestoresOptionTokenChild(t *testing.T) {
	lang := &Language{
		Name:        "git_rebase",
		SymbolNames: []string{"EOF", "source", "option", "-c", "-C"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source", Visible: true, Named: true},
			{Name: "option", Visible: true, Named: true},
			{Name: "-c", Visible: true, Named: false},
			{Name: "-C", Visible: true, Named: false},
		},
	}
	source := []byte("merge -C H second\nfixup -c A subject\n")
	arena := newNodeArena(arenaClassFull)
	upper := newLeafNodeInArena(arena, 2, true, 6, 8, Point{Column: 6}, Point{Column: 8})
	lower := newLeafNodeInArena(arena, 2, true, 24, 26, Point{Row: 1, Column: 6}, Point{Row: 1, Column: 8})
	root := newParentNodeInArena(arena, 1, true, []*Node{upper, lower}, nil, 0)

	normalizeGitRebaseCompatibility(root, source, lang)

	if got, want := upper.ChildCount(), 1; got != want {
		t.Fatalf("upper option child count = %d, want %d", got, want)
	}
	if got, want := upper.Child(0).Type(lang), "-C"; got != want {
		t.Fatalf("upper option child type = %q, want %q", got, want)
	}
	if got, want := lower.ChildCount(), 1; got != want {
		t.Fatalf("lower option child count = %d, want %d", got, want)
	}
	if got, want := lower.Child(0).Type(lang), "-c"; got != want {
		t.Fatalf("lower option child type = %q, want %q", got, want)
	}
}
