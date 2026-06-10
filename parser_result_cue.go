package gotreesitter

import (
	"unicode"
	"unicode/utf8"
)

// normalizeCueCompatibility aligns CUE trees with C tree-sitter output.
func normalizeCueCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCueRootLeadingTriviaStart(root, source, lang)
	normalizeCueValueLeafChildren(root, source, lang)
}

// normalizeCueRootLeadingTriviaStart moves the source_file root start past
// leading whitespace. C tree-sitter roots source_file at the first non-extra
// byte when a file begins with blank lines (root-span-leading-extras);
// gotreesitter roots at byte 0.
func normalizeCueRootLeadingTriviaStart(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "cue" || len(source) == 0 ||
		root.Type(lang) != "source_file" || len(root.children) == 0 {
		return
	}
	first := root.children[0]
	if first == nil || first.startByte == 0 || int(first.startByte) > len(source) {
		return
	}
	for _, b := range source[:first.startByte] {
		switch b {
		case ' ', '\t', '\n', '\r':
		default:
			return
		}
	}
	root.startByte = first.startByte
	root.startPoint = first.startPoint
}

// normalizeCueValueLeafChildren rebuilds collapsed `value` nodes.
//
// CUE's `_value: alias($._alias_expr, $.value)` aliases a hidden rule to the
// visible `value`. When the expression is a single leaf token (identifier,
// number, float), the Go reduce path collapses the alias onto the token:
// `value` comes out as a childless leaf where C produces value(identifier)
// etc. The C shape is determined by the source text, so synthesize the leaf
// child by classifying the covered text with the grammar's token
// definitions. Unknown shapes (keyword literals, operators, anything not
// classifiable) are left untouched.
func normalizeCueValueLeafChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "cue" || len(source) == 0 {
		return
	}
	valueSyms := map[Symbol]bool{}
	for i, name := range lang.SymbolNames {
		if name == "value" && symbolIsNamed(lang, Symbol(i)) {
			valueSyms[Symbol(i)] = true
		}
	}
	if len(valueSyms) == 0 {
		return
	}
	childSyms := map[string]Symbol{}
	for _, name := range []string{"identifier", "number", "float"} {
		if sym, ok := lang.symbolByNameAndNamed(name, true); ok {
			childSyms[name] = sym
		}
	}
	if len(childSyms) == 0 {
		return
	}
	walkResultTree(root, func(n *Node) {
		if !valueSyms[n.symbol] || resultChildCount(n) != 0 {
			return
		}
		if n.startByte >= n.endByte || int(n.endByte) > len(source) {
			return
		}
		kind := cueClassifyValueLeafText(source[n.startByte:n.endByte])
		if kind == "" {
			return
		}
		childSym, ok := childSyms[kind]
		if !ok {
			return
		}
		child := newLeafNodeInArena(n.ownerArena, childSym, true, n.startByte, n.endByte, n.startPoint, n.endPoint)
		child.parent = n
		child.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
	})
}

// cueClassifyValueLeafText classifies text as one of CUE's leaf expression
// tokens — "identifier", "number", "float" — or "" when it matches none of
// them (or matches a distinct keyword rule like true/false/null/top/bottom).
func cueClassifyValueLeafText(text []byte) string {
	switch string(text) {
	case "", "true", "false", "null", "_", "#", "_#", "_|_":
		return ""
	}
	if cueIsIdentifier(text) {
		return "identifier"
	}
	if cueIsFloat(text) {
		return "float"
	}
	if cueIsNumber(text) {
		return "number"
	}
	return ""
}

// cueIsIdentifier mirrors the identifier token:
//
//	optional('_#' | '#' | '_') (\p{L} | '$' | '_') (\p{L} | '$' | '_' | [0-9])*
func cueIsIdentifier(text []byte) bool {
	rest := text
	if len(rest) >= 2 && rest[0] == '_' && rest[1] == '#' {
		rest = rest[2:]
	} else if len(rest) >= 1 && (rest[0] == '#' || rest[0] == '_') {
		// `_` can also be the first identifier character; only strip it as a
		// prefix when more follows either way.
		if rest[0] == '#' {
			rest = rest[1:]
		}
	}
	if len(rest) == 0 {
		return false
	}
	r, size := utf8.DecodeRune(rest)
	if !(unicode.IsLetter(r) || r == '$' || r == '_') {
		return false
	}
	rest = rest[size:]
	for len(rest) > 0 {
		r, size := utf8.DecodeRune(rest)
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '$' || r == '_') {
			return false
		}
		rest = rest[size:]
	}
	return true
}

// cueIsNumber mirrors the number token: hex, binary, octal (sign allowed),
// and decimal (sign allowed, `_` digit separators).
func cueIsNumber(text []byte) bool {
	rest := text
	if len(rest) > 0 && (rest[0] == '-' || rest[0] == '+') {
		rest = rest[1:]
	}
	if len(rest) >= 2 && rest[0] == '0' {
		switch rest[1] {
		case 'x', 'X':
			return cueDigitRun(rest[2:], isHexDigit)
		case 'b', 'B':
			return cueDigitRun(rest[2:], func(c byte) bool { return c == '0' || c == '1' })
		case 'o', 'O':
			return cueDigitRun(rest[2:], func(c byte) bool { return c >= '0' && c <= '7' })
		}
	}
	return cueDigitRun(rest, func(c byte) bool { return c >= '0' && c <= '9' })
}

// cueIsFloat mirrors the float token's decimal forms:
//
//	optional('-') digits? '.' digits? exponent? | digits exponent
func cueIsFloat(text []byte) bool {
	rest := text
	if len(rest) > 0 && rest[0] == '-' {
		rest = rest[1:]
	}
	dot := -1
	exp := -1
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == '.':
			if dot >= 0 || exp >= 0 {
				return false
			}
			dot = i
		case rest[i] == 'e' || rest[i] == 'E':
			if exp >= 0 {
				return false
			}
			exp = i
		}
	}
	if dot < 0 && exp < 0 {
		return false
	}
	isDec := func(c byte) bool { return c >= '0' && c <= '9' }
	intPart := rest
	var fracPart, expPart []byte
	if exp >= 0 {
		expPart = intPart[exp+1:]
		intPart = intPart[:exp]
	}
	if dot >= 0 {
		fracPart = intPart[dot+1:]
		intPart = intPart[:dot]
	}
	if len(intPart) > 0 && !cueDigitRun(intPart, isDec) {
		return false
	}
	if len(fracPart) > 0 && !cueDigitRun(fracPart, isDec) {
		return false
	}
	if exp >= 0 {
		if dot < 0 && len(intPart) == 0 {
			return false
		}
		if len(expPart) > 0 && (expPart[0] == '+' || expPart[0] == '-') {
			expPart = expPart[1:]
		}
		if !cueDigitRun(expPart, isDec) {
			return false
		}
	}
	if dot >= 0 && len(intPart) == 0 && len(fracPart) == 0 {
		return false
	}
	return true
}

// cueDigitRun reports whether text is a non-empty digit run with optional
// single `_` separators between digits: digit (('_')? digit)*.
func cueDigitRun(text []byte, isDigit func(byte) bool) bool {
	if len(text) == 0 || !isDigit(text[0]) {
		return false
	}
	for i := 1; i < len(text); i++ {
		if text[i] == '_' {
			if i+1 >= len(text) || !isDigit(text[i+1]) {
				return false
			}
			i++
			continue
		}
		if !isDigit(text[i]) {
			return false
		}
	}
	return true
}
