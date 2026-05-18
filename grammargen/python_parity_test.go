package grammargen

import (
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPythonPatternMatchingParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "match event.get():\n" +
		"    case Click(position=(x, y)):\n" +
		"        handle_click_at(x, y)\n" +
		"    case KeyPress(key_name=\"Q\") | Quit():\n" +
		"        game.quit()\n" +
		"    case KeyPress(key_name=\"up arrow\"):\n" +
		"        game.go_north()\n" +
		"        ...\n" +
		"    case KeyPress():\n" +
		"        pass # Ignore other keystrokes\n" +
		"    case other_event:\n" +
		"        raise ValueError(f\"Unrecognized event: {other_event}\")\n"

	assertPythonParity(t, genLang, refLang, sample)
}

func TestPythonFStringLiteralParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "# nested!\n" +
		"f\"a {b(f'c {e} d')} e\"\n" +
		"f\"\"\"a\"{b}c\"\"\"\n" +
		"f\"\"\"a\"\"{b}c\"\"\"\n" +
		"f\"a {{}} e\"\n" +
		"f\"a {b}}}\"\n" +
		"f\"a {{{b}\"\n" +
		"f\"a {{b}}\"\n" +
		"f\"a {{{b}}}\"\n" +
		"f\"{c,}\"\n" +
		"f\"{yield d}\"\n" +
		"f\"{*a,}\"\n" +
		"\n" +
		"def function():\n" +
		"    return f\"\"\"\n" +
		"{\"string1\" if True else\n" +
		" \"string2\"}\"\"\"\n" +
		"\n" +
		"def test(self):\n" +
		"    self.assertEqual(f'''A complex trick: {\n" +
		"2  # two\n" +
		"}''', 'A complex trick: 2')\n"

	assertPythonParity(t, genLang, refLang, sample)
}

func TestPython2PrintChevronParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "def driver(file, gulp):\n" +
		"    print >> sys.stdout, 1, 2, 3\n" +
		"    print >> sys.stdout\n" +
		"    print >> gulp, 1, 2, 3,\n" +
		"    print >> file, 'hello world'\n"

	assertPythonParity(t, genLang, refLang, sample)
}

func TestPythonTypeAliasStatementParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	samples := []string{
		"type Point = tuple[float, float]\n",
		"type Point[T] = tuple[T, T]\n",
		"type IntFunc[**P] = Callable[P, int]\n",
		"type LabeledTuple[*Ts] = tuple[str, *Ts]\n",
		"type HashableSequence[T: Hashable] = Sequence[T]\n",
		"type IntOrStrSequence[T: (int, str)] = Sequence[T]\n",
		"type Point = tuple[float, float]\n" +
			"type Point[T] = tuple[T, T]\n" +
			"type IntFunc[**P] = Callable[P, int]  # ParamSpec\n" +
			"type LabeledTuple[*Ts] = tuple[str, *Ts]  # TypeVarTuple\n" +
			"type HashableSequence[T: Hashable] = Sequence[T]  # TypeVar with bound\n" +
			"type IntOrStrSequence[T: (int, str)] = Sequence[T]  # TypeVar with constraints\n",
	}

	for _, sample := range samples {
		assertPythonParity(t, genLang, refLang, sample)
	}
}

func TestPythonDoubleStarParity(t *testing.T) {
	genLang := loadGeneratedPythonLanguageForParity(t)
	refLang := grammars.PythonLanguage()
	adaptExternalScanner(refLang, genLang)

	samples := []string{
		"def g(**kwarg):\n    pass\n",
		"def g(h, i, /, j, *, k=100, **kwarg):\n    pass\n",
		"async def i(a, b=c, *c, **d):\n    a\n",
		"x = a ** b\n",
		"type IntFunc[**P] = Callable[P, int]\n",
	}

	for _, sample := range samples {
		assertPythonParity(t, genLang, refLang, sample)
	}
}

func TestTokenPrecStringDoesNotGloballyOverrideLongerBareString(t *testing.T) {
	g := NewGrammar("token_prec_prefix")
	g.Define("source_file", Choice(Sym("short"), Sym("long"), Seq(Str("q"), Sym("contextual"))))
	g.Define("short", Seq(Str("*"), Str("x")))
	g.Define("long", Seq(Str("**"), Str("y")))
	g.Define("contextual", Seq(Token(Prec(1, Str("*"))), Str("z")))

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}

	tree, err := gotreesitter.NewParser(lang).Parse([]byte("**y"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(3); got != want {
		t.Fatalf("root end byte = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.SExpr(lang), "(source_file (long))"; got != want {
		t.Fatalf("SExpr = %s, want %s", got, want)
	}
}

func loadGeneratedPythonLanguageForParity(t *testing.T) *gotreesitter.Language {
	t.Helper()

	gram := loadPythonGrammarJSONForTest(t)
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Python language: %v", err)
	}
	return genLang
}

func assertPythonParity(t *testing.T, genLang, refLang *gotreesitter.Language, sample string) {
	t.Helper()

	genTree, err := gotreesitter.NewParser(genLang).Parse([]byte(sample))
	if err != nil {
		t.Fatalf("generated parse returned error: %v", err)
	}
	refTree, err := gotreesitter.NewParser(refLang).Parse([]byte(sample))
	if err != nil {
		t.Fatalf("reference parse returned error: %v", err)
	}
	t.Cleanup(genTree.Release)
	t.Cleanup(refTree.Release)

	genRoot := genTree.RootNode()
	refRoot := refTree.RootNode()
	genSexp := genRoot.SExpr(genLang)
	refSexp := refRoot.SExpr(refLang)

	if genRoot.HasError() || refRoot.HasError() {
		t.Fatalf("error mismatch\nGEN hasError=%v\nGEN: %s\nREF hasError=%v\nREF: %s",
			genRoot.HasError(), genSexp, refRoot.HasError(), refSexp)
	}
	if genSexp != refSexp {
		t.Fatalf("SExpr mismatch\nGEN: %s\nREF: %s", genSexp, refSexp)
	}
	if divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 10); len(divs) > 0 {
		t.Fatalf("deep mismatch: %s\nGEN: %s\nREF: %s", divs[0].String(), genSexp, refSexp)
	}
}
