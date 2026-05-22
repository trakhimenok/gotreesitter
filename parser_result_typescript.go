package gotreesitter

type typeScriptNormalizationContext struct {
	source []byte
	lang   *Language

	canRewriteGenericCalls      bool
	canRewriteInstantiatedCalls bool
	canRewriteAsExpressions     bool
	canRewriteGenericArrows     bool
	canRewriteClassDeclarations bool
	canClearEnumBodyFields      bool

	callSym                Symbol
	callNamed              bool
	instantiationExprSym   Symbol
	instantiationExprNamed bool
	typeArgsSym            Symbol
	typeArgsNamed          bool
	argsSym                Symbol
	argsNamed              bool
	predefinedTypeSym      Symbol
	predefinedTypeNamed    bool
	asExpressionSym        Symbol
	asExpressionNamed      bool
	functionFieldID        FieldID
	typeArgsFieldID        FieldID
	argumentsFieldID       FieldID
	binaryExpressionSym    Symbol
	assignmentExprSym      Symbol
	assignmentExprNamed    bool
	ternaryExprSym         Symbol
	ternaryExprNamed       bool
	unionTypeSym           Symbol
	unionTypeNamed         bool
	intersectionTypeSym    Symbol
	intersectionTypeNamed  bool
	objectTypeSym          Symbol
	objectTypeNamed        bool
	propertySignatureSym   Symbol
	propertySignatureNamed bool
	typeAnnotationSym      Symbol
	typeAnnotationNamed    bool
	objectSym              Symbol
	pairSym                Symbol
	propertyIdentifierSym  Symbol
	colonSym               Symbol
	greaterThanSym         Symbol
	parenthesizedExprSym   Symbol
	lessThanSym            Symbol
	identifierSym          Symbol
	memberExpressionSym    Symbol
	sequenceExpressionSym  Symbol
	typeIdentifierSym      Symbol
	typeIdentifierNamed    bool
	hasTypeIdentifierSym   bool
	typeAssertionSym       Symbol
	arrowFunctionSym       Symbol
	typeParametersSym      Symbol
	typeParametersNamed    bool
	typeParameterSym       Symbol
	typeParameterNamed     bool
	expressionStatementSym Symbol
	classSym               Symbol
	classDeclarationSym    Symbol
	classDeclarationNamed  bool
	enumBodySym            Symbol
	enumAssignmentSym      Symbol
	importSym              Symbol
	hasImportSym           bool
	nameFieldID            FieldID
	typeParametersFieldID  FieldID
}

func normalizeTypeScriptCompatibility(root *Node, source []byte, lang *Language) {
	ctx, ok := newTypeScriptNormalizationContext(source, lang)
	if !ok || root == nil {
		return
	}

	walkResultTreeDenseFirst(root, func(n *Node) {
		normalizeTypeScriptIdentifierKeywordAliases(n, &ctx)
		normalizeTypeScriptImportKeywordNamedness(n, &ctx)
		if ctx.canClearEnumBodyFields && n.symbol == ctx.enumBodySym && len(n.fieldIDs) > 0 {
			limit := len(n.children)
			if len(n.fieldIDs) < limit {
				limit = len(n.fieldIDs)
			}
			for i := 0; i < limit; i++ {
				child := n.children[i]
				if child == nil || child.symbol != ctx.enumAssignmentSym {
					continue
				}
				n.fieldIDs[i] = 0
				if len(n.fieldSources) > i {
					n.fieldSources[i] = fieldSourceNone
				}
			}
		}
		for i, child := range n.children {
			for {
				var rewritten *Node
				switch {
				case ctx.canRewriteGenericCalls:
					rewritten = rewriteTypeScriptPredefinedGenericCall(child, &ctx)
				}
				if rewritten == nil && ctx.canRewriteInstantiatedCalls {
					rewritten = rewriteTypeScriptInstantiatedCall(child, &ctx)
				}
				if rewritten == nil && ctx.canRewriteAsExpressions {
					rewritten = rewriteTypeScriptAsExpressionCompatibility(child, &ctx)
				}
				if rewritten == nil && ctx.canRewriteGenericArrows {
					rewritten = rewriteTypeScriptGenericArrowTypeAssertion(child, &ctx)
				}
				if rewritten == nil && ctx.canRewriteClassDeclarations {
					rewritten = rewriteTypeScriptClassExpressionStatement(child, &ctx)
				}
				if rewritten == nil {
					break
				}
				n.children[i] = rewritten
				rewritten.parent = n
				rewritten.childIndex = int32(i)
				child = rewritten
			}
		}
	})
}

func normalizeTypeScriptIdentifierKeywordAliases(node *Node, ctx *typeScriptNormalizationContext) {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.identifierSym || len(node.children) != 1 {
		return
	}
	child := node.children[0]
	if child == nil || child.IsNamed() || child.IsExtra() {
		return
	}
	if child.startByte != node.startByte || child.endByte != node.endByte || child.startPoint != node.startPoint || child.endPoint != node.endPoint {
		return
	}
	node.children = nil
	node.fieldIDs = nil
	node.fieldSources = nil
}

func normalizeTypeScriptImportKeywordNamedness(node *Node, ctx *typeScriptNormalizationContext) {
	if node == nil || ctx == nil || !ctx.hasImportSym || node.symbol != ctx.importSym {
		return
	}
	node.setNamed(false)
}

func normalizeTypeScriptRecoveredNamespaceRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(root.children) < 4 {
		return
	}
	if lang.Name != "tsx" && lang.Name != "typescript" {
		return
	}
	rootType := root.Type(lang)
	if rootType != "ERROR" && rootType != "program" {
		return
	}
	stmtBlockSym, ok := lang.SymbolByName("statement_block")
	if !ok {
		return
	}
	internalModuleSym, ok := lang.SymbolByName("internal_module")
	if !ok {
		return
	}
	exprStmtSym, hasExprStmtSym := lang.SymbolByName("expression_statement")
	programSym, hasProgramSym := lang.SymbolByName("program")

	namespaceIdx := -1
	for i, child := range root.children {
		if child == nil || child.isExtra() {
			continue
		}
		if child.Type(lang) != "namespace" {
			if child.Type(lang) != "comment" {
				return
			}
			continue
		}
		namespaceIdx = i
		break
	}
	if namespaceIdx < 0 || namespaceIdx+2 >= len(root.children) {
		return
	}
	nameNode := root.children[namespaceIdx+1]
	openBrace := root.children[namespaceIdx+2]
	if nameNode == nil || openBrace == nil || nameNode.Type(lang) != "identifier" || openBrace.Type(lang) != "{" {
		return
	}

	bodyChildren := make([]*Node, 0, len(root.children)-(namespaceIdx+3))
	for i := namespaceIdx + 3; i < len(root.children); i++ {
		child := root.children[i]
		if child == nil {
			continue
		}
		if typeScriptWhitespaceOnlyRecoverySubtree(child, source) {
			continue
		}
		bodyChildren = append(bodyChildren, child)
	}
	if len(bodyChildren) == 0 {
		return
	}
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(bodyChildren))
		copy(buf, bodyChildren)
		bodyChildren = buf
	}

	stmtBlockNamed := symbolIsNamed(lang, stmtBlockSym)
	internalModuleNamed := symbolIsNamed(lang, internalModuleSym)
	block := newParentNodeInArena(root.ownerArena, stmtBlockSym, stmtBlockNamed, bodyChildren, nil, 0)
	block.startByte = openBrace.startByte
	block.startPoint = openBrace.startPoint
	if len(bodyChildren) > 0 {
		last := bodyChildren[len(bodyChildren)-1]
		block.endByte = last.endByte
		block.endPoint = last.endPoint
	}

	moduleChildren := []*Node{nameNode, block}
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(moduleChildren))
		copy(buf, moduleChildren)
		moduleChildren = buf
	}
	internalModule := newParentNodeInArena(root.ownerArena, internalModuleSym, internalModuleNamed, moduleChildren, nil, 0)
	internalModule.startByte = root.children[namespaceIdx].startByte
	internalModule.startPoint = root.children[namespaceIdx].startPoint
	internalModule.endByte = block.endByte
	internalModule.endPoint = block.endPoint

	wrapped := internalModule
	if hasExprStmtSym {
		exprStmtNamed := symbolIsNamed(lang, exprStmtSym)
		exprChildren := []*Node{internalModule}
		if root.ownerArena != nil {
			buf := root.ownerArena.allocNodeSlice(1)
			buf[0] = internalModule
			exprChildren = buf
		}
		exprStmt := newParentNodeInArena(root.ownerArena, exprStmtSym, exprStmtNamed, exprChildren, nil, 0)
		exprStmt.startByte = internalModule.startByte
		exprStmt.startPoint = internalModule.startPoint
		exprStmt.endByte = internalModule.endByte
		exprStmt.endPoint = internalModule.endPoint
		wrapped = exprStmt
	}

	newChildren := make([]*Node, 0, namespaceIdx+1)
	for i := 0; i < namespaceIdx; i++ {
		if root.children[i] != nil {
			newChildren = append(newChildren, root.children[i])
		}
	}
	newChildren = append(newChildren, wrapped)
	if root.ownerArena != nil {
		buf := root.ownerArena.allocNodeSlice(len(newChildren))
		copy(buf, newChildren)
		newChildren = buf
	}
	if hasProgramSym {
		retagResultRoot(root, programSym, symbolIsNamed(lang, programSym))
	}
	replaceNodeChildrenUnfielded(root, newChildren)
}

func typeScriptWhitespaceOnlyRecoverySubtree(node *Node, source []byte) bool {
	if node == nil || (!node.HasError() && node.symbol != errorSymbol) {
		return false
	}
	if int(node.endByte) > len(source) || node.startByte > node.endByte {
		return false
	}
	return bytesAreTrivia(source[node.startByte:node.endByte])
}

func newTypeScriptNormalizationContext(source []byte, lang *Language) (typeScriptNormalizationContext, bool) {
	ctx := typeScriptNormalizationContext{
		source: source,
		lang:   lang,
	}
	if lang == nil {
		return ctx, false
	}
	switch lang.Name {
	case "tsx", "typescript":
	default:
		return ctx, false
	}

	if syms, ok := languageSymbols(lang,
		"call_expression",
		"type_arguments",
		"arguments",
		"predefined_type",
		"binary_expression",
		">",
		"parenthesized_expression",
		"<",
		"identifier",
		"member_expression",
		"sequence_expression",
	); ok {
		ctx.canRewriteGenericCalls = true
		ctx.callSym = syms[0]
		ctx.callNamed = symbolIsNamed(lang, ctx.callSym)
		ctx.typeArgsSym = syms[1]
		ctx.typeArgsNamed = symbolIsNamed(lang, ctx.typeArgsSym)
		ctx.argsSym = syms[2]
		ctx.argsNamed = symbolIsNamed(lang, ctx.argsSym)
		ctx.predefinedTypeSym = syms[3]
		ctx.predefinedTypeNamed = symbolIsNamed(lang, ctx.predefinedTypeSym)
		ctx.binaryExpressionSym = syms[4]
		ctx.greaterThanSym = syms[5]
		ctx.parenthesizedExprSym = syms[6]
		ctx.lessThanSym = syms[7]
		ctx.identifierSym = syms[8]
		ctx.memberExpressionSym = syms[9]
		ctx.sequenceExpressionSym = syms[10]
		ctx.functionFieldID, _ = lang.FieldByName("function")
		ctx.typeArgsFieldID, _ = lang.FieldByName("type_arguments")
		ctx.argumentsFieldID, _ = lang.FieldByName("arguments")
		ctx.typeIdentifierSym, ctx.hasTypeIdentifierSym = lang.SymbolByName("type_identifier")
		if ctx.hasTypeIdentifierSym {
			ctx.typeIdentifierNamed = symbolIsNamed(lang, ctx.typeIdentifierSym)
		}
		if syms, ok := languageSymbols(lang, "instantiation_expression"); ok {
			ctx.instantiationExprSym = syms[0]
			ctx.instantiationExprNamed = symbolIsNamed(lang, ctx.instantiationExprSym)
			ctx.canRewriteInstantiatedCalls = ctx.functionFieldID != 0 && ctx.typeArgsFieldID != 0 && ctx.argumentsFieldID != 0
		}
	}

	if syms, ok := languageSymbols(lang,
		"as_expression",
		"assignment_expression",
		"ternary_expression",
		"union_type",
		"intersection_type",
	); ok {
		ctx.canRewriteAsExpressions = true
		ctx.asExpressionSym = syms[0]
		ctx.asExpressionNamed = symbolIsNamed(lang, ctx.asExpressionSym)
		ctx.assignmentExprSym = syms[1]
		ctx.assignmentExprNamed = symbolIsNamed(lang, ctx.assignmentExprSym)
		ctx.ternaryExprSym = syms[2]
		ctx.ternaryExprNamed = symbolIsNamed(lang, ctx.ternaryExprSym)
		ctx.unionTypeSym = syms[3]
		ctx.unionTypeNamed = symbolIsNamed(lang, ctx.unionTypeSym)
		ctx.intersectionTypeSym = syms[4]
		ctx.intersectionTypeNamed = symbolIsNamed(lang, ctx.intersectionTypeSym)
		if syms, ok := languageSymbols(lang,
			"object_type",
			"property_signature",
			"type_annotation",
			"object",
			"pair",
			"property_identifier",
			":",
		); ok {
			ctx.objectTypeSym = syms[0]
			ctx.objectTypeNamed = symbolIsNamed(lang, ctx.objectTypeSym)
			ctx.propertySignatureSym = syms[1]
			ctx.propertySignatureNamed = symbolIsNamed(lang, ctx.propertySignatureSym)
			ctx.typeAnnotationSym = syms[2]
			ctx.typeAnnotationNamed = symbolIsNamed(lang, ctx.typeAnnotationSym)
			ctx.objectSym = syms[3]
			ctx.pairSym = syms[4]
			ctx.propertyIdentifierSym = syms[5]
			ctx.colonSym = syms[6]
		}
	}

	if enumBodySym, ok := lang.SymbolByName("enum_body"); ok {
		if enumAssignmentSym, ok := lang.SymbolByName("enum_assignment"); ok {
			ctx.canClearEnumBodyFields = true
			ctx.enumBodySym = enumBodySym
			ctx.enumAssignmentSym = enumAssignmentSym
		}
	}
	ctx.importSym, ctx.hasImportSym = lang.SymbolByName("import")

	if syms, ok := visibleLanguageSymbols(lang, true,
		"type_assertion",
		"arrow_function",
		"type_arguments",
		"type_parameters",
		"type_parameter",
		"type_identifier",
	); ok {
		ctx.canRewriteGenericArrows = true
		ctx.typeAssertionSym = syms[0]
		ctx.arrowFunctionSym = syms[1]
		ctx.typeArgsSym = syms[2]
		ctx.typeParametersSym = syms[3]
		ctx.typeParametersNamed = symbolIsNamed(lang, ctx.typeParametersSym)
		ctx.typeParameterSym = syms[4]
		ctx.typeParameterNamed = symbolIsNamed(lang, ctx.typeParameterSym)
		ctx.typeIdentifierSym = syms[5]
		ctx.typeIdentifierNamed = symbolIsNamed(lang, ctx.typeIdentifierSym)
		ctx.nameFieldID, _ = lang.FieldByName("name")
		ctx.typeParametersFieldID, _ = lang.FieldByName("type_parameters")
	}

	if syms, ok := visibleLanguageSymbols(lang, true,
		"expression_statement",
		"class",
		"class_declaration",
	); ok {
		ctx.canRewriteClassDeclarations = true
		ctx.expressionStatementSym = syms[0]
		ctx.classSym = syms[1]
		ctx.classDeclarationSym = syms[2]
		ctx.classDeclarationNamed = symbolIsNamed(lang, ctx.classDeclarationSym)
		if ctx.nameFieldID == 0 {
			ctx.nameFieldID, _ = lang.FieldByName("name")
		}
	}

	return ctx, ctx.canRewriteGenericCalls || ctx.canRewriteInstantiatedCalls || ctx.canRewriteAsExpressions || ctx.canRewriteGenericArrows || ctx.canRewriteClassDeclarations || ctx.canClearEnumBodyFields
}

func rewriteTypeScriptGenericArrowTypeAssertion(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.typeAssertionSym || len(node.children) < 2 {
		return nil
	}
	typeArgs := node.children[0]
	arrow := node.children[len(node.children)-1]
	if typeArgs == nil || arrow == nil || typeArgs.symbol != ctx.typeArgsSym || arrow.symbol != ctx.arrowFunctionSym {
		return nil
	}

	typeParams := convertTypeScriptTypeArgumentsToParameters(typeArgs, ctx)
	if typeParams == nil {
		return nil
	}

	arena := node.ownerArena
	children := phpAllocChildren(arena, len(arrow.children)+1)
	children[0] = typeParams
	copy(children[1:], arrow.children)

	var fieldIDs []FieldID
	if ctx.typeParametersFieldID != 0 || len(arrow.fieldIDs) > 0 {
		if arena != nil {
			fieldIDs = arena.allocFieldIDSlice(len(children))
		} else {
			fieldIDs = make([]FieldID, len(children))
		}
		fieldIDs[0] = ctx.typeParametersFieldID
		copy(fieldIDs[1:], arrow.fieldIDs)
	}

	rewritten := cloneNodeInArena(arena, arrow)
	rewritten.children = children
	rewritten.fieldIDs = fieldIDs
	rewritten.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	populateParentNode(rewritten, rewritten.children)
	return rewritten
}

func convertTypeScriptTypeArgumentsToParameters(typeArgs *Node, ctx *typeScriptNormalizationContext) *Node {
	if typeArgs == nil || ctx == nil || ctx.lang == nil || typeArgs.symbol != ctx.typeArgsSym || len(typeArgs.children) == 0 {
		return nil
	}
	arena := typeArgs.ownerArena
	children := phpAllocChildren(arena, len(typeArgs.children))
	convertedNamed := 0
	for i, child := range typeArgs.children {
		if child == nil || !child.isNamed() {
			children[i] = child
			continue
		}
		if child.symbol != ctx.typeIdentifierSym {
			return nil
		}
		paramChildren := phpAllocChildren(arena, 1)
		paramChildren[0] = child
		var fieldIDs []FieldID
		if ctx.nameFieldID != 0 {
			if arena != nil {
				fieldIDs = arena.allocFieldIDSlice(1)
			} else {
				fieldIDs = make([]FieldID, 1)
			}
			fieldIDs[0] = ctx.nameFieldID
		}
		param := newParentNodeInArena(arena, ctx.typeParameterSym, ctx.typeParameterNamed, paramChildren, fieldIDs, child.productionID)
		param.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
		children[i] = param
		convertedNamed++
	}
	if convertedNamed == 0 {
		return nil
	}
	return newParentNodeInArena(arena, ctx.typeParametersSym, ctx.typeParametersNamed, children, nil, typeArgs.productionID)
}

func rewriteTypeScriptClassExpressionStatement(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.expressionStatementSym {
		return nil
	}
	var classNode *Node
	for _, child := range node.children {
		if child == nil || child.isExtra() {
			continue
		}
		if child.symbol == ctx.classSym {
			if classNode != nil {
				return nil
			}
			classNode = child
			continue
		}
		if child.isNamed() {
			return nil
		}
	}
	if classNode == nil || !typeScriptClassExpressionHasName(classNode, ctx) {
		return nil
	}
	children := cloneNodeSliceInArena(classNode.ownerArena, classNode.children)
	fieldIDs := cloneFieldIDSliceInArena(classNode.ownerArena, classNode.fieldIDs)
	decl := newParentNodeInArena(classNode.ownerArena, ctx.classDeclarationSym, ctx.classDeclarationNamed, children, fieldIDs, classNode.productionID)
	decl.fieldSources = cloneFieldSourceSliceInArena(classNode.ownerArena, classNode.fieldSources)
	if len(decl.fieldSources) == 0 {
		decl.fieldSources = defaultFieldSourcesInArena(classNode.ownerArena, fieldIDs)
	}
	return decl
}

func typeScriptClassExpressionHasName(node *Node, ctx *typeScriptNormalizationContext) bool {
	if node == nil || ctx == nil || ctx.lang == nil {
		return false
	}
	for i, child := range node.children {
		if child == nil {
			continue
		}
		if ctx.nameFieldID != 0 && i < len(node.fieldIDs) && node.fieldIDs[i] == ctx.nameFieldID {
			return child.symbol == ctx.typeIdentifierSym && child.endByte > child.startByte
		}
		if child.symbol == ctx.typeIdentifierSym && child.endByte > child.startByte {
			return true
		}
	}
	return false
}

func cloneFieldSourceSliceInArena(arena *nodeArena, fieldSources []uint8) []uint8 {
	if len(fieldSources) == 0 {
		return nil
	}
	if arena != nil {
		out := arena.allocFieldSourceSlice(len(fieldSources))
		copy(out, fieldSources)
		return out
	}
	out := make([]uint8, len(fieldSources))
	copy(out, fieldSources)
	return out
}

func rewriteTypeScriptPredefinedGenericCall(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil
	}
	left := node.children[0]
	gt := node.children[1]
	paren := node.children[2]
	if left == nil || gt == nil || paren == nil || left.symbol != ctx.binaryExpressionSym || gt.symbol != ctx.greaterThanSym || paren.symbol != ctx.parenthesizedExprSym {
		return nil
	}
	if len(left.children) != 3 || len(paren.children) != 3 {
		return nil
	}
	callee := left.children[0]
	lt := left.children[1]
	typeArg := left.children[2]
	if callee == nil || lt == nil || typeArg == nil || lt.symbol != ctx.lessThanSym {
		return nil
	}
	switch callee.Type(ctx.lang) {
	case "identifier", "member_expression":
	default:
		return nil
	}
	typeArg = normalizeTypeScriptGenericCallTypeArgument(typeArg, ctx)
	if typeArg == nil {
		return nil
	}
	arena := node.ownerArena
	if typeArg.ownerArena != arena {
		typeArg = cloneNodeInArena(arena, typeArg)
	}
	typeArgs := newParentNodeInArena(arena, ctx.typeArgsSym, ctx.typeArgsNamed, []*Node{lt, typeArg, gt}, nil, 0)
	argsChildren := typeScriptGenericCallArgumentChildren(paren, ctx.sequenceExpressionSym)
	if arena != nil && len(argsChildren) > 0 {
		buf := arena.allocNodeSlice(len(argsChildren))
		copy(buf, argsChildren)
		argsChildren = buf
	}
	args := newParentNodeInArena(arena, ctx.argsSym, ctx.argsNamed, argsChildren, nil, paren.productionID)

	callChildren := phpAllocChildren(arena, 3)
	callChildren[0] = callee
	callChildren[1] = typeArgs
	callChildren[2] = args
	var fieldIDs []FieldID
	if ctx.functionFieldID != 0 || ctx.typeArgsFieldID != 0 || ctx.argumentsFieldID != 0 {
		if arena != nil {
			fieldIDs = arena.allocFieldIDSlice(3)
		} else {
			fieldIDs = make([]FieldID, 3)
		}
		fieldIDs[0] = ctx.functionFieldID
		fieldIDs[1] = ctx.typeArgsFieldID
		fieldIDs[2] = ctx.argumentsFieldID
	}
	call := newParentNodeInArena(arena, ctx.callSym, ctx.callNamed, callChildren, fieldIDs, node.productionID)
	call.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	return call
}

func rewriteTypeScriptInstantiatedCall(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.callSym || len(node.children) != 2 {
		return nil
	}
	function := node.children[0]
	arguments := node.children[1]
	if function == nil || arguments == nil || function.symbol != ctx.instantiationExprSym || arguments.symbol != ctx.argsSym || len(function.children) != 2 {
		return nil
	}
	callee := function.children[0]
	typeArgs := function.children[1]
	if callee == nil || typeArgs == nil || typeArgs.symbol != ctx.typeArgsSym {
		return nil
	}
	children := phpAllocChildren(node.ownerArena, 3)
	children[0] = callee
	children[1] = typeArgs
	children[2] = arguments
	var fieldIDs []FieldID
	if ctx.functionFieldID != 0 || ctx.typeArgsFieldID != 0 || ctx.argumentsFieldID != 0 {
		if node.ownerArena != nil {
			fieldIDs = node.ownerArena.allocFieldIDSlice(3)
		} else {
			fieldIDs = make([]FieldID, 3)
		}
		fieldIDs[0] = ctx.functionFieldID
		fieldIDs[1] = ctx.typeArgsFieldID
		fieldIDs[2] = ctx.argumentsFieldID
	}
	call := newParentNodeInArena(node.ownerArena, ctx.callSym, ctx.callNamed, children, fieldIDs, node.productionID)
	call.fieldSources = defaultFieldSourcesInArena(node.ownerArena, fieldIDs)
	return call
}

func rewriteTypeScriptAsExpressionCompatibility(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil {
		return nil
	}
	if rewritten := rewriteTypeScriptAsAssignmentOrTernary(node, ctx); rewritten != nil {
		return rewritten
	}
	return rewriteTypeScriptAsTypeChain(node, ctx)
}

func rewriteTypeScriptAsAssignmentOrTernary(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.asExpressionSym || len(node.children) < 2 {
		return nil
	}
	valueIdx, typeIdx := 0, len(node.children)-1
	value := node.children[valueIdx]
	if value == nil {
		return nil
	}

	switch value.symbol {
	case ctx.assignmentExprSym:
		if len(value.children) < 2 {
			return nil
		}
		rightIdx := len(value.children) - 1
		rewrittenAs := cloneNodeInArena(node.ownerArena, node)
		asChildren := cloneNodeSliceInArena(node.ownerArena, node.children)
		asChildren[valueIdx] = value.children[rightIdx]
		rewrittenAs.children = asChildren
		populateParentNode(rewrittenAs, rewrittenAs.children)

		rewrittenAssign := cloneNodeInArena(node.ownerArena, value)
		assignChildren := cloneNodeSliceInArena(node.ownerArena, value.children)
		assignChildren[rightIdx] = rewrittenAs
		rewrittenAssign.children = assignChildren
		populateParentNode(rewrittenAssign, rewrittenAssign.children)
		return rewrittenAssign
	case ctx.ternaryExprSym:
		if len(value.children) < 3 {
			return nil
		}
		falseIdx := len(value.children) - 1
		rewrittenAs := cloneNodeInArena(node.ownerArena, node)
		asChildren := cloneNodeSliceInArena(node.ownerArena, node.children)
		asChildren[valueIdx] = value.children[falseIdx]
		rewrittenAs.children = asChildren
		populateParentNode(rewrittenAs, rewrittenAs.children)

		rewrittenTernary := cloneNodeInArena(node.ownerArena, value)
		ternaryChildren := cloneNodeSliceInArena(node.ownerArena, value.children)
		ternaryChildren[falseIdx] = rewrittenAs
		rewrittenTernary.children = ternaryChildren
		populateParentNode(rewrittenTernary, rewrittenTernary.children)
		return rewrittenTernary
	default:
		_ = typeIdx
		return nil
	}
}

func rewriteTypeScriptAsTypeChain(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil
	}
	baseAs, rewrittenType, ok := collapseTypeScriptAsTypeChain(node, ctx)
	if !ok || baseAs == nil || rewrittenType == nil || len(baseAs.children) < 2 {
		return nil
	}
	rewrittenAs := cloneNodeInArena(node.ownerArena, baseAs)
	asChildren := cloneNodeSliceInArena(node.ownerArena, baseAs.children)
	asChildren[len(asChildren)-1] = rewrittenType
	rewrittenAs.children = asChildren
	populateParentNode(rewrittenAs, rewrittenAs.children)
	return rewrittenAs
}

func collapseTypeScriptAsTypeChain(node *Node, ctx *typeScriptNormalizationContext) (*Node, *Node, bool) {
	if node == nil || ctx == nil || ctx.lang == nil || node.symbol != ctx.binaryExpressionSym || len(node.children) != 3 {
		return nil, nil, false
	}
	left := node.children[0]
	op := node.children[1]
	right := node.children[2]
	if left == nil || op == nil || right == nil {
		return nil, nil, false
	}
	var typeSym Symbol
	var typeNamed bool
	switch op.Type(ctx.lang) {
	case "|":
		typeSym = ctx.unionTypeSym
		typeNamed = ctx.unionTypeNamed
	case "&":
		typeSym = ctx.intersectionTypeSym
		typeNamed = ctx.intersectionTypeNamed
	default:
		return nil, nil, false
	}

	rightType := normalizeTypeScriptTypeExpression(right, ctx)
	if rightType == nil {
		return nil, nil, false
	}

	if left.symbol == ctx.asExpressionSym && len(left.children) >= 2 {
		leftType := normalizeTypeScriptTypeExpression(left.children[len(left.children)-1], ctx)
		if leftType == nil {
			return nil, nil, false
		}
		children := cloneNodeSliceInArena(node.ownerArena, []*Node{leftType, op, rightType})
		return left, newParentNodeInArena(node.ownerArena, typeSym, typeNamed, children, nil, node.productionID), true
	}

	leftAs, leftType, ok := collapseTypeScriptAsTypeChain(left, ctx)
	if !ok || leftAs == nil || leftType == nil {
		return nil, nil, false
	}
	children := cloneNodeSliceInArena(node.ownerArena, []*Node{leftType, op, rightType})
	return leftAs, newParentNodeInArena(node.ownerArena, typeSym, typeNamed, children, nil, node.productionID), true
}

func normalizeTypeScriptTypeExpression(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil {
		return nil
	}
	switch node.Type(ctx.lang) {
	case "type_identifier", "predefined_type", "union_type", "intersection_type", "object_type", "literal_type", "generic_type", "lookup_type", "template_literal_type", "conditional_type", "tuple_type", "array_type", "function_type", "constructor_type", "readonly_type", "type_query", "infer_type", "index_type_query", "nested_type_identifier":
		return node
	case "identifier":
		if ctx.hasTypeIdentifierSym {
			return newLeafNodeInArena(node.ownerArena, ctx.typeIdentifierSym, ctx.typeIdentifierNamed, node.startByte, node.endByte, node.startPoint, node.endPoint)
		}
		return node
	case "binary_expression":
		if len(node.children) != 3 || node.children[1] == nil {
			return nil
		}
		var typeSym Symbol
		var typeNamed bool
		switch node.children[1].Type(ctx.lang) {
		case "|":
			typeSym = ctx.unionTypeSym
			typeNamed = ctx.unionTypeNamed
		case "&":
			typeSym = ctx.intersectionTypeSym
			typeNamed = ctx.intersectionTypeNamed
		default:
			return nil
		}
		leftType := normalizeTypeScriptTypeExpression(node.children[0], ctx)
		rightType := normalizeTypeScriptTypeExpression(node.children[2], ctx)
		if leftType == nil || rightType == nil {
			return nil
		}
		children := cloneNodeSliceInArena(node.ownerArena, []*Node{leftType, node.children[1], rightType})
		return newParentNodeInArena(node.ownerArena, typeSym, typeNamed, children, nil, node.productionID)
	case "object":
		return rewriteTypeScriptObjectExpressionAsType(node, ctx)
	default:
		return nil
	}
}

func rewriteTypeScriptObjectExpressionAsType(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.Type(ctx.lang) != "object" {
		return nil
	}
	children := cloneNodeSliceInArena(node.ownerArena, node.children)
	changed := false
	for i, child := range children {
		if child == nil || child.Type(ctx.lang) != "pair" {
			continue
		}
		propSig := rewriteTypeScriptObjectPairAsPropertySignature(child, ctx)
		if propSig == nil {
			return nil
		}
		children[i] = propSig
		changed = true
	}
	if !changed && len(children) != 2 {
		return nil
	}
	return newParentNodeInArena(node.ownerArena, ctx.objectTypeSym, ctx.objectTypeNamed, children, nil, node.productionID)
}

func rewriteTypeScriptObjectPairAsPropertySignature(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil || node.Type(ctx.lang) != "pair" || len(node.children) < 3 {
		return nil
	}
	key := node.children[0]
	colon := node.children[1]
	value := node.children[len(node.children)-1]
	if key == nil || colon == nil || value == nil || key.Type(ctx.lang) != "property_identifier" || colon.Type(ctx.lang) != ":" {
		return nil
	}
	valueType := normalizeTypeScriptTypeExpression(value, ctx)
	if valueType == nil {
		return nil
	}
	typeAnnChildren := cloneNodeSliceInArena(node.ownerArena, []*Node{colon, valueType})
	typeAnnotation := newParentNodeInArena(node.ownerArena, ctx.typeAnnotationSym, ctx.typeAnnotationNamed, typeAnnChildren, nil, 0)
	propChildren := cloneNodeSliceInArena(node.ownerArena, []*Node{key, typeAnnotation})
	return newParentNodeInArena(node.ownerArena, ctx.propertySignatureSym, ctx.propertySignatureNamed, propChildren, nil, node.productionID)
}

func typeScriptGenericCallArgumentChildren(paren *Node, sequenceExpressionSym Symbol) []*Node {
	if paren == nil {
		return nil
	}
	if len(paren.children) != 3 || paren.children[1] == nil || paren.children[1].symbol != sequenceExpressionSym {
		return append([]*Node(nil), paren.children...)
	}
	seq := paren.children[1]
	out := make([]*Node, 0, len(seq.children)+2)
	out = append(out, paren.children[0])
	out = append(out, seq.children...)
	out = append(out, paren.children[2])
	return out
}

func normalizeTypeScriptGenericCallTypeArgument(node *Node, ctx *typeScriptNormalizationContext) *Node {
	if node == nil || ctx == nil || ctx.lang == nil {
		return nil
	}
	switch node.Type(ctx.lang) {
	case "predefined_type":
		return node
	case "type_identifier":
		if ctx.hasTypeIdentifierSym {
			return node
		}
	case "identifier":
		if typeKeywordSym, ok := typeScriptPredefinedTypeSymbol(ctx.lang, node.Text(ctx.source)); ok {
			typeKeywordNamed := symbolIsNamed(ctx.lang, typeKeywordSym)
			typeLeaf := newLeafNodeInArena(node.ownerArena, typeKeywordSym, typeKeywordNamed, node.startByte, node.endByte, node.startPoint, node.endPoint)
			return newParentNodeInArena(node.ownerArena, ctx.predefinedTypeSym, ctx.predefinedTypeNamed, []*Node{typeLeaf}, nil, 0)
		}
		if ctx.hasTypeIdentifierSym {
			typeIdentifierNamed := symbolIsNamed(ctx.lang, ctx.typeIdentifierSym)
			return newLeafNodeInArena(node.ownerArena, ctx.typeIdentifierSym, typeIdentifierNamed, node.startByte, node.endByte, node.startPoint, node.endPoint)
		}
	}
	return nil
}

func typeScriptPredefinedTypeSymbol(lang *Language, text string) (Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	switch text {
	case "any", "bigint", "boolean", "never", "number", "object", "string", "symbol", "undefined", "unknown", "void":
		return lang.SymbolByName(text)
	default:
		return 0, false
	}
}
