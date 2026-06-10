package gotreesitter

import "testing"

func httpTestLanguage() *Language {
	return &Language{
		Name:        "http",
		SymbolNames: []string{"EOF", "document", "section", "request_separator", "comment", "request"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "document", Visible: true, Named: true},
			{Name: "section", Visible: true, Named: true},
			{Name: "request_separator", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "request", Visible: true, Named: true},
		},
	}
}

func TestNormalizeHTTPDocumentSectionMerges(t *testing.T) {
	lang := httpTestLanguage()
	// Mirrors pre_request_script.http: Go splits a single C section into a
	// separator-led section plus separator-less continuation sections, which
	// must merge back into their predecessor.
	source := []byte("### a\n# c\nGET u\n### b\n")
	arena := newNodeArena(arenaClassFull)
	sep1 := newLeafNodeInArena(arena, 3, true, 0, 6, Point{}, Point{Row: 1})
	sec1 := newParentNodeInArena(arena, 2, true, []*Node{sep1}, nil, 0)
	comment := newLeafNodeInArena(arena, 4, true, 6, 10, Point{Row: 1}, Point{Row: 2})
	sec2 := newParentNodeInArena(arena, 2, true, []*Node{comment}, nil, 0)
	request := newLeafNodeInArena(arena, 5, true, 10, 16, Point{Row: 2}, Point{Row: 3})
	sec3 := newParentNodeInArena(arena, 2, true, []*Node{request}, nil, 0)
	sep2 := newLeafNodeInArena(arena, 3, true, 16, 22, Point{Row: 3}, Point{Row: 4})
	sec4 := newParentNodeInArena(arena, 2, true, []*Node{sep2}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{sec1, sec2, sec3, sec4}, nil, 0)

	normalizeHTTPCompatibility(root, source, lang)

	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("document child count = %d, want %d", got, want)
	}
	first := root.Child(0)
	if got, want := first.ChildCount(), 3; got != want {
		t.Fatalf("merged section child count = %d, want %d", got, want)
	}
	if first.StartByte() != 0 || first.EndByte() != 16 {
		t.Fatalf("merged section span = [%d:%d], want [0:16]", first.StartByte(), first.EndByte())
	}
	wantTypes := []string{"request_separator", "comment", "request"}
	for i, want := range wantTypes {
		if got := first.Child(i).Type(lang); got != want {
			t.Fatalf("merged child[%d] type = %q, want %q", i, got, want)
		}
	}
	if first.Child(2).Parent() != first {
		t.Fatal("merged child parent not updated")
	}
	second := root.Child(1)
	if second.StartByte() != 16 || second.EndByte() != 22 {
		t.Fatalf("second section span = [%d:%d], want [16:22]", second.StartByte(), second.EndByte())
	}
}

func TestNormalizeHTTPDocumentSectionMergesKeepsLeadingContentSection(t *testing.T) {
	lang := httpTestLanguage()
	// A document may open with a separator-less section; it must not merge
	// into anything, and a following separator-led section stays separate.
	source := []byte("# c\n### a\n")
	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 4, true, 0, 4, Point{}, Point{Row: 1})
	sec1 := newParentNodeInArena(arena, 2, true, []*Node{comment}, nil, 0)
	sep := newLeafNodeInArena(arena, 3, true, 4, 10, Point{Row: 1}, Point{Row: 2})
	sec2 := newParentNodeInArena(arena, 2, true, []*Node{sep}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{sec1, sec2}, nil, 0)

	normalizeHTTPCompatibility(root, source, lang)

	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("document child count = %d, want %d", got, want)
	}
}

func TestNormalizeHTTPDocumentSectionMergesAbsorbsBlankSections(t *testing.T) {
	lang := httpTestLanguage()
	// A childless section (hidden blank line only) merges into the previous
	// section, extending its span without adding children.
	source := []byte("# c\n\n")
	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 4, true, 0, 4, Point{}, Point{Row: 1})
	sec1 := newParentNodeInArena(arena, 2, true, []*Node{comment}, nil, 0)
	blank := newParentNodeInArena(arena, 2, true, nil, nil, 0)
	blank.startByte = 4
	blank.endByte = 5
	blank.startPoint = Point{Row: 1}
	blank.endPoint = Point{Row: 2}
	root := newParentNodeInArena(arena, 1, true, []*Node{sec1, blank}, nil, 0)

	normalizeHTTPCompatibility(root, source, lang)

	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("document child count = %d, want %d", got, want)
	}
	first := root.Child(0)
	if got, want := first.ChildCount(), 1; got != want {
		t.Fatalf("merged section child count = %d, want %d", got, want)
	}
	if first.EndByte() != 5 {
		t.Fatalf("merged section end = %d, want 5", first.EndByte())
	}
}
