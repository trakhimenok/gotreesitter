package gotreesitter

import (
	"bytes"
	"strings"
)

func csharpRecoverSourceTopLevelTypeDeclarationFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || arena == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	keyword := ""
	declName := ""
	switch {
	case csharpHasKeywordAt(source, start, "class"):
		keyword = "class"
		declName = "class_declaration"
	case csharpHasKeywordAt(source, start, "record"):
		keyword = "record"
		declName = "record_declaration"
	default:
		return nil, false
	}
	lang := p.language
	keywordEnd := start + uint32(len(keyword))
	nameStart, nameEnd, ok := csharpScanIdentifierAt(source, csharpSkipSpaceBytes(source, keywordEnd))
	if !ok {
		return nil, false
	}
	cursor := csharpSkipSpaceBytes(source, nameEnd)
	var parameterList *Node
	if cursor < end && source[cursor] == '(' {
		closeParen, ok := csharpFindMatchingParenByte(source, cursor, end)
		if !ok {
			return nil, false
		}
		parameterList, ok = csharpBuildLambdaParameterListNode(arena, source, lang, cursor, closeParen+1)
		if !ok {
			return nil, false
		}
		cursor = csharpSkipSpaceBytes(source, closeParen+1)
	}
	bodyStart := uint32(0)
	bodyEnd := uint32(0)
	headerEnd := end
	if openBrace := csharpFindTopLevelByte(source, cursor, end, '{'); openBrace < end {
		closeBrace := findMatchingBraceByte(source, int(openBrace), int(end))
		if closeBrace < 0 || uint32(closeBrace+1) > end {
			return nil, false
		}
		bodyStart = openBrace
		bodyEnd = uint32(closeBrace)
		headerEnd = openBrace
	} else if end > start && source[end-1] == ';' {
		headerEnd = end - 1
	} else {
		return nil, false
	}
	var baseList *Node
	if colon := csharpFindTopLevelByte(source, cursor, headerEnd, ':'); colon < headerEnd {
		baseList, ok = csharpBuildSourceBaseListNode(source, colon, headerEnd, lang, arena)
		if !ok {
			return nil, false
		}
	}
	commentStart := nameEnd
	if parameterList != nil {
		commentStart = parameterList.endByte
	}
	if baseList != nil {
		commentStart = baseList.endByte
	}
	comments := csharpBuildCommentNodesBetween(source, commentStart, headerEnd, lang, arena)
	var declarationList *Node
	if bodyStart < bodyEnd {
		members, ok := csharpRecoverSourceTypeMembersFromRange(source, bodyStart+1, bodyEnd, p, arena)
		if !ok {
			return nil, false
		}
		declarationList, ok = csharpBuildSourceDeclarationListNode(source, bodyStart, bodyEnd, members, lang, arena)
		if !ok {
			return nil, false
		}
	}
	declSym, ok := symbolByName(lang, declName)
	if !ok {
		return nil, false
	}
	keywordTok, ok := csharpBuildLeafNodeByName(arena, source, lang, keyword, start, keywordEnd)
	if !ok {
		return nil, false
	}
	nameNode, ok := csharpBuildIdentifierNodeFromSource(source, nameStart, nameEnd, lang, arena)
	if !ok {
		return nil, false
	}
	children := []*Node{keywordTok, nameNode}
	if parameterList != nil {
		children = append(children, parameterList)
	}
	if baseList != nil {
		children = append(children, baseList)
	}
	children = append(children, comments...)
	if declarationList != nil {
		children = append(children, declarationList)
	}
	if bodyStart == 0 && end > start && source[end-1] == ';' {
		if semiTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ";", end-1, end); ok {
			children = append(children, semiTok)
		}
	}
	buf := arena.allocNodeSlice(len(children))
	copy(buf, children)
	named := int(declSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[declSym].Named
	decl := newParentNodeInArena(arena, declSym, named, buf, nil, 0)
	extendNodeEndTo(decl, end, source)
	decl.hasError = false
	return decl, true
}

func csharpFindTopLevelByte(source []byte, start, end uint32, want byte) uint32 {
	if end > uint32(len(source)) {
		end = uint32(len(source))
	}
	parenDepth := 0
	bracketDepth := 0
	for i := start; i < end; i++ {
		if source[i] == want && parenDepth == 0 && bracketDepth == 0 {
			return i
		}
		switch source[i] {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
	}
	return end
}

func csharpBuildSourceBaseListNode(source []byte, colon, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || arena == nil || colon >= end || int(end) > len(source) || source[colon] != ':' {
		return nil, false
	}
	sym, ok := symbolByName(lang, "base_list")
	if !ok {
		return nil, false
	}
	colonTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ":", colon, colon+1)
	if !ok {
		return nil, false
	}
	children := []*Node{colonTok}
	items := csharpSplitTopLevelByComma(source, colon+1, end)
	for i, span := range items {
		itemStart, itemEnd := csharpTrimSpaceBounds(source, span[0], span[1])
		if itemStart >= itemEnd {
			continue
		}
		node, ok := csharpBuildSourceBaseTypeNode(source, itemStart, itemEnd, lang, arena)
		if !ok {
			return nil, false
		}
		children = append(children, node)
		if i < len(items)-1 {
			commaPos := csharpFindCommaBetween(source, span[1], items[i+1][0])
			if commaPos == 0 {
				commaPos = span[1]
			}
			commaTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ",", commaPos, commaPos+1)
			if !ok {
				return nil, false
			}
			children = append(children, commaTok)
		}
	}
	buf := arena.allocNodeSlice(len(children))
	copy(buf, children)
	named := int(sym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[sym].Named
	return newParentNodeInArena(arena, sym, named, buf, nil, 0), true
}

func csharpBuildSourceBaseTypeNode(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if end > start && source[end-1] == ')' {
		openParen := csharpFindTopLevelByte(source, start, end, '(')
		if openParen < end {
			typeNode, ok := csharpBuildTypeNameNodeFromSource(arena, source, lang, start, openParen)
			if !ok {
				return nil, false
			}
			args, ok := csharpBuildArgumentListNode(arena, source, lang, openParen, end)
			if !ok {
				return nil, false
			}
			sym, ok := symbolByName(lang, "primary_constructor_base_type")
			if !ok {
				return nil, false
			}
			named := int(sym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[sym].Named
			return newParentNodeInArena(arena, sym, named, []*Node{typeNode, args}, nil, 0), true
		}
	}
	return csharpBuildTypeNameNodeFromSource(arena, source, lang, start, end)
}

func csharpBuildCommentNodesBetween(source []byte, start, end uint32, lang *Language, arena *nodeArena) []*Node {
	var comments []*Node
	for _, span := range csharpSplitLeadingTopLevelCommentSpans(source, start, end) {
		comment, ok := csharpRecoverTopLevelCommentNodeFromRange(source, span[0], span[1], lang, arena)
		if ok {
			comments = append(comments, comment)
		}
	}
	return comments
}

func csharpRecoverSourceTypeMembersFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || arena == nil || start > end || int(end) > len(source) {
		return nil, false
	}
	if bytesAreTrivia(source[start:end]) {
		return nil, true
	}
	relSpans := csharpTopLevelChunkSpans(source[start:end])
	out := make([]*Node, 0, len(relSpans))
	for _, rel := range relSpans {
		spanStart := start + rel[0]
		spanEnd := start + rel[1]
		for _, part := range csharpSplitLeadingTopLevelCommentSpans(source, spanStart, spanEnd) {
			if comment, ok := csharpRecoverTopLevelCommentNodeFromRange(source, part[0], part[1], p.language, arena); ok {
				out = append(out, comment)
				continue
			}
			method, ok := csharpRecoverClassMethodDeclarationFromRange(source, part[0], part[1], p, arena)
			if !ok {
				return nil, false
			}
			out = append(out, method)
		}
	}
	return out, true
}

func csharpRecoverClassMethodDeclarationFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || arena == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end || source[end-1] != '}' {
		return nil, false
	}
	openBrace := csharpFindTopLevelByte(source, start, end, '{')
	if openBrace >= end {
		return nil, false
	}
	closeBrace := findMatchingBraceByte(source, int(openBrace), int(end))
	if closeBrace < 0 || uint32(closeBrace+1) != end {
		return nil, false
	}
	const prefix = "class __Q { "
	const suffix = " }\n"
	wrapped := make([]byte, 0, len(prefix)+int(end-start)+len(suffix))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, source[start:end]...)
	wrapped = append(wrapped, suffix...)
	bodyStart := uint32(len(prefix)) + (openBrace - start)
	bodyEnd := uint32(len(prefix)) + (uint32(closeBrace) - start)
	for i := bodyStart + 1; i < bodyEnd; i++ {
		wrapped[i] = ' '
	}
	tree, err := p.parseForRecovery(wrapped)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()
	method := csharpExtractRecoveredWrappedClassMethod(tree.RootNode(), p.language, arena)
	if method == nil {
		return nil, false
	}
	if !shiftNodeBytes(method, int64(start)-int64(len(prefix))) {
		return nil, false
	}
	statements, ok := csharpRecoverMethodBlockStatementsFromRange(source, openBrace+1, uint32(closeBrace), p, arena)
	if !ok {
		return nil, false
	}
	block, ok := csharpBuildRecoveredMethodBlockNode(source, p.language, arena, openBrace, uint32(closeBrace), statements)
	if !ok {
		return nil, false
	}
	if !csharpReplaceMethodBlock(method, p.language, block) {
		return nil, false
	}
	recomputeNodePointsFromBytes(method, source)
	return method, true
}

func csharpBuildSourceDeclarationListNode(source []byte, openBrace, closeBrace uint32, members []*Node, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || arena == nil || openBrace >= closeBrace || int(closeBrace) >= len(source) {
		return nil, false
	}
	sym, ok := symbolByName(lang, "declaration_list")
	if !ok {
		return nil, false
	}
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "{", openBrace, openBrace+1)
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "}", closeBrace, closeBrace+1)
	if !ok {
		return nil, false
	}
	children := make([]*Node, 0, len(members)+2)
	children = append(children, openTok)
	children = append(children, members...)
	children = append(children, closeTok)
	buf := arena.allocNodeSlice(len(children))
	copy(buf, children)
	named := int(sym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[sym].Named
	return newParentNodeInArena(arena, sym, named, buf, nil, 0), true
}

func csharpRecoverNonEmptyTypeDeclarationFromError(n *Node, source []byte, p *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if n == nil || lang == nil || arena == nil || n.Type(lang) != "ERROR" || len(n.children) == 0 {
		return nil, false
	}
	return csharpRecoverNonEmptyTypeDeclarationFromChildSlice(n.children, 0, source, p, lang, arena)
}

func csharpRecoverNonEmptyTopLevelTypeDeclarationFromChildren(children []*Node, startIdx int, source []byte, p *Parser, lang *Language, arena *nodeArena) (*Node, int, bool) {
	if startIdx < 0 || startIdx >= len(children) || lang == nil || arena == nil {
		return nil, startIdx, false
	}
	recovered, ok := csharpRecoverNonEmptyTypeDeclarationFromChildSlice(children[startIdx:], 0, source, p, lang, arena)
	if !ok || recovered == nil {
		return nil, startIdx, false
	}
	nextIdx := startIdx + 1
	for nextIdx < len(children) {
		child := children[nextIdx]
		if child == nil {
			nextIdx++
			continue
		}
		if child.startByte >= recovered.endByte {
			break
		}
		nextIdx++
	}
	return recovered, nextIdx, true
}

func csharpRecoverNonEmptyTypeDeclarationFromChildSlice(children []*Node, startIdx int, source []byte, p *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if startIdx < 0 || startIdx >= len(children) || lang == nil || arena == nil {
		return nil, false
	}
	type recoverySpec struct {
		initName string
		declName string
	}
	specs := []recoverySpec{
		{initName: "_class_declaration_initializer", declName: "class_declaration"},
		{initName: "_struct_declaration_initializer", declName: "struct_declaration"},
		{initName: "_record_declaration_initializer", declName: "record_declaration"},
	}
	for _, spec := range specs {
		for _, child := range children[startIdx:] {
			if child == nil || child.Type(lang) != spec.initName {
				continue
			}
			if recovered, ok := csharpBuildRecoveredTypeDeclarationWithBodyFromChildren(children[startIdx:], child, source, p, lang, arena, spec.declName); ok {
				return recovered, true
			}
		}
	}
	return nil, false
}

func csharpBuildRecoveredTypeDeclarationWithBodyFromChildren(children []*Node, initNode *Node, source []byte, p *Parser, lang *Language, arena *nodeArena, declName string) (*Node, bool) {
	if initNode == nil || lang == nil || arena == nil || int(initNode.endByte) > len(source) {
		return nil, false
	}
	openRel := bytes.IndexByte(source[initNode.endByte:], '{')
	if openRel < 0 {
		return nil, false
	}
	openBrace := int(initNode.endByte) + openRel
	closeBrace := findMatchingBraceByte(source, openBrace, len(source))
	if closeBrace < 0 || closeBrace <= openBrace {
		return nil, false
	}
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "{", uint32(openBrace), uint32(openBrace+1))
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "}", uint32(closeBrace), uint32(closeBrace+1))
	if !ok {
		return nil, false
	}
	members, ok := csharpRecoverTypeDeclarationBodyMembers(children, initNode, source, p, lang, arena, uint32(openBrace), uint32(closeBrace))
	if !ok || len(members) == 0 {
		return nil, false
	}
	bodyChildren := make([]*Node, 0, len(members)+2)
	bodyChildren = append(bodyChildren, openTok)
	bodyChildren = append(bodyChildren, members...)
	bodyChildren = append(bodyChildren, closeTok)
	declListSym, ok := symbolByName(lang, "declaration_list")
	if !ok {
		return nil, false
	}
	declListNamed := int(declListSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[declListSym].Named
	if arena != nil {
		buf := arena.allocNodeSlice(len(bodyChildren))
		copy(buf, bodyChildren)
		bodyChildren = buf
	}
	declList := newParentNodeInArena(arena, declListSym, declListNamed, bodyChildren, nil, 0)
	declSym, ok := symbolByName(lang, declName)
	if !ok {
		return nil, false
	}
	declNamed := int(declSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[declSym].Named
	declChildren := make([]*Node, 0, len(initNode.children)+1)
	for _, child := range initNode.children {
		if child != nil {
			declChildren = append(declChildren, cloneTreeNodesIntoArena(child, arena))
		}
	}
	declChildren = append(declChildren, declList)
	if arena != nil {
		buf := arena.allocNodeSlice(len(declChildren))
		copy(buf, declChildren)
		declChildren = buf
	}
	recovered := newParentNodeInArena(arena, declSym, declNamed, declChildren, nil, 0)
	recovered.hasError = false
	extendNodeEndTo(recovered, uint32(closeBrace+1), source)
	return recovered, true
}

func csharpRecoverTypeDeclarationBodyMembers(children []*Node, initNode *Node, source []byte, p *Parser, lang *Language, arena *nodeArena, openBrace, closeBrace uint32) ([]*Node, bool) {
	if lang == nil || arena == nil || openBrace >= closeBrace {
		return nil, false
	}
	members := make([]*Node, 0, len(children))
	for i := 0; i < len(children); {
		child := children[i]
		if child == nil || child == initNode || child.endByte <= openBrace+1 || child.startByte >= closeBrace {
			i++
			continue
		}
		if recovered, next, ok := csharpRecoverMethodDeclarationFromChildren(children, i, source, p, lang, arena, closeBrace); ok {
			members = append(members, recovered)
			i = next
			continue
		}
		if child.Type(lang) == "ERROR" {
			if child.startByte <= openBrace && child.endByte <= openBrace+1 {
				i++
				continue
			}
			return nil, false
		}
		member, ok := csharpRecoverTypeDeclarationBodyChild(child, lang, arena)
		if !ok {
			return nil, false
		}
		members = append(members, member)
		i++
	}
	return members, len(members) > 0
}

func csharpRecoverTypeDeclarationBodyChild(n *Node, lang *Language, arena *nodeArena) (*Node, bool) {
	if n == nil || lang == nil || arena == nil {
		return nil, false
	}
	if n.Type(lang) == "declaration" && len(n.children) == 1 && n.children[0] != nil {
		n = n.children[0]
	}
	switch n.Type(lang) {
	case "class_declaration",
		"struct_declaration",
		"record_declaration",
		"interface_declaration",
		"enum_declaration",
		"delegate_declaration",
		"constructor_declaration",
		"destructor_declaration",
		"field_declaration",
		"method_declaration",
		"property_declaration",
		"event_declaration",
		"indexer_declaration",
		"operator_declaration",
		"conversion_operator_declaration",
		"comment":
		return cloneTreeNodesIntoArena(n, arena), true
	default:
		return nil, false
	}
}

func csharpRecoverMethodDeclarationFromChildren(children []*Node, startIdx int, source []byte, p *Parser, lang *Language, arena *nodeArena, enclosingClose uint32) (*Node, int, bool) {
	if p == nil || lang == nil || arena == nil || startIdx < 0 || startIdx >= len(children) || int(enclosingClose) > len(source) {
		return nil, startIdx, false
	}
	i := startIdx
	header := make([]*Node, 0, 4)
	for i < len(children) {
		child := children[i]
		if child == nil || child.startByte >= enclosingClose {
			break
		}
		if child.Type(lang) != "modifier" {
			break
		}
		header = append(header, child)
		i++
	}
	if i >= len(children) || children[i] == nil || children[i].Type(lang) != "type" {
		return nil, startIdx, false
	}
	returnType := children[i]
	if len(returnType.children) == 1 && returnType.children[0] != nil {
		returnType = returnType.children[0]
	}
	header = append(header, returnType)
	i++
	if i >= len(children) || children[i] == nil || children[i].Type(lang) != "identifier" {
		return nil, startIdx, false
	}
	name := children[i]
	header = append(header, name)
	i++
	if i >= len(children) || children[i] == nil || children[i].Type(lang) != "parameter_list" {
		return nil, startIdx, false
	}
	params := children[i]
	header = append(header, params)
	i++
	openBracePos := int(csharpSkipSpaceBytes(source, params.endByte))
	if openBracePos >= int(enclosingClose) || source[openBracePos] != '{' {
		return nil, startIdx, false
	}
	closeBracePos := findMatchingBraceByte(source, openBracePos, int(enclosingClose))
	if closeBracePos < 0 || closeBracePos <= openBracePos {
		return nil, startIdx, false
	}
	statements := make([]*Node, 0, 8)
	nextIdx := i
	needSourceStatementRecovery := false
	for nextIdx < len(children) {
		child := children[nextIdx]
		if child == nil {
			nextIdx++
			continue
		}
		if child.startByte >= uint32(closeBracePos+1) {
			break
		}
		if child.endByte <= uint32(openBracePos+1) {
			nextIdx++
			continue
		}
		recovered, ok := csharpRecoverMethodBlockStatementsFromNode(child, lang, arena)
		if ok {
			statements = append(statements, recovered...)
			if csharpStatementsNeedSourceRecovery(recovered, source, lang) {
				needSourceStatementRecovery = true
			}
		} else if !bytesAreTrivia(source[child.startByte:child.endByte]) {
			needSourceStatementRecovery = true
		}
		nextIdx++
	}
	if len(source) <= csharpMaxTopLevelChunkRecoverySourceBytes &&
		(needSourceStatementRecovery || len(statements) == 0 && !bytesAreTrivia(source[openBracePos+1:closeBracePos])) {
		recoveredStatements, ok := csharpRecoverMethodBlockStatementsFromRange(source, uint32(openBracePos+1), uint32(closeBracePos), p, arena)
		if !ok {
			return nil, startIdx, false
		}
		statements = recoveredStatements
	}
	if len(statements) == 0 && !bytesAreTrivia(source[openBracePos+1:closeBracePos]) {
		return nil, startIdx, false
	}
	block, ok := csharpBuildRecoveredMethodBlockNode(source, lang, arena, uint32(openBracePos), uint32(closeBracePos), statements)
	if !ok {
		return nil, startIdx, false
	}
	methodSym, ok := symbolByName(lang, "method_declaration")
	if !ok {
		return nil, startIdx, false
	}
	methodNamed := int(methodSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[methodSym].Named
	methodChildren := make([]*Node, 0, len(header)+1)
	for _, child := range header {
		if child != nil {
			methodChildren = append(methodChildren, cloneTreeNodesIntoArena(child, arena))
		}
	}
	methodChildren = append(methodChildren, block)
	if arena != nil {
		buf := arena.allocNodeSlice(len(methodChildren))
		copy(buf, methodChildren)
		methodChildren = buf
	}
	method := newParentNodeInArena(arena, methodSym, methodNamed, methodChildren, nil, 0)
	method.hasError = false
	extendNodeEndTo(method, uint32(closeBracePos+1), source)
	for nextIdx < len(children) {
		child := children[nextIdx]
		if child == nil || child.startByte >= uint32(closeBracePos+1) {
			break
		}
		nextIdx++
	}
	return method, nextIdx, true
}

func csharpRecoverMethodBlockStatementsFromNode(n *Node, lang *Language, arena *nodeArena) ([]*Node, bool) {
	if n == nil || lang == nil || arena == nil {
		return nil, false
	}
	if n.Type(lang) == "statement" {
		if len(n.children) == 1 && n.children[0] != nil {
			return csharpRecoverMethodBlockStatementsFromNode(n.children[0], lang, arena)
		}
	}
	if csharpIsRecoveredMethodBlockStatement(n, lang) {
		return []*Node{cloneTreeNodesIntoArena(n, arena)}, true
	}
	if strings.HasPrefix(n.Type(lang), "block_repeat") {
		out := make([]*Node, 0, len(n.children))
		for _, child := range n.children {
			recovered, ok := csharpRecoverMethodBlockStatementsFromNode(child, lang, arena)
			if ok {
				out = append(out, recovered...)
			}
		}
		return out, len(out) > 0
	}
	return nil, false
}

func csharpIsRecoveredMethodBlockStatement(n *Node, lang *Language) bool {
	if n == nil || lang == nil {
		return false
	}
	typ := n.Type(lang)
	return typ == "comment" || typ == "local_function_statement" || strings.HasSuffix(typ, "_statement")
}

func csharpBuildRecoveredMethodBlockNode(source []byte, lang *Language, arena *nodeArena, openBrace, closeBrace uint32, statements []*Node) (*Node, bool) {
	if lang == nil || openBrace >= closeBrace || int(closeBrace+1) > len(source) {
		return nil, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nil, false
	}
	blockNamed := int(blockSym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[blockSym].Named
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "{", openBrace, openBrace+1)
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "}", closeBrace, closeBrace+1)
	if !ok {
		return nil, false
	}
	children := make([]*Node, 0, len(statements)+2)
	children = append(children, openTok)
	children = append(children, statements...)
	children = append(children, closeTok)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	block := newParentNodeInArena(arena, blockSym, blockNamed, children, nil, 0)
	block.hasError = false
	return block, true
}
