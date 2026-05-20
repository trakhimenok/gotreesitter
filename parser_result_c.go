package gotreesitter

import "strings"

func normalizeCCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCTranslationUnitRoot(root, lang)
	normalizeCPreprocessorDirectiveShapes(root, source, lang)
	normalizeCDeclarationBounds(root, source, lang)
	normalizeCBuiltinPrimitiveTypeIdentifiers(root, source, lang)
	normalizeCVariadicParameterEllipsis(root, lang)
	normalizeCSizeofUnknownTypeIdentifiers(root, source, lang)
	normalizeCCastUnknownTypeIdentifiers(root, source, lang)
	normalizeCBareTypeIdentifierExpressionStatements(root, source, lang)
	normalizeCPreprocNewlineSpans(root, source, lang)
	normalizeCPointerAssignmentPrecedence(root, lang)
}

func normalizeCTranslationUnitRoot(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "ERROR" {
		return
	}
	if lang.Name != "c" && lang.Name != "cpp" {
		return
	}
	sym, ok := symbolByName(lang, "translation_unit")
	if !ok || !rootLooksLikeCTopLevel(root, lang) {
		return
	}
	retagResultRoot(root, sym, symbolIsNamed(lang, sym))
}

func rootLooksLikeCTopLevel(root *Node, lang *Language) bool {
	if root == nil || lang == nil || len(root.children) == 0 {
		return false
	}
	sawTopLevel := false
	for _, child := range root.children {
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "preproc_if",
			"preproc_ifdef",
			"preproc_include",
			"preproc_def",
			"preproc_function_def",
			"preproc_call",
			"declaration",
			"function_definition",
			"linkage_specification",
			"type_definition",
			"struct_specifier",
			"union_specifier",
			"enum_specifier",
			"class_specifier",
			"namespace_definition",
			"template_declaration",
			"comment":
			sawTopLevel = true
		default:
			return false
		}
	}
	return sawTopLevel
}

func normalizeCDeclarationBounds(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	if lang.Name != "c" && lang.Name != "cpp" {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "declaration" {
			first, last := firstAndLastNonNilChild(n.children)
			if first != nil && n.startByte < first.startByte &&
				first.startByte <= uint32(len(source)) &&
				cBytesAreCommentOrTrivia(source[n.startByte:first.startByte]) {
				n.startByte = first.startByte
				n.startPoint = first.startPoint
			}
			if last != nil && n.endByte > last.endByte &&
				n.endByte <= uint32(len(source)) &&
				bytesAreTrivia(source[last.endByte:n.endByte]) {
				setNodeEndTo(n, last.endByte, source)
			}
		}
	})
}

func normalizeCPreprocessorDirectiveShapes(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(root.children) == 0 {
		return
	}
	if lang.Name != "c" && lang.Name != "cpp" {
		return
	}
	if root.Type(lang) != "translation_unit" {
		return
	}
	preprocDefSym, hasPreprocDef := symbolByName(lang, "preproc_def")
	preprocArgSym, hasPreprocArg := symbolByName(lang, "preproc_arg")
	nameFieldID, hasNameField := lang.FieldByName("name")
	valueFieldID, hasValueField := lang.FieldByName("value")
	preprocArgNamed := hasPreprocArg && symbolIsNamed(lang, preprocArgSym)

	out := make([]*Node, 0, len(root.children))
	changed := false
	for i := 0; i < len(root.children); i++ {
		child := root.children[i]
		if child == nil {
			continue
		}
		if hasPreprocDef && hasPreprocArg && hasNameField && hasValueField {
			if normalizeCWhitespaceSeparatedFunctionMacro(child, source, lang, preprocDefSym, preprocArgSym, preprocArgNamed, nameFieldID, valueFieldID) {
				changed = true
			}
		}
		if consumed, ok := normalizeCPreprocessorDirectiveRange(child, source, lang); ok {
			changed = true
			for i+1 < len(root.children) && root.children[i+1] != nil && root.children[i+1].startByte < consumed && root.children[i+1].endByte <= consumed {
				i++
			}
		}
		out = append(out, child)
	}
	if !changed {
		return
	}
	out = cloneNodeSliceIfArena(root.ownerArena, out)
	replaceNodeChildrenUnfielded(root, out)
	extendNodeToTrailingWhitespace(root, source)
}

func normalizeCWhitespaceSeparatedFunctionMacro(node *Node, source []byte, lang *Language, preprocDefSym, preprocArgSym Symbol, preprocArgNamed bool, nameFieldID, valueFieldID FieldID) bool {
	if node == nil || lang == nil || node.Type(lang) != "preproc_function_def" || len(node.children) < 3 || len(node.children) > 4 {
		return false
	}
	name := node.children[1]
	params := node.children[2]
	if name == nil || params == nil || name.Type(lang) != "identifier" || params.Type(lang) != "preproc_params" {
		return false
	}
	value := (*Node)(nil)
	if len(node.children) == 4 {
		value = node.children[3]
		if value == nil || value.Type(lang) != "preproc_arg" {
			return false
		}
	} else {
		value = newParentNodeInArena(node.ownerArena, preprocArgSym, preprocArgNamed, nil, nil, 0)
		value.startByte = params.startByte
		value.startPoint = params.startPoint
		value.endByte = params.endByte
		value.endPoint = params.endPoint
	}
	if name.endByte >= params.startByte || params.startByte > uint32(len(source)) {
		return false
	}
	if !bytesAreTrivia(source[name.endByte:params.startByte]) {
		return false
	}

	value.startByte = params.startByte
	value.startPoint = advancePointByBytes(Point{}, source[:params.startByte])
	if value.endByte < value.startByte {
		value.endByte = value.startByte
		value.endPoint = value.startPoint
	}

	children := []*Node{node.children[0], name, value}
	children = cloneNodeSliceIfArena(node.ownerArena, children)
	node.symbol = preprocDefSym
	node.setNamed(symbolIsNamed(lang, preprocDefSym))
	node.children = children
	ensureNodeFieldStorage(node, len(children))
	for i := range node.fieldIDs {
		node.fieldIDs[i] = 0
	}
	for i := range node.fieldSources {
		node.fieldSources[i] = fieldSourceNone
	}
	node.fieldIDs[1] = nameFieldID
	node.fieldIDs[2] = valueFieldID
	node.fieldSources[1] = fieldSourceDirect
	node.fieldSources[2] = fieldSourceDirect
	populateParentNode(node, node.children)
	return true
}

func normalizeCPreprocessorDirectiveRange(node *Node, source []byte, lang *Language) (uint32, bool) {
	if node == nil || lang == nil || len(node.children) == 0 {
		return 0, false
	}
	switch node.Type(lang) {
	case "preproc_def", "preproc_function_def", "preproc_call":
	default:
		return 0, false
	}
	arg := node.children[len(node.children)-1]
	if arg == nil || arg.Type(lang) != "preproc_arg" || node.startByte >= uint32(len(source)) {
		return 0, false
	}
	directiveEnd, valueEnd, ok := cScanPreprocessorDirectiveExtent(source, node.startByte)
	if !ok || directiveEnd <= node.endByte {
		return 0, false
	}
	valueStart := cScanPreprocessorValueStart(source, arg.startByte, valueEnd)
	if valueStart < arg.startByte || valueStart > valueEnd {
		valueStart = arg.startByte
	}
	arg.startByte = valueStart
	arg.startPoint = advancePointByBytes(Point{}, source[:valueStart])
	setNodeEndTo(arg, valueEnd, source)
	populateParentNode(node, node.children)
	extendNodeEndTo(node, directiveEnd, source)
	return directiveEnd, true
}

func cScanPreprocessorDirectiveExtent(source []byte, start uint32) (directiveEnd uint32, valueEnd uint32, ok bool) {
	if start >= uint32(len(source)) {
		return 0, 0, false
	}
	lineStart := int(start)
	lastValueEnd := lineStart
	for lineStart < len(source) {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' {
			lineEnd++
		}
		lastValueEnd = lineEnd
		if lineEnd > lineStart && source[lineEnd-1] == '\r' {
			lastValueEnd--
		}
		directiveEnd = uint32(lineEnd)
		if lineEnd < len(source) && source[lineEnd] == '\n' {
			directiveEnd++
		}
		if !cLineEndsWithContinuation(source[lineStart:lineEnd]) {
			return directiveEnd, uint32(lastValueEnd), true
		}
		lineStart = lineEnd + 1
	}
	return uint32(len(source)), uint32(lastValueEnd), true
}

func cScanPreprocessorValueStart(source []byte, start, end uint32) uint32 {
	if start > end || end > uint32(len(source)) {
		return start
	}
	i := start
	for i < end {
		switch source[i] {
		case ' ', '\t', '\n', '\r', '\f', '\\':
			i++
			continue
		default:
			return i
		}
	}
	return end
}

func cLineEndsWithContinuation(line []byte) bool {
	end := len(line)
	for end > 0 && (line[end-1] == ' ' || line[end-1] == '\t' || line[end-1] == '\f' || line[end-1] == '\r') {
		end--
	}
	if end == 0 || line[end-1] != '\\' {
		return false
	}
	backslashes := 0
	for i := end - 1; i >= 0 && line[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func cBytesAreCommentOrTrivia(b []byte) bool {
	for i := 0; i < len(b); {
		switch b[i] {
		case ' ', '\t', '\n', '\r', '\f':
			i++
		case '/':
			if i+1 >= len(b) {
				return false
			}
			switch b[i+1] {
			case '/':
				end, ok := cScanLineCommentEnd(b, i)
				if !ok {
					return false
				}
				i = end
			case '*':
				end, ok := cScanBlockCommentEnd(b, i)
				if !ok {
					return false
				}
				i = end
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}

func cScanLineCommentEnd(b []byte, start int) (int, bool) {
	if start+1 >= len(b) || b[start] != '/' || b[start+1] != '/' {
		return 0, false
	}
	i := start + 2
	for i < len(b) {
		if b[i] == '\n' {
			lineEnd := i
			if cLineEndsWithContinuation(b[start:lineEnd]) {
				i++
				continue
			}
			return i + 1, true
		}
		i++
	}
	return len(b), true
}

func cScanBlockCommentEnd(b []byte, start int) (int, bool) {
	if start+1 >= len(b) || b[start] != '/' || b[start+1] != '*' {
		return 0, false
	}
	for i := start + 2; i+1 < len(b); i++ {
		if b[i] == '*' && b[i+1] == '/' {
			return i + 2, true
		}
	}
	return 0, false
}

func normalizeCSizeofUnknownTypeIdentifiers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	typeDescriptorSym, ok := lang.SymbolByName("type_descriptor")
	if !ok {
		return
	}
	typeIdentifierSym, ok := lang.SymbolByName("type_identifier")
	if !ok {
		return
	}
	identifierSym, ok := lang.SymbolByName("identifier")
	if !ok {
		return
	}
	parenthesizedSym, ok := lang.SymbolByName("parenthesized_expression")
	if !ok {
		return
	}
	identifierNamed := symbolIsNamed(lang, identifierSym)
	parenthesizedNamed := symbolIsNamed(lang, parenthesizedSym)
	valueFieldID, hasValueField := lang.FieldByName("value")
	localTypes := collectCLocalTypeNames(root, source, lang)

	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "sizeof_expression" && len(n.children) == 4 {
			typeDescriptor := n.children[2]
			if typeDescriptor != nil && typeDescriptor.symbol == typeDescriptorSym && len(typeDescriptor.children) == 1 {
				typeIdent := typeDescriptor.children[0]
				if typeIdent != nil && typeIdent.symbol == typeIdentifierSym {
					name := canonicalCTypeName(typeIdent.Text(source))
					if _, ok := localTypes[name]; !ok {
						ident := newLeafNodeInArena(n.ownerArena, identifierSym, identifierNamed, typeIdent.startByte, typeIdent.endByte, typeIdent.startPoint, typeIdent.endPoint)
						paren := newParentNodeInArena(n.ownerArena, parenthesizedSym, parenthesizedNamed, []*Node{n.children[1], ident, n.children[3]}, nil, 0)
						replaceChildRangeWithSingleNode(n, 1, 4, paren)
						if hasValueField && len(n.children) > 1 {
							ensureNodeFieldStorage(n, len(n.children))
							n.fieldIDs[1] = valueFieldID
							n.fieldSources[1] = fieldSourceDirect
						}
					}
				}
			}
		}
	})
}

func normalizeCBuiltinPrimitiveTypeIdentifiers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	primitiveTypeSym, ok := lang.SymbolByName("primitive_type")
	if !ok {
		return
	}
	primitiveTypeNamed := symbolIsNamed(lang, primitiveTypeSym)
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "type_identifier" && isCBuiltinPrimitiveTypeName(canonicalCTypeName(n.Text(source))) {
			n.symbol = primitiveTypeSym
			n.setNamed(primitiveTypeNamed)
		}
	})
}

func normalizeCVariadicParameterEllipsis(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	variadicSym, ok := lang.SymbolByName("variadic_parameter")
	if !ok {
		return
	}
	ellipsisSym, ok := lang.SymbolByName("...")
	if !ok {
		return
	}
	ellipsisNamed := symbolIsNamed(lang, ellipsisSym)
	walkResultTree(root, func(n *Node) {
		if n.symbol == variadicSym && len(n.children) == 0 {
			child := newLeafNodeInArena(n.ownerArena, ellipsisSym, ellipsisNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
			populateParentNode(n, n.children)
		}
	})
}

func normalizeCPreprocNewlineSpans(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || (lang.Name != "c" && lang.Name != "cpp") || len(source) == 0 {
		return
	}
	nlSym, ok := symbolByName(lang, "\n")
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		for _, child := range n.children {
			if child != nil && child.symbol == nlSym && child.endByte < uint32(len(source)) {
				// Extend newline tokens to include consecutive newlines/whitespace
				end := child.endByte
				for end < uint32(len(source)) && (source[end] == '\n' || source[end] == '\r') {
					end++
				}
				if end > child.endByte {
					child.endByte = end
					child.endPoint = advancePointByBytes(Point{}, source[:end])
				}
			}
		}
	})
}

func normalizeCBareTypeIdentifierExpressionStatements(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	compoundSym, ok1 := symbolByName(lang, "compound_statement")
	typeIdSym, ok2 := symbolByName(lang, "type_identifier")
	semiSym, ok3 := symbolByName(lang, ";")
	exprStmtSym, ok4 := symbolByName(lang, "expression_statement")
	identSym, ok5 := symbolByName(lang, "identifier")
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		return
	}
	exprStmtNamed := symbolIsNamed(lang, exprStmtSym)
	identNamed := symbolIsNamed(lang, identSym)
	walkResultTree(root, func(n *Node) {
		if n.symbol == compoundSym {
			// Look for bare type_identifier ; pairs that should be expression_statement(identifier ;)
			newChildren := make([]*Node, 0, len(n.children))
			for i := 0; i < len(n.children); i++ {
				child := n.children[i]
				if child != nil && child.symbol == typeIdSym && i+1 < len(n.children) && n.children[i+1] != nil && n.children[i+1].symbol == semiSym {
					semi := n.children[i+1]
					ident := newLeafNodeInArena(n.ownerArena, identSym, identNamed, child.startByte, child.endByte, child.startPoint, child.endPoint)
					exprStmt := newParentNodeInArena(n.ownerArena, exprStmtSym, exprStmtNamed, []*Node{ident, semi}, nil, 0)
					exprStmt.startByte = child.startByte
					exprStmt.startPoint = child.startPoint
					exprStmt.endByte = semi.endByte
					exprStmt.endPoint = semi.endPoint
					newChildren = append(newChildren, exprStmt)
					i++ // skip the semicolon
					continue
				}
				newChildren = append(newChildren, child)
			}
			if len(newChildren) != len(n.children) {
				n.children = newChildren
			}
		}
	})
}

func normalizeCCastUnknownTypeIdentifiers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	syms, ok := cCastRewriteSymbolsForLanguage(lang)
	if !ok {
		return
	}
	localTypes := collectCLocalTypeNames(root, source, lang)

	walkResultTree(root, func(n *Node) {
		rewriteUnknownCCastAsCall(n, source, syms, localTypes)
	})

	walkResultTree(root, func(n *Node) {
		rewriteKnownCCallAsCast(n, source, syms, localTypes)
	})
}

type cCastRewriteSymbols struct {
	typeDescriptor     Symbol
	typeIdentifier     Symbol
	identifier         Symbol
	parenthesized      Symbol
	call               Symbol
	cast               Symbol
	argumentList       Symbol
	functionField      FieldID
	argumentsField     FieldID
	typeField          FieldID
	valueField         FieldID
	identifierNamed    bool
	typeDescNamed      bool
	typeIdentNamed     bool
	parenthesizedNamed bool
	callNamed          bool
	castNamed          bool
	argumentListNamed  bool
}

func cCastRewriteSymbolsForLanguage(lang *Language) (cCastRewriteSymbols, bool) {
	var syms cCastRewriteSymbols
	var ok bool
	if syms.typeDescriptor, ok = lang.SymbolByName("type_descriptor"); !ok {
		return syms, false
	}
	if syms.typeIdentifier, ok = lang.SymbolByName("type_identifier"); !ok {
		return syms, false
	}
	if syms.identifier, ok = lang.SymbolByName("identifier"); !ok {
		return syms, false
	}
	if syms.parenthesized, ok = lang.SymbolByName("parenthesized_expression"); !ok {
		return syms, false
	}
	if syms.call, ok = lang.SymbolByName("call_expression"); !ok {
		return syms, false
	}
	if syms.cast, ok = lang.SymbolByName("cast_expression"); !ok {
		return syms, false
	}
	if syms.argumentList, ok = lang.SymbolByName("argument_list"); !ok {
		return syms, false
	}
	if syms.functionField, ok = lang.FieldByName("function"); !ok {
		return syms, false
	}
	if syms.argumentsField, ok = lang.FieldByName("arguments"); !ok {
		return syms, false
	}
	if syms.typeField, ok = lang.FieldByName("type"); !ok {
		return syms, false
	}
	if syms.valueField, ok = lang.FieldByName("value"); !ok {
		return syms, false
	}
	syms.identifierNamed = symbolIsNamed(lang, syms.identifier)
	syms.typeDescNamed = symbolIsNamed(lang, syms.typeDescriptor)
	syms.typeIdentNamed = symbolIsNamed(lang, syms.typeIdentifier)
	syms.parenthesizedNamed = symbolIsNamed(lang, syms.parenthesized)
	syms.callNamed = symbolIsNamed(lang, syms.call)
	syms.castNamed = symbolIsNamed(lang, syms.cast)
	syms.argumentListNamed = symbolIsNamed(lang, syms.argumentList)
	return syms, true
}

func rewriteUnknownCCastAsCall(n *Node, source []byte, syms cCastRewriteSymbols, localTypes map[string]struct{}) {
	if n == nil || n.symbol != syms.cast || len(n.children) != 4 {
		return
	}
	typeDescriptor := n.children[1]
	value := n.children[3]
	if typeDescriptor == nil || value == nil || typeDescriptor.symbol != syms.typeDescriptor || len(typeDescriptor.children) != 1 {
		return
	}
	typeIdent := typeDescriptor.children[0]
	if typeIdent == nil || typeIdent.symbol != syms.typeIdentifier || value.symbol != syms.parenthesized {
		return
	}
	if _, ok := localTypes[typeIdent.Text(source)]; ok {
		return
	}

	ident := newLeafNodeInArena(n.ownerArena, syms.identifier, syms.identifierNamed, typeIdent.startByte, typeIdent.endByte, typeIdent.startPoint, typeIdent.endPoint)
	function := newParentNodeInArena(n.ownerArena, syms.parenthesized, syms.parenthesizedNamed, []*Node{n.children[0], ident, n.children[2]}, nil, 0)
	argsChildren := cloneNodeSliceIfArena(n.ownerArena, append([]*Node(nil), value.children...))
	arguments := newParentNodeInArena(n.ownerArena, syms.argumentList, syms.argumentListNamed, argsChildren, nil, 0)
	children := cloneNodeSliceIfArena(n.ownerArena, []*Node{function, arguments})
	fieldIDs := cloneFieldIDSliceInArena(n.ownerArena, []FieldID{syms.functionField, syms.argumentsField})
	setCRewriteChildren(n, syms.call, syms.callNamed, children, fieldIDs, []int{0, 1})
}

func rewriteKnownCCallAsCast(n *Node, source []byte, syms cCastRewriteSymbols, localTypes map[string]struct{}) {
	if n == nil || n.symbol != syms.call || len(n.children) != 2 {
		return
	}
	function := n.children[0]
	arguments := n.children[1]
	if function == nil || arguments == nil ||
		function.symbol != syms.parenthesized ||
		arguments.symbol != syms.argumentList ||
		len(function.children) < 3 {
		return
	}
	ident := firstChildWithSymbol(function.children, syms.identifier)
	if ident == nil {
		return
	}
	if _, ok := localTypes[canonicalCTypeName(ident.Text(source))]; !ok {
		return
	}
	valueNode := firstNamedChild(arguments.children)
	if valueNode == nil {
		return
	}

	typeIdent := newLeafNodeInArena(n.ownerArena, syms.typeIdentifier, syms.typeIdentNamed, ident.startByte, ident.endByte, ident.startPoint, ident.endPoint)
	typeDescriptor := newParentNodeInArena(n.ownerArena, syms.typeDescriptor, syms.typeDescNamed, []*Node{typeIdent}, nil, 0)
	children := cloneNodeSliceIfArena(n.ownerArena, []*Node{function.children[0], typeDescriptor, function.children[len(function.children)-1], valueNode})
	fieldIDs := make([]FieldID, len(children))
	fieldIDs[1] = syms.typeField
	fieldIDs[3] = syms.valueField
	fieldIDs = cloneFieldIDSliceInArena(n.ownerArena, fieldIDs)
	setCRewriteChildren(n, syms.cast, syms.castNamed, children, fieldIDs, []int{1, 3})
}

func firstChildWithSymbol(children []*Node, sym Symbol) *Node {
	for _, child := range children {
		if child != nil && child.symbol == sym {
			return child
		}
	}
	return nil
}

func firstNamedChild(children []*Node) *Node {
	for _, child := range children {
		if child != nil && child.isNamed() {
			return child
		}
	}
	return nil
}

func setCRewriteChildren(n *Node, symbol Symbol, named bool, children []*Node, fieldIDs []FieldID, directFieldIndexes []int) {
	n.symbol = symbol
	n.setNamed(named)
	n.children = children
	n.fieldIDs = fieldIDs
	n.fieldSources = make([]uint8, len(children))
	for _, idx := range directFieldIndexes {
		if idx >= 0 && idx < len(n.fieldSources) {
			n.fieldSources[idx] = fieldSourceDirect
		}
	}
	n.productionID = 0
	populateParentNode(n, n.children)
}

func normalizeCPointerAssignmentPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	if lang.Name != "c" && lang.Name != "cpp" {
		return
	}

	rewriteResultTreeChildrenPostorder(root, func(n *Node) *Node {
		return rewriteCPointerAssignmentPrecedence(n, lang)
	})
}

func rewriteCPointerAssignmentPrecedence(node *Node, lang *Language) *Node {
	if node == nil || lang == nil || node.Type(lang) != "pointer_expression" || len(node.children) != 2 {
		return nil
	}
	operator := node.children[0]
	assignment := node.children[1]
	if operator == nil || assignment == nil || operator.Type(lang) != "*" || assignment.Type(lang) != "assignment_expression" || len(assignment.children) != 3 {
		return nil
	}
	left := assignment.children[0]
	assignOp := assignment.children[1]
	right := assignment.children[2]
	if left == nil || assignOp == nil || right == nil || !isCAssignmentOperatorToken(assignOp.Type(lang)) {
		return nil
	}

	rewrittenPointer := cloneNodeInArena(node.ownerArena, node)
	rewrittenPointer.children = cloneNodeSliceInArena(node.ownerArena, []*Node{operator, left})
	populateParentNode(rewrittenPointer, rewrittenPointer.children)

	rewrittenAssign := cloneNodeInArena(node.ownerArena, assignment)
	rewrittenAssign.children = cloneNodeSliceInArena(node.ownerArena, []*Node{rewrittenPointer, assignOp, right})
	populateParentNode(rewrittenAssign, rewrittenAssign.children)
	return rewrittenAssign
}

func isCAssignmentOperatorToken(tok string) bool {
	if tok == "=" {
		return true
	}
	if !strings.HasSuffix(tok, "=") {
		return false
	}
	switch tok {
	case "==", "!=", "<=", ">=", "=>", "===", "!==":
		return false
	default:
		return true
	}
}

func isCBuiltinPrimitiveTypeName(name string) bool {
	switch name {
	case "char", "int", "float", "double", "void", "_Bool", "_Complex", "bool", "__int128",
		"size_t", "ssize_t", "ptrdiff_t", "intptr_t", "uintptr_t",
		"int8_t", "int16_t", "int32_t", "int64_t",
		"uint8_t", "uint16_t", "uint32_t", "uint64_t",
		"wchar_t", "char16_t", "char32_t":
		return true
	default:
		return false
	}
}

func canonicalCTypeName(name string) string {
	name = strings.TrimSpace(name)
	start, end := 0, len(name)
	for start < end && !isCTypeNameChar(name[start]) {
		start++
	}
	for end > start && !isCTypeNameChar(name[end-1]) {
		end--
	}
	return name[start:end]
}

func isCTypeNameChar(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

func collectCLocalTypeNames(root *Node, source []byte, lang *Language) map[string]struct{} {
	localTypes := make(map[string]struct{})
	if root == nil || lang == nil || lang.Name != "c" {
		return localTypes
	}
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "type_definition" {
			for _, child := range n.children {
				if child == nil || child.Type(lang) != "type_identifier" {
					continue
				}
				if name := canonicalCTypeName(child.Text(source)); name != "" {
					localTypes[name] = struct{}{}
				}
			}
		}
	})
	return localTypes
}
