package gotreesitter_test

// THROWAWAY diagnostic for the requirements grammar tables.

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestReqTablesDiag(t *testing.T) {
	lang := grammars.RequirementsLanguage()
	t.Logf("name=%s initialState=%d tokenCount=%d symbolCount=%d stateCount=%d", lang.Name, lang.InitialState, lang.TokenCount, lang.SymbolCount, lang.StateCount)
	for i, n := range lang.SymbolNames {
		meta := lang.SymbolMetadata[i]
		t.Logf("sym %d: %q visible=%v named=%v", i, n, meta.Visible, meta.Named)
	}
	// Dump state 0 action row (C ERROR_STATE analogue) and the initial state row.
	dumpRow := func(state gts.StateID) {
		if int(state) >= len(lang.ParseTable) {
			t.Logf("state %d not in dense table (len=%d, largeStateCount=%d)", state, len(lang.ParseTable), lang.LargeStateCount)
			return
		}
		row := lang.ParseTable[state]
		for sym := gts.Symbol(0); uint32(sym) < lang.TokenCount; sym++ {
			if int(sym) >= len(row) {
				break
			}
			idx := row[sym]
			if idx == 0 || int(idx) >= len(lang.ParseActions) {
				continue
			}
			acts := lang.ParseActions[idx].Actions
			t.Logf("state %d sym %d (%s): %+v", state, sym, lang.SymbolNames[sym], acts)
		}
	}
	t.Logf("=== state 0 row ===")
	dumpRow(0)
	t.Logf("=== initial state row ===")
	dumpRow(lang.InitialState)
	t.Logf("=== state 31 row ===")
	dumpRow(31)
}

func TestReqTokenTraceDiag(t *testing.T) {
	lang := grammars.RequirementsLanguage()
	src := []byte("pkg ; python_version >= '3.13'  # note\n")
	p := gts.NewParser(lang)
	p.SetGLRTrace(true)
	tree, err := p.Parse(src)
	fmt.Println("err:", err)
	if tree != nil {
		defer tree.Release()
	}
	_ = tree
}
