package gotreesitter

func normalizeGitRebaseCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "option", "-c", "-C")
}
