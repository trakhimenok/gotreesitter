package gotreesitter

// buildResultFromGLR picks the best stack and constructs the final tree.
// Prefers accepted stacks, then highest score, then most entries. When
// accepted stacks are otherwise tied, prefer the tree that retains an
// alias-target symbol before falling back to branch order.
func (p *Parser) buildResultFromGLR(stacks []glrStack, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	if len(stacks) == 0 {
		arena.Release()
		return parseErrorTree(source, p.language)
	}
	best := 0
	for i := 1; i < len(stacks); i++ {
		if stackCompareForResultSelection(p, &stacks[i], &stacks[best]) > 0 {
			best = i
		}
	}

	selected := stacks[best]
	if len(selected.entries) > 0 {
		return p.buildResult(selected.entries, source, arena, oldTree, reuseState, linkScratch)
	}
	if selected.gss.head == nil {
		return p.buildResult(nil, source, arena, oldTree, reuseState, linkScratch)
	}
	return p.buildResultFromNodes(nodesFromGSS(selected.gss), source, arena, oldTree, reuseState, linkScratch)
}

func stackCompareForResultSelection(p *Parser, a, b *glrStack) int {
	if a.dead != b.dead {
		if a.dead {
			return -1
		}
		return 1
	}
	if a.accepted != b.accepted {
		if a.accepted {
			return 1
		}
		return -1
	}
	if aErr, bErr := stackResultErrorRank(a), stackResultErrorRank(b); aErr != bErr {
		if aErr < bErr {
			return 1
		}
		return -1
	}
	if cmp := compareAcceptedStackAliasPreference(p, *a, *b); cmp != 0 {
		return cmp
	}
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	if a.shifted != b.shifted {
		if !a.shifted {
			return 1
		}
		return -1
	}
	aDepth := a.depth()
	bDepth := b.depth()
	if aDepth != bDepth {
		if aDepth > bDepth {
			return 1
		}
		return -1
	}
	if a.byteOffset != b.byteOffset {
		if a.byteOffset > b.byteOffset {
			return 1
		}
		return -1
	}
	if a.branchOrder != b.branchOrder {
		if a.branchOrder < b.branchOrder {
			return 1
		}
		return -1
	}
	return 0
}

func stackResultErrorRank(s *glrStack) int {
	if s == nil {
		return 2
	}
	nodes := resultNodesFromStack(*s)
	if len(nodes) == 0 {
		return 0
	}
	rank := 0
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil || rank == 2 {
			return
		}
		if n.IsError() {
			rank = 2
			return
		}
		if n.HasError() && rank == 0 {
			rank = 1
		}
		for _, child := range n.children {
			walk(child)
			if rank == 2 {
				return
			}
		}
	}
	for _, node := range nodes {
		walk(node)
		if rank == 2 {
			break
		}
	}
	return rank
}

func compareAcceptedStackAliasPreference(p *Parser, a, b glrStack) int {
	if p == nil || p.language == nil {
		return 0
	}
	aNodes := resultNodesFromStack(a)
	bNodes := resultNodesFromStack(b)
	if len(aNodes) != len(bNodes) {
		return 0
	}
	for i := range aNodes {
		if cmp := compareNodeAliasPreference(p, aNodes[i], bNodes[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func resultNodesFromStack(s glrStack) []*Node {
	if len(s.entries) > 0 {
		count := 0
		for i := range s.entries {
			if s.entries[i].node != nil {
				count++
			}
		}
		if count == 0 {
			return nil
		}
		nodes := make([]*Node, 0, count)
		for i := range s.entries {
			if s.entries[i].node != nil {
				nodes = append(nodes, s.entries[i].node)
			}
		}
		return nodes
	}
	if s.gss.head == nil {
		return nil
	}
	return nodesFromGSS(s.gss)
}

func compareNodeAliasPreference(p *Parser, a, b *Node) int {
	if a == b || a == nil || b == nil {
		return 0
	}
	if a.startByte != b.startByte ||
		a.endByte != b.endByte ||
		a.isExtra != b.isExtra ||
		a.isMissing != b.isMissing ||
		len(a.children) != len(b.children) {
		return 0
	}
	if a.symbol != b.symbol {
		aType := a.Type(p.language)
		bType := b.Type(p.language)
		if aType == bType {
			for i := range a.children {
				if cmp := compareNodeAliasPreference(p, a.children[i], b.children[i]); cmp != 0 {
					return cmp
				}
			}
			return 0
		}
		aAlias := p.isAliasTargetSymbol(a.symbol)
		bAlias := p.isAliasTargetSymbol(b.symbol)
		if aAlias != bAlias {
			if aAlias {
				return 1
			}
			return -1
		}
		return 0
	}
	for i := range a.children {
		if cmp := compareNodeAliasPreference(p, a.children[i], b.children[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func (p *Parser) isAliasTargetSymbol(sym Symbol) bool {
	if p == nil || int(sym) >= len(p.aliasTargetSymbol) {
		return false
	}
	return p.aliasTargetSymbol[sym]
}

// isNamedSymbol checks whether a symbol is a named symbol.
func (p *Parser) isNamedSymbol(sym Symbol) bool {
	if int(sym) < len(p.language.SymbolMetadata) {
		return p.language.SymbolMetadata[sym].Named
	}
	return false
}

func nodesFromGSS(stack gssStack) []*Node {
	if stack.head == nil {
		return nil
	}
	count := 0
	for n := stack.head; n != nil; n = n.prev {
		if n.entry.node != nil {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	nodes := make([]*Node, count)
	i := count - 1
	for n := stack.head; n != nil; n = n.prev {
		if n.entry.node != nil {
			nodes[i] = n.entry.node
			i--
		}
	}
	return nodes
}

func filterZeroWidthExtras(nodes []*Node, arena *nodeArena) []*Node {
	if len(nodes) == 0 {
		return nodes
	}
	keep := 0
	for _, n := range nodes {
		if n == nil || !n.isExtra || n.endByte > n.startByte {
			keep++
		}
	}
	if keep == len(nodes) || keep == 0 {
		return nodes
	}
	filtered := make([]*Node, 0, keep)
	for _, n := range nodes {
		if n != nil && n.isExtra && n.endByte == n.startByte {
			continue
		}
		filtered = append(filtered, n)
	}
	if arena != nil {
		out := arena.allocNodeSlice(len(filtered))
		copy(out, filtered)
		return out
	}
	return filtered
}

// buildResult constructs the final Tree from a stack of entries.
func (p *Parser) buildResult(stack []stackEntry, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	var nodes []*Node
	for _, entry := range stack {
		if entry.node != nil {
			nodes = append(nodes, entry.node)
		}
	}
	return p.buildResultFromNodes(nodes, source, arena, oldTree, reuseState, linkScratch)
}

func (p *Parser) buildResultFromNodes(nodes []*Node, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) *Tree {
	if len(nodes) == 0 {
		arena.Release()
		if isWhitespaceOnlySource(source) {
			return NewTree(nil, source, p.language)
		}
		return parseErrorTree(source, p.language)
	}

	if arena != nil && arena.used == 0 {
		arena.Release()
		arena = nil
	}

	expectedRootSymbol := Symbol(0)
	hasExpectedRoot := false
	shouldWireParentLinks := oldTree == nil
	if p != nil && p.hasRootSymbol {
		expectedRootSymbol = p.rootSymbol
		hasExpectedRoot = true
	}
	if oldTree != nil && oldTree.RootNode() != nil {
		expectedRootSymbol = oldTree.RootNode().symbol
		hasExpectedRoot = true
	}
	if p != nil && p.language != nil && p.language.Name == "python" {
		nodes, _ = repairPythonKeywordErrorNodes(nodes, source, arena, p.language)
		nodes = collapsePythonRootFragments(nodes, arena, p.language)
	}
	if hasExpectedRoot && len(nodes) > 1 {
		nodes = flattenRootSelfFragments(nodes, arena, expectedRootSymbol)
	}
	borrowedResolved := false
	var borrowed []*nodeArena
	getBorrowed := func() []*nodeArena {
		if borrowedResolved {
			return borrowed
		}
		borrowed = reuseState.retainBorrowed(arena)
		borrowedResolved = true
		return borrowed
	}

	if len(nodes) == 1 {
		candidate := nodes[0]
		candidate = flattenInvisibleRootChildren(candidate, arena, p.language)
		candidate = repairPythonKeywordErrorNode(candidate, source, arena, p.language)
		candidate = repairPythonRootNode(candidate, arena, p.language)
		if !hasExpectedRoot || candidate.symbol == expectedRootSymbol {
			extendNodeToTrailingWhitespace(candidate, source)
			p.normalizeRootSourceStart(candidate, source)
			normalizeKnownSpanAttribution(candidate, source, p)
			if shouldWireParentLinks {
				wireParentLinksWithScratch(candidate, linkScratch)
			}
			return newTreeWithArenas(candidate, source, p.language, arena, getBorrowed())
		}

		// Incremental reuse guard: if the only stacked node doesn't match the
		// previous root symbol, synthesize an expected root wrapper instead of
		// returning a reused child as the new tree root.
		rootChildren := make([]*Node, 1)
		rootChildren[0] = candidate
		if arena != nil {
			rootChildren = arena.allocNodeSlice(1)
			rootChildren[0] = candidate
		}
		root := newParentNodeInArena(arena, expectedRootSymbol, true, rootChildren, nil, 0)
		extendNodeToTrailingWhitespace(root, source)
		p.normalizeRootSourceStart(root, source)
		normalizeKnownSpanAttribution(root, source, p)
		if shouldWireParentLinks {
			wireParentLinksWithScratch(root, linkScratch)
		}
		return newTreeWithArenas(root, source, p.language, arena, getBorrowed())
	}

	// When multiple nodes remain on the stack, check whether all but one
	// are extras (e.g. leading whitespace/comments). If so, fold the extras
	// into the real root rather than wrapping everything in an error node.
	var realRoot *Node
	var allExtras []*Node
	var extras []*Node
	for _, n := range nodes {
		if n.isExtra {
			allExtras = append(allExtras, n)
			// Ignore invisible extras and zero-width extras in final-root
			// recovery; they should not force an error wrapper or inflate root
			// child counts.
			if p != nil && p.language != nil &&
				int(n.symbol) < len(p.language.SymbolMetadata) &&
				p.language.SymbolMetadata[n.symbol].Visible &&
				n.endByte > n.startByte {
				extras = append(extras, n)
			}
		} else {
			if realRoot != nil {
				realRoot = nil // more than one non-extra -> genuine error
				break
			}
			realRoot = n
		}
	}
	if realRoot != nil {
		returnRealRoot := !hasExpectedRoot || realRoot.symbol == expectedRootSymbol
		if reuseState != nil && reuseState.reusedAny {
			realRoot = cloneNodeInArena(arena, realRoot)
			realRoot.parent = nil
			realRoot.childIndex = -1
		}
		foldExtras := returnRealRoot && len(extras) > 0
		if foldExtras {
			for _, e := range allExtras {
				if e != nil && (e.IsError() || e.HasError()) {
					foldExtras = false
					break
				}
			}
		}
		if foldExtras {
			// Fold visible extras into the real root as leading/trailing children.
			merged := make([]*Node, 0, len(extras)+len(realRoot.children))
			leadingCount := 0
			for _, e := range extras {
				if e.startByte <= realRoot.startByte {
					merged = append(merged, e)
					leadingCount++
				}
			}
			merged = append(merged, realRoot.children...)
			for _, e := range extras {
				if e.startByte > realRoot.startByte {
					merged = append(merged, e)
				}
			}
			if arena != nil {
				out := arena.allocNodeSlice(len(merged))
				copy(out, merged)
				merged = out
			}
			realRoot.children = merged
			// Keep fieldIDs aligned with children: extras have no field (0).
			if len(realRoot.fieldIDs) > 0 {
				trailingCount := len(extras) - leadingCount
				padded := make([]FieldID, leadingCount+len(realRoot.fieldIDs)+trailingCount)
				copy(padded[leadingCount:], realRoot.fieldIDs)
				realRoot.fieldIDs = padded
				if len(realRoot.fieldSources) > 0 {
					paddedSources := make([]uint8, len(padded))
					copy(paddedSources[leadingCount:], realRoot.fieldSources)
					realRoot.fieldSources = paddedSources
				}
			}
			// Extend root range to cover the extras.
			for _, e := range extras {
				if e.startByte < realRoot.startByte {
					realRoot.startByte = e.startByte
					realRoot.startPoint = e.startPoint
				}
				if e.endByte > realRoot.endByte {
					realRoot.endByte = e.endByte
					realRoot.endPoint = e.endPoint
				}
			}
		}
		// Invisible extras should still contribute to the final root byte/point range.
		if returnRealRoot {
			for _, e := range allExtras {
				if e.startByte < realRoot.startByte {
					realRoot.startByte = e.startByte
					realRoot.startPoint = e.startPoint
				}
				if e.endByte > realRoot.endByte {
					realRoot.endByte = e.endByte
					realRoot.endPoint = e.endPoint
				}
			}
		}
		realRoot = repairPythonRootNode(realRoot, arena, p.language)
		if returnRealRoot || !realRoot.hasError {
			extendNodeToTrailingWhitespace(realRoot, source)
		}
		p.normalizeRootSourceStart(realRoot, source)
		normalizeKnownSpanAttribution(realRoot, source, p)
		if returnRealRoot {
			if shouldWireParentLinks {
				wireParentLinksWithScratch(realRoot, linkScratch)
			}
			return newTreeWithArenas(realRoot, source, p.language, arena, getBorrowed())
		}
	}

	rootChildren := filterZeroWidthExtras(nodes, arena)
	rootChildren, _ = repairPythonKeywordErrorNodes(rootChildren, source, arena, p.language)
	rootSymbol := rootChildren[len(rootChildren)-1].symbol
	rootHasError := false
	for _, n := range rootChildren {
		if n != nil && (n.IsError() || n.HasError()) {
			rootHasError = true
			break
		}
	}
	if hasExpectedRoot {
		if rootHasError {
			if p != nil && p.language != nil && p.language.Name == "dart" && dartProgramChildrenLookComplete(nodes, p.language) {
				rootSymbol = expectedRootSymbol
			} else {
				rootSymbol = errorSymbol
			}
		} else {
			rootSymbol = expectedRootSymbol
		}
	}
	root := newParentNodeInArena(arena, rootSymbol, true, rootChildren, nil, 0)
	if rootHasError && !(p != nil && p.language != nil && p.language.Name == "python" && hasExpectedRoot && pythonModuleChildrenLookComplete(rootChildren, p.language)) {
		root.hasError = true
	}
	root = repairPythonRootNode(root, arena, p.language)
	extendNodeToTrailingWhitespace(root, source)
	p.normalizeRootSourceStart(root, source)
	normalizeKnownSpanAttribution(root, source, p)
	if shouldWireParentLinks {
		wireParentLinksWithScratch(root, linkScratch)
	}
	return newTreeWithArenas(root, source, p.language, arena, getBorrowed())
}

func (p *Parser) normalizeRootSourceStart(root *Node, source []byte) {
	if root == nil || root.startByte == 0 || len(source) == 0 {
		return
	}
	// Included-range parses intentionally preserve range-local root spans.
	if p != nil && len(p.included) > 0 {
		return
	}
	root.startByte = 0
	root.startPoint = Point{}
}

// maxTreeWalkDepth prevents stack overflow in recursive tree walkers when
// parsing with grammargen-produced grammars that can create pathologically deep
// hidden-node chains (e.g. Scala with >1M levels).
const maxTreeWalkDepth = 5000

// normalizeKnownSpanAttribution applies narrow compatibility fixes where
// C tree-sitter attributes trailing trivia to a grouped node but this runtime
// currently drops it during child normalization.
func normalizeKnownSpanAttribution(root *Node, source []byte, p *Parser) {
	var lang *Language
	if p != nil {
		lang = p.language
	}
	if root == nil || lang == nil {
		return
	}

	switch lang.Name {
	case "bash":
		normalizeBashProgramVariableAssignments(root, lang)
	case "c":
		normalizeCTranslationUnitRoot(root, lang)
		normalizeCPreprocessorDirectiveShapes(root, source, lang)
		normalizeCDeclarationBounds(root, source, lang)
		normalizeCBuiltinPrimitiveTypeIdentifiers(root, source, lang)
		normalizeCVariadicParameterEllipsis(root, lang)
		normalizeCSizeofUnknownTypeIdentifiers(root, source, lang)
		normalizeCCastUnknownTypeIdentifiers(root, source, lang)
		normalizeCBareTypeIdentifierExpressionStatements(root, source, lang)
		normalizeCConditionClauseAssignments(root, source, lang, p)
		normalizeCPreprocNewlineSpans(root, source, lang)
		normalizeCPointerAssignmentPrecedence(root, lang)
	case "cpp", "cuda", "arduino":
		normalizeCBareTypeIdentifierExpressionStatements(root, source, lang)
		normalizeCConditionClauseAssignments(root, source, lang, p)
	case "c_sharp":
		normalizeCSharpRecoveredTopLevelChunks(root, source, p)
		normalizeCSharpRecoveredNamespaces(root, source, lang)
		normalizeCSharpRecoveredTypeDeclarations(root, source, lang)
		normalizeCollapsedNamedLeafChildren(root, lang, "implicit_type", "var")
		normalizeCSharpUnicodeIdentifierSpans(root, source, lang)
		normalizeCSharpQueryExpressions(root, source, p)
		normalizeCSharpInvocationStatements(root, source, lang)
		normalizeCSharpDereferenceLogicalAndCasts(root, source, lang)
		normalizeCSharpConditionalIsPatternExpressions(root, lang)
		normalizeCSharpTypeConstraintKeywords(root, lang)
		normalizeCSharpSwitchTupleCasePatterns(root, lang)
	case "caddy":
		normalizeTopLevelTrailingLineBreakSpan(root, source, lang)
	case "cobol", "COBOL":
		normalizeCobolLeadingAreaStart(root, source, lang)
		normalizeCobolTopLevelDefinitionEnd(root, source, lang)
		normalizeCobolDivisionSiblingEnds(root, source, lang)
		normalizeCobolPeriodChildren(root, source, lang)
	case "comment":
		normalizeCommentTrailingExtraTrivia(root, source, lang)
	case "cooklang":
		normalizeCooklangTrailingStepTail(root, source, lang)
	case "d":
		normalizeDSourceFileLeadingTrivia(root, source, lang)
		normalizeDModuleDefinitionBounds(root, lang)
		normalizeDCallExpressionTemplateTypes(root, lang)
		normalizeDCallExpressionPropertyTypes(root, lang)
		normalizeDCallExpressionSimpleTypeCallees(root, lang)
		normalizeDVariableTypeQualifiers(root, lang)
		normalizeDVariableStorageClassWrappers(root, lang)
	case "dart":
		normalizeDartConstructorSignatureKinds(root, source, lang)
		normalizeDartSingleTypeArgumentFreeCalls(root, lang)
		normalizeDartSwitchExpressionBodyFields(root, lang)
	case "elixir":
		normalizeElixirNestedCallTargetFields(root, lang)
	case "erlang":
		normalizeErlangSourceFileForms(root, lang)
	case "fortran":
		normalizeFortranStatementLineBreaks(root, source, lang)
		normalizeTopLevelTrailingLineBreakSpan(root, source, lang)
	case "go":
		normalizeGoSourceFileRoot(root, source, p)
		normalizeGoCompatibility(root, source, lang)
		normalizeRootEOFNewlineSpan(root, source, lang)
	case "haskell":
		normalizeHaskellImportsSpan(root, source, lang)
		normalizeHaskellZeroWidthTokens(root, lang)
		normalizeHaskellRootImportField(root, lang)
		normalizeHaskellDeclarationsSpan(root, source, lang)
		normalizeHaskellLocalBindsStarts(root, source, lang)
		normalizeHaskellQuasiquoteStarts(root, source, lang)
	case "hcl":
		normalizeHCLConfigFileRoot(root, lang)
	case "html":
		normalizeHTMLRecoveredNestedCustomTags(root, lang)
		normalizeHTMLRecoveredNestedCustomTagRanges(root, source, lang)
	case "ini":
		normalizeIniSectionStarts(root, lang)
	case "javascript":
		normalizeJavaScriptProgramStart(root, lang)
		normalizeJavaScriptTypeScriptOptionalChainLeaves(root, lang)
		normalizeJavaScriptTypeScriptCallPrecedence(root, lang)
		normalizeJavaScriptTypeScriptUnaryPrecedence(root, lang)
		normalizeJavaScriptTypeScriptBinaryPrecedence(root, lang)
		normalizeJavaScriptTrailingContinueComments(root, source, lang)
		normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)
		normalizeJavaScriptTopLevelObjectLiterals(root, lang)
	case "lua":
		normalizeLuaChunkLocalDeclarationFields(root, source, lang)
	case "make":
		normalizeMakeConditionalConsequenceFields(root, lang)
	case "nginx":
		normalizeNginxAttributeLineBreaks(root, source, lang)
	case "nim":
		normalizeNimTopLevelCallEnd(root, source, lang)
	case "pascal":
		normalizePascalTopLevelProgramEnd(root, source, lang)
		normalizePascalTrailingExtraTrivia(root, source, lang)
	case "perl":
		normalizePerlJoinAssignmentLists(root, source, lang)
		normalizePerlPushExpressionLists(root, source, lang)
		normalizePerlReturnExpressionLists(root, lang)
	case "php":
		normalizePHPSingletonTypeWrappers(root, lang)
		normalizePHPStaticFunctionFragments(root, source, lang)
	case "powershell":
		normalizePowerShellProgramShape(root, source, lang)
	case "pug":
		normalizeTopLevelTrailingLineBreakSpan(root, source, lang)
	case "python":
		normalizePythonTrailingSelfCalls(root, source, lang)
		normalizePythonPrintStatements(root, source, lang)
		normalizePythonInterpolationPatterns(root, lang)
		normalizeCollapsedNamedLeafChildren(root, lang, "pass_statement", "pass")
		normalizePythonStringContinuationEscapes(root, source, lang)
	case "rst":
		normalizeRSTTopLevelSectionEnd(root, source, lang)
	case "rust":
		normalizeRustRecoveredPatternStatementsRoot(root, source, p)
		normalizeRustRecoveredFunctionItems(root, source, lang)
		normalizeRustRecoveredStructExpressionRoot(root, source, lang)
		normalizeRustDotRangeExpressions(root, source, lang)
		normalizeRustTokenBindingPatterns(root, source, lang)
		normalizeRustRecoveredTokenTrees(root, source, lang)
		normalizeRustSourceFileRoot(root, source, lang)
	case "ruby":
		normalizeRubyThenStarts(root, lang)
		normalizeRubyTopLevelModuleBounds(root, source, lang)
	case "scala":
		normalizeScalaObjectTemplateBodyFragments(root, source, lang)
		normalizeScalaTemplateBodyObjectFragments(root, source, lang)
		normalizeScalaTemplateBodyRecoveredMembers(root, source, lang)
		normalizeScalaRecoveredObjectTemplateBodies(root, source, lang)
		normalizeScalaSplitFunctionDefinitions(root, source, lang)
		normalizeScalaTopLevelClassFragments(root, source, lang)
		normalizeScalaCompilationUnitRoot(root, source, lang)
		normalizeScalaDefinitionFields(root, source, lang)
		normalizeScalaTemplateBodyFunctionAnnotations(root, source, lang)
		normalizeScalaImportPathFields(root, lang)
		normalizeScalaTemplateBodyFunctionEnds(root, source, lang)
		normalizeScalaTrailingCommentOwnership(root, source, lang)
		normalizeScalaFunctionModifierFields(root, lang)
		normalizeScalaInterpolatedStringTail(root, source, lang)
		normalizeScalaCaseClauseEnds(root, source, lang)
		normalizeRootEOFNewlineSpan(root, source, lang)
	case "sql":
		normalizeSQLRecoveredSelectRoot(root, lang)
	case "svelte":
		normalizeSvelteTrailingExtraTrivia(root, source, lang)
	case "tsx", "typescript":
		normalizeJavaScriptTypeScriptOptionalChainLeaves(root, lang)
		normalizeJavaScriptTypeScriptCallPrecedence(root, lang)
		normalizeJavaScriptTypeScriptUnaryPrecedence(root, lang)
		normalizeJavaScriptTypeScriptBinaryPrecedence(root, lang)
		normalizeTypeScriptRecoveredNamespaceRoot(root, source, lang)
		normalizeTypeScriptCompatibility(root, source, lang)
		normalizeCollapsedNamedLeafChildren(root, lang, "existential_type", "*")
	case "yaml":
		normalizeYAMLRecoveredRoot(root, source, lang)
	case "zig":
		normalizeZigEmptyInitListFields(root, lang)
	}
}
