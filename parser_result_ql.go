package gotreesitter

// normalizeQLCompatibility aligns QL trees with C tree-sitter output.
func normalizeQLCompatibility(root *Node, source []byte, lang *Language) {
	normalizeQLSignatureTypeExprs(root, source, lang)
}

// normalizeQLSignatureTypeExprs resolves the signatureExpr GLR ambiguity the
// way the C parser does.
//
// QL's signatureExpr is choice(typeExpr, moduleExpr, predicateExpr), and an
// upper-case identifier path like `DataFlow::ConfigSig` parses both as
//
//	moduleExpr(moduleExpr `DataFlow`, ::, simpleId `ConfigSig`)
//	typeExpr(qualifier: moduleExpr `DataFlow`, ::, name: className `ConfigSig`)
//
// (declared conflict [simpleId, className]). The C oracle resolves the
// ambiguity to the typeExpr form in signature position; the Go GLR keeps
// moduleExpr. The two trees are isomorphic — only the outer node and the
// trailing name token differ — so rewrite moduleExpr→typeExpr and
// simpleId→className for moduleExpr children of signatureExpr whose trailing
// name is an upper-case identifier. Lower-case tails (className is an
// upper-id token in the grammar) and moduleInstantiation tails (`M::I<T>`)
// have no typeExpr parse in C either and stay untouched.
func normalizeQLSignatureTypeExprs(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "ql" || len(source) == 0 {
		return
	}
	signatureSym, ok := lang.symbolByNameAndNamed("signatureExpr", true)
	if !ok {
		return
	}
	moduleExprSym, ok := lang.symbolByNameAndNamed("moduleExpr", true)
	if !ok {
		return
	}
	typeExprSym, ok := lang.symbolByNameAndNamed("typeExpr", true)
	if !ok {
		return
	}
	simpleIdSym, ok := lang.symbolByNameAndNamed("simpleId", true)
	if !ok {
		return
	}
	classNameSym, ok := lang.symbolByNameAndNamed("className", true)
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.symbol != signatureSym || resultChildCount(n) != 1 {
			return
		}
		expr := n.children[0]
		if expr == nil || expr.symbol != moduleExprSym {
			return
		}
		var tail *Node
		switch resultChildCount(expr) {
		case 1:
			tail = expr.children[0]
		case 3:
			tail = expr.children[2]
		default:
			return
		}
		if tail == nil || tail.symbol != simpleIdSym || resultChildCount(tail) != 0 {
			return
		}
		if int(tail.startByte) >= len(source) {
			return
		}
		if c := source[tail.startByte]; c < 'A' || c > 'Z' {
			return
		}
		expr.symbol = typeExprSym
		tail.symbol = classNameSym
	})
}
