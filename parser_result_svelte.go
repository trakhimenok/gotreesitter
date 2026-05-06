package gotreesitter

func normalizeSvelteTrailingExtraTrivia(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "svelte" || root.Type(lang) != "document" || len(root.children) == 0 || len(source) == 0 {
		return
	}
	last := root.children[len(root.children)-1]
	if last == nil || last.IsNamed() || !last.IsExtra() || len(last.children) != 0 {
		return
	}
	if last.Type(lang) != "_tag_value_token1" {
		return
	}
	if last.startByte >= last.endByte || last.endByte != root.endByte || int(last.endByte) > len(source) {
		return
	}
	if !bytesAreTrivia(source[last.startByte:last.endByte]) {
		return
	}
	root.children = root.children[:len(root.children)-1]
	if len(root.fieldIDs) > len(root.children) {
		root.fieldIDs = root.fieldIDs[:len(root.children)]
	}
	if len(root.fieldSources) > len(root.children) {
		root.fieldSources = root.fieldSources[:len(root.children)]
	}
}
