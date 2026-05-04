package grammargen

import (
	"slices"
	"strings"
	"testing"
)

func TestExtendGrammarCopiesImportMetadata(t *testing.T) {
	base := NewGrammar("base")
	base.Define("program", Sym("identifier"))
	base.Define("identifier", Pat(`[a-z]+`))
	base.SetExtras(Pat(`\s+`))
	base.SetConflicts([]string{"program", "identifier"})
	base.SetExternals(Sym("_external_token"))
	base.SetInline("identifier")
	base.SetWord("identifier")
	base.SetSupertypes("program")
	base.ReservedWordSets = []ReservedWordSet{
		{Name: "global", Rules: []*Rule{Str("if"), Str("else")}},
	}
	base.Precedences = [][]PrecEntry{
		{{Name: "call"}, {IsSymbol: true, Name: "program"}},
	}
	base.EnableLRSplitting = true
	base.BinaryRepeatMode = true
	base.ChoiceLiftThreshold = 16
	base.Test("identifier", "abc", "")

	extended := ExtendGrammar("extended", base, func(g *Grammar) {
		g.Rules["identifier"].Value = `[A-Z]+`
		g.Extras[0].Value = `[ \t]+`
		g.Externals[0].Value = "_different_external"
		g.ReservedWordSets[0].Rules[0].Value = "when"
		g.Precedences[0][0].Name = "member"
	})

	if extended.Name != "extended" {
		t.Fatalf("extended.Name = %q, want extended", extended.Name)
	}
	if !extended.EnableLRSplitting || !extended.BinaryRepeatMode {
		t.Fatalf("extension did not inherit generator mode flags")
	}
	if extended.ChoiceLiftThreshold != 16 {
		t.Fatalf("ChoiceLiftThreshold = %d, want 16", extended.ChoiceLiftThreshold)
	}
	if got := extended.Word; got != "identifier" {
		t.Fatalf("Word = %q, want identifier", got)
	}
	if !slices.Equal(extended.Inline, []string{"identifier"}) {
		t.Fatalf("Inline = %v, want [identifier]", extended.Inline)
	}
	if !slices.Equal(extended.Supertypes, []string{"program"}) {
		t.Fatalf("Supertypes = %v, want [program]", extended.Supertypes)
	}
	if len(extended.Tests) != 1 || extended.Tests[0].Name != "identifier" {
		t.Fatalf("Tests = %+v, want copied identifier test", extended.Tests)
	}

	if base.Rules["identifier"].Value != `[a-z]+` {
		t.Fatalf("base rule was mutated through extension: %q", base.Rules["identifier"].Value)
	}
	if base.Extras[0].Value != `\s+` {
		t.Fatalf("base extra was mutated through extension: %q", base.Extras[0].Value)
	}
	if base.Externals[0].Value != "_external_token" {
		t.Fatalf("base external was mutated through extension: %q", base.Externals[0].Value)
	}
	if base.ReservedWordSets[0].Rules[0].Value != "if" {
		t.Fatalf("base reserved word was mutated through extension: %q", base.ReservedWordSets[0].Rules[0].Value)
	}
	if base.Precedences[0][0].Name != "call" {
		t.Fatalf("base precedence was mutated through extension: %q", base.Precedences[0][0].Name)
	}
}

func TestEmitGrammarGoIncludesImportMetadata(t *testing.T) {
	g := NewGrammar("metadata")
	g.Define("program", Sym("identifier"))
	g.Define("identifier", Pat(`[a-z]+`))
	g.ReservedWordSets = []ReservedWordSet{
		{Name: "global", Rules: []*Rule{Str("if")}},
	}
	g.Precedences = [][]PrecEntry{
		{{Name: "call"}, {IsSymbol: true, Name: "program"}},
	}
	g.EnableLRSplitting = true
	g.BinaryRepeatMode = true
	g.ChoiceLiftThreshold = 8
	g.Test("valid identifier", "abc", "(program (identifier))")
	g.TestError("invalid identifier", "123")

	source, err := EmitGrammarGo(g, "grammargen", "MetadataGrammar")
	if err != nil {
		t.Fatalf("EmitGrammarGo: %v", err)
	}
	text := string(source)
	for _, want := range []string{
		"g.ReservedWordSets = []ReservedWordSet{",
		"g.Precedences = [][]PrecEntry{",
		"{IsSymbol: true, Name: \"program\"}",
		"g.EnableLRSplitting = true",
		"g.BinaryRepeatMode = true",
		"g.ChoiceLiftThreshold = 8",
		"g.Test(\"valid identifier\", \"abc\", \"(program (identifier))\")",
		"g.TestError(\"invalid identifier\", \"123\")",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("emitted source missing %q:\n%s", want, text)
		}
	}
}

func TestImportedKotlinSwiftGrammarConstructors(t *testing.T) {
	kotlin := KotlinGrammar()
	if kotlin.Name != "kotlin" {
		t.Fatalf("KotlinGrammar().Name = %q, want kotlin", kotlin.Name)
	}
	if len(kotlin.Rules) != 201 {
		t.Fatalf("KotlinGrammar rule count = %d, want 201", len(kotlin.Rules))
	}
	if got, want := externalRuleNames(kotlin), []string{
		"_automatic_semicolon",
		"_import_list_delimiter",
		"safe_nav",
		"multiline_comment",
		"_string_start",
		"_string_end",
		"string_content",
		"_primary_constructor_keyword",
		"_import_dot",
	}; !slices.Equal(got, want) {
		t.Fatalf("KotlinGrammar externals = %v, want %v", got, want)
	}

	swift := SwiftGrammar()
	if swift.Name != "swift" {
		t.Fatalf("SwiftGrammar().Name = %q, want swift", swift.Name)
	}
	if len(swift.Rules) != 298 {
		t.Fatalf("SwiftGrammar rule count = %d, want 298", len(swift.Rules))
	}
	if got, want := externalRuleNames(swift), []string{
		"multiline_comment",
		"raw_str_part",
		"raw_str_continuing_indicator",
		"raw_str_end_part",
		"_implicit_semi",
		"_explicit_semi",
		"_arrow_operator_custom",
		"_dot_custom",
		"_conjunction_operator_custom",
		"_disjunction_operator_custom",
		"_nil_coalescing_operator_custom",
		"_eq_custom",
		"_eq_eq_custom",
		"_plus_then_ws",
		"_minus_then_ws",
		"_bang_custom",
		"_throws_keyword",
		"_rethrows_keyword",
		"default_keyword",
		"where_keyword",
		"else",
		"catch_keyword",
		"_as_custom",
		"_as_quest_custom",
		"_as_bang_custom",
		"_async_keyword_custom",
		"_custom_operator",
		"_hash_symbol_custom",
		"_directive_if",
		"_directive_elseif",
		"_directive_else",
		"_directive_endif",
		"_fake_try_bang",
	}; !slices.Equal(got, want) {
		t.Fatalf("SwiftGrammar externals = %v, want %v", got, want)
	}
}

func externalRuleNames(g *Grammar) []string {
	out := make([]string, len(g.Externals))
	for i, rule := range g.Externals {
		if rule != nil {
			out[i] = rule.Value
		}
	}
	return out
}
