package gotreesitter_test

// tree-sitter C starts the root node at the first non-whitespace byte:
// leading whitespace is token padding, excluded from every node extent,
// including the root's. normalizeRootSourceStart must therefore never pull a
// root back to byte 0 across leading whitespace (oracle-verified on the
// faust/css corpora, e.g. osc.dsp "\n\n\ndeclare …" → C root [3:…]; squirrel
// previously compensated per-grammar in normalizeSquirrelCompatibility).

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestRootStartsAtFirstNonWhitespaceByte(t *testing.T) {
	cases := []struct {
		name      string
		lang      *gts.Language
		src       string
		wantStart uint32
	}{
		{"css leading space", grammars.CssLanguage(), " a { color: red; }\n", 1},
		{"css leading newlines", grammars.CssLanguage(), "\n\na { color: red; }\n", 2},
		{"css no leading trivia", grammars.CssLanguage(), "a { color: red; }\n", 0},
		{"css leading comment keeps byte 0", grammars.CssLanguage(), "/* c */\na { color: red; }\n", 0},
		{"squirrel leading whitespace", grammars.SquirrelLanguage(), "\n\tx <- 1\n", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := gts.NewParser(tc.lang)
			tree, err := p.Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatalf("nil tree/root")
			}
			defer tree.Release()
			root := tree.RootNode()
			if got := root.StartByte(); got != tc.wantStart {
				t.Errorf("root start=%d want %d (root type %q, span [%d:%d])",
					got, tc.wantStart, root.Type(tc.lang), root.StartByte(), root.EndByte())
			}
			if got := root.EndByte(); got != uint32(len(tc.src)) {
				t.Errorf("root end=%d want %d (trailing whitespace must stay covered)",
					got, len(tc.src))
			}
		})
	}
}
