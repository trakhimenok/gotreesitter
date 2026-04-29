package gotreesitter

import (
	"testing"
	"unicode/utf16"
)

func utf16Units(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

func TestEncodeUTF16ToUTF8WithMapSurrogatePair(t *testing.T) {
	source := utf16Units("a😀\nb")
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)

	if got, want := string(utf8Source), "a😀\nb"; got != want {
		t.Fatalf("UTF-8 source = %q, want %q", got, want)
	}

	tests := []struct {
		byteOffset uint32
		unitOffset uint32
		point      Point
	}{
		{byteOffset: 0, unitOffset: 0, point: Point{Row: 0, Column: 0}},
		{byteOffset: 1, unitOffset: 1, point: Point{Row: 0, Column: 1}},
		{byteOffset: 5, unitOffset: 3, point: Point{Row: 0, Column: 3}},
		{byteOffset: 6, unitOffset: 4, point: Point{Row: 1, Column: 0}},
		{byteOffset: 7, unitOffset: 5, point: Point{Row: 1, Column: 1}},
	}
	for _, tc := range tests {
		unitOffset, ok := sourceMap.byteToUTF16Unit(tc.byteOffset)
		if !ok {
			t.Fatalf("byteToUTF16Unit(%d) returned !ok", tc.byteOffset)
		}
		if unitOffset != tc.unitOffset {
			t.Fatalf("byteToUTF16Unit(%d) = %d, want %d", tc.byteOffset, unitOffset, tc.unitOffset)
		}
		point, ok := sourceMap.pointForByte(tc.byteOffset)
		if !ok {
			t.Fatalf("pointForByte(%d) returned !ok", tc.byteOffset)
		}
		if point != tc.point {
			t.Fatalf("pointForByte(%d) = %+v, want %+v", tc.byteOffset, point, tc.point)
		}
	}

	for _, unitOffset := range []uint32{0, 1, 3, 4, 5} {
		byteOffset, ok := sourceMap.utf16UnitToByte(unitOffset)
		if !ok {
			t.Fatalf("utf16UnitToByte(%d) returned !ok", unitOffset)
		}
		roundTrip, ok := sourceMap.byteToUTF16Unit(byteOffset)
		if !ok {
			t.Fatalf("byteToUTF16Unit(%d) returned !ok", byteOffset)
		}
		if roundTrip < unitOffset {
			t.Fatalf("round trip for unit %d went backwards to %d via byte %d", unitOffset, roundTrip, byteOffset)
		}
	}
	if _, ok := sourceMap.utf16UnitToByte(2); ok {
		t.Fatal("utf16UnitToByte(2) returned ok for the middle of a surrogate pair")
	}
	for _, byteOffset := range []uint32{2, 3, 4} {
		if _, ok := sourceMap.byteToUTF16Unit(byteOffset); ok {
			t.Fatalf("byteToUTF16Unit(%d) returned ok for the middle of a UTF-8 rune", byteOffset)
		}
	}
}

func TestParseUTF16ArithmeticPreservesUTF16Metadata(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := utf16Units("1+2")

	tree, err := parser.ParseUTF16(source)
	if err != nil {
		t.Fatalf("ParseUTF16 failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("ParseUTF16 returned nil root")
	}
	if tree.SourceEncoding() != InputEncodingUTF16 {
		t.Fatalf("SourceEncoding = %s, want utf16", tree.SourceEncoding())
	}
	if got := string(tree.Source()); got != "1+2" {
		t.Fatalf("Source = %q, want %q", got, "1+2")
	}
	if got := tree.SourceUTF16(); len(got) != len(source) || got[0] != source[0] || got[2] != source[2] {
		t.Fatalf("SourceUTF16 = %v, want %v", got, source)
	}

	rng, ok := tree.UTF16RangeForNode(root)
	if !ok {
		t.Fatal("UTF16RangeForNode(root) returned !ok")
	}
	if got, want := rng.StartCodeUnit, uint32(0); got != want {
		t.Fatalf("root UTF16 start = %d, want %d", got, want)
	}
	if got, want := rng.EndCodeUnit, uint32(3); got != want {
		t.Fatalf("root UTF16 end = %d, want %d", got, want)
	}
	if got, want := rng.EndPoint, (Point{Row: 0, Column: 3}); got != want {
		t.Fatalf("root UTF16 end point = %+v, want %+v", got, want)
	}

	num := root.Child(2)
	if num == nil || num.Text(tree.Source()) != "2" {
		t.Fatalf("right child text = %q, want %q", num.Text(tree.Source()), "2")
	}
	numRange, ok := tree.UTF16RangeForNode(num)
	if !ok {
		t.Fatal("UTF16RangeForNode(num) returned !ok")
	}
	if got, want := numRange.StartCodeUnit, uint32(2); got != want {
		t.Fatalf("num UTF16 start = %d, want %d", got, want)
	}
	if got, want := numRange.EndCodeUnit, uint32(3); got != want {
		t.Fatalf("num UTF16 end = %d, want %d", got, want)
	}
}

func TestParseIncrementalUTF16WithEdit(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	oldSource := utf16Units("1+2")
	oldTree, err := parser.ParseUTF16(oldSource)
	if err != nil {
		t.Fatalf("ParseUTF16 old failed: %v", err)
	}

	newSource := utf16Units("1+3")
	if ok := oldTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, newSource); !ok {
		t.Fatal("EditUTF16 returned false")
	}

	incrTree, err := parser.ParseIncrementalUTF16(newSource, oldTree)
	if err != nil {
		t.Fatalf("ParseIncrementalUTF16 failed: %v", err)
	}
	freshTree, err := parser.ParseUTF16(newSource)
	if err != nil {
		t.Fatalf("fresh ParseUTF16 failed: %v", err)
	}

	if got, want := incrTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental UTF16 SExpr mismatch:\n  got:  %s\n  want: %s", got, want)
	}
	if got, want := incrTree.RootNode().Text(incrTree.Source()), "1+3"; got != want {
		t.Fatalf("incremental UTF16 root text = %q, want %q", got, want)
	}
}
