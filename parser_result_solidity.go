package gotreesitter

// normalizeSolidityMemberObjectWrappers collapses the redundant `expression`
// wrapper Go emits around the object operand of a `member_expression`.
//
// For `a.b`, the C tree-sitter-solidity oracle builds:
//
//	member_expression [ identifier(object) "." identifier(property) ]
//
// Go's GLR build instead wraps the object identifier in a unary `expression`
// node spanning the identical bytes:
//
//	member_expression [ expression(identifier)(object) "." identifier(property) ]
//
// When the object child is exactly such a single-identifier `expression` over
// the same span, this pass replaces it with the bare inner identifier so the
// shape matches C. The guard (single named identifier child, identical span)
// keeps it from touching genuine compound objects like `a.b.c` or `f().g`,
// whose object operand is itself a member/call expression, not a lone
// identifier wrapper.
func normalizeSolidityMemberObjectWrappers(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "solidity" {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) != "member_expression" {
			return
		}
		for i, child := range n.children {
			if child == nil || child.Type(lang) != "expression" {
				continue
			}
			if len(child.children) != 1 {
				continue
			}
			inner := child.children[0]
			if inner == nil || inner.Type(lang) != "identifier" {
				continue
			}
			// Only collapse a pure unary wrapper: the expression must add no
			// span of its own (no leading/trailing trivia captured) over the
			// inner identifier.
			if inner.startByte != child.startByte || inner.endByte != child.endByte {
				continue
			}
			inner.parent = n
			inner.childIndex = int32(i)
			n.children[i] = inner
		}
	})
}
