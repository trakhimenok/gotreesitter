package grammargen

import (
	"os"
	"testing"
)

func TestPythonKeywordIdentificationIncludesSoftKeywords(t *testing.T) {
	gram := loadPythonGrammarJSONForTest(t)

	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize Python grammar: %v", err)
	}

	want := map[string]bool{
		"match":  false,
		"case":   false,
		"except": false,
	}
	for _, symID := range ng.KeywordSymbols {
		if symID >= 0 && symID < len(ng.Symbols) {
			if _, ok := want[ng.Symbols[symID].Name]; ok {
				want[ng.Symbols[symID].Name] = true
			}
		}
	}

	for name, found := range want {
		if !found {
			t.Fatalf("keyword %q missing from normalized keyword set", name)
		}
	}
}

func TestPythonReservedWordsSurviveNormalization(t *testing.T) {
	// Python's grammar.json contains a top-level reserved.global block but
	// never uses RESERVED wrappers. As of the Go-parity fix, ImportGrammarJSON
	// now drops reserved word sets in that case because the runtime
	// buildReservedWordTables path mismatches tree-sitter's semantic and
	// actively harms parsing when every reserved word is a hard keyword that
	// the grammar already lexes directly. To keep this test exercising the
	// normalization path, re-attach a synthetic reserved word set that mirrors
	// what grammar.json advertises.
	gram := loadPythonGrammarJSONForTest(t)
	if len(gram.ReservedWordSets) == 0 {
		gram.ReservedWordSets = []ReservedWordSet{{
			Name: "global",
			Rules: []*Rule{
				Str("if"),
				Str("except"),
				Str("await"),
			},
		}}
	}

	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize Python grammar: %v", err)
	}
	if len(ng.ReservedWordSets) == 0 {
		t.Fatal("normalized grammar dropped reserved word sets")
	}
	if len(ng.ReservedWordSets[0]) == 0 {
		t.Fatal("normalized global reserved word set is empty")
	}

	want := map[string]bool{
		"if":     false,
		"except": false,
		"await":  false,
	}
	for _, symID := range ng.ReservedWordSets[0] {
		if symID >= 0 && symID < len(ng.Symbols) {
			if _, ok := want[ng.Symbols[symID].Name]; ok {
				want[ng.Symbols[symID].Name] = true
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("reserved word %q missing from normalized global set", name)
		}
	}
}

func TestPythonGenerateLanguageEmitsReservedWords(t *testing.T) {
	// See TestPythonReservedWordsSurviveNormalization: re-attach a synthetic
	// reserved set so generator coverage is retained after ImportGrammarJSON
	// auto-drops the sets in the absence of RESERVED wrappers.
	gram := loadPythonGrammarJSONForTest(t)
	if len(gram.ReservedWordSets) == 0 {
		gram.ReservedWordSets = []ReservedWordSet{{
			Name: "global",
			Rules: []*Rule{
				Str("if"),
				Str("except"),
				Str("await"),
			},
		}}
	}

	lang, err := GenerateLanguage(gram)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	if lang.LanguageVersion < 15 {
		t.Fatalf("LanguageVersion = %d, want >= 15", lang.LanguageVersion)
	}
	if lang.MaxReservedWordSetSize == 0 || len(lang.ReservedWords) == 0 {
		t.Fatalf("reserved words missing from generated language: stride=%d len=%d", lang.MaxReservedWordSetSize, len(lang.ReservedWords))
	}

	nonZeroSetIDs := 0
	for _, mode := range lang.LexModes {
		if mode.ReservedWordSetID > 0 {
			nonZeroSetIDs++
		}
	}
	if nonZeroSetIDs == 0 {
		t.Fatal("generated language has no lex modes with reserved word sets")
	}
}

func loadPythonGrammarJSONForTest(t *testing.T) *Grammar {
	t.Helper()

	candidates := []string{
		"/tmp/python-locked-26855ea/src/grammar.json",
		"/tmp/grammar_parity/python/src/grammar.json",
		".parity_seed/python/src/grammar.json",
		"../.parity_seed/python/src/grammar.json",
	}
	jsonPath := ""
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			jsonPath = candidate
			break
		}
	}
	if jsonPath == "" {
		t.Skip("Python grammar.json not available")
	}

	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Python grammar.json not available: %v", err)
	}

	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Python grammar.json: %v", err)
	}
	return gram
}
