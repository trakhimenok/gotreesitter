package gotreesitter

import "testing"

func TestNormalizeHTTPBlankLineSectionsFoldsIntoPreviousSection(t *testing.T) {
	lang := &Language{
		Name:        "http",
		SymbolNames: []string{"EOF", "document", "section", "comment", "request_separator"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "document", Visible: true, Named: true},
			{Name: "section", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "request_separator", Visible: true, Named: true},
		},
	}
	source := []byte("# c\n\n### next\n")
	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 3, true, 0, 4, Point{}, Point{Row: 1})
	firstSection := newParentNodeInArena(arena, 2, true, []*Node{comment}, nil, 0)
	blankSection := newLeafNodeInArena(arena, 2, true, 4, 5, Point{Row: 1}, Point{Row: 2})
	separator := newLeafNodeInArena(arena, 4, true, 5, 14, Point{Row: 2}, Point{Row: 3})
	secondSection := newParentNodeInArena(arena, 2, true, []*Node{separator}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{firstSection, blankSection, secondSection}, nil, 0)

	normalizeHTTPCompatibility(root, source, lang)

	if got, want := resultChildCount(root), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got, want := resultChildAt(root, 0).endByte, uint32(5); got != want {
		t.Fatalf("first section endByte = %d, want %d", got, want)
	}
	if got, want := resultChildAt(root, 0).endPoint, (Point{Row: 2}); got != want {
		t.Fatalf("first section endPoint = %#v, want %#v", got, want)
	}
	if got, want := resultChildAt(root, 1), secondSection; got != want {
		t.Fatalf("second section pointer changed")
	}
}

func TestNormalizeHTTPDocumentSectionsMergesContentUntilNextSeparator(t *testing.T) {
	lang := &Language{
		Name:        "http",
		SymbolNames: []string{"EOF", "document", "section", "request_separator", "comment", "request"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "document", Visible: true, Named: true},
			{Name: "section", Visible: true, Named: true},
			{Name: "request_separator", Visible: true, Named: true},
			{Name: "comment", Visible: true, Named: true},
			{Name: "request", Visible: true, Named: true},
		},
		FieldNames: []string{"", "request"},
	}
	source := []byte("### a\n# c\nGET /\n### b\n")
	arena := newNodeArena(arenaClassFull)
	firstSep := newLeafNodeInArena(arena, 3, true, 0, 6, Point{}, Point{Row: 1})
	firstSection := newParentNodeInArena(arena, 2, true, []*Node{firstSep}, nil, 0)
	comment := newLeafNodeInArena(arena, 4, true, 6, 10, Point{Row: 1}, Point{Row: 2})
	commentSection := newParentNodeInArena(arena, 2, true, []*Node{comment}, nil, 0)
	request := newLeafNodeInArena(arena, 5, true, 10, 16, Point{Row: 2}, Point{Row: 3})
	requestSection := newParentNodeInArena(arena, 2, true, []*Node{request}, []FieldID{1}, 0)
	requestSection.fieldSources = []uint8{fieldSourceDirect}
	secondSep := newLeafNodeInArena(arena, 3, true, 16, 22, Point{Row: 3}, Point{Row: 4})
	secondSection := newParentNodeInArena(arena, 2, true, []*Node{secondSep}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{firstSection, commentSection, requestSection, secondSection}, nil, 0)

	normalizeHTTPCompatibility(root, source, lang)

	if got, want := resultChildCount(root), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	merged := resultChildAt(root, 0)
	if got, want := resultChildCount(merged), 3; got != want {
		t.Fatalf("merged section child count = %d, want %d", got, want)
	}
	if got, want := merged.endByte, uint32(16); got != want {
		t.Fatalf("merged section endByte = %d, want %d", got, want)
	}
	if got, want := nodeFieldIDAt(merged, 2), FieldID(1); got != want {
		t.Fatalf("request field ID = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(merged.fieldSources, 2), uint8(fieldSourceDirect); got != want {
		t.Fatalf("request field source = %d, want %d", got, want)
	}
	if got, want := resultChildAt(root, 1), secondSection; got != want {
		t.Fatalf("second section pointer changed")
	}
}
