package gotreesitter

import "strings"

const cConditionClauseRepairPrefix = "void _tq_() { "

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
	root.symbol = sym
	root.isNamed = int(sym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[sym].Named
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
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
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
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
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
	preprocArgNamed := hasPreprocArg && int(preprocArgSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[preprocArgSym].Named

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
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	root.children = out
	root.fieldIDs = nil
	root.fieldSources = nil
	populateParentNode(root, out)
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
	if node.ownerArena != nil {
		buf := node.ownerArena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	node.symbol = preprocDefSym
	node.isNamed = int(preprocDefSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[preprocDefSym].Named
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
	identifierNamed := false
	if int(identifierSym) < len(lang.SymbolMetadata) {
		identifierNamed = lang.SymbolMetadata[identifierSym].Named
	}
	parenthesizedNamed := false
	if int(parenthesizedSym) < len(lang.SymbolMetadata) {
		parenthesizedNamed = lang.SymbolMetadata[parenthesizedSym].Named
	}
	valueFieldID, hasValueField := lang.FieldByName("value")
	localTypes := collectCLocalTypeNames(root, source, lang)

	var rewrite func(*Node)
	rewrite = func(n *Node) {
		if n == nil {
			return
		}
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
		for _, child := range n.children {
			rewrite(child)
		}
	}
	rewrite(root)
}

func normalizeCBuiltinPrimitiveTypeIdentifiers(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "c" {
		return
	}
	primitiveTypeSym, ok := lang.SymbolByName("primitive_type")
	if !ok {
		return
	}
	primitiveTypeNamed := false
	if int(primitiveTypeSym) < len(lang.SymbolMetadata) {
		primitiveTypeNamed = lang.SymbolMetadata[primitiveTypeSym].Named
	}
	var rewrite func(*Node)
	rewrite = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "type_identifier" && isCBuiltinPrimitiveTypeName(canonicalCTypeName(n.Text(source))) {
			n.symbol = primitiveTypeSym
			n.isNamed = primitiveTypeNamed
		}
		for _, child := range n.children {
			rewrite(child)
		}
	}
	rewrite(root)
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
	ellipsisNamed := false
	if int(ellipsisSym) < len(lang.SymbolMetadata) {
		ellipsisNamed = lang.SymbolMetadata[ellipsisSym].Named
	}
	var rewrite func(*Node)
	rewrite = func(n *Node) {
		if n == nil {
			return
		}
		if n.symbol == variadicSym && len(n.children) == 0 {
			child := newLeafNodeInArena(n.ownerArena, ellipsisSym, ellipsisNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
			populateParentNode(n, n.children)
		}
		for _, child := range n.children {
			rewrite(child)
		}
	}
	rewrite(root)
}

func normalizeCPreprocNewlineSpans(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || (lang.Name != "c" && lang.Name != "cpp") || len(source) == 0 {
		return
	}
	nlSym, ok := symbolByName(lang, "\n")
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
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
			walk(child)
		}
	}
	walk(root)
}

func normalizeCBareTypeIdentifierExpressionStatements(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "c", "cpp", "cuda", "arduino":
		// All C-family languages share the type_identifier vs expression_statement
		// ambiguity where the DFA lexer cannot distinguish typedefs from identifiers.
	default:
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
	exprStmtNamed := int(exprStmtSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[exprStmtSym].Named
	identNamed := int(identSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[identSym].Named
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
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
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeCCastUnknownTypeIdentifiers(root *Node, source []byte, lang *Language) {
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
	callSym, ok := lang.SymbolByName("call_expression")
	if !ok {
		return
	}
	castSym, ok := lang.SymbolByName("cast_expression")
	if !ok {
		return
	}
	argumentListSym, ok := lang.SymbolByName("argument_list")
	if !ok {
		return
	}
	functionFieldID, hasFunctionField := lang.FieldByName("function")
	argumentsFieldID, hasArgumentsField := lang.FieldByName("arguments")
	if !hasFunctionField || !hasArgumentsField {
		return
	}
	typeFieldID, hasTypeField := lang.FieldByName("type")
	valueFieldID, hasValueField := lang.FieldByName("value")
	if !hasTypeField || !hasValueField {
		return
	}
	identifierNamed := false
	if int(identifierSym) < len(lang.SymbolMetadata) {
		identifierNamed = lang.SymbolMetadata[identifierSym].Named
	}
	typeDescriptorNamed := false
	if int(typeDescriptorSym) < len(lang.SymbolMetadata) {
		typeDescriptorNamed = lang.SymbolMetadata[typeDescriptorSym].Named
	}
	typeIdentifierNamed := false
	if int(typeIdentifierSym) < len(lang.SymbolMetadata) {
		typeIdentifierNamed = lang.SymbolMetadata[typeIdentifierSym].Named
	}
	parenthesizedNamed := false
	if int(parenthesizedSym) < len(lang.SymbolMetadata) {
		parenthesizedNamed = lang.SymbolMetadata[parenthesizedSym].Named
	}
	callNamed := false
	if int(callSym) < len(lang.SymbolMetadata) {
		callNamed = lang.SymbolMetadata[callSym].Named
	}
	castNamed := false
	if int(castSym) < len(lang.SymbolMetadata) {
		castNamed = lang.SymbolMetadata[castSym].Named
	}
	argumentListNamed := false
	if int(argumentListSym) < len(lang.SymbolMetadata) {
		argumentListNamed = lang.SymbolMetadata[argumentListSym].Named
	}
	localTypes := collectCLocalTypeNames(root, source, lang)

	var rewrite func(*Node)
	rewrite = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "cast_expression" && len(n.children) == 4 {
			typeDescriptor := n.children[1]
			value := n.children[3]
			if typeDescriptor != nil && value != nil && typeDescriptor.symbol == typeDescriptorSym && len(typeDescriptor.children) == 1 {
				typeIdent := typeDescriptor.children[0]
				if typeIdent != nil && typeIdent.symbol == typeIdentifierSym && value.Type(lang) == "parenthesized_expression" {
					name := typeIdent.Text(source)
					if _, ok := localTypes[name]; !ok {
						ident := newLeafNodeInArena(n.ownerArena, identifierSym, identifierNamed, typeIdent.startByte, typeIdent.endByte, typeIdent.startPoint, typeIdent.endPoint)
						function := newParentNodeInArena(n.ownerArena, parenthesizedSym, parenthesizedNamed, []*Node{n.children[0], ident, n.children[2]}, nil, 0)
						argsChildren := append([]*Node(nil), value.children...)
						if n.ownerArena != nil && len(argsChildren) > 0 {
							buf := n.ownerArena.allocNodeSlice(len(argsChildren))
							copy(buf, argsChildren)
							argsChildren = buf
						}
						arguments := newParentNodeInArena(n.ownerArena, argumentListSym, argumentListNamed, argsChildren, nil, 0)
						children := []*Node{function, arguments}
						if n.ownerArena != nil {
							buf := n.ownerArena.allocNodeSlice(len(children))
							copy(buf, children)
							children = buf
						}
						fieldIDs := make([]FieldID, len(children))
						fieldIDs[0] = functionFieldID
						fieldIDs[1] = argumentsFieldID
						if n.ownerArena != nil {
							buf := n.ownerArena.allocFieldIDSlice(len(fieldIDs))
							copy(buf, fieldIDs)
							fieldIDs = buf
						}
						n.symbol = callSym
						n.isNamed = callNamed
						n.children = children
						n.fieldIDs = fieldIDs
						n.fieldSources = make([]uint8, len(children))
						n.fieldSources[0] = fieldSourceDirect
						n.fieldSources[1] = fieldSourceDirect
						n.productionID = 0
						for i, child := range n.children {
							if child == nil {
								continue
							}
							child.parent = n
							child.childIndex = i
						}
					}
				}
			}
		}
		for _, child := range n.children {
			rewrite(child)
		}
	}
	rewrite(root)

	var repair func(*Node)
	repair = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "call_expression" && len(n.children) == 2 {
			function := n.children[0]
			arguments := n.children[1]
			if function != nil && arguments != nil &&
				function.Type(lang) == "parenthesized_expression" &&
				arguments.Type(lang) == "argument_list" &&
				len(function.children) >= 3 {
				var ident *Node
				for _, child := range function.children {
					if child != nil && child.Type(lang) == "identifier" {
						ident = child
						break
					}
				}
				if ident != nil {
					name := canonicalCTypeName(ident.Text(source))
					if _, ok := localTypes[name]; ok {
						typeIdent := newLeafNodeInArena(n.ownerArena, typeIdentifierSym, typeIdentifierNamed, ident.startByte, ident.endByte, ident.startPoint, ident.endPoint)
						typeDescriptor := newParentNodeInArena(n.ownerArena, typeDescriptorSym, typeDescriptorNamed, []*Node{typeIdent}, nil, 0)
						var valueNode *Node
						for _, child := range arguments.children {
							if child != nil && child.isNamed {
								valueNode = child
								break
							}
						}
						if valueNode != nil {
							children := []*Node{function.children[0], typeDescriptor, function.children[len(function.children)-1], valueNode}
							if n.ownerArena != nil {
								buf := n.ownerArena.allocNodeSlice(len(children))
								copy(buf, children)
								children = buf
							}
							fieldIDs := make([]FieldID, len(children))
							fieldIDs[1] = typeFieldID
							fieldIDs[3] = valueFieldID
							if n.ownerArena != nil {
								buf := n.ownerArena.allocFieldIDSlice(len(fieldIDs))
								copy(buf, fieldIDs)
								fieldIDs = buf
							}
							n.symbol = castSym
							n.isNamed = castNamed
							n.children = children
							n.fieldIDs = fieldIDs
							n.fieldSources = make([]uint8, len(children))
							n.fieldSources[1] = fieldSourceDirect
							n.fieldSources[3] = fieldSourceDirect
							n.productionID = 0
							for i, child := range n.children {
								if child == nil {
									continue
								}
								child.parent = n
								child.childIndex = i
							}
						}
					}
				}
			}
		}
		for _, child := range n.children {
			repair(child)
		}
	}
	repair(root)
}

func normalizeCPointerAssignmentPrecedence(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	if lang.Name != "c" && lang.Name != "cpp" {
		return
	}

	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		for i, child := range n.children {
			walk(child)
			for {
				rewritten := rewriteCPointerAssignmentPrecedence(child, lang)
				if rewritten == nil {
					break
				}
				n.children[i] = rewritten
				rewritten.parent = n
				rewritten.childIndex = i
				child = rewritten
			}
		}
	}
	walk(root)
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
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
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
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
	return localTypes
}

// normalizeCConditionClauseAssignments repairs ERROR nodes that arise when
// grammargen's parser misidentifies assignment expressions (a = b) as
// declarations inside while/if/for condition clauses. The C/C++ grammar
// allows both declarations and expressions in condition_clause, and without
// an external scanner to distinguish type names from identifiers, grammargen
// may take the declaration path and fail.
//
// The fix: when we see an ERROR at the root level that contains tokens
// consistent with a control-flow statement + assignment in condition, we
// re-parse the condition portion as a standalone expression and rebuild
// the tree.
func normalizeCConditionClauseAssignments(root *Node, source []byte, lang *Language, parser *Parser) {
	if root == nil || lang == nil || parser == nil {
		return
	}
	switch lang.Name {
	case "c", "cpp", "cuda", "arduino":
	default:
		return
	}

	// Only act if the root itself is ERROR — this handles the case where
	// the entire statement failed to parse.
	if root.Type(lang) != "ERROR" {
		// Also walk children for ERROR nodes inside otherwise valid trees.
		normalizeCConditionClauseAssignmentsInner(root, source, lang, parser)
		return
	}

	// Check if root ERROR has children that look like a control-flow
	// statement with a broken condition: ERROR, (, tokens..., ), compound_statement
	// This is the pattern from `while ((a = b)) {}`
	if len(root.children) < 3 {
		return
	}

	// Find the keyword (while/if/for) in the ERROR children or as a direct child.
	var keyword string
	var keywordSym Symbol
	for _, kw := range []string{"while", "if", "for"} {
		sym, ok := symbolByName(lang, kw)
		if ok {
			for _, ch := range root.children {
				if ch != nil && ch.symbol == sym {
					keyword = kw
					keywordSym = sym
					break
				}
			}
		}
		if keyword != "" {
			break
		}
		// Also check ERROR children for the keyword.
		if root.children[0] != nil && root.children[0].Type(lang) == "ERROR" {
			errChild := root.children[0]
			for _, ch := range errChild.children {
				if ch != nil && ch.Type(lang) == kw {
					keyword = kw
					keywordSym, _ = symbolByName(lang, kw)
					break
				}
			}
		}
		if keyword != "" {
			break
		}
	}

	if keyword == "" {
		return
	}
	_ = keywordSym

	// Strategy: wrap the entire source in a function body so the statement
	// parses as a block_item, using identifiers instead of type_identifiers.
	// Actually, simpler: the runtime blob parser handles this correctly,
	// so if we have access to the blob language, re-parse with that.
	// But we don't necessarily have it here.
	//
	// Alternative: wrap the condition content in an expression statement
	// and parse that, then splice into a synthesized while_statement.
	// This gets complex quickly.
	//
	// Simplest viable fix: wrap the snippet in a function body so the parser
	// is forced to treat it as a statement, then splice the recovered
	// statement back under a translation_unit root.
	wrapped := []byte(cConditionClauseRepairPrefix + string(source[root.StartByte():root.EndByte()]) + " }")
	wrapTree, _ := parser.Parse(wrapped)
	if wrapTree == nil || wrapTree.RootNode() == nil {
		return
	}
	defer wrapTree.Release()
	if wrapTree.RootNode().HasError() {
		return
	}

	stmt := firstStatementInWrappedCRepair(wrapTree.RootNode(), lang)
	if stmt == nil || stmt.hasError {
		return
	}

	repairArena := root.ownerArena
	if repairArena == nil {
		repairArena = newNodeArena(arenaClassFull)
		root.ownerArena = repairArena
	}
	repaired := cloneTreeNodesIntoArena(stmt, repairArena)
	delta := int(root.StartByte()) - len(cConditionClauseRepairPrefix)
	if !shiftNodeOffsetsToSource(repaired, delta, source) {
		return
	}

	root.fieldIDs = nil
	root.fieldSources = nil
	root.productionID = 0
	if translationUnitSym, ok := symbolByName(lang, "translation_unit"); ok {
		root.symbol = translationUnitSym
		root.isNamed = int(translationUnitSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[translationUnitSym].Named
		root.children = cloneNodeSliceInArena(root.ownerArena, []*Node{repaired})
		populateParentNode(root, root.children)
		return
	}

	// Fallback for languages without a top-level translation unit symbol.
	stmtSym, ok := symbolByName(lang, repaired.Type(lang))
	if !ok {
		return
	}
	root.symbol = stmtSym
	root.isNamed = int(stmtSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[stmtSym].Named
	root.children = repaired.children
	populateParentNode(root, root.children)
}

func firstStatementInWrappedCRepair(root *Node, lang *Language) *Node {
	if root == nil || lang == nil || root.ChildCount() < 1 {
		return nil
	}
	funcDef := root.Child(0)
	if funcDef == nil || funcDef.Type(lang) != "function_definition" {
		return nil
	}
	for i := 0; i < funcDef.ChildCount(); i++ {
		ch := funcDef.Child(i)
		if ch == nil || ch.Type(lang) != "compound_statement" {
			continue
		}
		for j := 0; j < ch.ChildCount(); j++ {
			stmt := ch.Child(j)
			if stmt != nil && stmt.Type(lang) != "{" && stmt.Type(lang) != "}" {
				return stmt
			}
		}
	}
	return nil
}

func shiftNodeOffsetsToSource(n *Node, delta int, source []byte) bool {
	if n == nil {
		return true
	}
	start := int(n.startByte) + delta
	end := int(n.endByte) + delta
	if start < 0 || end < start || end > len(source) {
		return false
	}
	n.startByte = uint32(start)
	n.endByte = uint32(end)
	n.startPoint = advancePointByBytes(Point{}, source[:start])
	n.endPoint = advancePointByBytes(Point{}, source[:end])
	for _, ch := range n.children {
		if !shiftNodeOffsetsToSource(ch, delta, source) {
			return false
		}
	}
	return true
}

func normalizeCConditionClauseAssignmentsInner(root *Node, source []byte, lang *Language, parser *Parser) {
	_ = source
	_ = parser
	if root == nil || lang == nil {
		return
	}
	// Walk tree looking for ERROR children that could be repaired condition clauses.
	// For now, only handle the root-level ERROR case.
	// TODO: extend to handle ERROR nodes inside otherwise valid trees.
}
