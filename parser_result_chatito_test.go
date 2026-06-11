package gotreesitter

import "testing"

func TestNormalizeChatitoTrailingAliasBodyError(t *testing.T) {
	lang := &Language{
		Name:        "chatito",
		SymbolNames: []string{"EOF", "source", "alias_def", "alias_body", "word", "ERROR", "indent"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: false},
		},
	}
	source := []byte("~[minute]\n    58\n    59")
	arena := newNodeArena(arenaClassFull)
	defer arena.Release()

	body := newParentNode(arena, 3, true, []*Node{
		newLeafNodeInArena(arena, 6, false, 10, 14, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 4}),
		newLeafNodeInArena(arena, 4, true, 14, 16, Point{Row: 1, Column: 4}, Point{Row: 1, Column: 6}),
	}, nil, 0)
	alias := newParentNode(arena, 2, true, []*Node{body}, nil, 0)
	alias.startByte = 0
	alias.startPoint = Point{}
	body.endByte = 17
	body.endPoint = Point{Row: 2, Column: 0}
	alias.endByte = 17
	alias.endPoint = Point{Row: 2, Column: 0}
	err := newParentNode(arena, 5, true, []*Node{
		newLeafNodeInArena(arena, 6, false, 17, 21, Point{Row: 2, Column: 0}, Point{Row: 2, Column: 4}),
	}, nil, 0)
	err.endByte = uint32(len(source))
	err.endPoint = Point{Row: 2, Column: 6}
	err.setExtra(true)
	err.setHasError(true)
	root := newParentNode(arena, 1, true, []*Node{alias, err}, nil, 0)
	root.setHasError(true)

	normalizeChatitoCompatibility(root, source, lang)

	if got := resultChildCount(root); got != 1 {
		t.Fatalf("root child count = %d, want 1", got)
	}
	if got := alias.endByte; got != uint32(len(source)) {
		t.Fatalf("alias end = %d, want %d", got, len(source))
	}
	if got := body.endByte; got != uint32(len(source)) {
		t.Fatalf("body end = %d, want %d", got, len(source))
	}
	if got := resultChildCount(body); got != 4 {
		t.Fatalf("body child count = %d, want 4", got)
	}
	if got := body.children[3].Type(lang); got != "word" {
		t.Fatalf("trailing child type = %q, want word", got)
	}
	if got := body.children[3].Text(source); got != "59" {
		t.Fatalf("trailing word text = %q, want 59", got)
	}
	if root.HasError() {
		t.Fatalf("root still has error after absorbing trailing alias word")
	}
}
