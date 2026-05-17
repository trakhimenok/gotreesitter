package grammargen

import (
	"context"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestBuildLexDFAPrefersLongerStringOverSingleCharPattern(t *testing.T) {
	lexStates, modeOffsets, err := buildLexDFA(
		context.Background(),
		[]TerminalPattern{
			{SymbolID: 1, Rule: Pat(`[^\n]`), Priority: 0},
			{SymbolID: 2, Rule: Str("*/"), Priority: 0},
		},
		nil,
		nil,
		[]lexModeSpec{{
			validSymbols: map[int]bool{1: true, 2: true},
		}},
	)
	if err != nil {
		t.Fatalf("buildLexDFA: %v", err)
	}
	if len(modeOffsets) != 1 {
		t.Fatalf("len(modeOffsets) = %d, want 1", len(modeOffsets))
	}

	lexer := gotreesitter.NewLexer(lexStates, []byte("*/"))
	tok := lexer.Next(uint32(modeOffsets[0]))
	if got, want := tok.Symbol, gotreesitter.Symbol(2); got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(2); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestBuildLexDFAPrefersExtractionOrderForSameLengthTie(t *testing.T) {
	integer, err := expandPatternRule(`\d+`)
	if err != nil {
		t.Fatalf("expand integer: %v", err)
	}
	unquoted, err := expandPatternRule(`[^\r\n \t]+`)
	if err != nil {
		t.Fatalf("expand unquoted: %v", err)
	}
	lexStates, modeOffsets, err := buildLexDFA(
		context.Background(),
		[]TerminalPattern{
			{SymbolID: 2, Rule: integer, Priority: 0},
			{SymbolID: 1, Rule: unquoted, Priority: 0},
		},
		nil,
		nil,
		[]lexModeSpec{{
			validSymbols: map[int]bool{1: true, 2: true},
		}},
	)
	if err != nil {
		t.Fatalf("buildLexDFA: %v", err)
	}
	if len(modeOffsets) != 1 {
		t.Fatalf("len(modeOffsets) = %d, want 1", len(modeOffsets))
	}

	lexer := gotreesitter.NewLexer(lexStates, []byte("0"))
	tok := lexer.Next(uint32(modeOffsets[0]))
	if got, want := tok.Symbol, gotreesitter.Symbol(2); got != want {
		t.Fatalf("same-length token symbol = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("same-length token end = %d, want %d", got, want)
	}

	lexer = gotreesitter.NewLexer(lexStates, []byte("3rdparty"))
	tok = lexer.Next(uint32(modeOffsets[0]))
	if got, want := tok.Symbol, gotreesitter.Symbol(1); got != want {
		t.Fatalf("longer token symbol = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(8); got != want {
		t.Fatalf("longer token end = %d, want %d", got, want)
	}
}

func TestKeywordLikeInlinePatternClassification(t *testing.T) {
	if !isKeywordLikeInlinePattern(`[sS][uU][bB][gG][rR][aA][pP][hH]`) {
		t.Fatalf("case-insensitive keyword pattern should be keyword-like")
	}
	if isKeywordLikeInlinePattern(`[^\r\n \t]+`) {
		t.Fatalf("broad catch-all pattern should not be keyword-like")
	}
}

func TestBuildLexDFAPreservesStringOperatorBeforeLineComment(t *testing.T) {
	lineComment, err := expandPatternRule(`\/\/.*`)
	if err != nil {
		t.Fatalf("expand line comment: %v", err)
	}
	lexStates, modeOffsets, err := buildLexDFA(
		context.Background(),
		[]TerminalPattern{
			{SymbolID: 1, Rule: Str("//"), Priority: 0},
			{SymbolID: 2, Rule: lineComment, Priority: 0},
		},
		nil,
		nil,
		[]lexModeSpec{{
			validSymbols: map[int]bool{1: true, 2: true},
		}},
	)
	if err != nil {
		t.Fatalf("buildLexDFA: %v", err)
	}
	if len(modeOffsets) != 1 {
		t.Fatalf("len(modeOffsets) = %d, want 1", len(modeOffsets))
	}

	lexer := gotreesitter.NewLexer(lexStates, []byte("//rest"))
	tok := lexer.Next(uint32(modeOffsets[0]))
	if got, want := tok.Symbol, gotreesitter.Symbol(1); got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(2); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestLineBreakOnlyRuleDetectsOptionalCRLF(t *testing.T) {
	newline, err := expandPatternRule(`\r?\n`)
	if err != nil {
		t.Fatalf("expand newline pattern: %v", err)
	}
	if !isLineBreakOnlyRule(newline) {
		t.Fatalf(`\r?\n should be line-break-only`)
	}

	whitespace, err := expandPatternRule(`\s`)
	if err != nil {
		t.Fatalf("expand whitespace pattern: %v", err)
	}
	if isLineBreakOnlyRule(whitespace) {
		t.Fatalf(`\s should not be line-break-only`)
	}
}

func TestCollectTransitionMovesMatchesLegacyRanges(t *testing.T) {
	n := &nfa{
		states: []nfaState{
			{transitions: []nfaTransition{
				{lo: 'a', hi: 'f', nextState: 1},
				{lo: 'd', hi: 'h', nextState: 2},
			}},
			{transitions: []nfaTransition{
				{lo: 'b', hi: 'e', nextState: 3},
			}},
			{},
			{},
		},
		start: 0,
	}

	states := []int{0, 1}
	legacyRanges := collectTransitionRanges(n, states)
	moves := collectTransitionMoves(n, states)
	if len(moves) != len(legacyRanges) {
		t.Fatalf("len(moves) = %d, want %d", len(moves), len(legacyRanges))
	}

	for i, move := range moves {
		if move.lo != legacyRanges[i].lo || move.hi != legacyRanges[i].hi {
			t.Fatalf("move[%d] range = [%q,%q], want [%q,%q]", i, move.lo, move.hi, legacyRanges[i].lo, legacyRanges[i].hi)
		}
		want := moveTargets(n, states, legacyRanges[i].lo, legacyRanges[i].hi)
		if !sameIntSlice(move.targets, want) {
			t.Fatalf("move[%d] targets = %v, want %v", i, move.targets, want)
		}
	}
}
