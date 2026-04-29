package gotreesitter

import (
	"encoding/binary"
	"errors"
	"testing"
	"unicode/utf16"
)

func utf16Units(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

func utf16BytesForTest(t *testing.T, s string, order UTF16ByteOrder) []byte {
	t.Helper()

	var byteOrder binary.ByteOrder
	switch order {
	case UTF16LittleEndian:
		byteOrder = binary.LittleEndian
	case UTF16BigEndian:
		byteOrder = binary.BigEndian
	default:
		t.Fatalf("unsupported test byte order %v", order)
	}

	units := utf16Units(s)
	out := make([]byte, len(units)*2)
	for i, unit := range units {
		byteOrder.PutUint16(out[i*2:], unit)
	}
	return out
}

func assertUTF16UnitsEqual(t *testing.T, got, want []uint16) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("UTF-16 unit length = %d, want %d: got %v want %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("UTF-16 unit %d = %#04x, want %#04x: got %v want %v", i, got[i], want[i], got, want)
		}
	}
}

func TestDecodeUTF16BytesLittleAndBigEndian(t *testing.T) {
	want := utf16Units("a😀\nb")
	tests := []struct {
		name  string
		order UTF16ByteOrder
	}{
		{name: "little", order: UTF16LittleEndian},
		{name: "big", order: UTF16BigEndian},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeUTF16Bytes(utf16BytesForTest(t, "a😀\nb", tc.order), tc.order)
			if err != nil {
				t.Fatalf("DecodeUTF16Bytes failed: %v", err)
			}
			assertUTF16UnitsEqual(t, got, want)
		})
	}
}

func TestDecodeUTF16BytesRejectsInvalidInputs(t *testing.T) {
	if _, err := DecodeUTF16Bytes([]byte{0x31}, UTF16LittleEndian); !errors.Is(err, ErrInvalidUTF16ByteLength) {
		t.Fatalf("odd-length DecodeUTF16Bytes error = %v, want ErrInvalidUTF16ByteLength", err)
	}
	if _, err := DecodeUTF16Bytes([]byte{0x31, 0x00}, UTF16ByteOrder(255)); !errors.Is(err, ErrInvalidUTF16ByteOrder) {
		t.Fatalf("invalid-order DecodeUTF16Bytes error = %v, want ErrInvalidUTF16ByteOrder", err)
	}
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

	numSource, ok := tree.UTF16SourceForNode(num)
	if !ok {
		t.Fatal("UTF16SourceForNode(num) returned !ok")
	}
	assertUTF16UnitsEqual(t, numSource, utf16Units("2"))
}

func TestParseUTF16Bytes(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	for _, order := range []UTF16ByteOrder{UTF16LittleEndian, UTF16BigEndian} {
		t.Run(order.String(), func(t *testing.T) {
			tree, err := parser.ParseUTF16Bytes(utf16BytesForTest(t, "1+2", order), order)
			if err != nil {
				t.Fatalf("ParseUTF16Bytes failed: %v", err)
			}
			if got, want := tree.SourceEncoding(), InputEncodingUTF16; got != want {
				t.Fatalf("SourceEncoding = %s, want %s", got, want)
			}
			if got, want := string(tree.Source()), "1+2"; got != want {
				t.Fatalf("Source = %q, want %q", got, want)
			}
			assertUTF16UnitsEqual(t, tree.SourceUTF16(), utf16Units("1+2"))
			freshTree, err := parser.ParseUTF16(utf16Units("1+2"))
			if err != nil {
				t.Fatalf("fresh ParseUTF16 failed: %v", err)
			}
			if got, want := tree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
				t.Fatalf("ParseUTF16Bytes SExpr = %q, want %q", got, want)
			}
		})
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

func TestParseIncrementalUTF16BytesWithEdit(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	oldSource := utf16Units("1+2")
	oldTree, err := parser.ParseUTF16(oldSource)
	if err != nil {
		t.Fatalf("ParseUTF16 old failed: %v", err)
	}

	newSource := utf16Units("1+4")
	if ok := oldTree.EditUTF16(UTF16Edit{
		StartCodeUnit:  2,
		OldEndCodeUnit: 3,
		NewEndCodeUnit: 3,
	}, newSource); !ok {
		t.Fatal("EditUTF16 returned false")
	}

	incrTree, err := parser.ParseIncrementalUTF16Bytes(utf16BytesForTest(t, "1+4", UTF16BigEndian), oldTree, UTF16BigEndian)
	if err != nil {
		t.Fatalf("ParseIncrementalUTF16Bytes failed: %v", err)
	}
	if got, want := incrTree.RootNode().Text(incrTree.Source()), "1+4"; got != want {
		t.Fatalf("incremental UTF16 byte root text = %q, want %q", got, want)
	}
	assertUTF16UnitsEqual(t, incrTree.SourceUTF16(), newSource)
}
