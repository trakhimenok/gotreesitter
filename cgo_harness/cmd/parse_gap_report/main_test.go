//go:build cgo && treesitter_c_parity

package main

import "testing"

func TestFormatHighlightCaptureMismatchIncludesCountsAndFirstCaptures(t *testing.T) {
	onlyGo := []highlightCapture{
		{Name: "keyword", StartByte: 10, EndByte: 15},
		{Name: "type", StartByte: 20, EndByte: 24},
	}
	onlyC := []highlightCapture{
		{Name: "function", StartByte: 30, EndByte: 38},
	}

	got := formatHighlightCaptureMismatch(onlyGo, onlyC)
	want := "capture mismatch go_only=2 c_only=1 first_go_only=@keyword[10:15] first_c_only=@function[30:38]"
	if got != want {
		t.Fatalf("mismatch detail = %q, want %q", got, want)
	}
}

func TestDiffHighlightCaptures(t *testing.T) {
	goCaps := []highlightCapture{
		{Name: "keyword", StartByte: 0, EndByte: 5},
		{Name: "type", StartByte: 6, EndByte: 9},
	}
	cCaps := []highlightCapture{
		{Name: "keyword", StartByte: 0, EndByte: 5},
		{Name: "function", StartByte: 6, EndByte: 9},
	}

	onlyGo, onlyC := diffHighlightCaptures(goCaps, cCaps)
	if len(onlyGo) != 1 || onlyGo[0] != goCaps[1] {
		t.Fatalf("onlyGo = %#v, want %#v", onlyGo, []highlightCapture{goCaps[1]})
	}
	if len(onlyC) != 1 || onlyC[0] != cCaps[1] {
		t.Fatalf("onlyC = %#v, want %#v", onlyC, []highlightCapture{cCaps[1]})
	}
}
