package gotreesitter

func normalizeHackCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "async_modifier", "async")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "abstract_modifier", "abstract")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "static_modifier", "static")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "variadic_modifier", "...")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "visibility_modifier", "public", "protected", "private")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "true", "true")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "false", "false")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "null", "null")
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "scope_identifier", "parent", "self", "static")
}
