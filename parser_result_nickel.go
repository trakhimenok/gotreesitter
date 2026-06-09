package gotreesitter

// normalizeNickelCompatibility applies narrow post-build tree rewrites that keep
// gotreesitter output aligned with C tree-sitter for the Nickel grammar.
//
// The Nickel grammar defines last_field as CHOICE(field_decl, "..").  When the
// last field of a record is the open-record ellipsis ("..") rather than a
// field_decl, C tree-sitter keeps the anonymous ".." token as the single child
// of the named last_field node.  The reduction collapse path drops that token,
// leaving last_field as a zero-child leaf, so restore it here to match C
// (last_field ChildCount()==1).
func normalizeNickelCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "last_field", "..")
}
