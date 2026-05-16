//go:build !grammar_subset || grammar_subset_java

package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func findFirstNamedDescendant(node *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.IsNamed() && node.Type(lang) == typ {
		return node
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if found := findFirstNamedDescendant(node.NamedChild(i), lang, typ); found != nil {
			return found
		}
	}
	return nil
}

func assertMainStringArrayShape(t *testing.T, tree *gotreesitter.Tree, lang *gotreesitter.Language, src []byte) {
	t.Helper()

	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("expected parse without syntax errors, got sexpr: %s", root.SExpr(lang))
	}
	if root.NamedChildCount() != 2 {
		t.Fatalf("expected root to have 2 named children, got %d: %s", root.NamedChildCount(), root.SExpr(lang))
	}
	if got := root.NamedChild(0).Type(lang); got != "package_declaration" {
		t.Fatalf("root child[0] = %q, want package_declaration", got)
	}
	if got := root.NamedChild(1).Type(lang); got != "class_declaration" {
		t.Fatalf("root child[1] = %q, want class_declaration", got)
	}

	methodDecl := findFirstNamedDescendant(root, lang, "method_declaration")
	if methodDecl == nil {
		t.Fatalf("no method_declaration in parse tree: %s", root.SExpr(lang))
	}
	nameNode := methodDecl.ChildByFieldName("name", lang)
	if nameNode == nil || nameNode.Text(src) != "main" {
		got := "<nil>"
		if nameNode != nil {
			got = nameNode.Text(src)
		}
		t.Fatalf("method name = %q, want %q", got, "main")
	}

	params := findFirstNamedDescendant(methodDecl, lang, "formal_parameters")
	if params == nil {
		t.Fatalf("method_declaration missing formal_parameters: %s", methodDecl.SExpr(lang))
	}
	paramText := strings.Join(strings.Fields(params.Text(src)), "")
	if !strings.Contains(paramText, "String[]args") {
		t.Fatalf("formal_parameters = %q, want to contain String[]args", params.Text(src))
	}

	invocation := findFirstNamedDescendant(methodDecl, lang, "method_invocation")
	if invocation == nil {
		t.Fatalf("method_declaration missing method_invocation: %s", methodDecl.SExpr(lang))
	}
	if !strings.Contains(invocation.Text(src), "System.out.println") {
		t.Fatalf("method_invocation text = %q, want to contain System.out.println", invocation.Text(src))
	}
}

func TestJavaParseMainStringArrayRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`package com.example;

public class App {
    public static void main(String[] args) {
        System.out.println("hello");
    }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	assertMainStringArrayShape(t, tree, lang, src)
}

func TestJavaParseWithTokenSourceMainStringArrayRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`package com.example;

public class App {
    public static void main(String[] args) {
        System.out.println("hello");
    }
}
`)

	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	assertMainStringArrayShape(t, tree, lang, src)
}

func TestJavaParseWithTokenSourceContextualPermitsIdentifierRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    int permits = 1;
    permits++;
  }
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected contextual permits identifier to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseWithTokenSourceSealedPermitsClauseRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`sealed class A permits B {
}

final class B extends A {
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected sealed permits clause to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseWithTokenSourceCompactNestedGenericRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  Queue<IOConsumer<IndexWriter>> queue = new ConcurrentLinkedQueue<>();
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected compact nested generic to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaArrayInitializerTrailingCommaRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  int[] values = {
    1,
    2, // trailing comma remains optional
  };
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected trailing comma array initializer to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseWithTokenSourceShiftExpressionRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    int shifted = value >> 1;
  }
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected shift expression to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseWithTokenSourceUnsignedShiftExpressionRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    int shifted = value >>> 1;
  }
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected unsigned shift expression to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseWithTokenSourceTripleCompactGenericRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  Map<Class<? extends TW>, List<Class<? extends X>>> entries;
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected triple compact generic to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseWithTokenSourceUnderscoreResourceRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() throws Exception {
    try (Closeable _ = resource()) {
    }
  }
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected underscore resource to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseEnhancedForCompactNestedGenericRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    for (Map.Entry<String, List<X>> ent : xs.entrySet()) {
      String field = ent.getKey();
    }
  }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected compact nested generic enhanced-for to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseShiftExpressionAfterCompactAngleSplitter(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    int shifted = value >> 1;
  }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected Java shift expression to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}
