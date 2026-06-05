package gotreesitter_test

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestExternalScannerIncrementalReusePolicy(t *testing.T) {
	cases := []struct {
		name           string
		lang           func() *gotreesitter.Language
		source         func(int) []byte
		marker         string
		wantReuse      bool
		wantReason     string
		wantSubtreeMin uint64
		wantNoReparse  bool
	}{
		{
			name:           "typescript",
			lang:           grammars.TypescriptLanguage,
			source:         makeTypeScriptBenchmarkSource,
			marker:         "const v = ",
			wantReuse:      true,
			wantSubtreeMin: 1,
			wantNoReparse:  true,
		},
		{
			name:           "python",
			lang:           grammars.PythonLanguage,
			source:         makePythonBenchmarkSource,
			marker:         "v = ",
			wantReuse:      true,
			wantSubtreeMin: 1,
			wantNoReparse:  true,
		},
		{
			name:           "svelte",
			lang:           grammars.SvelteLanguage,
			source:         makeSvelteBenchmarkSource,
			marker:         "let v0 = ",
			wantReuse:      true,
			wantSubtreeMin: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			parser := gotreesitter.NewParser(lang)
			src := tc.source(128)
			sites := makeBenchmarkEditSites(src, tc.marker)
			if len(sites) == 0 {
				t.Fatalf("missing edit sites for marker %q", tc.marker)
			}
			site := sites[0]

			oldTree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("initial parse failed: %v", err)
			}
			requireCompleteParse(t, oldTree, src, lang, "initial")
			if oldTree.RootNode().HasError() {
				t.Fatal("initial parse produced error root")
			}

			next := append([]byte(nil), src...)
			toggleDigitAt(next, site.offset)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(site.offset),
				OldEndByte:  uint32(site.offset + 1),
				NewEndByte:  uint32(site.offset + 1),
				StartPoint:  site.start,
				OldEndPoint: site.end,
				NewEndPoint: site.end,
			})

			newTree, prof, err := parser.ParseIncrementalProfiled(next, oldTree)
			if err != nil {
				t.Fatalf("incremental parse failed: %v", err)
			}
			requireCompleteParse(t, newTree, next, lang, "incremental")
			if newTree.RootNode().HasError() {
				t.Fatal("incremental parse produced error root")
			}
			if tc.wantReuse {
				if prof.ReuseUnsupported {
					t.Fatalf("ReuseUnsupported = true, want false (reason=%q)", prof.ReuseUnsupportedReason)
				}
				if prof.ReusedSubtrees < tc.wantSubtreeMin {
					t.Fatalf("ReusedSubtrees = %d, want >= %d", prof.ReusedSubtrees, tc.wantSubtreeMin)
				}
				if tc.wantNoReparse && prof.ReparseNanos != 0 {
					t.Fatalf("ReparseNanos = %d, want 0 for token-invariant leaf edit", prof.ReparseNanos)
				}
				return
			}
			if !prof.ReuseUnsupported {
				t.Fatal("ReuseUnsupported = false, want true")
			}
			if prof.ReuseUnsupportedReason != tc.wantReason {
				t.Fatalf("ReuseUnsupportedReason = %q, want %q", prof.ReuseUnsupportedReason, tc.wantReason)
			}
		})
	}
}

func TestExternalScannerTokenInvariantLeafReuse(t *testing.T) {
	cases := []struct {
		name        string
		lang        func() *gotreesitter.Language
		source      []byte
		marker      []byte
		replacement byte
	}{
		{
			name:        "elixir identifier",
			lang:        grammars.ElixirLanguage,
			source:      []byte("value = 1\nIO.inspect(value)\n"),
			marker:      []byte("value"),
			replacement: 'w',
		},
		{
			name:        "julia line comment",
			lang:        grammars.JuliaLanguage,
			source:      []byte("# This file is part of Julia.\nx = 1\n"),
			marker:      []byte("This"),
			replacement: 'U',
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			offset := bytes.Index(tc.source, tc.marker)
			if offset < 0 {
				t.Fatalf("fixture missing marker %q", tc.marker)
			}
			next := append([]byte(nil), tc.source...)
			next[offset] = tc.replacement

			lang := tc.lang()
			parser := gotreesitter.NewParser(lang)
			fresh, err := parser.Parse(next)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			requireCompleteParse(t, fresh, next, lang, "fresh")
			if fresh.RootNode().HasError() {
				t.Fatalf("fresh parse has error: %s", fresh.RootNode().SExpr(lang))
			}

			oldTree, err := parser.Parse(tc.source)
			if err != nil {
				t.Fatalf("old parse: %v", err)
			}
			defer oldTree.Release()
			requireCompleteParse(t, oldTree, tc.source, lang, "old")
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointForOffset(tc.source, offset),
				OldEndPoint: pointForOffset(tc.source, offset+1),
				NewEndPoint: pointForOffset(next, offset+1),
			})

			newTree, profile, err := parser.ParseIncrementalProfiled(next, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer newTree.Release()
			requireCompleteParse(t, newTree, next, lang, "incremental")
			if profile.ReuseUnsupported {
				t.Fatalf("token-invariant edit fell back to fresh parse: %s", profile.ReuseUnsupportedReason)
			}
			if profile.ReparseNanos != 0 {
				t.Fatalf("ReparseNanos = %d, want 0 for token-invariant edit", profile.ReparseNanos)
			}
			if profile.ReusedSubtrees == 0 {
				t.Fatalf("token-invariant edit reused no subtrees: %+v", profile)
			}
			if got, want := newTree.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
				t.Fatalf("incremental tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func makeSvelteBenchmarkSource(funcCount int) []byte {
	var buf bytes.Buffer
	buf.WriteString("<script>\n")
	for i := 0; i < funcCount; i++ {
		buf.WriteString("  let v")
		buf.WriteString(stringInt(i))
		buf.WriteString(" = ")
		buf.WriteString(stringInt(i))
		buf.WriteString(";\n")
	}
	buf.WriteString("</script>\n\n")
	for i := 0; i < funcCount; i++ {
		buf.WriteString("{#if v")
		buf.WriteString(stringInt(i))
		buf.WriteString(" > 0}\n")
		buf.WriteString("  <section class=\"item\"><button>{v")
		buf.WriteString(stringInt(i))
		buf.WriteString("}</button></section>\n")
		buf.WriteString("{/if}\n")
	}
	return buf.Bytes()
}
