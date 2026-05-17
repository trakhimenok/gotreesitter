//go:build diagnostic

package grammargen

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

func TestDiagGenerateStages(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic test")
	}
	if getenvOr("DIAG_GENERATE_STAGES", "") != "1" {
		t.Skip("set DIAG_GENERATE_STAGES=1 to run per-stage generation diagnostics")
	}

	grammarName := strings.TrimSpace(getenvOr("DIAG_GRAMMAR", "cpp"))
	pg, ok := lookupImportParityGrammar(grammarName)
	if !ok {
		t.Fatalf("%s not found in importParityGrammars", grammarName)
	}

	gram, err := importParityGrammarSource(pg)
	if err != nil {
		t.Fatalf("import %s: %v", grammarName, err)
	}

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

	t.Logf("diag-generate: grammar=%s timeout=%s rules=%d extras=%d conflicts=%d externals=%d inline=%d word=%q lr_split=%v binary_repeat=%v choice_lift=%d broad_lex=%v",
		grammarName,
		timeout,
		len(gram.Rules),
		len(gram.Extras),
		len(gram.Conflicts),
		len(gram.Externals),
		len(gram.Inline),
		gram.Word,
		gram.EnableLRSplitting,
		gram.BinaryRepeatMode,
		gram.ChoiceLiftThreshold,
		useForcedBroadLexFallback(),
	)
	t.Logf("diag-generate: mem[start]=%s", diagGenerateMemSnapshot())

	bgCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	totalStart := time.Now()

	stageStart := time.Now()
	warnings := Validate(gram)
	t.Logf("stage=validate dur=%s warnings=%d mem=%s", time.Since(stageStart), len(warnings), diagGenerateMemSnapshot())

	stageStart = time.Now()
	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	t.Logf("stage=normalize dur=%s symbols=%d terminals=%d keywords=%d productions=%d tokens=%d extra_symbols=%d externals=%d mem=%s",
		time.Since(stageStart),
		len(ng.Symbols),
		len(ng.Terminals),
		len(ng.KeywordEntries),
		len(ng.Productions),
		ng.TokenCount(),
		len(ng.ExtraSymbols),
		len(ng.ExternalSymbols),
		diagGenerateMemSnapshot(),
	)
	if raw := strings.TrimSpace(os.Getenv("DIAG_PROD_LHS")); raw != "" {
		wanted := map[string]bool{}
		for _, part := range strings.Split(raw, ",") {
			if part = strings.TrimSpace(part); part != "" {
				wanted[part] = true
			}
		}
		for i, prod := range ng.Productions {
			if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) || !wanted[ng.Symbols[prod.LHS].Name] {
				continue
			}
			t.Logf("prod[%d] %s", i, diagFormatProd(ng, i, -1))
		}
	}

	stageStart = time.Now()
	tables, lrCtx, err := buildLRTablesWithProvenanceCtx(bgCtx, ng)
	if err != nil {
		t.Fatalf("build LR tables: %v", err)
	}
	mergedStates := 0
	if lrCtx.provenance != nil {
		mergedStates = lrCtx.provenance.mergedStateCount()
	}
	t.Logf("stage=build_lr dur=%s states=%d item_sets=%d merged_states=%d mem=%s",
		time.Since(stageStart),
		tables.StateCount,
		len(lrCtx.itemSets),
		mergedStates,
		diagGenerateMemSnapshot(),
	)
	if raw := strings.TrimSpace(os.Getenv("DIAG_ITEM_STATES")); raw != "" {
		itemFilter := strings.TrimSpace(os.Getenv("DIAG_ITEM_FILTER"))
		itemLimit := 0
		if rawLimit := strings.TrimSpace(os.Getenv("DIAG_ITEM_LIMIT")); rawLimit != "" {
			n, err := strconv.Atoi(rawLimit)
			if err != nil {
				t.Fatalf("parse DIAG_ITEM_LIMIT=%q: %v", rawLimit, err)
			}
			itemLimit = n
		}
		for _, rawState := range strings.Split(raw, ",") {
			rawState = strings.TrimSpace(rawState)
			if rawState == "" {
				continue
			}
			state, err := strconv.Atoi(rawState)
			if err != nil {
				t.Fatalf("parse DIAG_ITEM_STATES state %q: %v", rawState, err)
			}
			if state < 0 || state >= len(lrCtx.itemSets) {
				t.Logf("state[%d] out of range", state)
				continue
			}
			t.Logf("state[%d] items:", state)
			loggedItems := 0
			for _, ce := range lrCtx.itemSets[state].cores {
				lookaheads := diagFormatLookaheads(ng, &ce.lookaheads)
				formatted := diagFormatProd(ng, int(ce.prodIdx), int(ce.dot))
				if itemFilter != "" && !strings.Contains(formatted, itemFilter) {
					continue
				}
				if itemLimit > 0 && loggedItems >= itemLimit {
					break
				}
				t.Logf("  item%s %s", lookaheads, formatted)
				loggedItems++
			}
			if state < len(lrCtx.transitions) {
				row := lrCtx.transitions[state]
				for _, edge := range row {
					t.Logf("  goto %s -> %d", diagSymbolName(ng, int(edge.sym)), edge.target)
				}
			}
		}
	}
	defer lrCtx.releaseScratch()

	stageStart = time.Now()
	diags, err := resolveConflictsWithDiag(tables, ng, lrCtx.provenance)
	if err != nil {
		t.Fatalf("resolve conflicts: %v", err)
	}
	glrConflicts := 0
	for _, d := range diags {
		if d.Resolution == "GLR (multiple actions kept)" {
			glrConflicts++
		}
	}
	t.Logf("stage=resolve_conflicts dur=%s conflicts=%d glr_conflicts=%d mem=%s",
		time.Since(stageStart),
		len(diags),
		glrConflicts,
		diagGenerateMemSnapshot(),
	)
	if raw := strings.TrimSpace(os.Getenv("DIAG_CONFLICT_LOOKAHEADS")); raw != "" {
		wanted := map[string]bool{}
		for _, part := range strings.Split(raw, ",") {
			if part = strings.TrimSpace(part); part != "" {
				wanted[part] = true
			}
		}
		stateFilter := map[int]bool{}
		if rawStates := strings.TrimSpace(os.Getenv("DIAG_CONFLICT_STATES")); rawStates != "" {
			for _, part := range strings.Split(rawStates, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				state, err := strconv.Atoi(part)
				if err != nil {
					t.Fatalf("parse DIAG_CONFLICT_STATES state %q: %v", part, err)
				}
				stateFilter[state] = true
			}
		}
		for _, d := range diags {
			if len(stateFilter) > 0 && !stateFilter[d.State] {
				continue
			}
			if d.LookaheadSym < 0 || d.LookaheadSym >= len(ng.Symbols) || !wanted[ng.Symbols[d.LookaheadSym].Name] {
				continue
			}
			t.Logf("conflict[%s]: %s", ng.Symbols[d.LookaheadSym].Name, strings.ReplaceAll(d.String(ng), "\n", " | "))
		}
	}

	stageStart = time.Now()
	splitCandidates := newSplitOracle(diags, lrCtx.provenance, tables, ng).candidates()
	t.Logf("stage=split_candidates dur=%s candidates=%d lr_split_enabled=%v mem=%s",
		time.Since(stageStart),
		len(splitCandidates),
		gram.EnableLRSplitting,
		diagGenerateMemSnapshot(),
	)

	stageStart = time.Now()
	addNonterminalExtraChains(tables, ng, lrCtx)
	t.Logf("stage=extra_chains dur=%s extra_chain_state_start=%d mem=%s",
		time.Since(stageStart),
		tables.ExtraChainStateStart,
		diagGenerateMemSnapshot(),
	)

	stageStart = time.Now()
	tokenCount := ng.TokenCount()
	immediateTokens := make(map[int]bool)
	for _, term := range ng.Terminals {
		if term.Immediate {
			immediateTokens[term.SymbolID] = true
		}
	}
	keywordSet := make(map[int]bool, len(ng.KeywordSymbols))
	for _, ks := range ng.KeywordSymbols {
		keywordSet[ks] = true
	}
	stringPrefixExtensions := computeStringPrefixExtensions(ng.Terminals)
	termPatSyms := terminalPatternSymSet(ng)

	var lexModes []lexModeSpec
	var stateToMode []int
	var afterWSModes []afterWSModeEntry
	if useForcedBroadLexFallback() {
		allSyms := make(map[int]bool)
		for _, term := range ng.Terminals {
			allSyms[term.SymbolID] = true
		}
		for _, sym := range ng.ExtraSymbols {
			if sym > 0 && sym < tokenCount {
				allSyms[sym] = true
			}
		}
		lexModes = []lexModeSpec{{validSymbols: allSyms, skipWhitespace: true}}
		stateToMode = make([]int, tables.StateCount)
	} else {
		lexModes, stateToMode, afterWSModes = computeLexModes(
			tables.StateCount,
			tokenCount,
			func(state, sym int) bool {
				if acts, ok := tables.ActionTable[state]; ok {
					if entry, ok := acts[sym]; ok && len(entry) > 0 {
						return true
					}
				}
				return false
			},
			stringPrefixExtensions,
			ng.ExtraSymbols,
			tables.ExtraChainStateStart,
			immediateTokens,
			ng.ExternalSymbols,
			ng.WordSymbolID,
			keywordSet,
			termPatSyms,
			buildFollowTokensFunc(tables, tokenCount),
			patternImmediateTokenSet(ng),
		)
	}
	t.Logf("stage=compute_lex_modes dur=%s modes=%d state_to_mode=%d after_ws_modes=%d mem=%s",
		time.Since(stageStart),
		len(lexModes),
		len(stateToMode),
		len(afterWSModes),
		diagGenerateMemSnapshot(),
	)

	stageStart = time.Now()
	skipExtras := computeSkipExtras(ng)
	lexStates, lexModeOffsets, err := buildLexDFA(bgCtx, ng.Terminals, ng.ExtraSymbols, skipExtras, lexModes)
	if err != nil {
		t.Fatalf("build lex DFA: %v", err)
	}
	t.Logf("stage=build_lex_dfa dur=%s lex_states=%d lex_mode_offsets=%d mem=%s",
		time.Since(stageStart),
		len(lexStates),
		len(lexModeOffsets),
		diagGenerateMemSnapshot(),
	)

	var keywordLexStates []gotreesitter.LexState
	if len(ng.KeywordEntries) > 0 {
		stageStart = time.Now()
		keywordLexStates, _, err = buildLexDFA(bgCtx, ng.KeywordEntries, nil, nil, []lexModeSpec{{
			validSymbols:   allSymbolsSet(ng.KeywordEntries),
			skipWhitespace: false,
		}})
		if err != nil {
			t.Fatalf("build keyword DFA: %v", err)
		}
		t.Logf("stage=build_keyword_dfa dur=%s keyword_lex_states=%d mem=%s",
			time.Since(stageStart),
			len(keywordLexStates),
			diagGenerateMemSnapshot(),
		)
	}

	stageStart = time.Now()
	lang, err := assemble(ng, tables, lexStates, stateToMode, lexModeOffsets)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	lang.Name = gram.Name
	t.Logf("stage=assemble dur=%s symbol_count=%d token_count=%d state_count=%d lex_states=%d lex_modes=%d mem=%s",
		time.Since(stageStart),
		lang.SymbolCount,
		lang.TokenCount,
		lang.StateCount,
		len(lang.LexStates),
		len(lang.LexModes),
		diagGenerateMemSnapshot(),
	)

	t.Logf("stage=total dur=%s final_mem=%s", time.Since(totalStart), diagGenerateMemSnapshot())
}

func lookupImportParityGrammar(name string) (importParityGrammar, bool) {
	for _, pg := range importParityGrammars {
		if pg.name == name {
			return pg, true
		}
	}
	return importParityGrammar{}, false
}

func diagGenerateMemSnapshot() string {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return fmt.Sprintf(
		"alloc=%.1fMi heap_alloc=%.1fMi heap_inuse=%.1fMi sys=%.1fMi objs=%d gc=%d",
		float64(ms.Alloc)/(1024*1024),
		float64(ms.HeapAlloc)/(1024*1024),
		float64(ms.HeapInuse)/(1024*1024),
		float64(ms.Sys)/(1024*1024),
		ms.HeapObjects,
		ms.NumGC,
	)
}
