package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestLuaDiagEscOnly(t *testing.T) {
	lang := LuaLanguage()
	for _, src := range []string{
		`a = '\027'`,
		`a = '\\'`,
		`a = '\n'`,
		`b = (prev_char == '\027' and char == '\\') or char == 'x'`,
	} {
		parser := gotreesitter.NewParser(lang)
		tree, _ := parser.Parse([]byte(src))
		root := tree.RootNode()
		var walk func(n *gotreesitter.Node, depth int)
		walk = func(n *gotreesitter.Node, depth int) {
			t.Logf("%*s%s [%d:%d]", depth*2, "", n.Type(lang), n.StartByte(), n.EndByte())
			for i := 0; i < n.ChildCount(); i++ {
				walk(n.Child(i), depth+1)
			}
		}
		t.Logf("SRC %q hasErr=%v", src, root.HasError())
		walk(root, 1)
		tree.Release()
	}
}
