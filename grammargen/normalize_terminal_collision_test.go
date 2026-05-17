package grammargen

import "testing"

func TestNormalizeSeparatesAnonymousStringAndPatternTerminals(t *testing.T) {
	g := NewGrammar("terminal_collision")
	g.Define("source_file", Seq(
		Choice(
			Str(".*"),
			Pat(`.*`),
		),
		Str(";"),
	))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	var stringSymID int = -1
	collisionSymIDs := map[int]struct{}{}
	for _, term := range ng.Terminals {
		if term.Rule == nil {
			continue
		}
		if ng.Symbols[term.SymbolID].Name == ".*" {
			collisionSymIDs[term.SymbolID] = struct{}{}
		}
		if term.Rule.Kind == RuleString && term.Rule.Value == ".*" {
			stringSymID = term.SymbolID
		}
	}

	if stringSymID < 0 {
		t.Fatal("missing anonymous string terminal for \".*\"")
	}
	if ng.Symbols[stringSymID].Name != ".*" {
		t.Fatalf("string terminal display name = %q, want %q", ng.Symbols[stringSymID].Name, ".*")
	}
	if len(collisionSymIDs) != 2 {
		t.Fatalf("expected 2 distinct terminals named %q, got %d", ".*", len(collisionSymIDs))
	}
}

func TestPriorityInlinePatternsPrecedeAliasedInlinePatterns(t *testing.T) {
	g := NewGrammar("priority_inline_patterns")
	g.PriorityInlinePatterns = []string{`[0-9]+`}
	g.Define("source_file", Choice(
		Alias(Pat(`[A-Za-z0-9_]+`), "variable_name", true),
		Pat(`[0-9]+`),
	))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	numberOrder := -1
	variableOrder := -1
	for i, term := range ng.Terminals {
		name := ng.Symbols[term.SymbolID].Name
		switch name {
		case `[0-9]+`:
			numberOrder = i
		case "variable_name":
			variableOrder = i
		}
	}
	if numberOrder < 0 {
		t.Fatal("missing priority inline number terminal")
	}
	if variableOrder < 0 {
		t.Fatal("missing aliased variable terminal")
	}
	if numberOrder > variableOrder {
		t.Fatalf("priority inline terminal order = %d, aliased terminal order = %d; want priority first", numberOrder, variableOrder)
	}
}

func TestReusedStringTokenPreservesLexicalPrecedence(t *testing.T) {
	g := NewGrammar("reused_string_token_precedence")
	g.Define("source_file", Choice(
		Str("-"),
		Seq(Token(Prec(1, Str("-"))), Pat(`[0-9]+`)),
	))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	for _, term := range ng.Terminals {
		if term.Rule != nil && term.Rule.Kind == RuleString && term.Rule.Value == "-" {
			if term.Priority != -1000 {
				t.Fatalf("reused string token priority = %d, want -1000", term.Priority)
			}
			return
		}
	}
	t.Fatal("missing reused string token terminal")
}
