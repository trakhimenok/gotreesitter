package gotreesitter

import (
	"testing"
)

func TestPythonSourceMayContainFString(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "plain", src: `{"key": "value"}`, want: false},
		{name: "f single", src: `f'{x}'`, want: true},
		{name: "f double", src: `f"{x}"`, want: true},
		{name: "raw f", src: `rf"{x}"`, want: true},
		{name: "f raw", src: `Fr"{x}"`, want: true},
		{name: "ordinary raw", src: `r"{x}"`, want: false},
		{name: "identifier suffix", src: `self"{x}"`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pythonSourceMayContainFString([]byte(tt.src)); got != tt.want {
				t.Fatalf("pythonSourceMayContainFString(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestBuildResultFromNodesCollapsesPythonTerminalIfSuffix(t *testing.T) {
	lang := &Language{
		Name:       "python",
		FieldNames: []string{"", "condition", "consequence"},
		SymbolNames: []string{
			"",
			"module",
			"class_definition",
			"if",
			"comparison_operator",
			":",
			"_indent",
			"_simple_statements",
			"_dedent",
			"block",
			"if_statement",
			"expression_statement",
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)

	source := mustReadParserResultFixture(t, "python/terminal_if_suffix.py")

	classNode := newLeafNodeInArena(arena, 2, true, 0, 17, Point{}, Point{Row: 1, Column: 8})
	ifNode := newLeafNodeInArena(arena, 3, false, 18, 20, Point{Row: 3, Column: 0}, Point{Row: 3, Column: 2})
	condNode := newLeafNodeInArena(arena, 4, true, 21, 43, Point{Row: 3, Column: 3}, Point{Row: 3, Column: 25})
	colonNode := newLeafNodeInArena(arena, 5, false, 43, 44, Point{Row: 3, Column: 25}, Point{Row: 3, Column: 26})
	indentNode := newLeafNodeInArena(arena, 6, false, 44, 49, Point{Row: 3, Column: 26}, Point{Row: 4, Column: 4})
	exprNode := newLeafNodeInArena(arena, 11, true, 49, 64, Point{Row: 4, Column: 4}, Point{Row: 4, Column: 19})
	simpleNode := newParentNodeInArena(arena, 7, false, []*Node{exprNode}, nil, 0)
	dedentNode := newLeafNodeInArena(arena, 8, false, 65, 65, Point{Row: 5, Column: 0}, Point{Row: 5, Column: 0})

	tree := parser.buildResultFromNodes([]*Node{
		classNode,
		ifNode,
		condNode,
		colonNode,
		indentNode,
		simpleNode,
		dedentNode,
	}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if root.HasError() {
		t.Fatalf("expected collapsed Python root without error, got %s", root.SExpr(lang))
	}
	if got, want := root.NamedChildCount(), 2; got != want {
		t.Fatalf("named child count = %d, want %d in %s", got, want, root.SExpr(lang))
	}
	stmt := root.NamedChild(1)
	if stmt == nil || stmt.Type(lang) != "if_statement" {
		t.Fatalf("expected trailing if_statement, got %s", root.SExpr(lang))
	}
	if got, want := stmt.fieldIDs[1], FieldID(1); got != want {
		t.Fatalf("if condition field = %d, want %d", got, want)
	}
	if got, want := stmt.fieldIDs[3], FieldID(2); got != want {
		t.Fatalf("if consequence field = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesCollapsesPythonTerminalClassAndIfSuffix(t *testing.T) {
	lang := &Language{
		Name:       "python",
		FieldNames: []string{"", "name", "superclasses", "body", "condition", "consequence"},
		SymbolNames: []string{
			"",
			"module",
			"class",
			"identifier",
			"argument_list",
			":",
			"_indent",
			"_simple_statements",
			"_dedent",
			"block",
			"if",
			"expression",
			"if_statement",
			"class_definition",
			"module_repeat1",
			"function_definition",
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)

	source := mustReadParserResultFixture(t, "python/terminal_class_and_if_suffix.py")

	classKw := newLeafNodeInArena(arena, 2, false, 0, 5, Point{}, Point{Column: 5})
	className := newLeafNodeInArena(arena, 3, true, 6, 18, Point{Column: 6}, Point{Column: 18})
	argList := newLeafNodeInArena(arena, 4, true, 18, 37, Point{Column: 18}, Point{Column: 37})
	classColon := newLeafNodeInArena(arena, 5, false, 37, 38, Point{Column: 37}, Point{Column: 38})
	classIndent := newLeafNodeInArena(arena, 6, false, 38, 43, Point{Column: 38}, Point{Row: 1, Column: 4})
	fn := newLeafNodeInArena(arena, 15, true, 43, 77, Point{Row: 1, Column: 4}, Point{Row: 2, Column: 12})
	repeat := newParentNodeInArena(arena, 14, false, []*Node{fn}, nil, 0)
	ifKw := newLeafNodeInArena(arena, 10, false, 79, 81, Point{Row: 4, Column: 0}, Point{Row: 4, Column: 2})
	cond := newLeafNodeInArena(arena, 11, true, 82, 104, Point{Row: 4, Column: 3}, Point{Row: 4, Column: 25})
	ifColon := newLeafNodeInArena(arena, 5, false, 104, 105, Point{Row: 4, Column: 25}, Point{Row: 4, Column: 26})
	ifIndent := newLeafNodeInArena(arena, 6, false, 105, 110, Point{Row: 4, Column: 26}, Point{Row: 5, Column: 4})
	bodyStmt := newLeafNodeInArena(arena, 7, true, 110, 125, Point{Row: 5, Column: 4}, Point{Row: 5, Column: 19})
	dedent := newLeafNodeInArena(arena, 8, false, 126, 126, Point{Row: 6, Column: 0}, Point{Row: 6, Column: 0})

	tree := parser.buildResultFromNodes([]*Node{
		classKw,
		className,
		argList,
		classColon,
		classIndent,
		repeat,
		ifKw,
		cond,
		ifColon,
		ifIndent,
		bodyStmt,
		dedent,
		newLeafNodeInArena(arena, 99, false, 126, 126, Point{Row: 6, Column: 0}, Point{Row: 6, Column: 0}),
	}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if root.HasError() {
		t.Fatalf("expected collapsed Python root without error, got %s", root.SExpr(lang))
	}
	if got, want := root.NamedChildCount(), 2; got != want {
		t.Fatalf("named child count = %d, want %d in %s", got, want, root.SExpr(lang))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(lang) != "class_definition" {
		t.Fatalf("expected leading class_definition, got %s", root.SExpr(lang))
	}
	if got, want := classDef.fieldIDs[1], FieldID(1); got != want {
		t.Fatalf("class name field = %d, want %d", got, want)
	}
	if got, want := classDef.fieldIDs[2], FieldID(2); got != want {
		t.Fatalf("class superclasses field = %d, want %d", got, want)
	}
	if got, want := classDef.fieldIDs[4], FieldID(3); got != want {
		t.Fatalf("class body field = %d, want %d", got, want)
	}
	stmt := root.NamedChild(1)
	if stmt == nil || stmt.Type(lang) != "if_statement" {
		t.Fatalf("expected trailing if_statement, got %s", root.SExpr(lang))
	}
	if got, want := stmt.fieldIDs[1], FieldID(4); got != want {
		t.Fatalf("if condition field = %d, want %d", got, want)
	}
	if got, want := stmt.fieldIDs[3], FieldID(5); got != want {
		t.Fatalf("if consequence field = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesRepairsPythonIfWrappers(t *testing.T) {
	lang := &Language{
		Name:       "python",
		FieldNames: []string{"", "condition", "consequence"},
		SymbolNames: []string{
			"",
			"module",
			"if_statement",
			"if",
			"expression",
			"comparison_operator",
			":",
			"block",
			"_indent",
			"_simple_statements",
			"expression_statement",
			"primary_expression",
			"call",
			"_dedent",
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)

	condLeaf := newLeafNodeInArena(arena, 5, true, 3, 25, Point{Column: 3}, Point{Column: 25})
	condExpr := newParentNodeInArena(arena, 4, true, []*Node{condLeaf}, nil, 0)
	callLeaf := newLeafNodeInArena(arena, 12, true, 30, 45, Point{Row: 1, Column: 4}, Point{Row: 1, Column: 19})
	primary := newParentNodeInArena(arena, 11, true, []*Node{callLeaf}, nil, 0)
	expr := newParentNodeInArena(arena, 4, true, []*Node{primary}, nil, 0)
	stmt := newParentNodeInArena(arena, 10, true, []*Node{expr}, nil, 0)
	body := newParentNodeInArena(arena, 7, true, []*Node{
		newLeafNodeInArena(arena, 8, false, 26, 26, Point{Column: 26}, Point{Column: 26}),
		newParentNodeInArena(arena, 9, false, []*Node{stmt}, nil, 0),
		newLeafNodeInArena(arena, 13, false, 45, 45, Point{Row: 1, Column: 19}, Point{Row: 1, Column: 19}),
	}, nil, 0)
	ifStmt := newParentNodeInArena(arena, 2, true, []*Node{
		newLeafNodeInArena(arena, 3, false, 0, 2, Point{}, Point{Column: 2}),
		condExpr,
		newLeafNodeInArena(arena, 6, false, 25, 26, Point{Column: 25}, Point{Column: 26}),
		body,
	}, []FieldID{0, 1, 0, 2}, 0)
	module := newParentNodeInArena(arena, 1, true, []*Node{ifStmt}, nil, 0)

	tree := parser.buildResultFromNodes([]*Node{module}, []byte("if x == y:\n    f()\n"), arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	stmtNode := root.NamedChild(0)
	if stmtNode == nil || stmtNode.Type(lang) != "if_statement" {
		t.Fatalf("expected if_statement root child, got %s", root.SExpr(lang))
	}
	cond := stmtNode.Child(1)
	if cond == nil || cond.Type(lang) != "comparison_operator" {
		t.Fatalf("expected condition to unwrap to comparison_operator, got %s", root.SExpr(lang))
	}
	block := stmtNode.Child(3)
	if block == nil || block.Type(lang) != "block" {
		t.Fatalf("expected block consequence, got %s", root.SExpr(lang))
	}
	call := block.NamedChild(0)
	if call == nil || call.Type(lang) != "call" {
		t.Fatalf("expected block child to unwrap to call, got %s", root.SExpr(lang))
	}
	if got, want := block.startByte, call.startByte; got != want {
		t.Fatalf("block startByte = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesRepairsPythonBlockRangeWithoutWrapperChanges(t *testing.T) {
	lang := &Language{
		Name:       "python",
		FieldNames: []string{"", "condition", "consequence"},
		SymbolNames: []string{
			"",
			"module",
			"if_statement",
			"if",
			"comparison_operator",
			":",
			"block",
			"_indent",
			"call",
			"_dedent",
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)

	callLeaf := newLeafNodeInArena(arena, 8, true, 30, 45, Point{Row: 1, Column: 4}, Point{Row: 1, Column: 19})
	body := newParentNodeInArena(arena, 6, true, []*Node{
		newLeafNodeInArena(arena, 7, true, 26, 26, Point{Column: 26}, Point{Column: 26}),
		callLeaf,
		newLeafNodeInArena(arena, 9, true, 45, 45, Point{Row: 1, Column: 19}, Point{Row: 1, Column: 19}),
	}, nil, 0)
	ifStmt := newParentNodeInArena(arena, 2, true, []*Node{
		newLeafNodeInArena(arena, 3, false, 0, 2, Point{}, Point{Column: 2}),
		newLeafNodeInArena(arena, 4, true, 3, 25, Point{Column: 3}, Point{Column: 25}),
		newLeafNodeInArena(arena, 5, false, 25, 26, Point{Column: 25}, Point{Column: 26}),
		body,
	}, []FieldID{0, 1, 0, 2}, 0)
	module := newParentNodeInArena(arena, 1, true, []*Node{ifStmt}, nil, 0)

	tree := parser.buildResultFromNodes([]*Node{module}, []byte("if x:\n    f()\n"), arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	block := tree.RootNode().NamedChild(0).Child(3)
	if block == nil || block.Type(lang) != "block" {
		t.Fatalf("expected block consequence, got %s", tree.RootNode().SExpr(lang))
	}
	if got, want := block.ChildCount(), 1; got != want {
		t.Fatalf("block child count = %d, want %d", got, want)
	}
	if got, want := block.startByte, callLeaf.startByte; got != want {
		t.Fatalf("block startByte = %d, want %d", got, want)
	}
}

func TestBuildResultFromNodesRepairsPythonBlockEndToTrailingPunctuation(t *testing.T) {
	lang := &Language{
		Name:       "python",
		FieldNames: []string{"", "body"},
		SymbolNames: []string{
			"",
			"module",
			"class_definition",
			"class",
			"identifier",
			":",
			"block",
			"function_definition",
			"def",
			"parameters",
			"assignment",
			";",
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)

	semi := newLeafNodeInArena(arena, 11, false, 34, 35, Point{Row: 2, Column: 5}, Point{Row: 2, Column: 6})
	fnBlock := newParentNodeInArena(arena, 6, true, []*Node{
		newLeafNodeInArena(arena, 10, true, 29, 34, Point{Row: 2, Column: 0}, Point{Row: 2, Column: 5}),
		semi,
	}, nil, 0)
	fn := newParentNodeInArena(arena, 7, true, []*Node{
		newLeafNodeInArena(arena, 8, false, 8, 11, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 3}),
		newLeafNodeInArena(arena, 4, true, 12, 15, Point{Row: 1, Column: 4}, Point{Row: 1, Column: 7}),
		newLeafNodeInArena(arena, 9, true, 15, 17, Point{Row: 1, Column: 7}, Point{Row: 1, Column: 9}),
		newLeafNodeInArena(arena, 5, false, 17, 18, Point{Row: 1, Column: 9}, Point{Row: 1, Column: 10}),
		fnBlock,
	}, []FieldID{0, 0, 0, 0, 1}, 0)
	classBlock := newParentNodeInArena(arena, 6, true, []*Node{fn}, nil, 0)
	classDef := newParentNodeInArena(arena, 2, true, []*Node{
		newLeafNodeInArena(arena, 3, false, 0, 5, Point{}, Point{Column: 5}),
		newLeafNodeInArena(arena, 4, true, 6, 7, Point{Column: 6}, Point{Column: 7}),
		newLeafNodeInArena(arena, 5, false, 7, 8, Point{Column: 7}, Point{Column: 8}),
		classBlock,
	}, []FieldID{0, 0, 0, 1}, 0)
	module := newParentNodeInArena(arena, 1, true, []*Node{classDef}, nil, 0)

	tree := parser.buildResultFromNodes([]*Node{module}, []byte("class T:\ndef f():\nx=1;\n"), arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	gotFn := tree.RootNode().NamedChild(0).Child(3).NamedChild(0)
	if gotFn == nil || gotFn.Type(lang) != "function_definition" {
		t.Fatalf("expected function_definition, got %s", tree.RootNode().SExpr(lang))
	}
	if got, want := gotFn.endByte, semi.endByte; got != want {
		t.Fatalf("function_definition endByte = %d, want %d", got, want)
	}
}

func TestRepairPythonBlockPreservesOriginalEndAfterTrailingSemicolon(t *testing.T) {
	lang := &Language{
		Name: "python",
		SymbolNames: []string{
			"",
			"block",
			"assignment",
			";",
			"_indent",
		},
	}
	arena := acquireNodeArena(arenaClassFull)

	assign := newLeafNodeInArena(arena, 2, true, 10, 15, Point{Row: 1, Column: 8}, Point{Row: 1, Column: 13})
	semi := newLeafNodeInArena(arena, 3, false, 15, 16, Point{Row: 1, Column: 13}, Point{Row: 1, Column: 14})
	indent := newLeafNodeInArena(arena, 4, false, 9, 10, Point{Row: 1, Column: 7}, Point{Row: 1, Column: 8})
	block := newParentNodeInArena(arena, 1, true, []*Node{indent, assign, semi}, nil, 0)
	block.endByte = 28
	block.endPoint = Point{Row: 2, Column: 10}

	repaired, changed := repairPythonBlock(block, arena, lang, false)
	if !changed {
		t.Fatal("expected repairPythonBlock to preserve extended original end")
	}
	if repaired == nil || repaired.Type(lang) != "block" {
		t.Fatalf("expected repaired block, got %#v", repaired)
	}
	if got, want := repaired.endByte, block.endByte; got != want {
		t.Fatalf("block endByte = %d, want %d", got, want)
	}
	if got, want := repaired.endPoint, block.endPoint; got != want {
		t.Fatalf("block endPoint = %+v, want %+v", got, want)
	}
}

func TestNormalizePythonTrailingSelfCallsFoldsIntoNestedFunctionBlock(t *testing.T) {
	lang := &Language{
		Name: "python",
		SymbolNames: []string{
			"",
			"module",
			"block",
			"function_definition",
			"identifier",
			"parameters",
			"comment",
			"assignment",
			";",
			"call",
			"argument_list",
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	source := mustReadParserResultFixture(t, "python/trailing_self_call.py")

	fnName := newLeafNodeInArena(arena, 4, true, 8, 11, Point{Column: 8}, Point{Column: 11})
	body := newParentNodeInArena(arena, 2, true, []*Node{
		newLeafNodeInArena(arena, 7, true, 23, 28, Point{Row: 1, Column: 8}, Point{Row: 1, Column: 13}),
		newLeafNodeInArena(arena, 8, false, 28, 29, Point{Row: 1, Column: 13}, Point{Row: 1, Column: 14}),
	}, nil, 0)
	fn := newParentNodeInArena(arena, 3, true, []*Node{
		fnName,
		newParentNodeInArena(arena, 5, true, nil, nil, 0),
		newLeafNodeInArena(arena, 6, true, 15, 18, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 3}),
		body,
	}, nil, 0)
	fn.startByte = 4
	fn.startPoint = Point{Column: 4}
	call := newParentNodeInArena(arena, 9, true, []*Node{
		newLeafNodeInArena(arena, 4, true, 34, 37, Point{Row: 2, Column: 4}, Point{Row: 2, Column: 7}),
		newParentNodeInArena(arena, 10, true, nil, nil, 0),
	}, nil, 0)
	outerBlock := newParentNodeInArena(arena, 2, true, []*Node{fn, call}, nil, 0)
	module := newParentNodeInArena(arena, 1, true, []*Node{outerBlock}, nil, 0)

	normalizePythonTrailingSelfCalls(module, source, lang)

	block := module.Child(0)
	if block == nil || block.Type(lang) != "block" {
		t.Fatalf("expected block child, got %#v", block)
	}
	if got, want := block.ChildCount(), 1; got != want {
		t.Fatalf("outer block child count = %d, want %d", got, want)
	}
	gotFn := block.Child(0)
	if gotFn == nil || gotFn.Type(lang) != "function_definition" {
		t.Fatalf("expected function_definition child, got %s", block.SExpr(lang))
	}
	gotBody := gotFn.Child(gotFn.ChildCount() - 1)
	if gotBody == nil || gotBody.Type(lang) != "block" {
		t.Fatalf("expected nested block, got %s", gotFn.SExpr(lang))
	}
	last := gotBody.Child(gotBody.ChildCount() - 1)
	if last == nil || last.Type(lang) != "call" {
		t.Fatalf("expected trailing call folded into function body, got %s", gotBody.SExpr(lang))
	}
}

func TestPythonSourceMayContainFStringPatternNormalization(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "simple interpolation", src: `f"{name}"`, want: false},
		{name: "debug interpolation", src: `f"{value=}"`, want: false},
		{name: "tuple interpolation", src: `f"{a, b}"`, want: true},
		{name: "splat interpolation", src: `f"{*items}"`, want: true},
		{name: "literal braces", src: `f"{{a, b}}"`, want: false},
		{name: "non f string", src: `"regular {a, b}"`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pythonSourceMayContainFStringPatternNormalization([]byte(tt.src)); got != tt.want {
				t.Fatalf("pythonSourceMayContainFStringPatternNormalization(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestNormalizePythonCompatibilityRecordsRuntimeStats(t *testing.T) {
	lang := &Language{
		Name: "python",
		SymbolNames: []string{
			"",
			"module",
		},
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "module", Visible: true, Named: true},
		},
	}
	parser := &Parser{}
	root := newParentNodeInArena(nil, 1, true, nil, nil, 0)

	normalizePythonCompatibilityWithParser(root, []byte(";"), parser, lang)

	stats := parser.normalizationStats
	if got, want := stats.passesChecked, uint64(5); got != want {
		t.Fatalf("passesChecked = %d, want %d", got, want)
	}
	if got, want := stats.passesRun, uint64(1); got != want {
		t.Fatalf("passesRun = %d, want %d", got, want)
	}
	if got, want := stats.nodesVisited, uint64(1); got != want {
		t.Fatalf("nodesVisited = %d, want %d", got, want)
	}
	if stats.nodesRewritten != 0 {
		t.Fatalf("nodesRewritten = %d, want 0", stats.nodesRewritten)
	}
}

func TestPythonCompatibilitySourceGatesPreferCodeTokens(t *testing.T) {
	if pythonSourceMayContainCodeByte([]byte(`x = ";"; y = 1`), ';') != true {
		t.Fatal("expected code semicolon after string literal")
	}
	if flags := pythonCompatibilitySourceFlagsFor([]byte(`x = ";"; y = 1`)); !flags.trailingSelfCalls {
		t.Fatal("expected combined flags to detect code semicolon after string literal")
	}
	if pythonSourceMayContainCodeByte([]byte("\";\"\n# ;\n"), ';') {
		t.Fatal("did not expect semicolon inside string/comment to pass code gate")
	}
	if flags := pythonCompatibilitySourceFlagsFor([]byte("\";\"\n# ;\n")); flags.trailingSelfCalls {
		t.Fatal("did not expect combined flags to detect semicolon inside string/comment")
	}
	if !pythonSourceMayContainPrintChevron([]byte(`print >>sys.stderr, "x"`)) {
		t.Fatal("expected print-chevron statement gate")
	}
	if flags := pythonCompatibilitySourceFlagsFor([]byte(`print >>sys.stderr, "x"`)); !flags.printChevron {
		t.Fatal("expected combined flags to detect print-chevron statement")
	}
	if pythonSourceMayContainPrintChevron([]byte("\"print >> x\"\nprint_value = 1\nx >> 1")) {
		t.Fatal("did not expect split print/chevron occurrences to pass gate")
	}
	if flags := pythonCompatibilitySourceFlagsFor([]byte("\"print >> x\"\nprint_value = 1\nx >> 1")); flags.printChevron {
		t.Fatal("did not expect combined flags to detect split print/chevron occurrences")
	}
	if !pythonSourceMayContainCodeWord([]byte("if ok:\n    pass\n"), "pass") {
		t.Fatal("expected pass statement code word")
	}
	if flags := pythonCompatibilitySourceFlagsFor([]byte("if ok:\n    pass\n")); !flags.passWord {
		t.Fatal("expected combined flags to detect pass statement code word")
	}
	if pythonSourceMayContainCodeWord([]byte("\"may pass NULL\"\npassword = 1\n# pass\n"), "pass") {
		t.Fatal("did not expect pass inside string/comment or identifier to pass code gate")
	}
	if flags := pythonCompatibilitySourceFlagsFor([]byte("\"may pass NULL\"\npassword = 1\n# pass\n")); flags.passWord {
		t.Fatal("did not expect combined flags to detect pass inside string/comment or identifier")
	}
	if flags := pythonCompatibilitySourceFlagsFor([]byte(`f"{a, b}"`)); !flags.fStringPattern {
		t.Fatal("expected combined flags to detect f-string pattern normalization")
	}
	if flags := pythonCompatibilitySourceFlagsFor([]byte(`"regular {a, b}"`)); flags.fStringPattern {
		t.Fatal("did not expect combined flags to detect non-f-string pattern normalization")
	}
	if flags := pythonCompatibilitySourceFlagsFor([]byte("x = \"a\\\nb\"")); !flags.continuationEscape {
		t.Fatal("expected combined flags to detect continuation escape")
	}
}

func TestBuildResultFromNodesUnwrapsPythonModuleSimpleStatements(t *testing.T) {
	lang := &Language{
		Name: "python",
		SymbolNames: []string{
			"",
			"module",
			"_simple_statements",
			"import_from_statement",
			"comment",
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)

	source := mustReadParserResultFixture(t, "python/module_import_from.py")
	comment := newLeafNodeInArena(arena, 4, true, 0, 5, Point{}, Point{Column: 5})
	stmt := newLeafNodeInArena(arena, 3, true, 6, 21, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 15})
	wrapped := newParentNodeInArena(arena, 2, true, []*Node{stmt}, nil, 0)

	tree := parser.buildResultFromNodes([]*Node{comment, wrapped}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if root.HasError() {
		t.Fatalf("expected normalized Python root without error, got %s", root.SExpr(lang))
	}
	if got, want := root.NamedChildCount(), 2; got != want {
		t.Fatalf("named child count = %d, want %d in %s", got, want, root.SExpr(lang))
	}
	if child := root.NamedChild(1); child == nil || child.Type(lang) != "import_from_statement" {
		t.Fatalf("expected unwrapped import_from_statement, got %s", root.SExpr(lang))
	}
}

func TestBuildResultFromNodesUnwrapsPythonModuleAssignmentStatements(t *testing.T) {
	lang := &Language{
		Name: "python",
		SymbolNames: []string{
			"",
			"module",
			"_simple_statements",
			"expression_statement",
			"assignment",
			"comment",
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)

	source := mustReadParserResultFixture(t, "python/module_assignment.py")
	comment := newLeafNodeInArena(arena, 5, true, 0, 5, Point{}, Point{Column: 5})
	assign := newLeafNodeInArena(arena, 4, true, 6, 11, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 5})
	expr := newParentNodeInArena(arena, 3, true, []*Node{assign}, nil, 0)
	wrapped := newParentNodeInArena(arena, 2, true, []*Node{expr}, nil, 0)

	tree := parser.buildResultFromNodes([]*Node{comment, wrapped}, source, arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("buildResultFromNodes returned nil root")
	}
	if root.HasError() {
		t.Fatalf("expected normalized Python root without error, got %s", root.SExpr(lang))
	}
	if child := root.NamedChild(1); child == nil || child.Type(lang) != "assignment" {
		t.Fatalf("expected unwrapped assignment, got %s", root.SExpr(lang))
	}
}

func TestBuildResultFromNodesHoistsPythonClassSiblingsOutOfFunctionBody(t *testing.T) {
	lang := &Language{
		Name:       "python",
		FieldNames: []string{"", "body"},
		SymbolNames: []string{
			"",
			"module",
			"class_definition",
			"class",
			"identifier",
			":",
			"block",
			"function_definition",
			"def",
			"parameters",
			"assignment",
			"comment",
		},
	}
	parser := &Parser{
		language:      lang,
		rootSymbol:    1,
		hasRootSymbol: true,
	}
	arena := acquireNodeArena(arenaClassFull)

	assign := newLeafNodeInArena(arena, 10, true, 40, 50, Point{Row: 2, Column: 8}, Point{Row: 2, Column: 18})
	escapedComment := newLeafNodeInArena(arena, 11, true, 51, 60, Point{Row: 3, Column: 4}, Point{Row: 3, Column: 13})
	nextFnBody := newLeafNodeInArena(arena, 10, true, 90, 100, Point{Row: 5, Column: 8}, Point{Row: 5, Column: 18})
	nextFnBlock := newParentNodeInArena(arena, 6, true, []*Node{nextFnBody}, nil, 0)
	nextFn := newParentNodeInArena(arena, 7, true, []*Node{
		newLeafNodeInArena(arena, 8, false, 61, 64, Point{Row: 4, Column: 4}, Point{Row: 4, Column: 7}),
		newLeafNodeInArena(arena, 4, true, 65, 72, Point{Row: 4, Column: 8}, Point{Row: 4, Column: 15}),
		newLeafNodeInArena(arena, 9, true, 72, 78, Point{Row: 4, Column: 15}, Point{Row: 4, Column: 21}),
		newLeafNodeInArena(arena, 5, false, 78, 79, Point{Row: 4, Column: 21}, Point{Row: 4, Column: 22}),
		nextFnBlock,
	}, []FieldID{0, 0, 0, 0, 1}, 0)

	firstFnBlock := newParentNodeInArena(arena, 6, true, []*Node{assign, escapedComment, nextFn}, nil, 0)
	firstFn := newParentNodeInArena(arena, 7, true, []*Node{
		newLeafNodeInArena(arena, 8, false, 9, 12, Point{Row: 1, Column: 4}, Point{Row: 1, Column: 7}),
		newLeafNodeInArena(arena, 4, true, 13, 18, Point{Row: 1, Column: 8}, Point{Row: 1, Column: 13}),
		newLeafNodeInArena(arena, 9, true, 18, 24, Point{Row: 1, Column: 13}, Point{Row: 1, Column: 19}),
		newLeafNodeInArena(arena, 5, false, 24, 25, Point{Row: 1, Column: 19}, Point{Row: 1, Column: 20}),
		firstFnBlock,
	}, []FieldID{0, 0, 0, 0, 1}, 0)

	classBlock := newParentNodeInArena(arena, 6, true, []*Node{firstFn}, nil, 0)
	classDef := newParentNodeInArena(arena, 2, true, []*Node{
		newLeafNodeInArena(arena, 3, false, 0, 5, Point{}, Point{Column: 5}),
		newLeafNodeInArena(arena, 4, true, 6, 7, Point{Column: 6}, Point{Column: 7}),
		newLeafNodeInArena(arena, 5, false, 7, 8, Point{Column: 7}, Point{Column: 8}),
		classBlock,
	}, []FieldID{0, 0, 0, 1}, 0)
	module := newParentNodeInArena(arena, 1, true, []*Node{classDef}, nil, 0)

	tree := parser.buildResultFromNodes([]*Node{module}, []byte("class T:\n    def first():\n        pass\n"), arena, nil, nil, nil)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	classNode := root.NamedChild(0)
	if classNode == nil || classNode.Type(lang) != "class_definition" {
		t.Fatalf("expected class_definition root child, got %s", root.SExpr(lang))
	}
	block := classNode.Child(3)
	if block == nil || block.Type(lang) != "block" {
		t.Fatalf("expected class block, got %s", root.SExpr(lang))
	}
	if got, want := block.NamedChildCount(), 3; got != want {
		t.Fatalf("class block named child count = %d, want %d in %s", got, want, root.SExpr(lang))
	}
	first := block.NamedChild(0)
	if first == nil || first.Type(lang) != "function_definition" {
		t.Fatalf("expected first child to stay a function_definition, got %s", root.SExpr(lang))
	}
	firstBody := first.Child(4)
	if firstBody == nil || firstBody.Type(lang) != "block" {
		t.Fatalf("expected first function body, got %s", root.SExpr(lang))
	}
	if got, want := firstBody.NamedChildCount(), 1; got != want {
		t.Fatalf("first function body named child count = %d, want %d in %s", got, want, root.SExpr(lang))
	}
	if block.NamedChild(2) == nil || block.NamedChild(2).Type(lang) != "function_definition" {
		t.Fatalf("expected hoisted trailing function_definition, got %s", root.SExpr(lang))
	}
}
