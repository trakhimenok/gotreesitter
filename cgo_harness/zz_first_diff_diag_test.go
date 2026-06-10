//go:build cgo && treesitter_c_parity

package cgoharness

// THROWAWAY diagnostic: parse ONE file with the production parser and the C
// oracle, walk both trees in lockstep, and dump the first structural
// difference (type, span, or child count) with full sibling context.
//
//	REPRO_LANG=jq REPRO_FILE=/path/to/builtin.jq \
//	  go test . -tags 'cgo treesitter_c_parity' -run TestFirstDiffDiag -v

import (
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func fddTxt(src []byte, s, e uint32) string {
	if int(e) > len(src) {
		e = uint32(len(src))
	}
	r := src[s:e]
	if len(r) > 80 {
		r = r[:80]
	}
	return fmt.Sprintf("%q", string(r))
}

func fddDumpBoth(g *gts.Node, lang *gts.Language, c *sitter.Node, src []byte, path string, t *testing.T) {
	t.Logf("  FIRST-DIFF @%s", path)
	t.Logf("    go: type=%q [%d:%d] cc=%d", g.Type(lang), g.StartByte(), g.EndByte(), g.ChildCount())
	t.Logf("    c : kind=%q [%d:%d] cc=%d", c.Kind(), c.StartByte(), c.EndByte(), int(c.ChildCount()))
	for i := 0; i < g.ChildCount(); i++ {
		ch := g.Child(i)
		t.Logf("      go.child[%d]: type=%q [%d:%d] named=%v %s", i, ch.Type(lang), ch.StartByte(), ch.EndByte(), ch.IsNamed(), fddTxt(src, ch.StartByte(), ch.EndByte()))
	}
	for i := 0; i < int(c.ChildCount()); i++ {
		ch := c.Child(uint(i))
		t.Logf("      c.child[%d]: kind=%q [%d:%d] named=%v %s", i, ch.Kind(), ch.StartByte(), ch.EndByte(), ch.IsNamed(), fddTxt(src, uint32(ch.StartByte()), uint32(ch.EndByte())))
	}
}

func fddWalk(g *gts.Node, lang *gts.Language, c *sitter.Node, src []byte, path string, t *testing.T) bool {
	gType, cType := g.Type(lang), c.Kind()
	if gType != cType || g.StartByte() != uint32(c.StartByte()) || g.EndByte() != uint32(c.EndByte()) || g.ChildCount() != int(c.ChildCount()) {
		fddDumpBoth(g, lang, c, src, path, t)
		return true
	}
	for i := 0; i < g.ChildCount(); i++ {
		if fddWalk(g.Child(i), lang, c.Child(uint(i)), src, fmt.Sprintf("%s[%d]", path, i), t) {
			return true
		}
	}
	return false
}

func TestFirstDiffDiag(t *testing.T) {
	name := os.Getenv("REPRO_LANG")
	file := os.Getenv("REPRO_FILE")
	if name == "" || file == "" {
		t.Skip("set REPRO_LANG and REPRO_FILE")
	}
	var goLang *gts.Language
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			goLang = e.Language()
			break
		}
	}
	if goLang == nil {
		t.Fatalf("%s: not in grammars.AllLanguages", name)
	}
	cLang, err := ParityCLanguage(name)
	if err != nil {
		t.Fatalf("%s: no C reference: %v", name, err)
	}
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if os.Getenv("REPRO_DEBUG_DFA") == "1" {
		gts.DebugDFA.Store(true)
		defer gts.DebugDFA.Store(false)
	}
	gp := gts.NewParser(goLang)
	if os.Getenv("REPRO_GLR_TRACE") == "1" {
		gp.SetGLRTrace(true)
	}
	tr, _ := gp.Parse(src)
	if tr == nil || tr.RootNode() == nil {
		t.Fatalf("go parse failed")
	}
	defer tr.Release()
	cp := sitter.NewParser()
	defer cp.Close()
	_ = cp.SetLanguage(cLang)
	ct := cp.Parse(src, nil)
	if ct == nil || ct.RootNode() == nil {
		t.Fatalf("c parse failed")
	}
	defer ct.Close()
	t.Logf("=== %s (%d bytes) ===", file, len(src))
	t.Logf("  go stopReason=%v rootHasError=%v cRootHasError=%v", tr.ParseStopReason(), tr.RootNode().HasError(), ct.RootNode().HasError())
	if !fddWalk(tr.RootNode(), goLang, ct.RootNode(), src, "root", t) {
		t.Logf("  (no structural divergence)")
	}
}
