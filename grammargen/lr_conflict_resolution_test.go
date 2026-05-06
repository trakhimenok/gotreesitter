package grammargen

import "testing"

func TestRRPickBestUsesSymbolVsNamedPrecedenceOrder(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "declaration", Kind: SymbolNonterminal},
			{Name: "expression", Kind: SymbolNonterminal},
			{Name: "internal_module", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 0, RHS: []int{2}, Prec: 13, HasExplicitPrec: true},
			{LHS: 1, RHS: []int{2}},
		},
		PrecedenceOrder: &precOrderTable{
			symbolPositions:    map[string]int{"expression": 2},
			symbolLevels:       map[string]int{"expression": 0},
			namedPrecPositions: map[int]int{13: 1},
		},
	}

	got := rrPickBest([]lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
	}, ng)
	if len(got) != 1 || got[0].prodIdx != 1 {
		t.Fatalf("rrPickBest picked %+v, want expression reduce prodIdx=1", got)
	}
}

func TestResolveReduceReduceKeepsTypeValueSingleTokenAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: ">", Kind: SymbolTerminal},
			{Name: "string", Kind: SymbolNamedToken},
			{Name: "property_identifier", Kind: SymbolNonterminal},
			{Name: "predefined_type", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{1}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want both reduces kept", got)
	}
}

func TestResolveShiftReduceCanPreserveKeywordIdentifierCallAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "(", Kind: SymbolTerminal, Visible: true},
			{Name: "data", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "call_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{2}},
		},
		PreserveKeywordIdentifierConflicts: true,
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 4, prec: 100},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want keyword identifier ambiguity kept", got)
	}
}

func TestResolveShiftReducePrefersSpecificKeywordContinuation(t *testing.T) {
	tests := []struct {
		name  string
		shift lrAction
	}{
		{
			name:  "direct literal continuation",
			shift: lrAction{kind: lrShift, state: 10, lhsSym: 4},
		},
		{
			name:  "statement contributor continuation",
			shift: lrAction{kind: lrShift, state: 10, lhsSym: 5, lhsSyms: []int{6}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ng := &NormalizedGrammar{
				Symbols: []SymbolInfo{
					{Name: "end", Kind: SymbolTerminal},
					{Name: "(", Kind: SymbolTerminal, Visible: true},
					{Name: "null", Kind: SymbolTerminal, Visible: true, Named: false},
					{Name: "identifier", Kind: SymbolNonterminal},
					{Name: "null_literal", Kind: SymbolNonterminal},
					{Name: "_io_arguments", Kind: SymbolNonterminal},
					{Name: "open_statement", Kind: SymbolNonterminal},
				},
				Productions: []Production{
					{LHS: 3, RHS: []int{2}},
				},
				PreserveKeywordIdentifierConflicts: true,
			}

			got, err := resolveActionConflict(1, []lrAction{
				tc.shift,
				{kind: lrReduce, prodIdx: 0},
			}, ng)
			if err != nil {
				t.Fatalf("resolveActionConflict: %v", err)
			}
			if len(got) != 1 || got[0].kind != lrShift {
				t.Fatalf("resolved actions = %+v, want specific keyword shift", got)
			}
		})
	}
}

func TestPropagateEntryShiftMetadataThroughRepeatHelper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "(", Kind: SymbolTerminal},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "call_expression_repeat1", Kind: SymbolNonterminal},
			{Name: "argument_list", Kind: SymbolNonterminal},
			{Name: "call_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 5, RHS: []int{2, 3}, Prec: 80, HasExplicitPrec: true},
			{LHS: 3, RHS: []int{4}},
			{LHS: 4, RHS: []int{1}},
		},
	}
	ctx := &lrContext{
		tokenCount:       2,
		firstSets:        make([]bitset, len(ng.Symbols)),
		nullables:        make([]bool, len(ng.Symbols)),
		prodsByLHS:       map[int][]int{3: {1}, 4: {2}},
		repeatWrapperLHS: make([]bool, len(ng.Symbols)),
	}
	ctx.firstSets[3] = newBitset(2)
	ctx.firstSets[3].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrShift, state: 1, lhsSym: 4}},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 1}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	if len(got) != 1 || got[0].prec != 80 || got[0].lhsSym != 4 {
		t.Fatalf("shift action = %+v, want argument_list shift upgraded to prec 80", got)
	}
	foundCallLHS := false
	for _, lhs := range got[0].lhsSyms {
		if lhs == 5 {
			foundCallLHS = true
			break
		}
	}
	if !foundCallLHS {
		t.Fatalf("shift lhsSyms = %v, want call_expression contributor", got[0].lhsSyms)
	}
}

func TestResolveAuxToParentsUsesCachedReverseParents(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "expression", Kind: SymbolNonterminal},
			{Name: "value_repeat1", Kind: SymbolNonterminal},
			{Name: "value_token1", Kind: SymbolNamedToken},
		},
		Productions: []Production{
			{LHS: 1, RHS: []int{2}},
			{LHS: 0, RHS: []int{1}},
		},
		Conflicts: [][]int{{0}},
	}

	cache := getConflictResolutionCache(ng)
	got := resolveAuxToParents(2, ng, cache)
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("resolveAuxToParents(value_token1) = %v, want [0]", got)
	}

	again := resolveAuxToParents(2, ng, cache)
	if len(again) != 1 || again[0] != 0 {
		t.Fatalf("cached resolveAuxToParents(value_token1) = %v, want [0]", again)
	}
}
