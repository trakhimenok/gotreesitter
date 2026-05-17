//go:build !grammar_subset || (grammar_subset_kotlin && grammar_subset_swift)

package grammars

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestKotlinSwiftExternalScannerSpecs(t *testing.T) {
	kotlinSpec, ok := LookupExternalScannerSpec("kotlin")
	if !ok {
		t.Fatal("missing kotlin external scanner spec")
	}
	if got, want := kotlinSpec.UpstreamRepo, "https://github.com/fwcd/tree-sitter-kotlin"; got != want {
		t.Fatalf("kotlin repo = %q, want %q", got, want)
	}
	if got, want := kotlinSpec.UpstreamCommit, "cbed96ab13dbc082eeeb2e8333c342a62829c29d"; got != want {
		t.Fatalf("kotlin commit = %q, want %q", got, want)
	}
	if got, want := kotlinSpec.Externals, []string{
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
		t.Fatalf("kotlin externals = %v, want %v", got, want)
	}

	swiftSpec, ok := LookupExternalScannerSpec("SWIFT")
	if !ok {
		t.Fatal("missing swift external scanner spec")
	}
	if got, want := swiftSpec.UpstreamRepo, "https://github.com/alex-pinkus/tree-sitter-swift"; got != want {
		t.Fatalf("swift repo = %q, want %q", got, want)
	}
	if got, want := swiftSpec.UpstreamCommit, "41d6e5fe811ec94229ee71771174a8cce558dfee"; got != want {
		t.Fatalf("swift commit = %q, want %q", got, want)
	}
	if got, want := len(swiftSpec.Externals), swtTokenCount; got != want {
		t.Fatalf("swift external count = %d, want %d", got, want)
	}

	swiftSpec.Externals[0] = "mutated"
	again, ok := LookupExternalScannerSpec("swift")
	if !ok {
		t.Fatal("missing swift external scanner spec after mutation")
	}
	if got, want := again.Externals[0], "multiline_comment"; got != want {
		t.Fatalf("swift spec registry was mutated through lookup: got %q, want %q", got, want)
	}
}

func TestLanguageBoundExternalScannersBindBySymbolName(t *testing.T) {
	kotlinLang := externalBindingTestLanguage(
		"_extension_only",
		"_import_dot",
		"safe_nav",
		"_automatic_semicolon",
	)
	kotlinScanner, ok := KotlinExternalScanner{}.ExternalScannerForLanguage(kotlinLang).(KotlinExternalScanner)
	if !ok {
		t.Fatalf("KotlinExternalScanner binding type = %T, want KotlinExternalScanner", KotlinExternalScanner{}.ExternalScannerForLanguage(kotlinLang))
	}
	if got, want := kotlinScanner.externalToToken[0], -1; got != want {
		t.Fatalf("kotlin extension-only external mapped to token %d, want %d", got, want)
	}
	if got, want := kotlinScanner.externalToToken[1], kotlinTokImportDot; got != want {
		t.Fatalf("kotlin import-dot external mapped to token %d, want %d", got, want)
	}
	if got, want := kotlinScanner.externalToToken[2], kotlinTokSafeNav; got != want {
		t.Fatalf("kotlin safe-nav external mapped to token %d, want %d", got, want)
	}
	if got, want := kotlinScanner.symbols[kotlinTokSafeNav], gotreesitter.Symbol(3); got != want {
		t.Fatalf("kotlin safe-nav result symbol = %d, want %d", got, want)
	}

	swiftLang := externalBindingTestLanguage(
		"_fake_try_bang",
		"else",
		"_directive_else",
	)
	swiftScanner, ok := SwiftExternalScanner{}.ExternalScannerForLanguage(swiftLang).(SwiftExternalScanner)
	if !ok {
		t.Fatalf("SwiftExternalScanner binding type = %T, want SwiftExternalScanner", SwiftExternalScanner{}.ExternalScannerForLanguage(swiftLang))
	}
	if got, want := swiftScanner.externalToToken[0], swtTokFakeTryBang; got != want {
		t.Fatalf("swift fake-try-bang external mapped to token %d, want %d", got, want)
	}
	if got, want := swiftScanner.externalToToken[1], swtTokElseKeyword; got != want {
		t.Fatalf("swift else external mapped to token %d, want %d", got, want)
	}
	if got, want := swiftScanner.externalToToken[2], swtTokDirectiveElse; got != want {
		t.Fatalf("swift directive-else external mapped to token %d, want %d", got, want)
	}
	if got, want := swiftScanner.symbols[swtTokDirectiveElse], gotreesitter.Symbol(3); got != want {
		t.Fatalf("swift directive-else result symbol = %d, want %d", got, want)
	}
}

func TestExternalScannerBindingFallbackUsesUnmappedExternalSymbols(t *testing.T) {
	lang := externalBindingTestLanguage(
		"",
		"named_two",
		"",
	)
	names := []string{
		"missing_zero",
		"missing_one",
		"named_two",
	}

	symbols := make([]gotreesitter.Symbol, len(names))
	externalToToken := bindExternalScannerSymbolNames(lang, names, func(tokenIdx int, sym gotreesitter.Symbol) {
		symbols[tokenIdx] = sym
	})

	if got, want := externalToToken, []int{0, 2, 1}; !slices.Equal(got, want) {
		t.Fatalf("externalToToken = %v, want %v", got, want)
	}
	if got, want := symbols[0], gotreesitter.Symbol(1); got != want {
		t.Fatalf("fallback token 0 symbol = %d, want %d", got, want)
	}
	if got, want := symbols[1], gotreesitter.Symbol(3); got != want {
		t.Fatalf("fallback token 1 symbol = %d, want %d", got, want)
	}
	if got, want := symbols[2], gotreesitter.Symbol(2); got != want {
		t.Fatalf("named token 2 symbol = %d, want %d", got, want)
	}
}

func TestExternalScannerBindingFallbackKeepsSameCountAliases(t *testing.T) {
	lang := externalBindingTestLanguage(
		"alias_zero",
		"named_two",
		"alias_one",
	)
	names := []string{
		"missing_zero",
		"missing_one",
		"named_two",
	}

	symbols := make([]gotreesitter.Symbol, len(names))
	externalToToken := bindExternalScannerSymbolNames(lang, names, func(tokenIdx int, sym gotreesitter.Symbol) {
		symbols[tokenIdx] = sym
	})

	if got, want := externalToToken, []int{0, 2, 1}; !slices.Equal(got, want) {
		t.Fatalf("externalToToken = %v, want %v", got, want)
	}
	if got, want := symbols[0], gotreesitter.Symbol(1); got != want {
		t.Fatalf("fallback token 0 symbol = %d, want %d", got, want)
	}
	if got, want := symbols[1], gotreesitter.Symbol(3); got != want {
		t.Fatalf("fallback token 1 symbol = %d, want %d", got, want)
	}
	if got, want := symbols[2], gotreesitter.Symbol(2); got != want {
		t.Fatalf("named token 2 symbol = %d, want %d", got, want)
	}
}

func externalBindingTestLanguage(names ...string) *gotreesitter.Language {
	symbolNames := make([]string, len(names)+1)
	symbols := make([]gotreesitter.Symbol, len(names))
	for i, name := range names {
		symbolNames[i+1] = name
		symbols[i] = gotreesitter.Symbol(i + 1)
	}
	return &gotreesitter.Language{
		SymbolNames:     symbolNames,
		ExternalSymbols: symbols,
	}
}
