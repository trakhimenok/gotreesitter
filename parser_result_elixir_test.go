package gotreesitter

import "testing"

func TestNormalizeElixirCollapsedLiteralChildren(t *testing.T) {
	lang := testElixirCompatibilityLanguage()
	source := []byte("false true nil")
	arena := newNodeArena(arenaClassFull)
	falseNode := newLeafNodeInArena(arena, 8, true, 0, 5, Point{}, Point{Column: 5})
	trueNode := newLeafNodeInArena(arena, 8, true, 6, 10, Point{Column: 6}, Point{Column: 10})
	nilNode := newLeafNodeInArena(arena, 15, true, 11, 14, Point{Column: 11}, Point{Column: 14})
	root := newParentNodeInArena(arena, 1, true, []*Node{falseNode, trueNode, nilNode}, nil, 0)

	normalizeElixirCompatibility(root, source, lang)

	assertCollapsedKeywordChild(t, falseNode, lang, "false")
	assertCollapsedKeywordChild(t, trueNode, lang, "true")
	assertCollapsedKeywordChild(t, nilNode, lang, "nil")
}

func TestNormalizeElixirMapContentKeywordPairs(t *testing.T) {
	lang := testElixirCompatibilityLanguage()
	arena := newNodeArena(arenaClassFull)
	keyword := newLeafNodeInArena(arena, 5, true, 0, 9, Point{}, Point{Column: 9})
	value := newLeafNodeInArena(arena, 6, true, 9, 17, Point{Column: 9}, Point{Column: 17})
	pair := newParentNodeInArena(arena, 4, true, []*Node{keyword, value}, nil, 0)
	content := newParentNodeInArena(arena, 2, true, []*Node{pair}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{content}, nil, 0)

	normalizeElixirCompatibility(root, []byte(`shortcut:"syntax"`), lang)

	if got, want := content.ChildCount(), 1; got != want {
		t.Fatalf("map_content child count = %d, want %d", got, want)
	}
	keywords := content.Child(0)
	if keywords == nil || keywords.Type(lang) != "keywords" {
		t.Fatalf("map_content child = %v, want keywords", keywords)
	}
	if got := keywords.Child(0); got != pair {
		t.Fatalf("keywords child = %v, want original pair", got)
	}
	if got := pair.Parent(); got != keywords {
		t.Fatalf("pair parent = %v, want keywords", got)
	}
}

func TestNormalizeElixirMapContentUpdateOperatorWrapsRightKeywordPair(t *testing.T) {
	lang := testElixirCompatibilityLanguage()
	arena := newNodeArena(arenaClassFull)
	ident := newLeafNodeInArena(arena, 11, true, 2, 5, Point{Column: 2}, Point{Column: 5})
	bar := newLeafNodeInArena(arena, 12, false, 6, 7, Point{Column: 6}, Point{Column: 7})
	keyword := newLeafNodeInArena(arena, 5, true, 8, 13, Point{Column: 8}, Point{Column: 13})
	value := newLeafNodeInArena(arena, 6, true, 13, 20, Point{Column: 13}, Point{Column: 20})
	pair := newParentNodeInArena(arena, 4, true, []*Node{keyword, value}, nil, 0)
	content := newParentNodeInArena(arena, 2, true, []*Node{ident, bar, pair}, []FieldID{1, 2, 3}, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{content}, nil, 0)

	normalizeElixirCompatibility(root, []byte(`%{map | name:"Silly"}`), lang)

	if got, want := content.ChildCount(), 1; got != want {
		t.Fatalf("map_content child count = %d, want %d", got, want)
	}
	binary := content.Child(0)
	if binary == nil || binary.Type(lang) != "binary_operator" {
		t.Fatalf("map_content child = %v, want binary_operator", binary)
	}
	if got := content.FieldNameForChild(0, lang); got != "" {
		t.Fatalf("map_content child field = %q, want empty", got)
	}
	if got, want := binary.ChildCount(), 3; got != want {
		t.Fatalf("binary_operator child count = %d, want %d", got, want)
	}
	for i, want := range []string{"left", "operator", "right"} {
		if got := binary.FieldNameForChild(i, lang); got != want {
			t.Fatalf("binary_operator child[%d] field = %q, want %q", i, got, want)
		}
	}
	if got := binary.Child(0); got != ident {
		t.Fatalf("binary_operator child[0] = %v, want identifier", got)
	}
	if got := binary.Child(1); got != bar {
		t.Fatalf("binary_operator child[1] = %v, want | token", got)
	}
	keywords := binary.Child(2)
	if keywords == nil || keywords.Type(lang) != "keywords" {
		t.Fatalf("binary_operator child[2] = %v, want keywords", keywords)
	}
	if got := keywords.Child(0); got != pair {
		t.Fatalf("keywords child = %v, want original pair", got)
	}
}

func testElixirCompatibilityLanguage() *Language {
	return &Language{
		Name: "elixir",
		FieldNames: []string{
			"",
			"left",
			"operator",
			"right",
		},
		SymbolNames: []string{
			"EOF",
			"source",
			"map_content",
			"keywords",
			"pair",
			"keyword",
			"string",
			",",
			"boolean",
			"true",
			"false",
			"identifier",
			"|",
			"binary_operator",
			"=>",
			"nil",
			"nil",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source", Visible: true, Named: true},
			{Name: "map_content", Visible: true, Named: true},
			{Name: "keywords", Visible: true, Named: true},
			{Name: "pair", Visible: true, Named: true},
			{Name: "keyword", Visible: true, Named: true},
			{Name: "string", Visible: true, Named: true},
			{Name: ",", Visible: true, Named: false},
			{Name: "boolean", Visible: true, Named: true},
			{Name: "true", Visible: true, Named: false},
			{Name: "false", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "|", Visible: true, Named: false},
			{Name: "binary_operator", Visible: true, Named: true},
			{Name: "=>", Visible: true, Named: false},
			{Name: "nil", Visible: true, Named: true},
			{Name: "nil", Visible: true, Named: false},
		},
	}
}
