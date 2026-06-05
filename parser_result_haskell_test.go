package gotreesitter

import "testing"

func TestNormalizeHaskellCollapsedNamedLeafChildren(t *testing.T) {
	lang := &Language{
		Name: "haskell",
		SymbolNames: []string{
			"EOF",
			"haskell",
			"deriving_strategy",
			"stock",
			"anyclass",
			"via",
			"wildcard",
			"_",
			"wildcard",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "haskell", Visible: true, Named: true},
			{Name: "deriving_strategy", Visible: true, Named: true},
			{Name: "stock", Visible: true, Named: false},
			{Name: "anyclass", Visible: true, Named: false},
			{Name: "via", Visible: true, Named: false},
			{Name: "wildcard", Visible: true, Named: true},
			{Name: "_", Visible: true, Named: false},
			{Name: "wildcard", Visible: true, Named: true},
		},
	}
	source := []byte("stock anyclass via _ _ late")
	arena := newNodeArena(arenaClassFull)
	stockNode := newLeafNodeInArena(arena, 2, true, 0, 5, Point{}, Point{Column: 5})
	anyclassNode := newLeafNodeInArena(arena, 2, true, 6, 14, Point{Column: 6}, Point{Column: 14})
	viaNode := newLeafNodeInArena(arena, 2, true, 15, 18, Point{Column: 15}, Point{Column: 18})
	wildcardA := newLeafNodeInArena(arena, 6, true, 19, 20, Point{Column: 19}, Point{Column: 20})
	wildcardB := newLeafNodeInArena(arena, 8, true, 21, 22, Point{Column: 21}, Point{Column: 22})
	lateNode := newLeafNodeInArena(arena, 2, true, 23, 27, Point{Column: 23}, Point{Column: 27})
	root := newParentNodeInArena(arena, 1, true, []*Node{stockNode, anyclassNode, viaNode, wildcardA, wildcardB, lateNode}, nil, 0)

	normalizeHaskellCollapsedNamedLeafChildren(root, source, lang)

	assertCollapsedKeywordChild(t, stockNode, lang, "stock")
	assertCollapsedKeywordChild(t, anyclassNode, lang, "anyclass")
	assertCollapsedKeywordChild(t, viaNode, lang, "via")
	assertCollapsedKeywordChild(t, wildcardA, lang, "_")
	assertCollapsedKeywordChild(t, wildcardB, lang, "_")
	if got := lateNode.ChildCount(); got != 0 {
		t.Fatalf("non-matching deriving_strategy child count = %d, want 0", got)
	}
}

func TestNormalizeHaskellZeroWidthTokensDropsEmptySeparators(t *testing.T) {
	lang := &Language{
		Name:        "haskell",
		SymbolNames: []string{"EOF", "haskell", "pragma", "_token1", "haddock", "header"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "haskell", Visible: true, Named: true},
			{Name: "pragma", Visible: true, Named: true},
			{Name: "_token1", Visible: false, Named: false},
			{Name: "haddock", Visible: true, Named: true},
			{Name: "header", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	pragma := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	sep1 := newLeafNodeInArena(arena, 3, false, 4, 4, Point{Column: 4}, Point{Column: 4})
	haddock := newLeafNodeInArena(arena, 4, true, 5, 12, Point{Row: 1}, Point{Row: 1, Column: 7})
	sep2 := newLeafNodeInArena(arena, 3, false, 12, 12, Point{Row: 1, Column: 7}, Point{Row: 1, Column: 7})
	header := newLeafNodeInArena(arena, 5, true, 12, 20, Point{Row: 1, Column: 7}, Point{Row: 2, Column: 8})
	root := newParentNodeInArena(arena, 1, true, []*Node{pragma, sep1, haddock, sep2, header}, nil, 0)

	normalizeHaskellZeroWidthTokens(root, lang)

	if got, want := len(root.children), 3; got != want {
		t.Fatalf("len(root.children) = %d, want %d", got, want)
	}
	if got := root.children[0].Type(lang); got != "pragma" {
		t.Fatalf("child[0] = %q, want pragma", got)
	}
	if got := root.children[1].Type(lang); got != "haddock" {
		t.Fatalf("child[1] = %q, want haddock", got)
	}
	if got := root.children[2].Type(lang); got != "header" {
		t.Fatalf("child[2] = %q, want header", got)
	}
}

func TestNormalizeHaskellZeroWidthTokensFiltersFinalRefsWithoutDrain(t *testing.T) {
	lang := &Language{
		Name:        "haskell",
		SymbolNames: []string{"EOF", "haskell", "pragma", "_token1", "haddock", "header"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "haskell", Visible: true, Named: true},
			{Name: "pragma", Visible: true, Named: true},
			{Name: "_token1", Visible: false, Named: false},
			{Name: "haddock", Visible: true, Named: true},
			{Name: "header", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	pragma := newCompactFullLeafInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	pragma.parseState = 12
	sep := newCompactFullLeafInArena(arena, 3, false, 4, 4, Point{Column: 4}, Point{Column: 4})
	sep.parseState = 13
	header := newCompactFullLeafInArena(arena, 5, true, 12, 20, Point{Row: 1, Column: 7}, Point{Row: 2, Column: 8})
	header.parseState = 15
	parent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{
		newStackEntryCompactFullLeaf(pragma.parseState, pragma),
		newStackEntryCompactFullLeaf(sep.parseState, sep),
		newStackEntryCompactFullLeaf(header.parseState, header),
	}, 0, 20, Point{}, Point{Row: 2, Column: 8}, false)
	parent.parseState = 16
	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)

	normalizeHaskellZeroWidthTokens(root, lang)

	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children during normalize = %d, want 0", got)
	}
	if !nodeHasFinalChildRefs(root) {
		t.Fatal("root lost final-child refs")
	}
	if got := root.ChildCount(); got != 2 {
		t.Fatalf("root child count = %d, want 2", got)
	}
	if got := root.Child(0).Type(lang); got != "pragma" {
		t.Fatalf("root child 0 = %q, want pragma", got)
	}
	if got := root.Child(1).Type(lang); got != "header" {
		t.Fatalf("root child 1 = %q, want header", got)
	}
}

func TestNormalizeHaskellRootImportFieldSetsImportsField(t *testing.T) {
	lang := &Language{
		Name:        "haskell",
		SymbolNames: []string{"EOF", "haskell", "pragma", "haddock", "header", "imports", "declarations"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "haskell", Visible: true, Named: true},
			{Name: "pragma", Visible: true, Named: true},
			{Name: "haddock", Visible: true, Named: true},
			{Name: "header", Visible: true, Named: true},
			{Name: "imports", Visible: true, Named: true},
			{Name: "declarations", Visible: true, Named: true},
		},
		FieldNames: []string{"", "imports", "declarations"},
	}

	arena := newNodeArena(arenaClassFull)
	pragma := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	haddock := newLeafNodeInArena(arena, 3, true, 5, 12, Point{Row: 1}, Point{Row: 1, Column: 7})
	header := newLeafNodeInArena(arena, 4, true, 12, 20, Point{Row: 1, Column: 7}, Point{Row: 2, Column: 8})
	imports := newLeafNodeInArena(arena, 5, true, 21, 30, Point{Row: 3}, Point{Row: 3, Column: 9})
	declarations := newLeafNodeInArena(arena, 6, true, 31, 40, Point{Row: 4}, Point{Row: 4, Column: 9})
	root := newParentNodeInArena(arena, 1, true, []*Node{pragma, haddock, header, imports, declarations}, nil, 0)

	normalizeHaskellRootImportField(root, lang)

	if got, want := len(root.fieldIDs), len(root.children); got != want {
		t.Fatalf("len(root.fieldIDs) = %d, want %d", got, want)
	}
	if got, want := root.fieldIDs[3], FieldID(1); got != want {
		t.Fatalf("fieldIDs[3] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(root.fieldSources, 3), uint8(fieldSourceInherited); got != want {
		t.Fatalf("fieldSources[3] = %d, want %d", got, want)
	}
	if got, want := root.fieldIDs[4], FieldID(2); got != want {
		t.Fatalf("fieldIDs[4] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(root.fieldSources, 4), uint8(fieldSourceInherited); got != want {
		t.Fatalf("fieldSources[4] = %d, want %d", got, want)
	}
}

func TestNormalizeHaskellRootImportFieldSetsFinalRefFieldsWithoutDrain(t *testing.T) {
	lang := &Language{
		Name:        "haskell",
		SymbolNames: []string{"EOF", "haskell", "pragma", "imports", "declarations"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "haskell", Visible: true, Named: true},
			{Name: "pragma", Visible: true, Named: true},
			{Name: "imports", Visible: true, Named: true},
			{Name: "declarations", Visible: true, Named: true},
		},
		FieldNames: []string{"", "imports", "declarations"},
	}

	arena := newNodeArena(arenaClassFull)
	arena.finalChildRefs = true
	pragma := newCompactFullLeafInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	pragma.parseState = 12
	imports := newCompactFullLeafInArena(arena, 3, true, 5, 12, Point{Row: 1}, Point{Row: 1, Column: 7})
	imports.parseState = 13
	declarations := newCompactFullLeafInArena(arena, 4, true, 13, 20, Point{Row: 2}, Point{Row: 2, Column: 7})
	declarations.parseState = 14
	parent := newPendingParentInArena(arena, 1, true, 0, []stackEntry{
		newStackEntryCompactFullLeaf(pragma.parseState, pragma),
		newStackEntryCompactFullLeaf(imports.parseState, imports),
		newStackEntryCompactFullLeaf(declarations.parseState, declarations),
	}, 0, 20, Point{}, Point{Row: 2, Column: 7}, false)
	parent.parseState = 15
	entry := newStackEntryPendingParent(parent.parseState, parent)
	root := materializeStackEntryPendingParent(arena, &entry, pendingParentMaterializeForFinalTree)

	normalizeHaskellRootImportField(root, lang)

	if got := arena.finalChildRefsMaterializedParents; got != 0 {
		t.Fatalf("final child ref range materialized parents = %d, want 0", got)
	}
	if got := arena.finalChildRefsSingleChildMaterializedChildren; got != 0 {
		t.Fatalf("final child ref single children during normalize = %d, want 0", got)
	}
	if !nodeHasFinalChildRefs(root) {
		t.Fatal("root lost final-child refs")
	}
	if got, want := root.fieldIDs[1], FieldID(1); got != want {
		t.Fatalf("fieldIDs[1] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(root.fieldSources, 1), uint8(fieldSourceInherited); got != want {
		t.Fatalf("fieldSources[1] = %d, want %d", got, want)
	}
	if got, want := root.fieldIDs[2], FieldID(2); got != want {
		t.Fatalf("fieldIDs[2] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(root.fieldSources, 2), uint8(fieldSourceInherited); got != want {
		t.Fatalf("fieldSources[2] = %d, want %d", got, want)
	}
}

func TestNormalizeHaskellDeclarationsSpanExtendsToTrailingTrivia(t *testing.T) {
	lang := &Language{
		Name:        "haskell",
		SymbolNames: []string{"EOF", "haskell", "declarations"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "haskell", Visible: true, Named: true},
			{Name: "declarations", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	decls := newLeafNodeInArena(arena, 2, true, 10, 14, Point{Row: 1}, Point{Row: 1, Column: 4})
	root := newParentNodeInArena(arena, 1, true, []*Node{decls}, nil, 0)
	root.endByte = 15
	root.endPoint = Point{Row: 2}

	normalizeHaskellDeclarationsSpan(root, []byte("0123456789body\n"), lang)

	if got, want := decls.endByte, uint32(15); got != want {
		t.Fatalf("decls.endByte = %d, want %d", got, want)
	}
	if got, want := decls.endPoint, root.endPoint; got != want {
		t.Fatalf("decls.endPoint = %#v, want %#v", got, want)
	}
}
