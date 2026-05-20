package gotreesitter

import "strings"

func normalizeCobolCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCobolLeadingAreaStart(root, source, lang)
	normalizeCobolTopLevelDefinitionEnd(root, source, lang)
	normalizeCobolDivisionSiblingEnds(root, source, lang)
}
func normalizeCobolLeadingAreaStart(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || len(source) == 0 {
		return
	}
	start := firstNonWhitespaceByte(source)
	if start == 0 {
		// COBOL fixed format: columns 1-6 are sequence numbers (non-whitespace).
		// Detect this pattern and use column 7 (byte 6) as the adjusted start.
		if len(source) >= 7 && (source[6] == ' ' || source[6] == '*' || source[6] == '-' || source[6] == '/') {
			start = 6
		} else {
			return
		}
	}
	startPoint := advancePointByBytes(Point{}, source[:start])
	setNodeStartTo := func(n *Node) {
		if n == nil || n.startByte == start {
			return
		}
		n.startByte = start
		n.startPoint = startPoint
	}
	setNodeStartTo(root)
	if resultChildCount(root) == 0 {
		return
	}
	def := (*Node)(nil)
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			def = child
			break
		}
	}
	if def == nil {
		return
	}
	setNodeStartTo(def)
	if resultChildCount(def) == 0 {
		return
	}
	for i := 0; i < resultChildCount(def); i++ {
		child := resultChildAt(def, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "identification_division" {
			setNodeStartTo(child)
			break
		}
	}
}

func normalizeCobolTopLevelDefinitionEnd(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || resultChildCount(root) == 0 {
		return
	}
	def := (*Node)(nil)
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			def = child
			break
		}
	}
	if def == nil {
		return
	}
	end := lastNonTriviaByteEnd(source)
	if end == 0 || end >= def.endByte {
		return
	}
	def.endByte = end
	def.endPoint = advancePointByBytes(Point{}, source[:end])
}

func normalizeCobolDivisionSiblingEnds(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || resultChildCount(root) == 0 {
		return
	}
	def := (*Node)(nil)
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			def = child
			break
		}
	}
	if def == nil {
		return
	}
	childCount := resultChildCount(def)
	for i := 0; i+1 < childCount; i++ {
		cur := resultChildAt(def, i)
		next := resultChildAt(def, i+1)
		if cur == nil || next == nil || cur.IsExtra() || next.IsExtra() {
			continue
		}
		if !strings.HasSuffix(cur.Type(lang), "_division") {
			continue
		}
		end := lastNonTriviaByteEnd(source[:next.startByte])
		if end == 0 || end <= cur.startByte || end >= cur.endByte {
			continue
		}
		cur.endByte = end
		cur.endPoint = advancePointByBytes(Point{}, source[:end])
	}
}
