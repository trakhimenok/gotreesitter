package gotreesitter

import "testing"

func TestNormalizeKotlinInterpolatedCallExpressionWrapsCallSuffix(t *testing.T) {
	lang := testKotlinCompatibilityLanguage()
	arena := newNodeArena(arenaClassFull)
	navigation := newLeafNodeInArena(arena, 3, true, 0, 11, Point{}, Point{Column: 11})
	callSuffix := newLeafNodeInArena(arena, 4, true, 11, 13, Point{Column: 11}, Point{Column: 13})
	interpolated := newParentNodeInArena(arena, 2, true, []*Node{navigation, callSuffix}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{interpolated}, nil, 0)

	normalizeKotlinCompatibility(root, []byte("Instant.now()"), lang)

	if got, want := interpolated.ChildCount(), 1; got != want {
		t.Fatalf("interpolated_expression child count = %d, want %d", got, want)
	}
	call := interpolated.Child(0)
	if call == nil || call.Type(lang) != "call_expression" {
		t.Fatalf("interpolated_expression child = %v, want call_expression", call)
	}
	if got, want := call.StartByte(), uint32(0); got != want {
		t.Fatalf("call_expression.StartByte() = %d, want %d", got, want)
	}
	if got, want := call.EndByte(), uint32(13); got != want {
		t.Fatalf("call_expression.EndByte() = %d, want %d", got, want)
	}
	if got, want := call.ChildCount(), 2; got != want {
		t.Fatalf("call_expression child count = %d, want %d", got, want)
	}
	if got := call.Child(0); got != navigation {
		t.Fatalf("call_expression child[0] = %v, want original navigation_expression", got)
	}
	if got := call.Child(1); got != callSuffix {
		t.Fatalf("call_expression child[1] = %v, want original call_suffix", got)
	}
	if navigation.Parent() != call {
		t.Fatal("navigation_expression parent was not updated to call_expression")
	}
	if callSuffix.Parent() != call {
		t.Fatal("call_suffix parent was not updated to call_expression")
	}
}

func testKotlinCompatibilityLanguage() *Language {
	return &Language{
		Name: "kotlin",
		SymbolNames: []string{
			"EOF",
			"source_file",
			"interpolated_expression",
			"navigation_expression",
			"call_suffix",
			"call_expression",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "source_file", Visible: true, Named: true},
			{Name: "interpolated_expression", Visible: true, Named: true},
			{Name: "navigation_expression", Visible: true, Named: true},
			{Name: "call_suffix", Visible: true, Named: true},
			{Name: "call_expression", Visible: true, Named: true},
		},
	}
}
