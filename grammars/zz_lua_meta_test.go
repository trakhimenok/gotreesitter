package grammars

import "testing"

func TestLuaHiddenMeta(t *testing.T) {
	lang := LuaLanguage()
	for i, name := range lang.SymbolNames {
		if name == "_singlequote_string_content" || name == "_doublequote_string_content" || name == "escape_sequence" || name == "string_content" {
			m := lang.SymbolMetadata[i]
			t.Logf("sym %d %q visible=%v named=%v", i, name, m.Visible, m.Named)
		}
	}
}
