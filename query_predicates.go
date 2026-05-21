package gotreesitter

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func (q *Query) matchesPredicates(predicates []QueryPredicate, captures []QueryCapture, lang *Language, source []byte) bool {
	if len(predicates) == 0 {
		return true
	}

	for _, pred := range predicates {
		if !q.matchesPredicate(pred, captures, lang, source) {
			return false
		}
	}

	return true
}

func (q *Query) matchesPredicate(pred QueryPredicate, captures []QueryCapture, lang *Language, source []byte) bool {
	switch pred.kind {
	case predicateEq:
		return textEqualityPredicateMatches(pred, captures, source, true)
	case predicateNotEq:
		return textEqualityPredicateMatches(pred, captures, source, false)
	case predicateMatch, predicateLuaMatch:
		return regexPredicateMatches(pred, captures, source, false)
	case predicateNotMatch:
		return regexPredicateMatches(pred, captures, source, true)
	case predicateAnyEq:
		return anyCaptureTextEquals(pred, captures, source, true)
	case predicateAnyNotEq:
		return anyCaptureTextEquals(pred, captures, source, false)
	case predicateAnyMatch:
		return anyCaptureRegexMatches(pred, captures, source, false)
	case predicateAnyNotMatch:
		return anyCaptureRegexMatches(pred, captures, source, true)
	case predicateAnyOf:
		left, ok := captureText(pred.leftCapture, captures, source)
		return ok && stringInList(left, pred.values)
	case predicateNotAnyOf:
		left, ok := captureText(pred.leftCapture, captures, source)
		return ok && !stringInList(left, pred.values)
	case predicateHasAncestor:
		return ancestorPredicateMatches(pred, captures, lang, false)
	case predicateNotHasAncestor:
		return ancestorPredicateMatches(pred, captures, lang, true)
	case predicateHasParent:
		return parentPredicateMatches(pred, captures, lang, false)
	case predicateNotHasParent:
		return parentPredicateMatches(pred, captures, lang, true)
	case predicateIs:
		return predicateIsSatisfied(pred, captures)
	case predicateIsNot:
		return !predicateIsSatisfied(pred, captures)
	case predicateCount:
		return countPredicateMatches(pred, captures)
	case predicateIsExported:
		return captureTextIsExported(pred.leftCapture, captures, source)
	case predicateSet, predicateOffset, predicateSelectAdjacent, predicateStrip:
		return true
	default:
		return false
	}
}

func (q *Query) predicatesStillViable(predicates []QueryPredicate, captures []QueryCapture, source []byte) bool {
	if len(predicates) == 0 {
		return true
	}

	for _, pred := range predicates {
		if !predicateStillViable(pred, captures, source) {
			return false
		}
	}

	return true
}

func predicateStillViable(pred QueryPredicate, captures []QueryCapture, source []byte) bool {
	switch pred.kind {
	case predicateEq:
		return textEqualityPredicateStillViable(pred, captures, source, true)
	case predicateNotEq:
		return textEqualityPredicateStillViable(pred, captures, source, false)
	case predicateMatch, predicateLuaMatch:
		return regexPredicateStillViable(pred, captures, source, false)
	case predicateNotMatch:
		return regexPredicateStillViable(pred, captures, source, true)
	case predicateAnyOf:
		return listPredicateStillViable(pred, captures, source, true)
	case predicateNotAnyOf:
		return listPredicateStillViable(pred, captures, source, false)
	case predicateIsExported:
		return exportedPredicateStillViable(pred, captures, source)
	default:
		return true
	}
}

func textEqualityPredicateStillViable(pred QueryPredicate, captures []QueryCapture, source []byte, wantEqual bool) bool {
	left, ok := captureText(pred.leftCapture, captures, source)
	if !ok {
		return true
	}
	right, ok := predicateRightText(pred, captures, source)
	if !ok {
		return true
	}
	return (left == right) == wantEqual
}

func regexPredicateStillViable(pred QueryPredicate, captures []QueryCapture, source []byte, negated bool) bool {
	left, ok := captureText(pred.leftCapture, captures, source)
	if !ok {
		return true
	}
	if pred.regex == nil {
		return negated
	}
	matched := pred.regex.MatchString(left)
	if negated {
		return !matched
	}
	return matched
}

func listPredicateStillViable(pred QueryPredicate, captures []QueryCapture, source []byte, wantInList bool) bool {
	left, ok := captureText(pred.leftCapture, captures, source)
	if !ok {
		return true
	}
	return stringInList(left, pred.values) == wantInList
}

func exportedPredicateStillViable(pred QueryPredicate, captures []QueryCapture, source []byte) bool {
	text, ok := captureText(pred.leftCapture, captures, source)
	return !ok || textIsExported(text)
}

func predicatesCanRejectMatch(predicates []QueryPredicate) bool {
	for _, pred := range predicates {
		switch pred.kind {
		case predicateSet, predicateOffset, predicateSelectAdjacent, predicateStrip:
			continue
		default:
			return true
		}
	}
	return false
}

// applyDirectives applies capture-modifying directives (#select-adjacent!,
// #strip!) to the captures list after a match has been accepted.
func (q *Query) applyDirectives(predicates []QueryPredicate, captures []QueryCapture, source []byte) []QueryCapture {
	for _, pred := range predicates {
		switch pred.kind {
		case predicateSelectAdjacent:
			captures = applySelectAdjacent(pred, captures)
		case predicateStrip:
			captures = applyStrip(pred, captures, source)
		}
	}
	return captures
}

// applySelectAdjacent filters the captures named by pred.leftCapture to only
// those that are byte-adjacent to at least one capture named by
// pred.rightCapture. "Adjacent" means one node's end byte equals the other's
// start byte.
func applySelectAdjacent(pred QueryPredicate, captures []QueryCapture) []QueryCapture {
	anchors := captureBoundaries(pred.rightCapture, captures)
	if len(anchors) == 0 {
		return removeCapturesByName(captures, pred.leftCapture)
	}
	return keepAdjacentCaptures(captures, pred.leftCapture, anchors)
}

type captureBoundary struct {
	start, end uint32
}

func captureBoundaries(name string, captures []QueryCapture) []captureBoundary {
	var boundaries []captureBoundary
	for _, c := range captures {
		if c.Name == name && c.Node != nil {
			boundaries = append(boundaries, captureBoundary{c.Node.StartByte(), c.Node.EndByte()})
		}
	}
	return boundaries
}

func removeCapturesByName(captures []QueryCapture, name string) []QueryCapture {
	out := captures[:0]
	for _, c := range captures {
		if c.Name != name {
			out = append(out, c)
		}
	}
	return out
}

func keepAdjacentCaptures(captures []QueryCapture, name string, anchors []captureBoundary) []QueryCapture {
	out := captures[:0]
	for _, c := range captures {
		if c.Name != name || nodeAdjacentToBoundaries(c.Node, anchors) {
			out = append(out, c)
		}
	}
	return out
}

func nodeAdjacentToBoundaries(n *Node, boundaries []captureBoundary) bool {
	if n == nil {
		return false
	}
	nStart := n.StartByte()
	nEnd := n.EndByte()
	for _, boundary := range boundaries {
		if nEnd == boundary.start || nStart == boundary.end {
			return true
		}
	}
	return false
}

// applyStrip applies the #strip! directive: for each capture named by
// pred.leftCapture, it sets TextOverride to the node's text with all
// matches of pred.regex removed.
func applyStrip(pred QueryPredicate, captures []QueryCapture, source []byte) []QueryCapture {
	if pred.regex == nil {
		return captures
	}
	// Mutate captures in place: directive application owns this slice and the
	// updated TextOverride should be visible to downstream consumers.
	for i := range captures {
		if captures[i].Name == pred.leftCapture && captures[i].Node != nil {
			text := captures[i].Node.Text(source)
			stripped := pred.regex.ReplaceAllString(text, "")
			if stripped != text {
				captures[i].TextOverride = stripped
			}
		}
	}
	return captures
}

func captureNodes(name string, captures []QueryCapture) []*Node {
	var nodes []*Node
	for _, c := range captures {
		if c.Name == name && c.Node != nil {
			nodes = append(nodes, c.Node)
		}
	}
	return nodes
}

func predicateRightText(pred QueryPredicate, captures []QueryCapture, source []byte) (string, bool) {
	if pred.rightCapture == "" {
		return pred.literal, true
	}
	return captureText(pred.rightCapture, captures, source)
}

func textEqualityPredicateMatches(pred QueryPredicate, captures []QueryCapture, source []byte, wantEqual bool) bool {
	left, ok := captureText(pred.leftCapture, captures, source)
	if !ok {
		return false
	}
	right, ok := predicateRightText(pred, captures, source)
	if !ok {
		return false
	}
	return (left == right) == wantEqual
}

func regexPredicateMatches(pred QueryPredicate, captures []QueryCapture, source []byte, negated bool) bool {
	left, ok := captureText(pred.leftCapture, captures, source)
	if !ok {
		return false
	}
	if pred.regex == nil {
		return negated
	}
	matched := pred.regex.MatchString(left)
	if negated {
		return !matched
	}
	return matched
}

func anyCaptureTextEquals(pred QueryPredicate, captures []QueryCapture, source []byte, wantEqual bool) bool {
	nodes := captureNodes(pred.leftCapture, captures)
	if len(nodes) == 0 {
		return false
	}
	right, ok := predicateRightText(pred, captures, source)
	if !ok {
		return false
	}
	for _, n := range nodes {
		if (n.Text(source) == right) == wantEqual {
			return true
		}
	}
	return false
}

func anyCaptureRegexMatches(pred QueryPredicate, captures []QueryCapture, source []byte, negated bool) bool {
	nodes := captureNodes(pred.leftCapture, captures)
	if len(nodes) == 0 || pred.regex == nil {
		return false
	}
	for _, n := range nodes {
		matched := pred.regex.MatchString(n.Text(source))
		if negated {
			matched = !matched
		}
		if matched {
			return true
		}
	}
	return false
}

func stringInList(value string, values []string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}

func ancestorPredicateMatches(pred QueryPredicate, captures []QueryCapture, lang *Language, negated bool) bool {
	return captureNodePredicateMatches(pred.leftCapture, captures, negated, func(n *Node) bool {
		return nodeHasAncestorType(n, pred.values, lang)
	})
}

func parentPredicateMatches(pred QueryPredicate, captures []QueryCapture, lang *Language, negated bool) bool {
	return captureNodePredicateMatches(pred.leftCapture, captures, negated, func(n *Node) bool {
		parent := n.Parent()
		return parent != nil && nodeTypeMatchesAny(parent, pred.values, lang)
	})
}

func captureNodePredicateMatches(name string, captures []QueryCapture, negated bool, matches func(*Node) bool) bool {
	nodes := captureNodes(name, captures)
	if len(nodes) == 0 {
		return false
	}
	matched := anyCaptureNodeMatches(nodes, matches)
	if negated {
		return !matched
	}
	return matched
}

func anyCaptureNodeMatches(nodes []*Node, matches func(*Node) bool) bool {
	for _, n := range nodes {
		if matches(n) {
			return true
		}
	}
	return false
}

func countPredicateMatches(pred QueryPredicate, captures []QueryCapture) bool {
	count := 0
	for _, c := range captures {
		if c.Name == pred.leftCapture && c.Node != nil {
			count++
		}
	}
	switch pred.countOp {
	case ">":
		return count > pred.countValue
	case "<":
		return count < pred.countValue
	case ">=":
		return count >= pred.countValue
	case "<=":
		return count <= pred.countValue
	case "==":
		return count == pred.countValue
	case "!=":
		return count != pred.countValue
	default:
		return false
	}
}

func captureTextIsExported(name string, captures []QueryCapture, source []byte) bool {
	text, ok := captureText(name, captures, source)
	return ok && textIsExported(text)
}

func textIsExported(text string) bool {
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text)
	return r != utf8.RuneError && unicode.IsUpper(r)
}

func typeNameMatchesAny(typeName string, names []string) bool {
	for _, n := range names {
		if n == typeName {
			return true
		}
	}
	return false
}

func nodeTypeMatchesAny(node *Node, typeNames []string, lang *Language) bool {
	if node == nil || lang == nil {
		return false
	}
	if typeNameMatchesAny(node.Type(lang), typeNames) {
		return true
	}
	nodeInternal := node.Symbol()
	nodePublic := lang.PublicSymbol(nodeInternal)
	for _, typeName := range typeNames {
		if nodeSymbolMatchesTypeName(nodeInternal, nodePublic, typeName, lang) {
			return true
		}
	}
	return false
}

func nodeSymbolMatchesTypeName(nodeInternal Symbol, nodePublic Symbol, typeName string, lang *Language) bool {
	symbol, ok := lang.SymbolByName(typeName)
	if !ok {
		return false
	}
	if nodeSymbolMatches(nodeInternal, nodePublic, symbol) {
		return true
	}
	return lang.IsSupertype(symbol) && supertypeContainsNodeSymbol(symbol, nodeInternal, nodePublic, lang)
}

func nodeSymbolMatches(nodeInternal Symbol, nodePublic Symbol, candidate Symbol) bool {
	return nodeInternal == candidate || nodePublic == candidate
}

func supertypeContainsNodeSymbol(supertype Symbol, nodeInternal Symbol, nodePublic Symbol, lang *Language) bool {
	for _, child := range lang.SupertypeChildren(supertype) {
		if child == nodeInternal || lang.PublicSymbol(child) == nodePublic {
			return true
		}
	}
	return false
}

func nodeHasAncestorType(node *Node, typeNames []string, lang *Language) bool {
	if node == nil || lang == nil {
		return false
	}
	for p := node.Parent(); p != nil; p = p.Parent() {
		if nodeTypeMatchesAny(p, typeNames, lang) {
			return true
		}
	}
	return false
}

func capturePropertyMatches(captureName string, property string) bool {
	prop := strings.Trim(property, "\"")
	switch prop {
	case "local":
		return strings.Contains(captureName, "local") || strings.Contains(captureName, "parameter")
	case "local.parameter", "parameter":
		return strings.Contains(captureName, "parameter")
	case "function":
		return strings.Contains(captureName, "function")
	case "var", "variable":
		return strings.Contains(captureName, "var") || strings.Contains(captureName, "variable")
	}
	if captureName == prop {
		return true
	}
	return strings.HasSuffix(captureName, "."+prop)
}

func predicateIsSatisfied(pred QueryPredicate, captures []QueryCapture) bool {
	if pred.property == "" {
		return false
	}
	if pred.leftCapture != "" {
		nodes := captureNodes(pred.leftCapture, captures)
		if len(nodes) == 0 {
			return false
		}
		return capturePropertyMatches(pred.leftCapture, pred.property)
	}

	for _, c := range captures {
		if capturePropertyMatches(c.Name, pred.property) {
			return true
		}
	}
	return false
}

func captureText(name string, captures []QueryCapture, source []byte) (string, bool) {
	for _, c := range captures {
		if c.Name == name {
			if source == nil {
				return "", false
			}
			return c.Node.Text(source), true
		}
	}
	return "", false
}
