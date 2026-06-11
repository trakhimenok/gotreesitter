package gotreesitter

import "testing"

func newRTestLanguage() *Language {
	return &Language{
		Name: "r",
		SymbolNames: []string{
			"EOF", "program", "string", "'", "\"", "string_content", "escape_sequence",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "program", Visible: true, Named: true},
			{Name: "string", Visible: true, Named: true},
			{Name: "'", Visible: true},
			{Name: "\"", Visible: true},
			{Name: "string_content", Visible: true, Named: true},
			{Name: "escape_sequence", Visible: true, Named: true},
		},
	}
}

// buildRString assembles a string node: open quote, optional middle child,
// close quote, wrapped in a program root.
func buildRString(arena *nodeArena, quoteSym Symbol, openStart, contentEnd uint32, mid *Node) (*Node, *Node) {
	open := newLeafNodeInArena(arena, quoteSym, false, openStart, openStart+1, Point{Column: openStart}, Point{Column: openStart + 1})
	close := newLeafNodeInArena(arena, quoteSym, false, contentEnd, contentEnd+1, Point{Column: contentEnd}, Point{Column: contentEnd + 1})
	children := []*Node{open}
	if mid != nil {
		children = append(children, mid)
	}
	children = append(children, close)
	str := newParentNodeInArena(arena, 2, true, children, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{str}, nil, 0)
	return root, str
}

// Collapsed single-escape content: Go aliases the hidden content rule onto
// its only escape_sequence — string_content spans just the escape with no
// child. C spans the whole content with the escape as a child.
func TestNormalizeRStringContentSingleEscapeRebuilt(t *testing.T) {
	lang := newRTestLanguage()
	// `".\\1"` — content [1:5] = `.` `\` `\` `1`, escape `\\` at [2:4]
	source := []byte(`".\\1"`)
	arena := newNodeArena(arenaClassFull)
	mid := newLeafNodeInArena(arena, 5, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	_, str := buildRString(arena, 4, 0, 5, mid)

	normalizeRCompatibility(str.parent, source, lang)

	if got, want := mid.StartByte(), uint32(1); got != want {
		t.Fatalf("string_content start = %d, want %d", got, want)
	}
	if got, want := mid.EndByte(), uint32(5); got != want {
		t.Fatalf("string_content end = %d, want %d", got, want)
	}
	if got, want := mid.ChildCount(), 1; got != want {
		t.Fatalf("string_content child count = %d, want %d", got, want)
	}
	esc := mid.Child(0)
	if got, want := esc.Type(lang), "escape_sequence"; got != want {
		t.Fatalf("child type = %q, want %q", got, want)
	}
	if esc.StartByte() != 2 || esc.EndByte() != 4 {
		t.Fatalf("escape span = [%d:%d], want [2:4]", esc.StartByte(), esc.EndByte())
	}
	if !esc.IsNamed() {
		t.Fatalf("escape_sequence should be named")
	}
	if got, want := esc.StartPoint(), (Point{Column: 2}); got != want {
		t.Fatalf("escape start point = %+v, want %+v", got, want)
	}
	if got, want := esc.EndPoint(), (Point{Column: 4}); got != want {
		t.Fatalf("escape end point = %+v, want %+v", got, want)
	}
}

// Trailing escape after a text chunk: `"end\\"`.
func TestNormalizeRStringContentTrailingEscapeRebuilt(t *testing.T) {
	lang := newRTestLanguage()
	source := []byte(`"end\\"`)
	arena := newNodeArena(arenaClassFull)
	mid := newLeafNodeInArena(arena, 5, true, 4, 6, Point{Column: 4}, Point{Column: 6})
	_, str := buildRString(arena, 4, 0, 6, mid)

	normalizeRCompatibility(str.parent, source, lang)

	if mid.StartByte() != 1 || mid.EndByte() != 6 {
		t.Fatalf("string_content span = [%d:%d], want [1:6]", mid.StartByte(), mid.EndByte())
	}
	if got, want := mid.ChildCount(), 1; got != want {
		t.Fatalf("string_content child count = %d, want %d", got, want)
	}
	esc := mid.Child(0)
	if esc.StartByte() != 4 || esc.EndByte() != 6 {
		t.Fatalf("escape span = [%d:%d], want [4:6]", esc.StartByte(), esc.EndByte())
	}
}

// Escape-only content `"\\"`: span already matches, but the escape child is
// missing and must be synthesized.
func TestNormalizeRStringContentEscapeOnlyChildRestored(t *testing.T) {
	lang := newRTestLanguage()
	source := []byte(`"\\"`)
	arena := newNodeArena(arenaClassFull)
	mid := newLeafNodeInArena(arena, 5, true, 1, 3, Point{Column: 1}, Point{Column: 3})
	_, str := buildRString(arena, 4, 0, 3, mid)

	normalizeRCompatibility(str.parent, source, lang)

	if mid.StartByte() != 1 || mid.EndByte() != 3 {
		t.Fatalf("string_content span = [%d:%d], want [1:3]", mid.StartByte(), mid.EndByte())
	}
	if got, want := mid.ChildCount(), 1; got != want {
		t.Fatalf("string_content child count = %d, want %d", got, want)
	}
}

// Single-quoted variant with interior escape: `'a\'b'`.
func TestNormalizeRStringContentSingleQuoted(t *testing.T) {
	lang := newRTestLanguage()
	source := []byte(`'a\'b'`)
	arena := newNodeArena(arenaClassFull)
	mid := newLeafNodeInArena(arena, 5, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	_, str := buildRString(arena, 3, 0, 5, mid)

	normalizeRCompatibility(str.parent, source, lang)

	if mid.StartByte() != 1 || mid.EndByte() != 5 {
		t.Fatalf("string_content span = [%d:%d], want [1:5]", mid.StartByte(), mid.EndByte())
	}
	if got, want := mid.ChildCount(), 1; got != want {
		t.Fatalf("string_content child count = %d, want %d", got, want)
	}
	esc := mid.Child(0)
	if esc.StartByte() != 2 || esc.EndByte() != 4 {
		t.Fatalf("escape span = [%d:%d], want [2:4]", esc.StartByte(), esc.EndByte())
	}
}

// Already-correct shapes must be untouched (idempotence).
func TestNormalizeRStringContentLeavesCorrectShapesAlone(t *testing.T) {
	lang := newRTestLanguage()
	// `"\n\t"` — two escapes, correct shape from the parser
	source := []byte(`"\n\t"`)
	arena := newNodeArena(arenaClassFull)
	esc1 := newLeafNodeInArena(arena, 6, true, 1, 3, Point{Column: 1}, Point{Column: 3})
	esc2 := newLeafNodeInArena(arena, 6, true, 3, 5, Point{Column: 3}, Point{Column: 5})
	mid := newParentNodeInArena(arena, 5, true, []*Node{esc1, esc2}, nil, 0)
	_, str := buildRString(arena, 4, 0, 5, mid)

	normalizeRCompatibility(str.parent, source, lang)

	if mid.StartByte() != 1 || mid.EndByte() != 5 {
		t.Fatalf("string_content span = [%d:%d], want [1:5]", mid.StartByte(), mid.EndByte())
	}
	if got, want := mid.ChildCount(), 2; got != want {
		t.Fatalf("string_content child count = %d, want %d", got, want)
	}
	if mid.Child(0) != esc1 || mid.Child(1) != esc2 {
		t.Fatalf("existing escape children were replaced")
	}
}

// Plain content without escapes is already correct and must not change.
func TestNormalizeRStringContentNoEscapesUntouched(t *testing.T) {
	lang := newRTestLanguage()
	source := []byte(`"abc"`)
	arena := newNodeArena(arenaClassFull)
	mid := newLeafNodeInArena(arena, 5, true, 1, 4, Point{Column: 1}, Point{Column: 4})
	_, str := buildRString(arena, 4, 0, 4, mid)

	normalizeRCompatibility(str.parent, source, lang)

	if mid.StartByte() != 1 || mid.EndByte() != 4 {
		t.Fatalf("string_content span = [%d:%d], want [1:4]", mid.StartByte(), mid.EndByte())
	}
	if got, want := mid.ChildCount(), 0; got != want {
		t.Fatalf("string_content child count = %d, want %d", got, want)
	}
}

// Content whose backslash run is not a valid escape_sequence must be left
// alone — the parse already diverged in some other way and rewriting could
// invent structure the C oracle does not have.
func TestNormalizeRStringContentInvalidEscapeBails(t *testing.T) {
	lang := newRTestLanguage()
	source := []byte(`"a\8b"`)
	arena := newNodeArena(arenaClassFull)
	mid := newLeafNodeInArena(arena, 5, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	_, str := buildRString(arena, 4, 0, 5, mid)

	normalizeRCompatibility(str.parent, source, lang)

	if mid.StartByte() != 2 || mid.EndByte() != 4 {
		t.Fatalf("string_content span = [%d:%d], want untouched [2:4]", mid.StartByte(), mid.EndByte())
	}
	if got, want := mid.ChildCount(), 0; got != want {
		t.Fatalf("string_content child count = %d, want %d", got, want)
	}
}

// rEscapeSequenceLen must mirror the grammar's escape_sequence token exactly.
func TestREscapeSequenceLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{`\\`, 2, true},
		{`\n`, 2, true},
		{`\"`, 2, true},
		{`\'`, 2, true},
		{"\\\n", 2, true},   // backslash-newline is a valid escape
		{`\0`, 2, true},     // octal, 1 digit
		{`\07`, 3, true},    // octal, 2 digits
		{`\077`, 4, true},   // octal, 3 digits
		{`\0777`, 4, true},  // octal stops at 3 digits
		{`\x4`, 3, true},    // hex, 1 digit
		{`\x41`, 4, true},   // hex, 2 digits
		{`\x415`, 4, true},  // hex stops at 2 digits
		{`\u4`, 3, true},      // unicode, 1-4 hex
		{"\\u4142", 6, true},  // unicode, 4 hex
		{"\\u41425", 6, true}, // unicode stops at 4 hex
		{`\u{41}`, 6, true},
		{`\u{4142}x`, 8, true},
		{`\U41425868z`, 10, true},
		{`\U{1F600}`, 9, true},
		{`\8`, 0, false},  // not octal, [^0-9xuU] excludes digits
		{`\9`, 0, false},
		{`\x`, 0, false},  // x requires a hex digit
		{`\xz`, 0, false},
		{`\u`, 0, false},
		{`\u{}`, 0, false},
		{`\u{12345}`, 0, false}, // too many hex digits in braces
		{`\u{41`, 0, false},     // unterminated brace
		{`\`, 0, false},         // dangling backslash
	}
	for _, c := range cases {
		got, ok := rEscapeSequenceLen([]byte(c.in))
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("rEscapeSequenceLen(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
