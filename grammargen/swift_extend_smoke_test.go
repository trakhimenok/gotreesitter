package grammargen

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Verifies that SwiftGrammar composes through ExtendGrammar and produces a
// runnable Language that parses trivial Swift.
func TestSwiftGrammarExtendSmoke(t *testing.T) {
	skipHeavyGrammarExtendSmokeUnderRace(t, "Swift")

	g := ExtendGrammar("swift_smoke", SwiftGrammar(), func(g *Grammar) {})
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte(`func f() {}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if tree.RootNode().HasError() {
		t.Fatalf("parse error in trivial Swift: %s", tree.RootNode().SExpr(lang))
	}
}

// Verifies that KotlinGrammar composes through ExtendGrammar and produces a
// runnable Language that parses trivial Kotlin.
func TestKotlinGrammarExtendSmoke(t *testing.T) {
	skipHeavyGrammarExtendSmokeUnderRace(t, "Kotlin")

	g := ExtendGrammar("kotlin_smoke", KotlinGrammar(), func(g *Grammar) {})
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte(`val x = 1`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if tree.RootNode().HasError() {
		t.Fatalf("parse error in trivial Kotlin: %s", tree.RootNode().SExpr(lang))
	}
}

func skipHeavyGrammarExtendSmokeUnderRace(t *testing.T, grammarName string) {
	t.Helper()
	if raceEnabled {
		t.Skipf("skipping full %s grammar generation under -race; non-race parity and constructor tests cover this path", grammarName)
	}
}
