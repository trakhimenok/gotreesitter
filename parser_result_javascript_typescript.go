package gotreesitter

import "bytes"

func normalizeJavaScriptCompatibility(root *Node, source []byte, lang *Language) {
	normalizeJavaScriptProgramStart(root, lang)
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "empty_statement", ";")
	normalizeJavaScriptTypeScriptStatementKeywordLeaves(root, source, lang)
	normalizeJavaScriptTypeScriptOptionalChainLeaves(root, lang)
	normalizeJavaScriptTypeScriptCallPrecedence(root, lang)
	normalizeJavaScriptTypeScriptUnaryPrecedence(root, lang)
	normalizeJavaScriptTypeScriptBinaryPrecedence(root, lang)
	normalizeJavaScriptTrailingContinueComments(root, source, lang)
	normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)
	normalizeJavaScriptTopLevelDeclarationBounds(root, lang)
	normalizeJavaScriptTopLevelObjectLiterals(root, lang)
	normalizeJavaScriptProgramEnd(root, source, lang)
	normalizeJavaScriptTypeScriptStatementKeywordLeaves(root, source, lang)
}

func normalizeTypeScriptTreeCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCollapsedNamedLeafChildrenBySource(root, source, lang, "empty_statement", ";")
	normalizeJavaScriptTypeScriptStatementKeywordLeaves(root, source, lang)
	normalizeJavaScriptTypeScriptOptionalChainLeaves(root, lang)
	normalizeJavaScriptTypeScriptCallPrecedence(root, lang)
	normalizeJavaScriptTypeScriptUnaryPrecedence(root, lang)
	normalizeJavaScriptTypeScriptBinaryPrecedence(root, lang)
	normalizeTypeScriptRecoveredNamespaceRoot(root, source, lang)
	normalizeJavaScriptTopLevelDeclarationBounds(root, lang)
	normalizeTypeScriptCompatibility(root, source, lang)
	normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)
	normalizeCollapsedNamedLeafChildren(root, lang, "existential_type", "*")
	normalizeJavaScriptTypeScriptStatementKeywordLeaves(root, source, lang)
}

func normalizeJavaScriptTypeScriptStatementKeywordLeaves(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	ifStmtSym, hasIfStmt := symbolByName(lang, "if_statement")
	whileStmtSym, hasWhileStmt := symbolByName(lang, "while_statement")
	ifSym, ifNamed, hasIf := symbolMeta(lang, "if")
	whileSym, whileNamed, hasWhile := symbolMeta(lang, "while")
	closeBraceSym, hasCloseBrace := symbolByName(lang, "}")
	if (!hasIfStmt || !hasIf) && (!hasWhileStmt || !hasWhile) {
		return
	}

	walkResultTreeDenseFirst(root, func(n *Node) {
		if hasIfStmt && hasIf && n.symbol == ifStmtSym {
			normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbol(n, source, "if", ifSym, ifNamed, closeBraceSym, hasCloseBrace)
			return
		}
		if hasWhileStmt && hasWhile && n.symbol == whileStmtSym {
			normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbol(n, source, "while", whileSym, whileNamed, closeBraceSym, hasCloseBrace)
		}
	})
}

func normalizeJavaScriptTypeScriptStatementKeywordLeaf(n *Node, source []byte, lang *Language, keyword string) {
	keywordSym, ok := symbolByName(lang, keyword)
	if !ok {
		return
	}
	normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbol(n, source, keyword, keywordSym, symbolIsNamed(lang, keywordSym), 0, false)
}

func normalizeJavaScriptTypeScriptStatementKeywordLeafWithSymbol(n *Node, source []byte, keyword string, keywordSym Symbol, keywordNamed bool, closeBraceSym Symbol, hasCloseBrace bool) {
	end := n.startByte + uint32(len(keyword))
	if int(end) > len(source) || !bytes.Equal(source[n.startByte:end], []byte(keyword)) {
		return
	}
	childCount := resultChildCount(n)
	if childCount == 0 {
		keywordNode := newLeafNodeInArena(n.ownerArena, keywordSym, keywordNamed, n.startByte, end, n.startPoint, advancePointByBytes(n.startPoint, source[n.startByte:end]))
		replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(n.ownerArena, []*Node{keywordNode}))
		return
	}
	first := resultChildAt(n, 0)
	if first != nil && first.symbol == keywordSym && first.startByte == n.startByte && first.endByte == end {
		return
	}
	keywordNode := newLeafNodeInArena(n.ownerArena, keywordSym, keywordNamed, n.startByte, end, n.startPoint, advancePointByBytes(n.startPoint, source[n.startByte:end]))

	children := make([]*Node, 0, childCount+1)
	for i := 0; i < childCount; i++ {
		children = append(children, resultChildAt(n, i))
	}
	if first != nil && hasCloseBrace && first.symbol == closeBraceSym {
		children[0] = keywordNode
		if len(n.fieldIDs) == childCount {
			n.fieldIDs[0] = 0
		}
		if len(n.fieldSources) == childCount {
			n.fieldSources[0] = fieldSourceNone
		}
	} else if first == nil || first.startByte > n.startByte {
		children = append([]*Node{keywordNode}, children...)
		n.fieldIDs = prependFieldID(n.ownerArena, n.fieldIDs, childCount)
		n.fieldSources = prependFieldSource(n.ownerArena, n.fieldSources, childCount)
	} else {
		children[0] = keywordNode
		if len(n.fieldIDs) == childCount {
			n.fieldIDs[0] = 0
		}
		if len(n.fieldSources) == childCount {
			n.fieldSources[0] = fieldSourceNone
		}
	}
	n.children = cloneNodeSliceInArena(n.ownerArena, children)
	if n.ownerArena != nil {
		n.ownerArena.clearFinalChildRefs(n)
	}
	populateParentNode(n, n.children)
}

func prependFieldID(arena *nodeArena, fieldIDs []FieldID, oldLen int) []FieldID {
	if len(fieldIDs) != oldLen {
		return nil
	}
	out := make([]FieldID, oldLen+1)
	copy(out[1:], fieldIDs)
	return cloneFieldIDSliceInArena(arena, out)
}

func prependFieldSource(arena *nodeArena, fieldSources []uint8, oldLen int) []uint8 {
	if len(fieldSources) != oldLen {
		return nil
	}
	out := make([]uint8, oldLen+1)
	copy(out[1:], fieldSources)
	if arena != nil {
		buf := arena.allocFieldSourceSlice(len(out))
		copy(buf, out)
		return buf
	}
	return out
}

func normalizeJavaScriptTopLevelObjectLiterals(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || root.Type(lang) != "program" {
		return
	}
	exprSym, exprNamed, ok := symbolMeta(lang, "expression_statement")
	if !ok {
		return
	}
	objectSym, objectNamed, ok := symbolMeta(lang, "object")
	if !ok {
		return
	}
	pairSym, pairNamed, ok := symbolMeta(lang, "pair")
	if !ok {
		return
	}
	propSym, _, ok := symbolMeta(lang, "property_identifier")
	if !ok {
		return
	}
	for i, child := range root.children {
		repl, ok := rewriteJavaScriptTopLevelObjectLiteral(child, lang, root.ownerArena, exprSym, exprNamed, objectSym, objectNamed, pairSym, pairNamed, propSym)
		if ok {
			root.children[i] = repl
		}
	}
}

func normalizeJavaScriptProgramStart(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || root.Type(lang) != "program" {
		return
	}
	first, _ := firstAndLastNonNilChild(root.children)
	if first == nil {
		return
	}
	root.startByte = first.startByte
	root.startPoint = first.startPoint
}

func normalizeJavaScriptProgramEnd(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || root.endByte >= uint32(len(source)) {
		return
	}
	switch root.Type(lang) {
	case "program", "ERROR":
	default:
		return
	}
	tail := source[root.endByte:]
	if !bytesAreTrivia(tail) && !bytesAreJavaScriptStatementTerminatorTail(tail) {
		return
	}
	extendNodeEndTo(root, uint32(len(source)), source)
}

func bytesAreJavaScriptStatementTerminatorTail(b []byte) bool {
	seenSemicolon := false
	for _, c := range b {
		switch c {
		case ';':
			seenSemicolon = true
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return seenSemicolon
}

func normalizeJavaScriptTopLevelExpressionStatementBounds(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "program" {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if normalizeJavaScriptTopLevelBoundsFinalRefs(root, lang, func(name string) bool {
		return name == "expression_statement"
	}) {
		return
	}
	for _, child := range root.children {
		if child == nil || child.Type(lang) != "expression_statement" || len(child.children) == 0 {
			continue
		}
		first, last := firstAndLastNonNilChild(child.children)
		if first == nil || last == nil {
			continue
		}
		child.startByte = first.startByte
		child.startPoint = first.startPoint
		child.endByte = last.endByte
		child.endPoint = last.endPoint
	}
}

func normalizeJavaScriptTopLevelDeclarationBounds(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "program" {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if normalizeJavaScriptTopLevelBoundsFinalRefs(root, lang, func(name string) bool {
		switch name {
		case "lexical_declaration",
			"variable_declaration",
			"function_declaration",
			"generator_function_declaration",
			"class_declaration",
			"import_statement",
			"export_statement":
			return true
		default:
			return false
		}
	}) {
		return
	}
	for _, child := range root.children {
		if child == nil || len(child.children) == 0 {
			continue
		}
		switch child.Type(lang) {
		case "lexical_declaration",
			"variable_declaration",
			"function_declaration",
			"generator_function_declaration",
			"class_declaration",
			"import_statement",
			"export_statement":
		default:
			continue
		}
		first, last := firstAndLastNonNilChild(child.children)
		if first == nil || last == nil {
			continue
		}
		child.startByte = first.startByte
		child.startPoint = first.startPoint
		child.endByte = last.endByte
		child.endPoint = last.endPoint
	}
}

func normalizeJavaScriptTopLevelBoundsFinalRefs(root *Node, lang *Language, match func(string) bool) bool {
	view := resultMutableChildrenForMutation(root)
	if !view.hasFinalChildRefs() {
		return false
	}
	for i := 0; i < view.Len(); i++ {
		entry, ok := view.Entry(i)
		if !ok || !match(symbolTypeName(lang, stackEntryNodeSymbol(entry))) {
			continue
		}
		first, last, ok := firstAndLastStackEntryChild(root.ownerArena, entry)
		if !ok {
			continue
		}
		setStackEntryStart(entry, stackEntryNodeStartByte(first), stackEntryNodeStartPoint(first))
		setStackEntryEnd(entry, stackEntryNodeEndByte(last), stackEntryNodeEndPoint(last))
	}
	return true
}

func firstAndLastStackEntryChild(arena *nodeArena, entry stackEntry) (stackEntry, stackEntry, bool) {
	childCount := stackEntryNodeChildCount(entry)
	if childCount == 0 {
		return stackEntry{}, stackEntry{}, false
	}
	childAt := func(i int) (stackEntry, bool) {
		if parent := stackEntryPendingParent(entry); parent != nil {
			child := parent.childEntry(arena, i)
			return child, stackEntryHasNode(child)
		}
		if node := stackEntryNode(entry); node != nil {
			return nodeChildEntryAtNoMaterialize(node, i)
		}
		return stackEntry{}, false
	}
	firstIdx := 0
	first, ok := childAt(firstIdx)
	for !ok && firstIdx+1 < childCount {
		firstIdx++
		first, ok = childAt(firstIdx)
	}
	if !ok {
		return stackEntry{}, stackEntry{}, false
	}
	lastIdx := childCount - 1
	last, ok := childAt(lastIdx)
	for !ok && lastIdx > firstIdx {
		lastIdx--
		last, ok = childAt(lastIdx)
	}
	if !ok {
		return stackEntry{}, stackEntry{}, false
	}
	return first, last, true
}

func setStackEntryStart(entry stackEntry, startByte uint32, startPoint Point) {
	if node := stackEntryNode(entry); node != nil {
		node.startByte = startByte
		node.startPoint = startPoint
		return
	}
	if node := stackEntryNoTreeNode(entry); node != nil {
		node.startByte = startByte
		return
	}
	if leaf := stackEntryCompactFullLeaf(entry); leaf != nil {
		leaf.startByte = startByte
		leaf.startPoint = startPoint
		return
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		parent.startByte = startByte
		parent.startPoint = startPoint
	}
}

func normalizeJavaScriptTrailingContinueComments(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || len(source) == 0 {
		return
	}
	walkResultTreeDenseFirst(root, func(n *Node) {
		normalizeJavaScriptTrailingContinueCommentSiblings(n, source, lang)
	})
}

func normalizeJavaScriptTrailingContinueCommentSiblings(parent *Node, source []byte, lang *Language) {
	if parent == nil || len(parent.children) < 3 || parent.Type(lang) != "statement_block" {
		return
	}
	for i := 1; i+1 < len(parent.children); i++ {
		if comment, ok := extractJavaScriptTrailingContinueComment(parent.children[i], source, lang); ok {
			insertJavaScriptStatementBlockComment(parent, i, comment)
			i++
			continue
		}
		stmt := parent.children[i]
		if stmt == nil || stmt.Type(lang) != "if_statement" || len(stmt.children) < 3 {
			continue
		}
		branch := stmt.children[len(stmt.children)-1]
		comment, ok := extractJavaScriptTrailingContinueComment(branch, source, lang)
		if !ok {
			continue
		}
		stmt.endByte = branch.endByte
		stmt.endPoint = branch.endPoint
		insertJavaScriptStatementBlockComment(parent, i, comment)
		i++
	}
}

func extractJavaScriptTrailingContinueComment(node *Node, source []byte, lang *Language) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "continue_statement" || len(node.children) < 3 {
		return nil, false
	}
	comment := node.children[len(node.children)-1]
	if comment == nil || comment.Type(lang) != "comment" || comment.startByte >= comment.endByte {
		return nil, false
	}
	if int(comment.endByte) > len(source) || !bytes.HasPrefix(source[comment.startByte:comment.endByte], []byte("//")) {
		return nil, false
	}
	prev := node.children[len(node.children)-2]
	if prev == nil || prev.endByte > comment.startByte || bytesContainLineBreak(source[prev.endByte:comment.startByte]) {
		return nil, false
	}
	node.children = node.children[:len(node.children)-1]
	if len(node.fieldIDs) > len(node.children) {
		node.fieldIDs = node.fieldIDs[:len(node.children)]
		if len(node.fieldSources) > len(node.children) {
			node.fieldSources = node.fieldSources[:len(node.children)]
		}
	}
	node.endByte = prev.endByte
	node.endPoint = prev.endPoint
	return comment, true
}

func insertJavaScriptStatementBlockComment(parent *Node, childIdx int, comment *Node) {
	if parent == nil || comment == nil || childIdx < 0 || childIdx >= len(parent.children) {
		return
	}
	parent.children = append(parent.children[:childIdx+1], append([]*Node{comment}, parent.children[childIdx+1:]...)...)
	if len(parent.fieldIDs) > 0 {
		fieldIDs := append([]FieldID(nil), parent.fieldIDs[:childIdx+1]...)
		fieldIDs = append(fieldIDs, 0)
		fieldIDs = append(fieldIDs, parent.fieldIDs[childIdx+1:]...)
		parent.fieldIDs = fieldIDs
		if len(parent.fieldSources) > 0 {
			fieldSources := append([]uint8(nil), parent.fieldSources[:childIdx+1]...)
			fieldSources = append(fieldSources, fieldSourceNone)
			fieldSources = append(fieldSources, parent.fieldSources[childIdx+1:]...)
			parent.fieldSources = fieldSources
		}
	}
	populateParentNode(parent, parent.children)
}

func normalizeJavaScriptTypeScriptOptionalChainLeaves(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if _, ok := symbolByName(lang, "optional_chain"); !ok {
		return
	}

	walkResultTreeDenseFirst(root, func(n *Node) {
		if n.Type(lang) == "optional_chain" && len(n.children) == 1 {
			child := n.children[0]
			if child != nil && !child.IsNamed() && !child.IsExtra() &&
				child.startByte == n.startByte && child.endByte == n.endByte &&
				child.startPoint == n.startPoint && child.endPoint == n.endPoint {
				n.children = nil
				n.fieldIDs = nil
				n.fieldSources = nil
			}
		}
	})
}

func normalizeJavaScriptTypeScriptCallPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if _, ok := symbolByName(lang, "call_expression"); !ok {
		return
	}

	walkResultTreeDenseFirst(root, func(n *Node) {
		for i, child := range n.children {
			if rewritten := rewriteJavaScriptTypeScriptCallPrecedence(child, lang); rewritten != nil {
				n.children[i] = rewritten
				rewritten.parent = n
				rewritten.childIndex = int32(i)
			}
		}
	})
}

func rewriteJavaScriptTypeScriptCallPrecedence(node *Node, lang *Language) *Node {
	if node == nil || lang == nil || node.Type(lang) != "call_expression" || len(node.children) != 2 {
		return nil
	}
	function := node.children[0]
	arguments := node.children[1]
	if function == nil || arguments == nil {
		return nil
	}
	return rewriteJavaScriptTypeScriptCallTarget(function, arguments, node, lang)
}

func rewriteJavaScriptTypeScriptCallTarget(target, arguments, callNode *Node, lang *Language) *Node {
	if target == nil || arguments == nil || callNode == nil || lang == nil {
		return nil
	}
	if isJavaScriptTypeScriptCallableShape(target, lang) {
		rewrittenCall := cloneNodeInArena(callNode.ownerArena, callNode)
		rewrittenCall.children = cloneNodeSliceInArena(callNode.ownerArena, []*Node{target, arguments})
		populateParentNode(rewrittenCall, rewrittenCall.children)
		return rewrittenCall
	}

	switch target.Type(lang) {
	case "unary_expression":
		if len(target.children) < 2 {
			return nil
		}
		operandIdx := len(target.children) - 1
		rewrittenOperand := rewriteJavaScriptTypeScriptCallTarget(target.children[operandIdx], arguments, callNode, lang)
		if rewrittenOperand == nil {
			return nil
		}
		rewrittenUnary := cloneNodeInArena(callNode.ownerArena, target)
		unaryChildren := cloneNodeSliceInArena(callNode.ownerArena, target.children)
		unaryChildren[operandIdx] = rewrittenOperand
		rewrittenUnary.children = unaryChildren
		populateParentNode(rewrittenUnary, rewrittenUnary.children)
		return rewrittenUnary
	case "binary_expression":
		operator, rightIdx, ok := javaScriptTypeScriptBinaryOperatorAndRight(target, lang)
		if !ok || rightIdx < 0 || rightIdx >= len(target.children) {
			return nil
		}
		if operator == nil {
			return nil
		}
		if _, ok := javaScriptTypeScriptBinaryOperatorPrecedence(operator.Type(lang)); !ok {
			return nil
		}
		rewrittenRight := rewriteJavaScriptTypeScriptCallTarget(target.children[rightIdx], arguments, callNode, lang)
		if rewrittenRight == nil {
			return nil
		}
		rewrittenBinary := cloneNodeInArena(callNode.ownerArena, target)
		binaryChildren := cloneNodeSliceInArena(callNode.ownerArena, target.children)
		binaryChildren[rightIdx] = rewrittenRight
		rewrittenBinary.children = binaryChildren
		populateParentNode(rewrittenBinary, rewrittenBinary.children)
		return rewrittenBinary
	default:
		return nil
	}
}

func javaScriptTypeScriptBinaryOperatorAndRight(node *Node, lang *Language) (*Node, int, bool) {
	if node == nil || lang == nil || node.Type(lang) != "binary_expression" || len(node.children) < 3 {
		return nil, -1, false
	}
	operatorIdx := -1
	rightIdx := -1
	for i := 0; i < len(node.children); i++ {
		switch node.FieldNameForChild(i, lang) {
		case "operator":
			operatorIdx = i
		case "right":
			rightIdx = i
		}
	}
	if operatorIdx < 0 && len(node.children) >= 2 {
		operatorIdx = 1
	}
	if rightIdx < 0 {
		for i := len(node.children) - 1; i >= 0; i-- {
			child := node.children[i]
			if child == nil || child.isExtra() {
				continue
			}
			if i != operatorIdx {
				rightIdx = i
				break
			}
		}
	}
	if operatorIdx < 0 || rightIdx < 0 || operatorIdx >= len(node.children) {
		return nil, -1, false
	}
	return node.children[operatorIdx], rightIdx, true
}

func isJavaScriptTypeScriptCallableShape(node *Node, lang *Language) bool {
	if node == nil || lang == nil {
		return false
	}
	switch node.Type(lang) {
	case "identifier", "member_expression", "subscript_expression", "call_expression", "parenthesized_expression":
		return true
	default:
		return false
	}
}

func normalizeJavaScriptTypeScriptUnaryPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if _, ok := symbolByName(lang, "unary_expression"); !ok {
		return
	}

	rewriteResultTreeChildrenPostorder(root, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptUnaryPrecedence(n, lang)
	})
}

func rewriteJavaScriptTypeScriptUnaryPrecedence(node *Node, lang *Language) *Node {
	if node == nil || lang == nil || node.Type(lang) != "unary_expression" || len(node.children) < 2 {
		return nil
	}
	operandIdx := len(node.children) - 1
	operand := node.children[operandIdx]
	if operand == nil || operand.Type(lang) != "binary_expression" || len(operand.children) != 3 {
		return nil
	}
	if _, ok := javaScriptTypeScriptBinaryOperatorPrecedence(operand.children[1].Type(lang)); !ok {
		return nil
	}

	rewrittenUnary := cloneNodeInArena(node.ownerArena, node)
	unaryChildren := cloneNodeSliceInArena(node.ownerArena, node.children)
	unaryChildren[operandIdx] = operand.children[0]
	rewrittenUnary.children = unaryChildren
	populateParentNode(rewrittenUnary, rewrittenUnary.children)

	rewrittenBinary := cloneNodeInArena(node.ownerArena, operand)
	binaryChildren := cloneNodeSliceInArena(node.ownerArena, operand.children)
	binaryChildren[0] = rewrittenUnary
	rewrittenBinary.children = binaryChildren
	populateParentNode(rewrittenBinary, rewrittenBinary.children)
	return rewrittenBinary
}

func normalizeJavaScriptTypeScriptBinaryPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if _, ok := symbolByName(lang, "binary_expression"); !ok {
		return
	}

	rewriteResultTreeChildrenPostorder(root, func(n *Node) *Node {
		return rewriteJavaScriptTypeScriptBinaryPrecedence(n, lang)
	})
}

func rewriteJavaScriptTypeScriptBinaryPrecedence(node *Node, lang *Language) *Node {
	if node == nil || lang == nil || node.Type(lang) != "binary_expression" || len(node.children) != 3 {
		return nil
	}
	left := node.children[0]
	op := node.children[1]
	right := node.children[2]
	if left == nil || op == nil || right == nil {
		return nil
	}
	parentPrec, ok := javaScriptTypeScriptBinaryOperatorPrecedence(op.Type(lang))
	if !ok {
		return nil
	}

	if left.Type(lang) == "binary_expression" && len(left.children) == 3 {
		leftOp := left.children[1]
		if leftOp != nil {
			leftPrec, ok := javaScriptTypeScriptBinaryOperatorPrecedence(leftOp.Type(lang))
			if ok && parentPrec > leftPrec {
				rotatedInner := cloneNodeInArena(node.ownerArena, node)
				rotatedInner.children = cloneNodeSliceInArena(node.ownerArena, []*Node{left.children[2], op, right})
				populateParentNode(rotatedInner, rotatedInner.children)

				rotatedOuter := cloneNodeInArena(node.ownerArena, left)
				rotatedOuter.children = cloneNodeSliceInArena(node.ownerArena, []*Node{left.children[0], leftOp, rotatedInner})
				populateParentNode(rotatedOuter, rotatedOuter.children)
				return rotatedOuter
			}
		}
	}

	if right.Type(lang) == "binary_expression" && len(right.children) == 3 {
		rightOp := right.children[1]
		if rightOp != nil {
			rightPrec, ok := javaScriptTypeScriptBinaryOperatorPrecedence(rightOp.Type(lang))
			if ok && parentPrec >= rightPrec && !javaScriptTypeScriptBinaryOperatorRightAssociative(op.Type(lang)) {
				rotatedInner := cloneNodeInArena(node.ownerArena, node)
				rotatedInner.children = cloneNodeSliceInArena(node.ownerArena, []*Node{left, op, right.children[0]})
				populateParentNode(rotatedInner, rotatedInner.children)

				rotatedOuter := cloneNodeInArena(node.ownerArena, right)
				rotatedOuter.children = cloneNodeSliceInArena(node.ownerArena, []*Node{rotatedInner, rightOp, right.children[2]})
				populateParentNode(rotatedOuter, rotatedOuter.children)
				return rotatedOuter
			}
		}
	}

	return nil
}

func javaScriptTypeScriptBinaryOperatorPrecedence(op string) (int, bool) {
	switch op {
	case "??":
		return 1, true
	case "||":
		return 2, true
	case "&&":
		return 3, true
	case "|":
		return 4, true
	case "^":
		return 5, true
	case "&":
		return 6, true
	case "==", "!=", "===", "!==":
		return 7, true
	case "<", "<=", ">", ">=", "instanceof", "in":
		return 8, true
	case "<<", ">>", ">>>":
		return 9, true
	case "+", "-":
		return 10, true
	case "*", "/", "%":
		return 11, true
	case "**":
		return 12, true
	default:
		return 0, false
	}
}

func javaScriptTypeScriptBinaryOperatorRightAssociative(op string) bool {
	return op == "**"
}

func rewriteJavaScriptTopLevelObjectLiteral(node *Node, lang *Language, arena *nodeArena, exprSym Symbol, exprNamed bool, objectSym Symbol, objectNamed bool, pairSym Symbol, pairNamed bool, propSym Symbol) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "statement_block" || len(node.children) != 3 {
		return nil, false
	}
	if node.children[0] == nil || node.children[0].Type(lang) != "{" || node.children[2] == nil || node.children[2].Type(lang) != "}" {
		return nil, false
	}
	label := node.children[1]
	if label == nil || label.Type(lang) != "labeled_statement" || len(label.children) != 3 {
		return nil, false
	}
	key := label.children[0]
	colon := label.children[1]
	valueStmt := label.children[2]
	if key == nil || key.Type(lang) != "statement_identifier" || colon == nil || colon.Type(lang) != ":" || valueStmt == nil || valueStmt.Type(lang) != "expression_statement" || len(valueStmt.children) != 1 || valueStmt.children[0] == nil {
		return nil, false
	}
	pair := newParentNodeInArena(arena, pairSym, pairNamed, []*Node{
		aliasedNodeInArena(arena, lang, key, propSym),
		colon,
		valueStmt.children[0],
	}, nil, 0)
	for fieldIdx, fieldName := range lang.FieldNames {
		switch fieldName {
		case "key":
			ensureNodeFieldStorage(pair, len(pair.children))
			pair.fieldIDs[0] = FieldID(fieldIdx)
			pair.fieldSources[0] = fieldSourceDirect
		case "value":
			ensureNodeFieldStorage(pair, len(pair.children))
			pair.fieldIDs[2] = FieldID(fieldIdx)
			pair.fieldSources[2] = fieldSourceDirect
		}
	}
	object := newParentNodeInArena(arena, objectSym, objectNamed, []*Node{
		node.children[0],
		pair,
		node.children[2],
	}, nil, 0)
	return newParentNodeInArena(arena, exprSym, exprNamed, []*Node{object}, nil, 0), true
}
