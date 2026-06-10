package gotreesitter

// normalizeHTTPCompatibility aligns gotreesitter's http parse tree with C
// tree-sitter for known shape gaps.
func normalizeHTTPCompatibility(root *Node, source []byte, lang *Language) {
	normalizeHTTPDocumentSectionMerges(root, source, lang)
}

// normalizeHTTPDocumentSectionMerges merges document-level `section` nodes
// that do not begin with a request_separator into their preceding section.
//
// The http grammar defines section as
// prec.right(choice(seq(request_separator, optional(_section_content)),
// _section_content)) under document = repeat(section), with a GLR conflict on
// the hidden right-recursive _section_content. C tree-sitter's resolution
// keeps consuming content into the current section, so in its output only the
// first section may lack a leading `###` request_separator. gotreesitter's
// GLR instead starts a fresh section at comment/variable/script/request
// boundaries. Because _section_content is hidden, its elements flatten
// directly into section, so concatenating the split sections' children
// reproduces the C shape exactly.
func normalizeHTTPDocumentSectionMerges(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "http" || root.Type(lang) != "document" || len(root.children) < 2 {
		return
	}
	secSym, ok := lang.symbolByNameAndNamed("section", true)
	if !ok {
		return
	}
	sepSym, ok := lang.symbolByNameAndNamed("request_separator", true)
	if !ok {
		return
	}
	merged := false
	newChildren := make([]*Node, 0, len(root.children))
	var prev *Node
	for _, child := range root.children {
		if child != nil && prev != nil && child.symbol == secSym && prev.symbol == secSym &&
			!httpSectionStartsWithSeparator(child, sepSym) {
			prev.children = appendNodeChildrenInArena(prev, child)
			// The merged span must extend over the absorbed section even when
			// it contributed no visible children (hidden blank-line tokens),
			// so wire parent links manually instead of recomputing the span
			// from children via populateParentNode.
			for i, c := range prev.children {
				setNodeParentLink(c, prev, i)
				if c.hasError() {
					prev.setHasError(true)
				}
			}
			prev.endByte = child.endByte
			prev.endPoint = child.endPoint
			merged = true
			continue
		}
		newChildren = append(newChildren, child)
		prev = child
	}
	if !merged {
		return
	}
	root.children = cloneNodeSliceInArena(root.ownerArena, newChildren)
	// Preserve the document's own span: only rewire parent links.
	for i, c := range root.children {
		setNodeParentLink(c, root, i)
	}
}

func httpSectionStartsWithSeparator(section *Node, sepSym Symbol) bool {
	if section == nil || len(section.children) == 0 {
		return false
	}
	first := section.children[0]
	return first != nil && first.symbol == sepSym
}

// appendNodeChildrenInArena returns prev's children with next's children
// appended, preserving any field assignments either node carried.
func appendNodeChildrenInArena(prev, next *Node) []*Node {
	combined := make([]*Node, 0, len(prev.children)+len(next.children))
	combined = append(combined, prev.children...)
	combined = append(combined, next.children...)
	if prev.fieldIDs != nil || next.fieldIDs != nil {
		fieldIDs := make([]FieldID, len(combined))
		for i := range prev.children {
			if i < len(prev.fieldIDs) {
				fieldIDs[i] = prev.fieldIDs[i]
			}
		}
		for i := range next.children {
			if i < len(next.fieldIDs) {
				fieldIDs[len(prev.children)+i] = next.fieldIDs[i]
			}
		}
		prev.fieldIDs = cloneFieldIDSliceInArena(prev.ownerArena, fieldIDs)
		prev.fieldSources = defaultFieldSourcesInArena(prev.ownerArena, prev.fieldIDs)
	}
	return cloneNodeSliceInArena(prev.ownerArena, combined)
}
