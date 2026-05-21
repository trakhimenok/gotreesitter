package gotreesitter_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseLanguageSample(t *testing.T, name, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()

	var entry grammars.LangEntry
	var report grammars.ParseSupport
	found := false
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			entry = e
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s language entry not found", name)
	}
	found = false
	for _, r := range grammars.AuditParseSupport() {
		if r.Name == name {
			report = r
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s parse support entry not found", name)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)

	var (
		tree *gotreesitter.Tree
		err  error
	)
	switch report.Backend {
	case grammars.ParseBackendTokenSource:
		tree, err = parser.ParseWithTokenSource(srcBytes, entry.TokenSourceFactory(srcBytes, lang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, err = parser.Parse(srcBytes)
	default:
		t.Fatalf("unsupported %s backend: %s", name, report.Backend)
	}
	if err != nil {
		t.Fatalf("%s parse failed: %v", name, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s parse returned nil tree/root", name)
	}
	if tree.RootNode().HasError() {
		t.Fatalf("%s parse has error: %s", name, tree.ParseRuntime().Summary())
	}
	return tree, lang
}

func TestParseAsmImmediateIntStaysInt(t *testing.T) {
	src := grammars.ParseSmokeSample("asm")
	tree, lang := parseLanguageSample(t, "asm", src)
	t.Cleanup(tree.Release)

	node := tree.RootNode().NamedDescendantForByteRange(19, 20)
	if node == nil {
		t.Fatal("missing named descendant for asm immediate")
	}
	if got, want := node.Type(lang), "int"; got != want {
		t.Fatalf("asm immediate type = %q, want %q", got, want)
	}
}

func TestParseFennelImmediateNumberStaysNumber(t *testing.T) {
	src := grammars.ParseSmokeSample("fennel")
	tree, lang := parseLanguageSample(t, "fennel", src)
	t.Cleanup(tree.Release)

	node := tree.RootNode().NamedDescendantForByteRange(8, 9)
	if node == nil {
		t.Fatal("missing named descendant for fennel number")
	}
	if got, want := node.Type(lang), "number"; got != want {
		t.Fatalf("fennel binding value type = %q, want %q", got, want)
	}
}

func TestParseForthBuiltinOperatorBeatsWord(t *testing.T) {
	src := grammars.ParseSmokeSample("forth")
	tree, lang := parseLanguageSample(t, "forth", src)
	t.Cleanup(tree.Release)

	node := tree.RootNode().NamedDescendantForByteRange(13, 14)
	if node == nil {
		t.Fatal("missing named descendant for forth operator")
	}
	if got, want := node.Type(lang), "operator"; got != want {
		t.Fatalf("forth operator type = %q, want %q", got, want)
	}
}

func TestParseMesonCommandArgumentPrefersVariableunit(t *testing.T) {
	src := grammars.ParseSmokeSample("meson")
	tree, lang := parseLanguageSample(t, "meson", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("meson root child count = %d, want %d", got, want)
	}
	cmd := root.Child(0)
	if cmd == nil {
		t.Fatal("meson root child is nil")
	}
	if got, want := cmd.Type(lang), "normal_command"; got != want {
		t.Fatalf("meson root child type = %q, want %q", got, want)
	}
	arg := cmd.Child(2)
	if arg == nil {
		t.Fatal("meson command argument child is nil")
	}
	if got, want := arg.Type(lang), "variableunit"; got != want {
		t.Fatalf("meson command argument type = %q, want %q", got, want)
	}
}

func TestParseJavaCollapsedModifierAndWildcardChildren(t *testing.T) {
	src := "package p;\n\nimport com.example.*;\n\nclass X { private X() {} }\n"
	tree, lang := parseLanguageSample(t, "java", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	modifiers := firstNodeByTypeAndText(root, lang, []byte(src), "modifiers", "private")
	if modifiers == nil {
		t.Fatalf("missing Java private modifiers node: %s", root.SExpr(lang))
	}
	if got, want := modifiers.ChildCount(), 1; got != want {
		t.Fatalf("modifiers.ChildCount() = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if child := modifiers.Child(0); child == nil || child.Type(lang) != "private" {
		if child == nil {
			t.Fatalf("modifiers child = nil; root=%s", root.SExpr(lang))
		}
		t.Fatalf("modifiers child type = %q, want private; root=%s", child.Type(lang), root.SExpr(lang))
	}

	asterisk := firstNodeByTypeAndText(root, lang, []byte(src), "asterisk", "*")
	if asterisk == nil {
		t.Fatalf("missing Java asterisk node: %s", root.SExpr(lang))
	}
	if got, want := asterisk.ChildCount(), 1; got != want {
		t.Fatalf("asterisk.ChildCount() = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if child := asterisk.Child(0); child == nil || child.Type(lang) != "*" {
		if child == nil {
			t.Fatalf("asterisk child = nil; root=%s", root.SExpr(lang))
		}
		t.Fatalf("asterisk child type = %q, want *; root=%s", child.Type(lang), root.SExpr(lang))
	}
}

func firstNodeByTypeAndText(root *gotreesitter.Node, lang *gotreesitter.Language, source []byte, typ, text string) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.Type(lang) == typ && root.Text(source) == text {
		return root
	}
	for _, child := range root.Children() {
		if got := firstNodeByTypeAndText(child, lang, source, typ, text); got != nil {
			return got
		}
	}
	return nil
}

func TestParseJavaScriptJSXSelfClosingAttributeExpression(t *testing.T) {
	src := "const el = <Avatar userId={foo.creatorId} />\n"
	tree, lang := parseLanguageSample(t, "javascript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("javascript root child count = %d, want %d", got, want)
	}
	stmt := root.Child(0)
	if stmt == nil {
		t.Fatal("javascript root child is nil")
	}
	if got, want := stmt.Type(lang), "lexical_declaration"; got != want {
		t.Fatalf("javascript root child type = %q, want %q", got, want)
	}
}

func TestParseJavaScriptJSXNamespacedSpreadChildren(t *testing.T) {
	src := "const el = <Foo:Bar bar={}>{...children}</Foo:Bar>\n"
	tree, lang := parseLanguageSample(t, "javascript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("javascript root child count = %d, want %d", got, want)
	}
	stmt := root.Child(0)
	if stmt == nil {
		t.Fatal("javascript root child is nil")
	}
	if got, want := stmt.Type(lang), "lexical_declaration"; got != want {
		t.Fatalf("javascript root child type = %q, want %q", got, want)
	}
}

func TestParseTSXJSXSelfClosingAttributeExpression(t *testing.T) {
	src := "const el = <Avatar userId={foo.creatorId} />\n"
	tree, lang := parseLanguageSample(t, "tsx", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("tsx root child count = %d, want %d", got, want)
	}
	stmt := root.Child(0)
	if stmt == nil {
		t.Fatal("tsx root child is nil")
	}
	if got, want := stmt.Type(lang), "lexical_declaration"; got != want {
		t.Fatalf("tsx root child type = %q, want %q", got, want)
	}
}

func TestParseJavaScriptJSXMultipleAttributesAfterExpression(t *testing.T) {
	src := "const el = <Foo bar=\"string\" baz={2} data-i8n=\"dialogs.welcome.heading\" bam />\n"
	tree, lang := parseLanguageSample(t, "javascript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("javascript root child count = %d, want %d", got, want)
	}
	stmt := root.Child(0)
	if stmt == nil {
		t.Fatal("javascript root child is nil")
	}
	if got, want := stmt.Type(lang), "lexical_declaration"; got != want {
		t.Fatalf("javascript root child type = %q, want %q", got, want)
	}
	attrPos := strings.Index(src, "data-i8n")
	if attrPos < 0 {
		t.Fatal("data-i8n attribute not found in sample")
	}
	node := root.NamedDescendantForByteRange(uint32(attrPos), uint32(attrPos+len("data-i8n")))
	if node == nil {
		t.Fatal("javascript data-i8n descendant is nil")
	}
	if got, want := node.Type(lang), "property_identifier"; got != want {
		t.Fatalf("javascript data-i8n type = %q, want %q", got, want)
	}
}

func TestParseTSXJSXMultipleAttributesAfterExpression(t *testing.T) {
	src := "const el = <Foo bar=\"string\" baz={2} data-i8n=\"dialogs.welcome.heading\" bam />\n"
	tree, lang := parseLanguageSample(t, "tsx", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("tsx root child count = %d, want %d", got, want)
	}
	stmt := root.Child(0)
	if stmt == nil {
		t.Fatal("tsx root child is nil")
	}
	if got, want := stmt.Type(lang), "lexical_declaration"; got != want {
		t.Fatalf("tsx root child type = %q, want %q", got, want)
	}
	attrPos := strings.Index(src, "data-i8n")
	if attrPos < 0 {
		t.Fatal("data-i8n attribute not found in sample")
	}
	node := root.NamedDescendantForByteRange(uint32(attrPos), uint32(attrPos+len("data-i8n")))
	if node == nil {
		t.Fatal("tsx data-i8n descendant is nil")
	}
	if got, want := node.Type(lang), "property_identifier"; got != want {
		t.Fatalf("tsx data-i8n type = %q, want %q", got, want)
	}
}

func TestParseJavaScriptJSXStatementBoundaryAfterClosingElement(t *testing.T) {
	src := "var a = <Foo></Foo>\n" +
		"b = <Foo.Bar></Foo.Bar>\n"
	tree, lang := parseLanguageSample(t, "javascript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.NamedChildCount(), 2; got != want {
		t.Fatalf("javascript root named child count = %d, want %d", got, want)
	}
	if stmt := root.NamedChild(0); stmt == nil || stmt.Type(lang) != "variable_declaration" {
		if stmt == nil {
			t.Fatal("javascript first statement is nil")
		}
		t.Fatalf("javascript first statement type = %q, want %q", stmt.Type(lang), "variable_declaration")
	}
	if stmt := root.NamedChild(1); stmt == nil || stmt.Type(lang) != "expression_statement" {
		if stmt == nil {
			t.Fatal("javascript second statement is nil")
		}
		t.Fatalf("javascript second statement type = %q, want %q", stmt.Type(lang), "expression_statement")
	}
}

func TestParseTSXJSXStatementBoundaryAfterClosingElement(t *testing.T) {
	src := "var a = <Foo></Foo>\n" +
		"b = <Foo.Bar></Foo.Bar>\n"
	tree, lang := parseLanguageSample(t, "tsx", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.NamedChildCount(), 2; got != want {
		t.Fatalf("tsx root named child count = %d, want %d", got, want)
	}
	if stmt := root.NamedChild(0); stmt == nil || stmt.Type(lang) != "variable_declaration" {
		if stmt == nil {
			t.Fatal("tsx first statement is nil")
		}
		t.Fatalf("tsx first statement type = %q, want %q", stmt.Type(lang), "variable_declaration")
	}
	if stmt := root.NamedChild(1); stmt == nil || stmt.Type(lang) != "expression_statement" {
		if stmt == nil {
			t.Fatal("tsx second statement is nil")
		}
		t.Fatalf("tsx second statement type = %q, want %q", stmt.Type(lang), "expression_statement")
	}
}
