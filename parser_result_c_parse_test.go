package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestNormalizeCVariadicParameterEllipsis(t *testing.T) {
	const src = "void *f(void *, size_t, size_t, int, ...);\n"
	tree, lang := parseByLanguageName(t, "c", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected C parse error: %s", root.SExpr(lang))
	}

	variadic := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "variadic_parameter"
	})
	if variadic == nil {
		t.Fatalf("missing variadic_parameter: %s", root.SExpr(lang))
	}
	if got, want := variadic.ChildCount(), 1; got != want {
		t.Fatalf("variadic_parameter child count = %d, want %d: %s", got, want, variadic.SExpr(lang))
	}
	if got, want := variadic.Child(0).Type(lang), "..."; got != want {
		t.Fatalf("variadic_parameter child type = %q, want %q", got, want)
	}
}

func TestNormalizeCUnresolvedTypedefLikeCallShape(t *testing.T) {
	const src = "int f(void) { return (mstime_t)(server.unixtime - server.master->lastinteraction) * 1000; }\n"
	tree, lang := parseByLanguageName(t, "c", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected C parse error: %s", root.SExpr(lang))
	}

	wantText := "(mstime_t)(server.unixtime - server.master->lastinteraction)"
	call := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == wantText
	})
	if call == nil {
		t.Fatalf("missing unresolved typedef-like call_expression: %s", root.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "cast_expression" && n.Text([]byte(src)) == wantText
	}); bad != nil {
		t.Fatalf("unexpected cast_expression for unresolved typedef-like call: %s", bad.SExpr(lang))
	}
}

func TestNormalizeCPPConditionClauseAssignmentShape(t *testing.T) {
	const src = "while ((a = b)) {}\n"
	tree, lang := parseByLanguageName(t, "cpp", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected cpp parse error: %s", root.SExpr(lang))
	}

	whileStmt := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "while_statement"
	})
	if whileStmt == nil {
		t.Fatalf("missing while_statement: %s", root.SExpr(lang))
	}

	assign := firstNode(whileStmt, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "assignment_expression" && n.Text([]byte(src)) == "a = b"
	})
	if assign == nil {
		t.Fatalf("missing assignment_expression in while condition: %s", whileStmt.SExpr(lang))
	}
	if got := countNodes(root, func(n *gotreesitter.Node) bool { return n.Type(lang) == "ERROR" }); got != 0 {
		t.Fatalf("unexpected cpp ERROR nodes after normalization: %d\n%s", got, root.SExpr(lang))
	}
}

func TestNormalizeCUDABareTypeIdentifiersBecomeExpressionStatements(t *testing.T) {
	const src = "{\n  _abc;\n  d_EG123;\n}\n"
	tree, lang := parseByLanguageName(t, "cuda", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected cuda parse error: %s", root.SExpr(lang))
	}

	body := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "compound_statement"
	})
	if body == nil {
		t.Fatalf("missing compound_statement: %s", root.SExpr(lang))
	}
	if got := countNodes(body, func(n *gotreesitter.Node) bool { return n.Type(lang) == "expression_statement" }); got != 2 {
		t.Fatalf("cuda expression_statement count = %d, want 2: %s", got, body.SExpr(lang))
	}
	if bad := firstNode(body, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_identifier" && (n.Text([]byte(src)) == "_abc" || n.Text([]byte(src)) == "d_EG123")
	}); bad != nil {
		t.Fatalf("bare identifier still parsed as type_identifier: %s", bad.SExpr(lang))
	}
}
