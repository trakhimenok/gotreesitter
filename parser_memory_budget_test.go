package gotreesitter_test

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestParserMemoryBudgetStopsParse(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	defer gotreesitter.ResetParseEnvConfigCacheForTests()

	parser := gotreesitter.NewParser(grammars.GoLanguage())
	var source bytes.Buffer
	source.WriteString("package p\nfunc f() {\n")
	for i := 0; i < 20000; i++ {
		source.WriteString("var x = 1\n")
	}
	source.WriteString("}\n")
	tree, err := parser.Parse(source.Bytes())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	defer tree.Release()

	if got, want := tree.ParseStopReason(), gotreesitter.ParseStopMemoryBudget; got != want {
		t.Fatalf("ParseStopReason() = %q, want %q (runtime=%s)", got, want, tree.ParseRuntime().Summary())
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
	rt := tree.ParseRuntime()
	if rt.MemoryBudgetBytes <= 0 {
		t.Fatalf("MemoryBudgetBytes = %d, want > 0", rt.MemoryBudgetBytes)
	}
	if rt.ArenaBytesAllocated < rt.MemoryBudgetBytes && rt.ScratchBytesAllocated < rt.MemoryBudgetBytes {
		t.Fatalf(
			"budget stop without exhausted region: arena=%d scratch=%d budget=%d",
			rt.ArenaBytesAllocated,
			rt.ScratchBytesAllocated,
			rt.MemoryBudgetBytes,
		)
	}
}
