//go:build diagnostic

package grammargen

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestDiagParseSample(t *testing.T) {
	if getenvOr("DIAG_PARSE_SAMPLE", "") != "1" {
		t.Skip("set DIAG_PARSE_SAMPLE=1 to parse a sample with a generated language")
	}
	if getenvOr("DIAG_DEBUG_DFA", "") == "1" {
		gotreesitter.DebugDFA.Store(true)
		defer gotreesitter.DebugDFA.Store(false)
	}

	grammarName := strings.TrimSpace(getenvOr("DIAG_GRAMMAR", "bash"))
	pg, ok := lookupImportParityGrammar(grammarName)
	if !ok {
		t.Fatalf("%s not found in importParityGrammars", grammarName)
	}

	gram, err := importParityGrammarSource(pg)
	if err != nil {
		t.Fatalf("import %s: %v", grammarName, err)
	}
	applyDiagGrammarKnobs(t, gram)

	src := []byte(getenvOr("DIAG_PARSE_SOURCE", ""))
	if path := strings.TrimSpace(os.Getenv("DIAG_PARSE_SOURCE_FILE")); path != "" {
		src, err = os.ReadFile(path)
		if err != nil {
			t.Fatalf("read DIAG_PARSE_SOURCE_FILE=%q: %v", path, err)
		}
	}
	if len(src) == 0 {
		src = []byte(grammarsParseSmokeSample(grammarName))
	}
	cases := []diagParseCase{{name: "sample", src: src}}
	if rawCases := strings.TrimSpace(os.Getenv("DIAG_PARSE_CASES")); rawCases != "" {
		cases = splitDiagParseCases(rawCases)
	}
	if path := strings.TrimSpace(os.Getenv("DIAG_PARSE_CASES_FILE")); path != "" {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read DIAG_PARSE_CASES_FILE=%q: %v", path, err)
		}
		cases = splitDiagParseCases(string(source))
	}

	timeout := pg.genTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if raw := strings.TrimSpace(os.Getenv("DIAG_GENERATE_TIMEOUT")); raw != "" {
		override, err := time.ParseDuration(raw)
		if err != nil {
			t.Fatalf("parse DIAG_GENERATE_TIMEOUT=%q: %v", raw, err)
		}
		timeout = override
	}

	t.Logf("diag-parse: grammar=%s timeout=%s cases=%d inline=%d binary_repeat=%v choice_lift=%d",
		grammarName, timeout, len(cases), len(gram.Inline), gram.BinaryRepeatMode, gram.ChoiceLiftThreshold)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	genLang, err := GenerateLanguageWithContext(ctx, gram)
	if err != nil {
		t.Fatalf("generate %s: %v", grammarName, err)
	}
	refLang := pg.blobFunc()
	adaptExternalScanner(refLang, genLang)
	if getenvOr("DIAG_USE_REF_EXTERNAL_LEX_STATES", "") == "1" {
		els := grammars.LookupExternalLexStates(grammarName)
		if els == nil {
			t.Fatalf("no registered external lex states for %s", grammarName)
		}
		genLang.ExternalLexStates = els
		t.Logf("diag-parse: using reference external lex states rows=%d", len(els))
	}
	t.Logf("diag-parse: gen_external_symbols=%s", diagExternalSymbols(genLang))
	t.Logf("diag-parse: ref_external_symbols=%s", diagExternalSymbols(refLang))
	t.Logf("diag-parse: gen_external_lex_rows=%d ref_external_lex_rows=%d",
		len(genLang.ExternalLexStates), len(refLang.ExternalLexStates))
	if rawStates := strings.TrimSpace(os.Getenv("DIAG_ACTION_STATES")); rawStates != "" {
		for _, rawState := range strings.Split(rawStates, ",") {
			rawState = strings.TrimSpace(rawState)
			if rawState == "" {
				continue
			}
			state, err := strconv.Atoi(rawState)
			if err != nil {
				t.Fatalf("parse DIAG_ACTION_STATES state %q: %v", rawState, err)
			}
			t.Logf("diag-parse: gen_state_%d_actions=%s", state, diagStateActions(genLang, gotreesitter.StateID(state)))
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			genParser := gotreesitter.NewParser(genLang)
			refParser := gotreesitter.NewParser(refLang)
			if getenvOr("DIAG_GLR_TRACE", "") == "1" {
				genParser.SetGLRTrace(true)
			}
			genTree, _ := genParser.Parse(tc.src)
			refTree, _ := refParser.Parse(tc.src)
			if genTree != nil {
				defer genTree.Release()
			}
			if refTree != nil {
				defer refTree.Release()
			}
			genRoot := genTree.RootNode()
			refRoot := refTree.RootNode()
			genSexp := safeSExpr(genRoot, genLang, 256)
			refSexp := safeSExpr(refRoot, refLang, 256)
			t.Logf("src=%q", diagShort(string(tc.src), 400))
			t.Logf("gen: states=%d symbols=%d tokens=%d has_error=%v sexpr=%s",
				genLang.StateCount, genLang.SymbolCount, genLang.TokenCount, genRoot.HasError(), diagShort(genSexp, 1200))
			t.Logf("ref: states=%d symbols=%d tokens=%d has_error=%v sexpr=%s",
				refLang.StateCount, refLang.SymbolCount, refLang.TokenCount, refRoot.HasError(), diagShort(refSexp, 1200))
			if divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 8); len(divs) > 0 {
				for i, div := range divs {
					t.Logf("div[%d]: %s", i, div.String())
				}
			}
		})
	}
}

type diagParseCase struct {
	name string
	src  []byte
}

func splitDiagParseCases(raw string) []diagParseCase {
	parts := strings.Split(raw, "\n---\n")
	cases := make([]diagParseCase, 0, len(parts))
	for i, part := range parts {
		part = strings.Trim(part, "\n")
		if part == "" {
			continue
		}
		name := "case_" + strconv.Itoa(i+1)
		if line, rest, ok := strings.Cut(part, "\n"); ok && strings.HasPrefix(line, "# name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "# name:"))
			part = rest
		}
		cases = append(cases, diagParseCase{name: name, src: []byte(part)})
	}
	return cases
}

func applyDiagGrammarKnobs(t *testing.T, gram *Grammar) {
	t.Helper()
	if getenvOr("DIAG_ENABLE_LR_SPLIT", "") == "1" {
		gram.EnableLRSplitting = true
	}
	if getenvOr("DIAG_BINARY_REPEAT", "") == "1" {
		gram.BinaryRepeatMode = true
	}
	if raw := strings.TrimSpace(os.Getenv("DIAG_CHOICE_LIFT_THRESHOLD")); raw != "" {
		threshold, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("parse DIAG_CHOICE_LIFT_THRESHOLD=%q: %v", raw, err)
		}
		gram.ChoiceLiftThreshold = threshold
	}
	if getenvOr("DIAG_CLEAR_INLINE", "") == "1" {
		gram.Inline = nil
	}
}

func grammarsParseSmokeSample(name string) string {
	for _, pg := range importParityGrammars {
		if pg.name == name && len(pg.samples) > 0 {
			return pg.samples[0]
		}
	}
	return ""
}

func diagShort(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func diagExternalSymbols(lang *gotreesitter.Language) string {
	if lang == nil {
		return ""
	}
	names := make([]string, 0, len(lang.ExternalSymbols))
	for i, sym := range lang.ExternalSymbols {
		name := ""
		if int(sym) >= 0 && int(sym) < len(lang.SymbolNames) {
			name = lang.SymbolNames[sym]
		}
		names = append(names, strconv.Itoa(i)+":"+name+"="+strconv.Itoa(int(sym)))
	}
	return strings.Join(names, ",")
}

func diagStateActions(lang *gotreesitter.Language, state gotreesitter.StateID) string {
	if lang == nil {
		return ""
	}
	parts := make([]string, 0)
	for sym := gotreesitter.Symbol(0); uint32(sym) < lang.SymbolCount; sym++ {
		idx := lookupActionIndexForLanguage(lang, state, sym)
		if idx == 0 || int(idx) >= len(lang.ParseActions) {
			continue
		}
		name := ""
		if int(sym) < len(lang.SymbolNames) {
			name = lang.SymbolNames[sym]
		}
		parts = append(parts, strconv.Itoa(int(sym))+":"+name+"=>"+diagParseActions(lang.ParseActions[idx].Actions, lang))
	}
	return strings.Join(parts, "; ")
}

func diagParseActions(actions []gotreesitter.ParseAction, lang *gotreesitter.Language) string {
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		switch action.Type {
		case gotreesitter.ParseActionShift:
			parts = append(parts, "shift:"+strconv.Itoa(int(action.State)))
		case gotreesitter.ParseActionReduce:
			name := ""
			if int(action.Symbol) < len(lang.SymbolNames) {
				name = lang.SymbolNames[action.Symbol]
			}
			parts = append(parts, "reduce:"+name+"/"+strconv.Itoa(int(action.ChildCount))+"/p"+strconv.Itoa(int(action.ProductionID))+"/dp"+strconv.Itoa(int(action.DynamicPrecedence)))
		case gotreesitter.ParseActionAccept:
			parts = append(parts, "accept")
		case gotreesitter.ParseActionRecover:
			parts = append(parts, "recover:"+strconv.Itoa(int(action.State)))
		default:
			parts = append(parts, "action:"+strconv.Itoa(int(action.Type)))
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}
