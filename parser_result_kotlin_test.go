package gotreesitter

import "testing"

func kotlinLeadingTriviaTestLanguage() *Language {
	return &Language{
		Name:        "kotlin",
		SymbolNames: []string{"EOF", "source_file", "import_list", "identifier", "simple_identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "import_list", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "simple_identifier", Visible: true, Named: true},
		},
	}
}

func TestNormalizeKotlinCollapsedIdentifierChildren(t *testing.T) {
	lang := kotlinLeadingTriviaTestLanguage()
	// Mirrors `import benchmarks.*`: a single-element identifier must wrap a
	// simple_identifier child, as in C tree-sitter.
	source := []byte("import benchmarks.*")
	arena := newNodeArena(arenaClassFull)
	identifier := newLeafNodeInArena(arena, 3, true, 7, 17, Point{Column: 7}, Point{Column: 17})
	root := newParentNodeInArena(arena, 1, true, []*Node{identifier}, nil, 0)

	normalizeKotlinCompatibility(root, source, lang)

	if got, want := identifier.ChildCount(), 1; got != want {
		t.Fatalf("identifier child count = %d, want %d", got, want)
	}
	child := identifier.Child(0)
	if got, want := child.Type(lang), "simple_identifier"; got != want {
		t.Fatalf("identifier child type = %q, want %q", got, want)
	}
	if child.StartByte() != identifier.StartByte() || child.EndByte() != identifier.EndByte() {
		t.Fatalf("identifier child span = [%d:%d], want [%d:%d]",
			child.StartByte(), child.EndByte(), identifier.StartByte(), identifier.EndByte())
	}
	if !child.IsNamed() {
		t.Fatal("simple_identifier child should be named")
	}
}

func kotlinCallableReferenceTestLanguage() *Language {
	return &Language{
		Name: "kotlin",
		SymbolNames: []string{
			"EOF", "source_file", "navigation_expression", "navigation_suffix",
			"simple_identifier", "callable_reference", "type_identifier", "::", "class",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "navigation_expression", Visible: true, Named: true},
			{Name: "navigation_suffix", Visible: true, Named: true},
			{Name: "simple_identifier", Visible: true, Named: true},
			{Name: "callable_reference", Visible: true, Named: true},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "::", Visible: true, Named: false},
			{Name: "class", Visible: true, Named: false},
		},
	}
}

func TestNormalizeKotlinCallableReferenceNavigations(t *testing.T) {
	lang := kotlinCallableReferenceTestLanguage()
	// Mirrors `Exception::class`: C tree-sitter resolves the ambiguity to
	// callable_reference(type_identifier "::" class).
	source := []byte("Exception::class")
	arena := newNodeArena(arenaClassFull)
	base := newLeafNodeInArena(arena, 4, true, 0, 9, Point{}, Point{Column: 9})
	op := newLeafNodeInArena(arena, 7, false, 9, 11, Point{Column: 9}, Point{Column: 11})
	target := newLeafNodeInArena(arena, 8, false, 11, 16, Point{Column: 11}, Point{Column: 16})
	suffix := newParentNodeInArena(arena, 3, true, []*Node{op, target}, nil, 0)
	nav := newParentNodeInArena(arena, 2, true, []*Node{base, suffix}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{nav}, nil, 0)

	normalizeKotlinCompatibility(root, source, lang)

	if got, want := nav.Type(lang), "callable_reference"; got != want {
		t.Fatalf("node type = %q, want %q", got, want)
	}
	if got, want := nav.ChildCount(), 3; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	wantTypes := []string{"type_identifier", "::", "class"}
	for i, want := range wantTypes {
		if got := nav.Child(i).Type(lang); got != want {
			t.Fatalf("child[%d] type = %q, want %q", i, got, want)
		}
	}
	if p := nav.Child(0).Parent(); p != nav {
		t.Fatal("rewritten child parent not updated")
	}
}

func TestNormalizeKotlinCallableReferenceNavigationsSkipsChainedBase(t *testing.T) {
	lang := kotlinCallableReferenceTestLanguage()
	// Mirrors `a.b::c`: a chained base stays a navigation_expression in C.
	source := []byte("a.b::c")
	arena := newNodeArena(arenaClassFull)
	innerBase := newLeafNodeInArena(arena, 4, true, 0, 3, Point{}, Point{Column: 3})
	op := newLeafNodeInArena(arena, 7, false, 3, 5, Point{Column: 3}, Point{Column: 5})
	target := newLeafNodeInArena(arena, 4, true, 5, 6, Point{Column: 5}, Point{Column: 6})
	suffix := newParentNodeInArena(arena, 3, true, []*Node{op, target}, nil, 0)
	// Base is itself a navigation_expression, not a bare simple_identifier.
	baseNav := newParentNodeInArena(arena, 2, true, []*Node{innerBase}, nil, 0)
	nav := newParentNodeInArena(arena, 2, true, []*Node{baseNav, suffix}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{nav}, nil, 0)

	normalizeKotlinCompatibility(root, source, lang)

	if got, want := nav.Type(lang), "navigation_expression"; got != want {
		t.Fatalf("node type = %q, want %q", got, want)
	}
	if got, want := nav.ChildCount(), 2; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
}

func kotlinReceiverTestLanguage() *Language {
	return &Language{
		Name: "kotlin",
		SymbolNames: []string{
			"EOF", "source_file", "function_declaration", "receiver_type",
			"user_type", "type_identifier", "simple_identifier", ".", "fun",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "function_declaration", Visible: true, Named: true},
			{Name: "receiver_type", Visible: true, Named: true},
			{Name: "user_type", Visible: true, Named: true},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "simple_identifier", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "fun", Visible: true, Named: false},
		},
	}
}

func TestNormalizeKotlinReceiverFunctionNames(t *testing.T) {
	lang := kotlinReceiverTestLanguage()
	// Mirrors `fun A.B.f()` parsed with the dotted path swallowed by the
	// receiver and a zero-width simple_identifier where the name should be.
	source := []byte("fun A.B.f()")
	arena := newNodeArena(arenaClassFull)
	funTok := newLeafNodeInArena(arena, 8, false, 0, 3, Point{}, Point{Column: 3})
	tA := newLeafNodeInArena(arena, 5, true, 4, 5, Point{Column: 4}, Point{Column: 5})
	d1 := newLeafNodeInArena(arena, 7, false, 5, 6, Point{Column: 5}, Point{Column: 6})
	tB := newLeafNodeInArena(arena, 5, true, 6, 7, Point{Column: 6}, Point{Column: 7})
	d2 := newLeafNodeInArena(arena, 7, false, 7, 8, Point{Column: 7}, Point{Column: 8})
	tF := newLeafNodeInArena(arena, 5, true, 8, 9, Point{Column: 8}, Point{Column: 9})
	user := newParentNodeInArena(arena, 4, true, []*Node{tA, d1, tB, d2, tF}, nil, 0)
	recv := newParentNodeInArena(arena, 3, true, []*Node{user}, nil, 0)
	zeroName := newLeafNodeInArena(arena, 6, true, 9, 9, Point{Column: 9}, Point{Column: 9})
	fn := newParentNodeInArena(arena, 2, true, []*Node{funTok, recv, zeroName}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{fn}, nil, 0)

	normalizeKotlinCompatibility(root, source, lang)

	if got, want := fn.ChildCount(), 4; got != want {
		t.Fatalf("function child count = %d, want %d", got, want)
	}
	wantTypes := []string{"fun", "receiver_type", ".", "simple_identifier"}
	for i, want := range wantTypes {
		if got := fn.Child(i).Type(lang); got != want {
			t.Fatalf("child[%d] type = %q, want %q", i, got, want)
		}
	}
	name := fn.Child(3)
	if name.StartByte() != 8 || name.EndByte() != 9 {
		t.Fatalf("name span = [%d:%d], want [8:9]", name.StartByte(), name.EndByte())
	}
	if recv.EndByte() != 7 || user.EndByte() != 7 {
		t.Fatalf("receiver/user end = %d/%d, want 7/7", recv.EndByte(), user.EndByte())
	}
	if got, want := user.ChildCount(), 3; got != want {
		t.Fatalf("user_type child count = %d, want %d", got, want)
	}
	if p := name.Parent(); p != fn {
		t.Fatal("name parent not updated")
	}
}

func TestNormalizeKotlinReceiverFunctionNamesSkipsRealNames(t *testing.T) {
	lang := kotlinReceiverTestLanguage()
	// `fun A.f()` already parsed correctly: a non-zero-width name must not be
	// rewritten.
	source := []byte("fun A.f()")
	arena := newNodeArena(arenaClassFull)
	funTok := newLeafNodeInArena(arena, 8, false, 0, 3, Point{}, Point{Column: 3})
	tA := newLeafNodeInArena(arena, 5, true, 4, 5, Point{Column: 4}, Point{Column: 5})
	user := newParentNodeInArena(arena, 4, true, []*Node{tA}, nil, 0)
	recv := newParentNodeInArena(arena, 3, true, []*Node{user}, nil, 0)
	dot := newLeafNodeInArena(arena, 7, false, 5, 6, Point{Column: 5}, Point{Column: 6})
	name := newLeafNodeInArena(arena, 6, true, 6, 7, Point{Column: 6}, Point{Column: 7})
	fn := newParentNodeInArena(arena, 2, true, []*Node{funTok, recv, dot, name}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{fn}, nil, 0)

	normalizeKotlinCompatibility(root, source, lang)

	if got, want := fn.ChildCount(), 4; got != want {
		t.Fatalf("function child count = %d, want %d", got, want)
	}
	if got, want := user.ChildCount(), 1; got != want {
		t.Fatalf("user_type child count = %d, want %d", got, want)
	}
}

func TestNormalizeKotlinSourceFileLeadingTriviaStart(t *testing.T) {
	lang := kotlinLeadingTriviaTestLanguage()
	// Mirrors CacheRedirector.kt, which begins with a newline: C tree-sitter
	// roots the source_file at byte 1.
	source := []byte("\nimport a.b")
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, true, 1, uint32(len(source)), Point{Row: 1}, Point{Row: 1, Column: 10})
	root := newParentNodeInArena(arena, 1, true, []*Node{child}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}
	root.endByte = uint32(len(source))
	root.endPoint = Point{Row: 1, Column: 10}

	normalizeKotlinCompatibility(root, source, lang)

	if got, want := root.StartByte(), uint32(1); got != want {
		t.Fatalf("root start byte = %d, want %d", got, want)
	}
	if got, want := root.StartPoint(), (Point{Row: 1}); got != want {
		t.Fatalf("root start point = %+v, want %+v", got, want)
	}
}

func TestNormalizeKotlinSourceFileLeadingTriviaStartRejectsNonTrivia(t *testing.T) {
	lang := kotlinLeadingTriviaTestLanguage()
	source := []byte("x import a.b")
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, true, 2, uint32(len(source)), Point{Column: 2}, Point{Column: uint32(len(source))})
	root := newParentNodeInArena(arena, 1, true, []*Node{child}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}

	normalizeKotlinCompatibility(root, source, lang)

	if got, want := root.StartByte(), uint32(0); got != want {
		t.Fatalf("root start byte = %d, want %d", got, want)
	}
}
