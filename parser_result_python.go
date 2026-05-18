package gotreesitter

import "bytes"

func normalizePythonCompatibility(root *Node, source []byte, lang *Language) {
	if len(source) == 0 {
		return
	}
	if bytes.IndexByte(source, ';') >= 0 {
		normalizePythonTrailingSelfCalls(root, source, lang)
	}
	if bytes.Contains(source, []byte("print")) && bytes.Contains(source, []byte(">>")) {
		normalizePythonPrintStatements(root, source, lang)
	}
	if bytes.IndexByte(source, '{') >= 0 && pythonSourceMayContainFString(source) {
		normalizePythonInterpolationPatterns(root, lang)
	}
	if bytes.Contains(source, []byte("pass")) {
		normalizeCollapsedNamedLeafChildren(root, lang, "pass_statement", "pass")
	}
	if bytes.Contains(source, []byte("\\\n")) || bytes.Contains(source, []byte("\\\r\n")) {
		normalizePythonStringContinuationEscapes(root, source, lang)
	}
}

func pythonSourceMayContainFString(source []byte) bool {
	for i, c := range source {
		if c != '"' && c != '\'' {
			continue
		}
		start := i
		for start > 0 && i-start < 3 && pythonStringPrefixByte(source[start-1]) {
			start--
		}
		if start == i {
			continue
		}
		if start > 0 && pythonIdentifierByte(source[start-1]) {
			continue
		}
		for _, p := range source[start:i] {
			if p == 'f' || p == 'F' {
				return true
			}
		}
	}
	return false
}

func pythonStringPrefixByte(c byte) bool {
	switch c {
	case 'b', 'B', 'f', 'F', 'r', 'R', 'u', 'U':
		return true
	default:
		return false
	}
}

func pythonIdentifierByte(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}

func normalizePythonInterpolationPatterns(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "python" {
		return
	}
	patternListSym, ok := symbolByName(lang, "pattern_list")
	if !ok {
		return
	}
	listSplatPatternSym, hasListSplatPattern := symbolByName(lang, "list_splat_pattern")
	expressionListSym, hasExpressionList := symbolByName(lang, "expression_list")
	listSplatSym, hasListSplat := symbolByName(lang, "list_splat")

	patternListNamed := false
	if int(patternListSym) < len(lang.SymbolMetadata) {
		patternListNamed = lang.SymbolMetadata[patternListSym].Named
	}
	listSplatPatternNamed := false
	if hasListSplatPattern && int(listSplatPatternSym) < len(lang.SymbolMetadata) {
		listSplatPatternNamed = lang.SymbolMetadata[listSplatPatternSym].Named
	}

	var rewrite func(*Node, bool)
	rewrite = func(n *Node, inInterpolation bool) {
		if n == nil {
			return
		}
		here := inInterpolation || n.Type(lang) == "interpolation"
		if here {
			if hasExpressionList && n.symbol == expressionListSym {
				n.symbol = patternListSym
				n.isNamed = patternListNamed
			}
			if hasListSplatPattern && hasListSplat && n.symbol == listSplatSym {
				n.symbol = listSplatPatternSym
				n.isNamed = listSplatPatternNamed
			}
		}
		for _, child := range n.children {
			rewrite(child, here)
		}
	}
	rewrite(root, false)
}

func normalizePythonPrintStatements(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return
	}
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		for _, child := range node.children {
			walk(child)
		}
		switch node.Type(lang) {
		case "module", "block":
			rewritten, changed := rewritePythonStatementList(node.children, source, lang)
			if !changed {
				return
			}
			node.children = cloneNodeSliceInArena(node.ownerArena, rewritten)
			node.fieldIDs = nil
			node.fieldSources = nil
			populateParentNode(node, node.children)
		}
	}
	walk(root)
}

func normalizePythonTrailingSelfCalls(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return
	}
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		for _, child := range node.children {
			walk(child)
		}
		if node.Type(lang) != "block" {
			return
		}
		rewritten, changed := foldPythonTrailingSelfCallsInBlock(node.children, source, lang)
		if !changed {
			return
		}
		node.children = cloneNodeSliceInArena(node.ownerArena, rewritten)
		node.fieldIDs = nil
		node.fieldSources = nil
		populateParentNode(node, node.children)
	}
	walk(root)
}

func foldPythonTrailingSelfCallsInBlock(children []*Node, source []byte, lang *Language) ([]*Node, bool) {
	if len(children) < 2 || lang == nil || lang.Name != "python" || len(source) == 0 {
		return children, false
	}
	var out []*Node
	for i := 0; i < len(children); i++ {
		cur := children[i]
		if i+1 >= len(children) {
			if out != nil {
				out = append(out, cur)
			}
			continue
		}
		next := children[i+1]
		rewritten, ok := foldPythonTrailingSelfCallIntoNestedFunction(cur, next, source, lang)
		if !ok {
			if out != nil {
				out = append(out, cur)
			}
			continue
		}
		if out == nil {
			out = make([]*Node, 0, len(children))
			out = append(out, children[:i]...)
		}
		out = append(out, rewritten)
		i++
	}
	if out == nil {
		return children, false
	}
	return out, true
}

func foldPythonTrailingSelfCallIntoNestedFunction(fnNode, trailingCall *Node, source []byte, lang *Language) (*Node, bool) {
	if fnNode == nil || trailingCall == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return nil, false
	}
	if fnNode.Type(lang) != "function_definition" || trailingCall.Type(lang) != "call" {
		return nil, false
	}
	if trailingCall.startPoint.Column != fnNode.startPoint.Column {
		return nil, false
	}
	fnName, ok := pythonFunctionDefinitionNameNode(fnNode, lang)
	if !ok || fnName == nil {
		return nil, false
	}
	callName, ok := pythonCallIdentifierNode(trailingCall, lang)
	if !ok || callName == nil {
		return nil, false
	}
	if !pythonNodeTextEqual(fnName, callName, source) {
		return nil, false
	}
	bodyIndex := -1
	var body *Node
	for i, child := range fnNode.children {
		if child != nil && child.Type(lang) == "block" {
			bodyIndex = i
			body = child
		}
	}
	if bodyIndex < 0 || body == nil || !pythonBlockEndsWithSemicolon(body, lang) {
		return nil, false
	}

	bodyClone := cloneNodeInArena(body.ownerArena, body)
	bodyChildren := make([]*Node, 0, len(body.children)+1)
	bodyChildren = append(bodyChildren, body.children...)
	bodyChildren = append(bodyChildren, trailingCall)
	bodyClone.children = cloneNodeSliceInArena(bodyClone.ownerArena, bodyChildren)
	bodyClone.fieldIDs = nil
	bodyClone.fieldSources = nil
	populateParentNode(bodyClone, bodyClone.children)

	fnClone := cloneNodeInArena(fnNode.ownerArena, fnNode)
	fnChildren := append([]*Node(nil), fnNode.children...)
	fnChildren[bodyIndex] = bodyClone
	fnClone.children = cloneNodeSliceInArena(fnClone.ownerArena, fnChildren)
	fnClone.fieldIDs = append([]FieldID(nil), fnNode.fieldIDs...)
	fnClone.fieldSources = append([]uint8(nil), fnNode.fieldSources...)
	populateParentNode(fnClone, fnClone.children)
	return fnClone, true
}

func rewritePythonStatementList(children []*Node, source []byte, lang *Language) ([]*Node, bool) {
	if len(children) == 0 || lang == nil || lang.Name != "python" {
		return children, false
	}
	var out []*Node
	for i, child := range children {
		if child == nil {
			if out != nil {
				out = append(out, nil)
			}
			continue
		}
		if rewritten, ok := rewriteMalformedPythonPrintStatement(child, source, lang); ok {
			if out == nil {
				out = make([]*Node, 0, len(children))
				out = append(out, children[:i]...)
			}
			out = append(out, rewritten)
			continue
		}
		if out != nil {
			out = append(out, child)
		}
	}
	if out == nil {
		return children, false
	}
	return out, true
}

func rewriteMalformedPythonPrintStatement(node *Node, source []byte, lang *Language) (*Node, bool) {
	if node == nil || lang == nil || lang.Name != "python" {
		return nil, false
	}
	bin, extras, ok := pythonMalformedPrintStatementParts(node, source, lang)
	if !ok || bin == nil || len(bin.children) < 3 {
		return nil, false
	}
	printStmtSym, ok := symbolByName(lang, "print_statement")
	if !ok {
		return nil, false
	}
	chevronSym, ok := symbolByName(lang, "chevron")
	if !ok {
		return nil, false
	}
	printSym, ok := symbolByName(lang, "print")
	if !ok {
		return nil, false
	}

	printNamed := false
	if int(printSym) < len(lang.SymbolMetadata) {
		printNamed = lang.SymbolMetadata[printSym].Named
	}
	printStmtNamed := true
	if int(printStmtSym) < len(lang.SymbolMetadata) {
		printStmtNamed = lang.SymbolMetadata[printStmtSym].Named
	}
	chevronNamed := true
	if int(chevronSym) < len(lang.SymbolMetadata) {
		chevronNamed = lang.SymbolMetadata[chevronSym].Named
	}

	left := bin.children[0]
	op := bin.children[1]
	dest := bin.children[2]
	printLeaf := cloneNodeInArena(node.ownerArena, left)
	printLeaf.symbol = printSym
	printLeaf.isNamed = printNamed
	printLeaf.children = nil
	printLeaf.fieldIDs = nil
	printLeaf.fieldSources = nil

	chevron := cloneNodeInArena(node.ownerArena, bin)
	chevron.symbol = chevronSym
	chevron.isNamed = chevronNamed
	chevron.children = cloneNodeSliceInArena(chevron.ownerArena, []*Node{op, dest})
	chevron.fieldIDs = nil
	chevron.fieldSources = nil
	chevron.productionID = 0
	populateParentNode(chevron, chevron.children)

	rewritten := cloneNodeInArena(node.ownerArena, node)
	children := make([]*Node, 0, 2+len(extras))
	children = append(children, printLeaf, chevron)
	children = append(children, extras...)
	rewritten.symbol = printStmtSym
	rewritten.isNamed = printStmtNamed
	rewritten.children = cloneNodeSliceInArena(rewritten.ownerArena, children)
	rewritten.fieldIDs = nil
	rewritten.fieldSources = nil
	rewritten.productionID = 0
	populateParentNode(rewritten, rewritten.children)
	return rewritten, true
}

func pythonMalformedPrintStatementParts(node *Node, source []byte, lang *Language) (*Node, []*Node, bool) {
	if node == nil || lang == nil || lang.Name != "python" {
		return nil, nil, false
	}
	switch node.Type(lang) {
	case "binary_operator":
		if pythonIsPrintChevronBinary(node, source, lang) {
			return node, nil, true
		}
	case "tuple_expression":
		if len(node.children) == 0 {
			return nil, nil, false
		}
		bin := node.children[0]
		if pythonIsPrintChevronBinary(bin, source, lang) {
			return bin, node.children[1:], true
		}
	}
	return nil, nil, false
}

func pythonIsPrintChevronBinary(node *Node, source []byte, lang *Language) bool {
	if node == nil || lang == nil || lang.Name != "python" || len(node.children) != 3 {
		return false
	}
	if node.Type(lang) != "binary_operator" {
		return false
	}
	left := node.children[0]
	op := node.children[1]
	if left == nil || op == nil {
		return false
	}
	if left.Type(lang) != "identifier" || op.Type(lang) != ">>" {
		return false
	}
	if left.startByte >= left.endByte || int(left.endByte) > len(source) {
		return false
	}
	return string(source[left.startByte:left.endByte]) == "print"
}

func normalizePythonModuleChildren(nodes []*Node, arena *nodeArena, lang *Language) []*Node {
	if len(nodes) == 0 || lang == nil || lang.Name != "python" {
		return nodes
	}
	out := make([]*Node, 0, len(nodes))
	changed := false
	for _, node := range nodes {
		if node == nil {
			continue
		}
		normalized, nodeChanged := normalizePythonModuleNode(node, lang)
		if nodeChanged {
			out = append(out, normalized)
			changed = true
			continue
		}
		out = append(out, node)
	}
	if !changed {
		return nodes
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		return buf
	}
	return out
}

func normalizePythonModuleNode(node *Node, lang *Language) (*Node, bool) {
	changed := false
	for node != nil {
		if node.Type(lang) == "_simple_statements" && len(node.children) == 1 {
			child := node.children[0]
			if child != nil && child.IsNamed() {
				node = child
				changed = true
				continue
			}
		}
		if node.Type(lang) == "expression_statement" && len(node.children) == 1 {
			child := node.children[0]
			if child != nil && child.IsNamed() {
				node = child
				changed = true
				continue
			}
		}
		if (node.Type(lang) == "expression" || node.Type(lang) == "primary_expression") && len(node.children) == 1 {
			child := node.children[0]
			if child != nil && child.IsNamed() {
				node = child
				changed = true
				continue
			}
		}
		break
	}
	return node, changed
}

func repairPythonRootNode(root *Node, arena *nodeArena, lang *Language) *Node {
	if root == nil || lang == nil || lang.Name != "python" || root.Type(lang) != "module" {
		return root
	}
	children := collapsePythonRootFragments(root.children, arena, lang)
	changed := len(children) != len(root.children)
	if !changed {
		for i := range children {
			if children[i] != root.children[i] {
				changed = true
				break
			}
		}
	}

	var repaired []*Node
	for i, child := range children {
		fixed := repairPythonTopLevelNode(child, arena, lang)
		if fixed != child {
			changed = true
			if repaired == nil {
				repaired = make([]*Node, 0, len(children))
				repaired = append(repaired, children[:i]...)
			}
		}
		if repaired != nil {
			repaired = append(repaired, fixed)
		}
	}
	if repaired == nil {
		repaired = children
	}

	if !changed {
		if root.hasError && pythonModuleChildrenLookComplete(repaired, lang) {
			cloned := cloneNodeInArena(arena, root)
			cloned.hasError = false
			return cloned
		}
		return root
	}

	cloned := cloneNodeInArena(arena, root)
	if arena != nil {
		buf := arena.allocNodeSlice(len(repaired))
		copy(buf, repaired)
		repaired = buf
	}
	cloned.children = repaired
	cloned.fieldIDs = nil
	cloned.fieldSources = nil
	if pythonModuleChildrenLookComplete(repaired, lang) {
		cloned.hasError = false
	}
	return cloned
}

func repairPythonKeywordErrorNodes(nodes []*Node, source []byte, arena *nodeArena, lang *Language) ([]*Node, bool) {
	if len(nodes) == 0 || lang == nil || lang.Name != "python" || len(source) == 0 {
		return nodes, false
	}
	var out []*Node
	for i, node := range nodes {
		repaired := repairPythonKeywordErrorNode(node, source, arena, lang)
		if repaired != node {
			if out == nil {
				out = make([]*Node, 0, len(nodes))
				out = append(out, nodes[:i]...)
			}
		}
		if out != nil {
			out = append(out, repaired)
		}
	}
	if out == nil {
		return nodes, false
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	return out, true
}

func repairPythonKeywordErrorNode(node *Node, source []byte, arena *nodeArena, lang *Language) *Node {
	if node == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return node
	}
	if node.Type(lang) == "ERROR" && len(node.children) == 0 {
		if keyword, ok := pythonKeywordLeafSymbol(node, source, lang); ok {
			named := false
			if int(keyword) < len(lang.SymbolMetadata) {
				named = lang.SymbolMetadata[keyword].Named
			}
			repl := newLeafNodeInArena(arena, keyword, named, node.startByte, node.endByte, node.startPoint, node.endPoint)
			repl.isExtra = node.isExtra
			return repl
		}
	}
	if len(node.children) == 0 {
		return node
	}
	var children []*Node
	for i, child := range node.children {
		repaired := repairPythonKeywordErrorNode(child, source, arena, lang)
		if repaired != child {
			if children == nil {
				children = make([]*Node, 0, len(node.children))
				children = append(children, node.children[:i]...)
			}
		}
		if children != nil {
			children = append(children, repaired)
		}
	}
	finalChildren := node.children
	if children != nil {
		finalChildren = children
	}
	if node.Type(lang) == "ERROR" && len(finalChildren) == 1 {
		child := finalChildren[0]
		if child != nil &&
			!child.IsError() &&
			!child.HasError() &&
			child.startByte == node.startByte &&
			child.endByte == node.endByte {
			return child
		}
	}
	if children == nil {
		return node
	}
	cloned := cloneNodeInArena(arena, node)
	if arena != nil {
		buf := arena.allocNodeSlice(len(finalChildren))
		copy(buf, finalChildren)
		finalChildren = buf
	}
	cloned.children = finalChildren
	return cloned
}

func pythonKeywordLeafSymbol(node *Node, source []byte, lang *Language) (Symbol, bool) {
	if node == nil || node.startByte >= node.endByte || int(node.endByte) > len(source) {
		return 0, false
	}
	text := string(source[node.startByte:node.endByte])
	if text == "" {
		return 0, false
	}
	sym, ok := symbolByName(lang, text)
	if !ok {
		return 0, false
	}
	if int(sym) >= len(lang.SymbolMetadata) {
		return 0, false
	}
	meta := lang.SymbolMetadata[sym]
	if meta.Named {
		return 0, false
	}
	return sym, true
}

func repairPythonTopLevelNode(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || lang == nil || lang.Name != "python" {
		return node
	}
	return repairPythonNode(node, arena, lang)
}

func repairPythonNode(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || lang == nil || lang.Name != "python" {
		return node
	}
	normalized, changed := normalizePythonModuleNode(node, lang)
	if changed {
		node = normalized
	}
	switch node.Type(lang) {
	case "class_definition":
		return repairPythonClassDefinition(node, arena, lang)
	case "function_definition":
		return repairPythonFunctionDefinition(node, arena, lang)
	case "if_statement":
		return repairPythonIfStatement(node, arena, lang)
	case "block":
		repaired, _ := repairPythonBlock(node, arena, lang, false)
		return repaired
	default:
		return node
	}
}

func repairPythonClassDefinition(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || node.Type(lang) != "class_definition" || len(node.children) == 0 {
		return node
	}
	bodyIndex := -1
	for i, child := range node.children {
		if child != nil && child.Type(lang) == "block" {
			bodyIndex = i
		}
	}
	if bodyIndex < 0 {
		return node
	}
	body := node.children[bodyIndex]
	repairedBody, changed := repairPythonBlock(body, arena, lang, true)
	if !changed {
		return node
	}

	cloned := cloneNodeInArena(arena, node)
	children := make([]*Node, len(node.children))
	copy(children, node.children)
	children[bodyIndex] = repairedBody
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	cloned.children = children
	if repairedBody != nil {
		cloned.endByte = repairedBody.endByte
		cloned.endPoint = repairedBody.endPoint
	}
	return cloned
}

func repairPythonFunctionDefinition(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || node.Type(lang) != "function_definition" || len(node.children) == 0 {
		return node
	}
	bodyIndex := -1
	for i, child := range node.children {
		if child != nil && child.Type(lang) == "block" {
			bodyIndex = i
		}
	}
	if bodyIndex < 0 {
		return node
	}
	body := node.children[bodyIndex]
	repairedBody, changed := repairPythonBlock(body, arena, lang, false)
	if !changed {
		return node
	}

	cloned := cloneNodeInArena(arena, node)
	children := make([]*Node, len(node.children))
	copy(children, node.children)
	children[bodyIndex] = repairedBody
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	cloned.children = children
	if repairedBody != nil {
		cloned.endByte = repairedBody.endByte
		cloned.endPoint = repairedBody.endPoint
	}
	return cloned
}

func repairPythonIfStatement(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || node.Type(lang) != "if_statement" || len(node.children) == 0 {
		return node
	}
	var children []*Node
	for i, child := range node.children {
		repaired := repairPythonNode(child, arena, lang)
		if repaired != child {
			if children == nil {
				children = make([]*Node, 0, len(node.children))
				children = append(children, node.children[:i]...)
			}
		}
		if children != nil {
			children = append(children, repaired)
		}
	}
	if children == nil {
		return node
	}

	cloned := cloneNodeInArena(arena, node)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	cloned.children = children
	last := children[len(children)-1]
	if last != nil {
		cloned.endByte = last.endByte
		cloned.endPoint = last.endPoint
	}
	return cloned
}

func repairPythonBlock(node *Node, arena *nodeArena, lang *Language, allowHoist bool) (*Node, bool) {
	if node == nil || node.Type(lang) != "block" {
		return node, false
	}
	var out []*Node
	changed := false
	processedPending := false

	for i, cur := range node.children {
		if cur == nil {
			continue
		}
		norm, normChanged := normalizePythonModuleNode(cur, lang)
		if normChanged {
			changed = true
			if out == nil {
				out = pythonBlockOutputPrefix(node.children, i)
			}
		}
		cur = norm
		if cur != nil {
			switch cur.Type(lang) {
			case "_indent", "_dedent":
				changed = true
				if out == nil {
					out = pythonBlockOutputPrefix(node.children, i)
				}
				continue
			case "_simple_statements":
				flat := flattenPythonSimpleStatements(cur, nil, lang)
				if len(flat) > 0 {
					changed = true
					if out == nil {
						out = pythonBlockOutputPrefix(node.children, i)
					}
					pending := prependPythonBlockPending(flat, node.children[i+1:])
					out = repairPythonBlockPending(pending, out, arena, lang, allowHoist)
					processedPending = true
					break
				}
			}
		}
		if processedPending {
			break
		}

		if allowHoist && cur != nil && cur.Type(lang) == "function_definition" {
			repairedFn, hoisted, split := splitPythonOvernestedFunction(cur, arena, lang)
			if split {
				changed = true
				if out == nil {
					out = pythonBlockOutputPrefix(node.children, i)
				}
				repairedFn = repairPythonNode(repairedFn, arena, lang)
				out = append(out, repairedFn)
				if len(hoisted) > 0 {
					pending := prependPythonBlockPending(hoisted, node.children[i+1:])
					out = repairPythonBlockPending(pending, out, arena, lang, allowHoist)
					processedPending = true
					break
				}
				continue
			}
		}

		repaired := repairPythonNode(cur, arena, lang)
		if repaired != cur {
			changed = true
			if out == nil {
				out = pythonBlockOutputPrefix(node.children, i)
			}
		}
		if out != nil {
			out = append(out, repaired)
		}
	}

	if !changed {
		firstNamed := pythonBlockStartAnchor(node.children, lang)
		lastSpan := pythonBlockEndAnchor(node.children)
		if firstNamed == nil || lastSpan == nil {
			return node, false
		}
		wantEndByte, wantEndPoint := lastSpan.endByte, lastSpan.endPoint
		if pythonBlockShouldPreserveOriginalEnd(node, node.children, lang) {
			wantEndByte, wantEndPoint = node.endByte, node.endPoint
		}
		if node.startByte == firstNamed.startByte &&
			node.startPoint == firstNamed.startPoint &&
			node.endByte == wantEndByte &&
			node.endPoint == wantEndPoint {
			return node, false
		}
		changed = true
		out = pythonBlockOutputPrefix(node.children, len(node.children))
	}

	cloned := cloneNodeInArena(arena, node)
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	cloned.children = out
	cloned.fieldIDs = nil
	cloned.fieldSources = nil
	firstNamed := pythonBlockStartAnchor(out, lang)
	lastSpan := pythonBlockEndAnchor(out)
	if firstNamed != nil {
		cloned.startByte = firstNamed.startByte
		cloned.startPoint = firstNamed.startPoint
	}
	if lastSpan != nil {
		cloned.endByte = lastSpan.endByte
		cloned.endPoint = lastSpan.endPoint
		if pythonBlockShouldPreserveOriginalEnd(node, out, lang) {
			cloned.endByte = node.endByte
			cloned.endPoint = node.endPoint
		}
	}
	return cloned, true
}

func repairPythonBlockPending(pending []*Node, out []*Node, arena *nodeArena, lang *Language, allowHoist bool) []*Node {
	for len(pending) > 0 {
		cur := pending[0]
		pending = pending[1:]
		if cur == nil {
			continue
		}
		norm, normChanged := normalizePythonModuleNode(cur, lang)
		if normChanged {
			cur = norm
		}
		if cur != nil {
			switch cur.Type(lang) {
			case "_indent", "_dedent":
				continue
			case "_simple_statements":
				flat := flattenPythonSimpleStatements(cur, nil, lang)
				if len(flat) > 0 {
					pending = prependPythonBlockPending(flat, pending)
					continue
				}
			}
		}

		if allowHoist && cur != nil && cur.Type(lang) == "function_definition" {
			repairedFn, hoisted, split := splitPythonOvernestedFunction(cur, arena, lang)
			if split {
				repairedFn = repairPythonNode(repairedFn, arena, lang)
				out = append(out, repairedFn)
				if len(hoisted) > 0 {
					pending = prependPythonBlockPending(hoisted, pending)
				}
				continue
			}
		}

		out = append(out, repairPythonNode(cur, arena, lang))
	}
	return out
}

func pythonBlockOutputPrefix(children []*Node, end int) []*Node {
	out := make([]*Node, 0, len(children))
	for _, child := range children[:end] {
		if child != nil {
			out = append(out, child)
		}
	}
	return out
}

func prependPythonBlockPending(prefix, pending []*Node) []*Node {
	next := make([]*Node, 0, len(prefix)+len(pending))
	next = append(next, prefix...)
	next = append(next, pending...)
	return next
}

func pythonBlockStartAnchor(children []*Node, lang *Language) *Node {
	for _, child := range children {
		if child == nil {
			continue
		}
		typ := child.Type(lang)
		if typ == "_indent" || typ == "_dedent" {
			continue
		}
		if child.endByte > child.startByte || child.IsNamed() {
			return child
		}
	}
	return nil
}

func pythonBlockEndAnchor(children []*Node) *Node {
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		if child != nil && child.endByte > child.startByte {
			return child
		}
	}
	return nil
}

func pythonBlockShouldPreserveOriginalEnd(node *Node, children []*Node, lang *Language) bool {
	if node == nil || lang == nil || len(children) == 0 {
		return false
	}
	lastSpan := pythonBlockEndAnchor(children)
	if lastSpan == nil || node.endByte <= lastSpan.endByte {
		return false
	}
	lastChild := pythonBlockLastChild(children)
	return lastChild != nil && lastChild.Type(lang) == ";"
}

func pythonBlockLastChild(children []*Node) *Node {
	for i := len(children) - 1; i >= 0; i-- {
		if children[i] != nil {
			return children[i]
		}
	}
	return nil
}

func pythonBlockEndsWithSemicolon(node *Node, lang *Language) bool {
	if node == nil || lang == nil || len(node.children) == 0 {
		return false
	}
	lastChild := node.children[len(node.children)-1]
	return lastChild != nil && lastChild.Type(lang) == ";"
}

func pythonFunctionDefinitionNameNode(node *Node, lang *Language) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "function_definition" {
		return nil, false
	}
	for _, child := range node.children {
		if child != nil && child.Type(lang) == "identifier" {
			return child, true
		}
	}
	return nil, false
}

func pythonCallIdentifierNode(node *Node, lang *Language) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "call" || len(node.children) == 0 {
		return nil, false
	}
	fn := node.children[0]
	if fn != nil && fn.Type(lang) == "identifier" {
		return fn, true
	}
	return nil, false
}

func pythonNodeTextEqual(a, b *Node, source []byte) bool {
	if a == nil || b == nil || len(source) == 0 {
		return false
	}
	if a.startByte >= a.endByte || b.startByte >= b.endByte {
		return false
	}
	if int(a.endByte) > len(source) || int(b.endByte) > len(source) {
		return false
	}
	if a.endByte-a.startByte != b.endByte-b.startByte {
		return false
	}
	return bytes.Equal(source[a.startByte:a.endByte], source[b.startByte:b.endByte])
}

func splitPythonOvernestedFunction(node *Node, arena *nodeArena, lang *Language) (*Node, []*Node, bool) {
	if node == nil || node.Type(lang) != "function_definition" {
		return node, nil, false
	}
	bodyIndex := -1
	for i, child := range node.children {
		if child != nil && child.Type(lang) == "block" {
			bodyIndex = i
		}
	}
	if bodyIndex < 0 {
		return node, nil, false
	}
	body := node.children[bodyIndex]
	if body == nil || len(body.children) == 0 {
		return node, nil, false
	}
	fnColumn := node.startPoint.Column
	hoistStart := -1
	for i, child := range body.children {
		if child == nil || !child.IsNamed() {
			continue
		}
		if child.startPoint.Column <= fnColumn {
			hoistStart = i
			break
		}
	}
	if hoistStart <= 0 {
		return node, nil, false
	}

	kept := append([]*Node(nil), body.children[:hoistStart]...)
	hoisted := append([]*Node(nil), body.children[hoistStart:]...)
	if len(kept) == 0 {
		return node, nil, false
	}

	newBody := cloneNodeInArena(arena, body)
	if arena != nil {
		buf := arena.allocNodeSlice(len(kept))
		copy(buf, kept)
		kept = buf
	}
	newBody.children = kept
	newBody.fieldIDs = nil
	newBody.fieldSources = nil
	lastKept := kept[len(kept)-1]
	newBody.endByte = lastKept.endByte
	newBody.endPoint = lastKept.endPoint

	newFn := cloneNodeInArena(arena, node)
	fnChildren := make([]*Node, len(node.children))
	copy(fnChildren, node.children)
	fnChildren[bodyIndex] = newBody
	if arena != nil {
		buf := arena.allocNodeSlice(len(fnChildren))
		copy(buf, fnChildren)
		fnChildren = buf
	}
	newFn.children = fnChildren
	newFn.endByte = newBody.endByte
	newFn.endPoint = newBody.endPoint
	return newFn, hoisted, true
}

func flattenPythonSimpleStatements(node *Node, out []*Node, lang *Language) []*Node {
	if node == nil {
		return out
	}
	switch node.Type(lang) {
	case "_simple_statements", "_simple_statements_repeat1":
		for _, child := range node.children {
			out = flattenPythonSimpleStatements(child, out, lang)
		}
		return out
	case "expression_statement":
		if len(node.children) == 1 && node.children[0] != nil && node.children[0].IsNamed() {
			return append(out, node.children[0])
		}
	}
	if node.IsNamed() || (lang != nil && node.Type(lang) == ";") {
		return append(out, node)
	}
	return out
}

func normalizePythonStringContinuationEscapes(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return
	}
	escapeSym, ok := symbolByName(lang, "escape_sequence")
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "string_content" && n.startByte < n.endByte && int(n.endByte) <= len(source) {
			n.children = addPythonContinuationEscapes(n, source, escapeSym)
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func addPythonContinuationEscapes(node *Node, source []byte, escapeSym Symbol) []*Node {
	if node == nil || node.startByte >= node.endByte || int(node.endByte) > len(source) {
		return node.children
	}
	children := node.children
	changed := false
	for i := int(node.startByte); i+1 < int(node.endByte); i++ {
		if source[i] != '\\' {
			continue
		}
		end := i + 2
		if source[i+1] == '\r' && end < int(node.endByte) && source[end] == '\n' {
			end++
		} else if source[i+1] != '\n' {
			continue
		}
		found := false
		for _, child := range children {
			if child != nil && child.startByte == uint32(i) && child.endByte == uint32(end) && child.symbol == escapeSym {
				found = true
				break
			}
		}
		if found {
			i = end - 1
			continue
		}
		startPoint := advancePointByBytes(Point{}, source[:i])
		esc := newLeafNodeInArena(node.ownerArena, escapeSym, true, uint32(i), uint32(end), startPoint, advancePointByBytes(startPoint, source[i:end]))
		insertAt := len(children)
		for idx, child := range children {
			if child == nil || child.startByte > uint32(i) {
				insertAt = idx
				break
			}
		}
		next := make([]*Node, 0, len(children)+1)
		next = append(next, children[:insertAt]...)
		next = append(next, esc)
		next = append(next, children[insertAt:]...)
		if node.ownerArena != nil {
			buf := node.ownerArena.allocNodeSlice(len(next))
			copy(buf, next)
			next = buf
		}
		children = next
		changed = true
		i = end - 1
	}
	if !changed {
		return node.children
	}
	return children
}

func pythonSyntheticClassFieldIDs(arena *nodeArena, childCount int, hasArgList bool, lang *Language) []FieldID {
	fieldIDs := make([]FieldID, childCount)
	if arena != nil {
		fieldIDs = arena.allocFieldIDSlice(childCount)
	}
	if fid, ok := lang.FieldByName("name"); ok && childCount > 1 {
		fieldIDs[1] = fid
	}
	if hasArgList {
		if fid, ok := lang.FieldByName("superclasses"); ok && childCount > 2 {
			fieldIDs[2] = fid
		}
		if fid, ok := lang.FieldByName("body"); ok && childCount > 4 {
			fieldIDs[4] = fid
		}
		return fieldIDs
	}
	if fid, ok := lang.FieldByName("body"); ok && childCount > 3 {
		fieldIDs[3] = fid
	}
	return fieldIDs
}

func pythonSyntheticFunctionFieldIDs(arena *nodeArena, childCount int, lang *Language) []FieldID {
	fieldIDs := make([]FieldID, childCount)
	if arena != nil {
		fieldIDs = arena.allocFieldIDSlice(childCount)
	}
	if fid, ok := lang.FieldByName("name"); ok && childCount > 1 {
		fieldIDs[1] = fid
	}
	if fid, ok := lang.FieldByName("parameters"); ok && childCount > 2 {
		fieldIDs[2] = fid
	}
	if fid, ok := lang.FieldByName("body"); ok && childCount > 4 {
		fieldIDs[4] = fid
	}
	return fieldIDs
}
func pythonSyntheticIfFieldIDs(arena *nodeArena, childCount int, lang *Language) []FieldID {
	fieldIDs := make([]FieldID, childCount)
	if arena != nil {
		fieldIDs = arena.allocFieldIDSlice(childCount)
	}
	if fid, ok := lang.FieldByName("condition"); ok && childCount > 1 {
		fieldIDs[1] = fid
	}
	if fid, ok := lang.FieldByName("consequence"); ok && childCount > 3 {
		fieldIDs[3] = fid
	}
	return fieldIDs
}

func pythonModuleChildrenLookComplete(nodes []*Node, lang *Language) bool {
	if len(nodes) == 0 {
		return false
	}
	seen := 0
	for _, n := range nodes {
		if n == nil || n.isExtra {
			continue
		}
		if n.IsNamed() {
			seen++
			continue
		}
		switch n.Type(lang) {
		case "_simple_statements":
			seen++
		default:
			return false
		}
	}
	return seen > 0
}
