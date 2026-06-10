package gotreesitter

import "unicode/utf8"

// normalizeRCompatibility aligns R trees with C tree-sitter output.
func normalizeRCompatibility(root *Node, source []byte, lang *Language) {
	normalizeRStringContents(root, source, lang)
}

// normalizeRStringContents rebuilds collapsed string_content nodes.
//
// R's grammar aliases the hidden rules `_single_quoted_string_content` /
// `_double_quoted_string_content` (repeat1 of invisible text chunks and
// visible escape_sequence tokens) to the visible `string_content`. When the
// content holds exactly one escape_sequence, the Go reduce path collapses
// the aliased hidden rule onto that single visible child: string_content
// ends up spanning only the escape bytes and loses the escape_sequence
// child. C instead materializes string_content over the full content span
// (open quote end → close quote start) with the escape_sequence as a child.
// Contents with two or more escape_sequences already build the C shape.
//
// The C shape is fully determined by the source bytes, so rebuild it: reset
// the string_content span to the content span and synthesize one
// escape_sequence leaf per escape token found by re-lexing the content with
// the grammar's exact escape_sequence definition. Bail out (leave the node
// untouched) on any unexpected shape or invalid escape.
func normalizeRStringContents(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "r" || len(source) == 0 {
		return
	}
	stringSym, ok := lang.symbolByNameAndNamed("string", true)
	if !ok {
		return
	}
	escapeSym, ok := lang.symbolByNameAndNamed("escape_sequence", true)
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.symbol != stringSym || resultChildCount(n) != 3 {
			return
		}
		open, mid, close := n.children[0], n.children[1], n.children[2]
		if open == nil || mid == nil || close == nil {
			return
		}
		if !rIsQuoteToken(open, source) || !rIsQuoteToken(close, source) ||
			source[open.startByte] != source[close.startByte] {
			return
		}
		if !mid.IsNamed() || nodeSymbolDisplayName(lang, mid.symbol) != "string_content" {
			return
		}
		contentStart, contentEnd := open.endByte, close.startByte
		if contentStart >= contentEnd || int(contentEnd) > len(source) {
			return
		}
		if resultChildCount(mid) != 0 {
			// Multi-escape contents already carry the C shape; never touch
			// nodes that have children.
			return
		}
		escapes, ok := rScanStringEscapes(source[contentStart:contentEnd])
		if !ok {
			return
		}
		if mid.startByte == contentStart && mid.endByte == contentEnd && len(escapes) == 0 {
			return // already correct
		}
		if len(escapes) == 0 {
			// No escapes but a span mismatch — not the known collapse
			// signature; leave it alone.
			return
		}
		contentStartPoint := open.endPoint
		children := make([]*Node, 0, len(escapes))
		prevEnd := contentStart
		point := contentStartPoint
		for i, span := range escapes {
			escStart := contentStart + uint32(span[0])
			escEnd := contentStart + uint32(span[1])
			point = advancePointByBytes(point, source[prevEnd:escStart])
			startPoint := point
			point = advancePointByBytes(point, source[escStart:escEnd])
			child := newLeafNodeInArena(mid.ownerArena, escapeSym, true, escStart, escEnd, startPoint, point)
			child.parent = mid
			child.childIndex = int32(i)
			children = append(children, child)
			prevEnd = escEnd
		}
		mid.startByte = contentStart
		mid.endByte = contentEnd
		mid.startPoint = contentStartPoint
		mid.endPoint = close.startPoint
		mid.children = cloneNodeSliceInArena(mid.ownerArena, children)
	})
}

func rIsQuoteToken(n *Node, source []byte) bool {
	if n == nil || n.IsNamed() || n.endByte != n.startByte+1 || int(n.endByte) > len(source) {
		return false
	}
	c := source[n.startByte]
	return c == '"' || c == '\''
}

// nodeSymbolDisplayName returns the raw blob symbol name for sym (alias
// symbols included), or "" when out of range.
func nodeSymbolDisplayName(lang *Language, sym Symbol) string {
	if lang == nil || int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
		return ""
	}
	return lang.SymbolNames[sym]
}

// rScanStringEscapes returns the [start,end) offsets of every
// escape_sequence token inside an R string content chunk, or ok=false if a
// backslash run does not form a valid escape.
func rScanStringEscapes(content []byte) ([][2]int, bool) {
	var out [][2]int
	for i := 0; i < len(content); {
		if content[i] != '\\' {
			i++
			continue
		}
		l, ok := rEscapeSequenceLen(content[i:])
		if !ok {
			return nil, false
		}
		out = append(out, [2]int{i, i + l})
		i += l
	}
	return out, true
}

// rEscapeSequenceLen mirrors tree-sitter-r's escape_sequence token:
//
//	token.immediate(seq('\\', choice(
//	  /[^0-9xuU]/,
//	  /[0-7]{1,3}/,
//	  /x[0-9a-fA-F]{1,2}/,
//	  /u[0-9a-fA-F]{1,4}/, /u\{[0-9a-fA-F]{1,4}\}/,
//	  /U[0-9a-fA-F]{1,8}/, /U\{[0-9a-fA-F]{1,8}\}/,
//	)))
//
// with the lexer's maximal-munch behavior. b must start with a backslash.
func rEscapeSequenceLen(b []byte) (int, bool) {
	if len(b) < 2 || b[0] != '\\' {
		return 0, false
	}
	c := b[1]
	switch {
	case c >= '0' && c <= '7':
		n := 1
		for n < 3 && 1+n < len(b) && b[1+n] >= '0' && b[1+n] <= '7' {
			n++
		}
		return 1 + n, true
	case c == '8' || c == '9':
		return 0, false
	case c == 'x':
		n := rCountHex(b[2:], 2)
		if n == 0 {
			return 0, false
		}
		return 2 + n, true
	case c == 'u':
		return rUnicodeEscapeLen(b, 4)
	case c == 'U':
		return rUnicodeEscapeLen(b, 8)
	default:
		// /[^0-9xuU]/ — any other single character, multibyte included.
		_, size := utf8.DecodeRune(b[1:])
		return 1 + size, true
	}
}

func rUnicodeEscapeLen(b []byte, maxHex int) (int, bool) {
	if len(b) > 2 && b[2] == '{' {
		n := rCountHex(b[3:], maxHex)
		if n == 0 || len(b) < 3+n+1 || b[3+n] != '}' {
			return 0, false
		}
		return 3 + n + 1, true
	}
	n := rCountHex(b[2:], maxHex)
	if n == 0 {
		return 0, false
	}
	return 2 + n, true
}

func rCountHex(b []byte, max int) int {
	n := 0
	for n < max && n < len(b) && isHexDigit(b[n]) {
		n++
	}
	return n
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
