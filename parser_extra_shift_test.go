package gotreesitter

import "testing"

func TestApplyActionTerminalExtraShiftPreservesCurrentState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	tok := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}

	parser.applyAction(&s, ParseAction{
		Type:  ParseActionShift,
		State: 0,
		Extra: true,
	}, tok, &anyReduced, &nodeCount, nil, nil, nil, nil, false, nil)

	if got, want := s.top().state, lang.InitialState; got != want {
		t.Fatalf("top state = %d, want %d", got, want)
	}
	if got, want := stackEntryNode(s.top()).parseState, lang.InitialState; got != want {
		t.Fatalf("extra leaf parseState = %d, want %d", got, want)
	}
	if got, want := stackEntryNode(s.top()).preGotoState, lang.InitialState; got != want {
		t.Fatalf("extra leaf preGotoState = %d, want %d", got, want)
	}
}

func TestApplyActionNonterminalExtraShiftUsesActionState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}

	parser.applyAction(&s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
		Extra: true,
	}, tok, &anyReduced, &nodeCount, nil, nil, nil, nil, false, nil)

	if got := s.top().state; got != targetState {
		t.Fatalf("top state = %d, want %d", got, targetState)
	}
	if got := stackEntryNode(s.top()).parseState; got != targetState {
		t.Fatalf("extra leaf parseState = %d, want %d", got, targetState)
	}
	if got, want := stackEntryNode(s.top()).preGotoState, lang.InitialState; got != want {
		t.Fatalf("extra leaf preGotoState = %d, want %d", got, want)
	}
}

func TestApplyActionNonExtraShiftUsesActionState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     1,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}

	parser.applyAction(&s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
	}, tok, &anyReduced, &nodeCount, nil, nil, nil, nil, false, nil)

	if got := s.top().state; got != targetState {
		t.Fatalf("top state = %d, want %d", got, targetState)
	}
	if got := stackEntryNode(s.top()).parseState; got != targetState {
		t.Fatalf("leaf parseState = %d, want %d", got, targetState)
	}
}

func TestApplyActionNoTreeCompactShiftUsesNoTreeNode(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	parser.noTreeBenchmarkOnly = true
	parser.compactNoTreeShiftLeaves = true

	arena := newNodeArena(arenaClassFull)
	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     1,
		StartByte:  2,
		EndByte:    3,
		StartPoint: Point{Row: 0, Column: 2},
		EndPoint:   Point{Row: 0, Column: 3},
	}

	parser.applyAction(&s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
	}, tok, &anyReduced, &nodeCount, arena, nil, nil, nil, false, nil)

	if got := stackEntryNode(s.top()); got != nil {
		t.Fatalf("stackEntryNode = %p, want nil", got)
	}
	leaf := stackEntryNoTreeNode(s.top())
	if leaf == nil {
		t.Fatal("stackEntryNoTreeNode = nil, want compact leaf")
	}
	if got := leaf.parseState; got != targetState {
		t.Fatalf("compact leaf parseState = %d, want %d", got, targetState)
	}
	if got, want := leaf.preGotoState, lang.InitialState; got != want {
		t.Fatalf("compact leaf preGotoState = %d, want %d", got, want)
	}
	if got := arena.leafNodesConstructed; got != 0 {
		t.Fatalf("leafNodesConstructed = %d, want 0", got)
	}
	if got := arena.noTreeLeafNodesConstructed; got != 1 {
		t.Fatalf("noTreeLeafNodesConstructed = %d, want 1", got)
	}
}

func TestApplyActionNoTreeMissingShiftStaysNode(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	parser.noTreeBenchmarkOnly = true
	parser.compactNoTreeShiftLeaves = true

	arena := newNodeArena(arenaClassFull)
	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     1,
		StartByte:  2,
		EndByte:    2,
		StartPoint: Point{Row: 0, Column: 2},
		EndPoint:   Point{Row: 0, Column: 2},
		Missing:    true,
	}

	parser.applyAction(&s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
	}, tok, &anyReduced, &nodeCount, arena, nil, nil, nil, false, nil)

	leaf := stackEntryNode(s.top())
	if leaf == nil {
		t.Fatal("stackEntryNode = nil, want full missing leaf")
	}
	if compact := stackEntryNoTreeNode(s.top()); compact != nil {
		t.Fatalf("stackEntryNoTreeNode = %p, want nil", compact)
	}
	if !leaf.isMissing() || !leaf.hasError() {
		t.Fatalf("missing leaf flags: missing=%v hasError=%v, want both true", leaf.isMissing(), leaf.hasError())
	}
	if got := arena.leafNodesConstructed; got != 1 {
		t.Fatalf("leafNodesConstructed = %d, want 1", got)
	}
	if got := arena.noTreeLeafNodesConstructed; got != 0 {
		t.Fatalf("noTreeLeafNodesConstructed = %d, want 0", got)
	}
}
