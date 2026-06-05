package gotreesitter

import "testing"

func TestNormalizeRustBooleanLiteralRestoresKeywordChild(t *testing.T) {
	lang := &Language{
		Name:        "rust",
		SymbolNames: []string{"", "source_file", "boolean_literal", "true", "false", "empty_statement", ";"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "boolean_literal", Visible: true, Named: true},
			{Name: "true", Visible: true, Named: false},
			{Name: "false", Visible: true, Named: false},
			{Name: "empty_statement", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
		},
	}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("true")
	booleanLiteral := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	root := newParentNodeInArena(arena, 1, true, []*Node{booleanLiteral}, nil, 0)

	normalizeResultCompatibility(root, source, parser)

	if got, want := booleanLiteral.ChildCount(), 1; got != want {
		t.Fatalf("boolean literal child count = %d, want %d", got, want)
	}
	child := booleanLiteral.Child(0)
	if child == nil {
		t.Fatal("boolean literal child = nil")
	}
	if got, want := child.Type(lang), "true"; got != want {
		t.Fatalf("boolean literal child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored true child should be anonymous")
	}
}

func TestNormalizeRustEmptyStatementRestoresSemicolonChild(t *testing.T) {
	lang := &Language{
		Name:        "rust",
		SymbolNames: []string{"", "source_file", "empty_statement", ";", "remaining_field_pattern", ".."},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "empty_statement", Visible: true, Named: true},
			{Name: ";", Visible: true, Named: false},
			{Name: "remaining_field_pattern", Visible: true, Named: true},
			{Name: "..", Visible: true, Named: false},
		},
	}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte(";")
	emptyStatement := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, 1, true, []*Node{emptyStatement}, nil, 0)

	normalizeResultCompatibility(root, source, parser)

	if got, want := emptyStatement.ChildCount(), 1; got != want {
		t.Fatalf("empty statement child count = %d, want %d", got, want)
	}
	child := emptyStatement.Child(0)
	if child == nil {
		t.Fatal("empty statement child = nil")
	}
	if got, want := child.Type(lang), ";"; got != want {
		t.Fatalf("empty statement child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored semicolon child should be anonymous")
	}
}

func TestNormalizeRustRemainingFieldPatternRestoresDotDotChild(t *testing.T) {
	lang := &Language{
		Name:        "rust",
		SymbolNames: []string{"", "source_file", "remaining_field_pattern", "..", "range_expression", "..="},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "remaining_field_pattern", Visible: true, Named: true},
			{Name: "..", Visible: true, Named: false},
			{Name: "range_expression", Visible: true, Named: true},
			{Name: "..=", Visible: true, Named: false},
		},
	}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("..")
	remaining := newLeafNodeInArena(arena, 2, true, 0, 2, Point{}, Point{Column: 2})
	root := newParentNodeInArena(arena, 1, true, []*Node{remaining}, nil, 0)

	normalizeResultCompatibility(root, source, parser)

	if got, want := remaining.ChildCount(), 1; got != want {
		t.Fatalf("remaining field pattern child count = %d, want %d", got, want)
	}
	child := remaining.Child(0)
	if child == nil {
		t.Fatal("remaining field pattern child = nil")
	}
	if got, want := child.Type(lang), ".."; got != want {
		t.Fatalf("remaining field pattern child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored .. child should be anonymous")
	}
}

func TestNormalizeRustRangeExpressionRestoresOperatorChild(t *testing.T) {
	lang := &Language{
		Name:        "rust",
		SymbolNames: []string{"", "source_file", "range_expression", "..", "..="},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "range_expression", Visible: true, Named: true},
			{Name: "..", Visible: true, Named: false},
			{Name: "..=", Visible: true, Named: false},
		},
	}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("..=")
	rangeExpr := newLeafNodeInArena(arena, 2, true, 0, 3, Point{}, Point{Column: 3})
	root := newParentNodeInArena(arena, 1, true, []*Node{rangeExpr}, nil, 0)

	normalizeResultCompatibility(root, source, parser)

	if got, want := rangeExpr.ChildCount(), 1; got != want {
		t.Fatalf("range expression child count = %d, want %d", got, want)
	}
	child := rangeExpr.Child(0)
	if child == nil {
		t.Fatal("range expression child = nil")
	}
	if got, want := child.Type(lang), "..="; got != want {
		t.Fatalf("range expression child type = %q, want %q", got, want)
	}
	if child.IsNamed() {
		t.Fatal("restored ..= child should be anonymous")
	}
}

func TestRustExtractRecoveredTopLevelNodesWithOffsetClonesIntoDestinationArena(t *testing.T) {
	lang := &Language{
		Name:        "rust",
		SymbolNames: []string{"", "source_file", "line_comment", "function_item", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "line_comment", Visible: true, Named: true},
			{Name: "function_item", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
		},
	}
	source := []byte("// c\nfn f() {}\n")
	srcArena := acquireNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(srcArena, 2, true, 0, 4, Point{}, Point{Column: 4})
	ident := newLeafNodeInArena(srcArena, 4, true, 8, 9, Point{Row: 1, Column: 3}, Point{Row: 1, Column: 4})
	fn := newParentNodeInArena(srcArena, 3, true, []*Node{ident}, nil, 0)
	fn.startByte = 5
	fn.endByte = 14
	fn.startPoint = Point{Row: 1}
	fn.endPoint = Point{Row: 1, Column: 9}
	root := newParentNodeInArena(srcArena, 1, true, []*Node{comment, fn}, nil, 0)
	root.startByte = 0
	root.endByte = uint32(len(source))
	root.startPoint = Point{}
	root.endPoint = Point{Row: 2}
	populateParentNode(root, root.children)

	dstArena := acquireNodeArena(arenaClassFull)
	nodes := rustExtractRecoveredTopLevelNodesWithOffset(root, lang, dstArena, 100, Point{Row: 10, Column: 5})
	if got, want := len(nodes), 2; got != want {
		t.Fatalf("recovered node count = %d, want %d", got, want)
	}
	if nodes[0].ownerArena != dstArena || nodes[1].ownerArena != dstArena {
		t.Fatal("recovered nodes were not cloned into destination arena")
	}
	if got, want := nodes[0].StartByte(), uint32(100); got != want {
		t.Fatalf("comment start byte = %d, want %d", got, want)
	}
	if got, want := nodes[0].StartPoint(), (Point{Row: 10, Column: 5}); got != want {
		t.Fatalf("comment start point = %+v, want %+v", got, want)
	}
	if got, want := nodes[1].StartByte(), uint32(105); got != want {
		t.Fatalf("function start byte = %d, want %d", got, want)
	}
	if got, want := nodes[1].StartPoint(), (Point{Row: 11, Column: 0}); got != want {
		t.Fatalf("function start point = %+v, want %+v", got, want)
	}
	child := nodes[1].NamedChild(0)
	if child == nil {
		t.Fatal("function identifier child = nil")
	}
	if got, want := child.StartPoint(), (Point{Row: 11, Column: 3}); got != want {
		t.Fatalf("identifier start point = %+v, want %+v", got, want)
	}
}

func TestRustCanonicalDotRangeBuildsOperatorChildren(t *testing.T) {
	lang := &Language{
		Name:        "rust",
		SymbolNames: []string{"", "range_expression", "..", "..="},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "range_expression", Visible: true, Named: true},
			{Name: "..", Visible: true, Named: false},
			{Name: "..=", Visible: true, Named: false},
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte(".. ..    ..=..")

	node, ok := rustBuildCanonicalDotRangeNode(arena, source, lang, 0, uint32(len(source)))
	if !ok {
		t.Fatal("rustBuildCanonicalDotRangeNode returned false")
	}
	if got, want := node.ChildCount(), 3; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got, want := node.Child(1).Type(lang), "..="; got != want {
		t.Fatalf("operator child type = %q, want %q", got, want)
	}
	if got, want := node.Child(2).Type(lang), "range_expression"; got != want {
		t.Fatalf("right child type = %q, want %q", got, want)
	}
	if got, want := node.Child(2).ChildCount(), 1; got != want {
		t.Fatalf("right child count = %d, want %d", got, want)
	}
	if got, want := node.Child(2).Child(0).Type(lang), ".."; got != want {
		t.Fatalf("right operator child type = %q, want %q", got, want)
	}

	source = []byte("..\n    ..=..")
	node, ok = rustBuildCanonicalDotRangeNode(arena, source, lang, 0, uint32(len(source)))
	if !ok {
		t.Fatal("rustBuildCanonicalDotRangeNode for leading range returned false")
	}
	if got, want := node.Type(lang), "range_expression"; got != want {
		t.Fatalf("leading range root type = %q, want %q", got, want)
	}
	if got, want := node.ChildCount(), 3; got != want {
		t.Fatalf("leading range child count = %d, want %d", got, want)
	}
}

func TestNormalizeRustTokenBindingPatterns(t *testing.T) {
	lang := &Language{
		Name: "rust",
		SymbolNames: []string{
			"",
			"source_file",
			"token_tree_pattern",
			"(",
			"identifier",
			"=>",
			"metavariable",
			":",
			")",
			"token_binding_pattern",
			"fragment_specifier",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: true},
			{Named: true},
			{Named: false},
			{Named: true},
			{Named: false},
			{Named: true},
			{Named: false},
			{Named: false},
			{Named: true},
			{Named: true},
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("(x => $e:expr)")

	tokenTree := newParentNodeInArena(arena, 2, true, []*Node{
		newLeafNodeInArena(arena, 3, false, 0, 1, Point{}, Point{Column: 1}),
		newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2}),
		newLeafNodeInArena(arena, 5, false, 3, 5, Point{Column: 3}, Point{Column: 5}),
		newLeafNodeInArena(arena, 6, true, 6, 8, Point{Column: 6}, Point{Column: 8}),
		newLeafNodeInArena(arena, 7, false, 8, 9, Point{Column: 8}, Point{Column: 9}),
		newLeafNodeInArena(arena, 4, true, 9, 13, Point{Column: 9}, Point{Column: 13}),
		newLeafNodeInArena(arena, 8, false, 13, 14, Point{Column: 13}, Point{Column: 14}),
	}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{tokenTree}, nil, 0)

	normalizeRustTokenBindingPatternsAndRecoveredTokenTrees(root, source, lang)

	pattern := root.Child(0)
	if pattern == nil || pattern.Type(lang) != "token_tree_pattern" {
		t.Fatalf("expected token_tree_pattern child, got %#v", pattern)
	}
	if got, want := pattern.ChildCount(), 5; got != want {
		t.Fatalf("token_tree_pattern child count = %d, want %d", got, want)
	}
	binding := pattern.Child(3)
	if binding == nil || binding.Type(lang) != "token_binding_pattern" {
		t.Fatalf("expected token_binding_pattern, got %s", pattern.SExpr(lang))
	}
	if got, want := binding.ChildCount(), 2; got != want {
		t.Fatalf("token_binding_pattern child count = %d, want %d", got, want)
	}
	if child := binding.Child(1); child == nil || child.Type(lang) != "fragment_specifier" {
		t.Fatalf("expected fragment_specifier child, got %s", binding.SExpr(lang))
	}
}

func TestNormalizeRustRecoveredFunctionItems(t *testing.T) {
	lang := &Language{
		Name: "rust",
		SymbolNames: []string{
			"",
			"source_file",
			"identifier",
			"(",
			"_pattern",
			":",
			"impl",
			"_type",
			"function_item",
			"parameters",
			"parameter",
			"abstract_type",
			"type_parameters",
			"lifetime_parameter",
			"lifetime",
			"generic_type",
			"type_identifier",
			"type_arguments",
			"block",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: true},
			{Named: true},
			{Named: false},
			{Named: false},
			{Named: false},
			{Named: false},
			{Named: false},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
			{Named: true},
		},
	}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("fn foo(bar: impl for<'a> Baz<Quux<'a>>) {}\n")

	root := newParentNodeInArena(arena, errorSymbol, true, []*Node{
		newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0),
		newLeafNodeInArena(arena, 2, true, 3, 6, advancePointByBytes(Point{}, source[:3]), advancePointByBytes(Point{}, source[:6])),
		newLeafNodeInArena(arena, 3, false, 6, 7, advancePointByBytes(Point{}, source[:6]), advancePointByBytes(Point{}, source[:7])),
		newParentNodeInArena(arena, 4, false, []*Node{
			newLeafNodeInArena(arena, 2, true, 7, 10, advancePointByBytes(Point{}, source[:7]), advancePointByBytes(Point{}, source[:10])),
		}, nil, 0),
		newLeafNodeInArena(arena, 5, false, 10, 11, advancePointByBytes(Point{}, source[:10]), advancePointByBytes(Point{}, source[:11])),
		newLeafNodeInArena(arena, 6, false, 12, 16, advancePointByBytes(Point{}, source[:12]), advancePointByBytes(Point{}, source[:16])),
		newParentNodeInArena(arena, 7, false, nil, nil, 0),
		newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0),
	}, nil, 0)
	root.children[0].startByte = 0
	root.children[0].endByte = 2
	root.children[0].startPoint = Point{}
	root.children[0].endPoint = advancePointByBytes(Point{}, source[:2])
	root.children[6].startByte = 17
	root.children[6].endByte = 33
	root.children[6].startPoint = advancePointByBytes(Point{}, source[:17])
	root.children[6].endPoint = advancePointByBytes(Point{}, source[:33])
	root.children[7].startByte = 34
	root.children[7].endByte = 35
	root.children[7].startPoint = advancePointByBytes(Point{}, source[:34])
	root.children[7].endPoint = advancePointByBytes(Point{}, source[:35])
	root.startByte = 0
	root.endByte = 35
	root.startPoint = Point{}
	root.endPoint = advancePointByBytes(Point{}, source[:35])
	populateParentNode(root, root.children)

	normalizeResultCompatibility(root, source, parser)

	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatalf("expected recovered Rust root without error, got %s", root.SExpr(lang))
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	want := "(source_file (function_item (identifier) (parameters (parameter (identifier) (abstract_type (type_parameters (lifetime_parameter (lifetime (identifier)))) (generic_type (type_identifier) (type_arguments (generic_type (type_identifier) (type_arguments (lifetime (identifier))))))))) (block)))"
	if got := root.SExpr(lang); got != want {
		t.Fatalf("recovered Rust SExpr mismatch\nGOT:  %s\nWANT: %s", got, want)
	}
}

func TestNormalizeRustRecoveredPatternStatementsRetagsCleanTopLevelRoot(t *testing.T) {
	lang := &Language{
		Name: "rust",
		SymbolNames: []string{
			"",
			"source_file",
			"line_comment",
			"use_declaration",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Named: true},
			{Named: true},
			{Named: true},
		},
	}
	parser := &Parser{language: lang}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("// doc\nuse foo;\n")
	comment := newLeafNodeInArena(arena, 2, true, 0, 6, Point{}, Point{Column: 6})
	useDecl := newLeafNodeInArena(arena, 3, true, 7, 15, Point{Row: 1}, Point{Row: 1, Column: 8})
	root := newParentNodeInArena(arena, errorSymbol, true, []*Node{comment, useDecl}, nil, 0)
	root.startByte = 0
	root.endByte = uint32(len(source))

	normalizeRustRecoveredPatternStatementsRoot(root, source, parser)

	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatalf("root has error after top-level retag: %s", root.SExpr(lang))
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if root.Child(0) != comment || root.Child(1) != useDecl {
		t.Fatalf("retag should preserve existing top-level children: %s", root.SExpr(lang))
	}
}

func TestRustCompatibilitySourceFlags(t *testing.T) {
	if flags := rustCompatibilitySourceFlagsFor([]byte("let x = 1\n")); flags.collapsedNamedLeafChildren || flags.dotRangeExpressions || flags.docCommentRanges || flags.tokenBindingPatterns || flags.recoveredFunctionItems {
		t.Fatalf("unexpected Rust compatibility flags for plain binding: %+v", flags)
	}
	if flags := rustCompatibilitySourceFlagsFor([]byte("let x = true;")); !flags.collapsedNamedLeafChildren {
		t.Fatalf("expected collapsed leaf flag for true/semicolon source: %+v", flags)
	}
	if flags := rustCompatibilitySourceFlagsFor([]byte("let r = 1..=3;")); !flags.dotRangeExpressions || !flags.collapsedNamedLeafChildren {
		t.Fatalf("expected dot range and collapsed leaf flags for range source: %+v", flags)
	}
	if flags := rustCompatibilitySourceFlagsFor([]byte("/// docs\nfn f() {}\n")); !flags.docCommentRanges || !flags.recoveredFunctionItems {
		t.Fatalf("expected doc comment and recovered function flags: %+v", flags)
	}
	if flags := rustCompatibilitySourceFlagsFor([]byte("macro_rules! m { ($e:expr) => {} }")); !flags.tokenBindingPatterns {
		t.Fatalf("expected token binding flag for macro metavariable source: %+v", flags)
	}
}
