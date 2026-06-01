package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	grm "github.com/odvcencio/gotreesitter/grammars"
)

// TestForestDispatchParity verifies the (default-on) forest fast path is
// invisible: for a dispatched language (css ∈ languageWantsForest) the forest
// tree must be byte-identical to production — same s-expr AND same root byte
// span — and anything the forest declines (malformed input, non-dispatched
// languages) must match production because we fall back to it.
// SetGLRForestEnabled(false) yields the production baseline; (true) is the
// default-on dispatch.
func TestForestDispatchParity(t *testing.T) {
	css := grm.CssLanguage()

	var big strings.Builder
	for i := 0; i < 60; i++ {
		big.WriteString(".cls-" + cssN(i) + " { color: red; margin: 0 1px 2px 3px; padding: 1em; }\n")
		big.WriteString("#id-" + cssN(i) + " > a:hover, .x .y { background: url(/img.png) no-repeat; }\n")
	}
	clean := []string{
		"a { color: red; }\n",
		".cls { margin: 0; padding: 1px 2px; z-index: 5; }\n",
		"@media (max-width: 600px) { .x { display: none; } }\n",
		"div > p + span ~ a:not(.z)::before { content: \"x\"; }\n",
		":root { --c: #fff; } body { color: var(--c); transform: matrix(1,2,3,4,5,6); }\n",
		big.String(),
	}
	malformed := []string{
		"a { color: red;\n",
		".x { ; } @media\n",
	}

	check := func(label string, lang *gts.Language, src string) {
		gts.SetGLRForestEnabled(false)
		prod, err := gts.NewParser(lang).Parse([]byte(src))
		if err != nil {
			t.Errorf("%s: prod parse failed: %v", label, err)
			return
		}
		defer prod.Release()
		want := prod.RootNode().SExpr(lang)
		wantEnd := prod.RootNode().EndByte()
		gts.SetGLRForestEnabled(true)
		got, err := gts.NewParser(lang).Parse([]byte(src))
		if err != nil {
			t.Errorf("%s: forest parse failed: %v", label, err)
			return
		}
		defer got.Release()
		if got.RootNode().SExpr(lang) != want {
			t.Errorf("%s: forest dispatch s-expr diverged for %q", label, src)
		}
		if got.RootNode().EndByte() != wantEnd {
			t.Errorf("%s: forest dispatch root endByte %d != production %d for %q",
				label, got.RootNode().EndByte(), wantEnd, src)
		}
	}

	for _, s := range clean {
		check("css-clean", css, s)
	}
	for _, s := range malformed {
		check("css-malformed-fallback", css, s)
	}
	// Bash is dispatched only after root compatibility normalization.
	check("bash-dispatched", grm.BashLanguage(), "f() { echo a; }\n")
	// Non-dispatched languages must be untouched even with the switch on.
	check("go-untouched", grm.GoLanguage(), "package p\nfunc f() { return }\n")
	check("rust-untouched", grm.RustLanguage(), "fn main() {}\n")
	gts.SetGLRForestEnabled(true)
}

func TestForestExperimentalAppliesBashCompatibility(t *testing.T) {
	gts.SetGLRForestEnabled(false)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("url=`(curl -SsL https://registry.npmjs.org/npm/$t; echo \"\") \\\n     | sed -e 's/^.*tarball\":\"//' \\\n     | sed -e 's/\".*$//'`\n\nret=$?\n")
	lang := grm.BashLanguage()
	prod, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer prod.Release()

	forest, ok := gts.NewParser(lang).ParseForestExperimental(src)
	if !ok || forest == nil || forest.RootNode() == nil {
		t.Fatalf("forest experimental ok=%v tree nil=%v", ok, forest == nil)
	}
	defer forest.Release()
	root := forest.RootNode()
	if got, want := root.SExpr(lang), prod.RootNode().SExpr(lang); got != want {
		t.Fatalf("forest experimental Bash compatibility mismatch\n got: %s\nwant: %s", got, want)
	}
	if got, want := root.NamedChildCount(), 2; got != want {
		t.Fatalf("forest Bash root named child count = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
}

func TestForestDispatchReportsAcceptedRuntime(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("f() { echo a; }\n")
	tree, err := gts.NewParser(grm.BashLanguage()).Parse(src)
	if err != nil {
		t.Fatalf("forest dispatch parse: %v", err)
	}
	defer tree.Release()
	rt := tree.ParseRuntime()
	if rt.StopReason != gts.ParseStopAccepted {
		t.Fatalf("forest dispatch stop reason = %q, want %q (%s)", rt.StopReason, gts.ParseStopAccepted, rt.Summary())
	}
	if rt.SourceLen != uint32(len(src)) || rt.ExpectedEOFByte != uint32(len(src)) || rt.LastTokenEndByte != uint32(len(src)) || !rt.LastTokenWasEOF {
		t.Fatalf("forest dispatch runtime mismatch: %s", rt.Summary())
	}
}

func TestForestDispatchPromotesJavaScript(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("function foo() {}\nfoo()\nlet plus1 = x => x + 1\nasync function* bar() { yield 1; }\n")
	lang := grm.JavascriptLanguage()
	gts.SetGLRForestEnabled(false)
	prod, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer prod.Release()

	gts.SetGLRForestEnabled(true)
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("forest dispatch parse: %v", err)
	}
	defer tree.Release()
	if got, want := tree.RootNode().SExpr(lang), prod.RootNode().SExpr(lang); got != want {
		t.Fatalf("JavaScript forest dispatch diverged\n got: %s\nwant: %s", got, want)
	}
	rt := tree.ParseRuntime()
	if rt.StopReason != gts.ParseStopAccepted || !rt.LastTokenWasEOF || rt.TokensConsumed != 0 {
		t.Fatalf("JavaScript did not use forest accepted runtime: %s", rt.Summary())
	}
}

func TestForestDispatchPromotesCSharp(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte(`using System;
class C {
  string Format(int x) => $"value={x}";
  void M() {
    foreach (var item in new[] {1, 2, 3}) {
      Console.WriteLine(Format(item));
    }
  }
}
`)
	lang := grm.CSharpLanguage()
	gts.SetGLRForestEnabled(false)
	prod, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer prod.Release()

	gts.SetGLRForestEnabled(true)
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("forest dispatch parse: %v", err)
	}
	defer tree.Release()
	if got, want := tree.RootNode().SExpr(lang), prod.RootNode().SExpr(lang); got != want {
		t.Fatalf("C# forest dispatch diverged\n got: %s\nwant: %s", got, want)
	}
	rt := tree.ParseRuntime()
	if rt.StopReason != gts.ParseStopAccepted || !rt.LastTokenWasEOF || rt.TokensConsumed != 0 {
		t.Fatalf("C# did not use forest accepted runtime: %s", rt.Summary())
	}
}

func TestForestTreeIncrementalEditSupportsCSSReuse(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte(".a { color: red; margin: 1px; padding: 4px; }\n.b { color: blue; transform: translateX(1px); }\n")
	const oldNeedle = "margin: 1px"
	offset := strings.Index(string(src), oldNeedle) + len("margin: ")
	if offset < len("margin: ") || len(src) <= offset || src[offset] != '1' {
		t.Fatalf("css fixture drifted: byte %d = %q, want '1'", offset, src[offset])
	}

	edited := append([]byte(nil), src...)
	edited[offset] = '2'
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(src, offset),
		OldEndPoint: pointForOffset(src, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	}

	parser := gts.NewParser(grm.CssLanguage())
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	oldTree.Edit(edit)

	newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()
	if got, want := newTree.RootNode().EndByte(), uint32(len(edited)); got != want {
		t.Fatalf("incremental root end = %d, want %d", got, want)
	}
	if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" {
		t.Fatalf("profile reuse unsupported = %v reason %q, want CSS reuse path",
			profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	if got, want := newTree.RootNode().SExpr(grm.CssLanguage()), freshTree.RootNode().SExpr(grm.CssLanguage()); got != want {
		t.Fatalf("incremental CSS tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
	}
}

func TestForestTreeIncrementalEditSupportsCMakeReuse(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("cmake_minimum_required(VERSION 3.20)\nproject(demo)\nadd_library(demo STATIC demo.cc)\ntarget_compile_definitions(demo PRIVATE VALUE=1)\n")
	oldNeedle := []byte("VALUE=1")
	offset := strings.Index(string(src), string(oldNeedle)) + len("VALUE=")
	if offset < len("VALUE=") || src[offset] != '1' {
		t.Fatalf("cmake fixture drifted: byte %d = %q, want '1'", offset, src[offset])
	}

	edited := append([]byte(nil), src...)
	edited[offset] = '2'
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(src, offset),
		OldEndPoint: pointForOffset(src, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	}

	parser := gts.NewParser(grm.CmakeLanguage())
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	oldTree.Edit(edit)

	newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()
	if got, want := newTree.RootNode().EndByte(), uint32(len(edited)); got != want {
		t.Fatalf("incremental root end = %d, want %d", got, want)
	}
	if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" {
		t.Fatalf("profile reuse unsupported = %v reason %q, want CMake reuse path",
			profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
	}
	if profile.ReparseNanos != 0 {
		t.Fatalf("profile reparse nanos = %d, want 0 for CMake text-invariant leaf reuse", profile.ReparseNanos)
	}
	if profile.ReusedSubtrees == 0 {
		t.Fatalf("profile reused subtrees = 0, want CMake sibling reuse")
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	if got, want := newTree.RootNode().SExpr(grm.CmakeLanguage()), freshTree.RootNode().SExpr(grm.CmakeLanguage()); got != want {
		t.Fatalf("incremental CMake tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
	}
}

func cssN(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func pointForOffset(src []byte, offset int) gts.Point {
	var pt gts.Point
	for _, b := range src[:offset] {
		if b == '\n' {
			pt.Row++
			pt.Column = 0
		} else {
			pt.Column++
		}
	}
	return pt
}
