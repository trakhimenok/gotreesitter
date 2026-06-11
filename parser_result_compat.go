package gotreesitter

type resultCompatibilityContext struct {
	root   *Node
	source []byte
	parser *Parser
	lang   *Language
}

// normalizeResultCompatibility applies narrow post-build tree rewrites that
// keep gotreesitter output aligned with C tree-sitter and existing recovery
// expectations for grammars with known normalization gaps.
func normalizeResultCompatibility(root *Node, source []byte, p *Parser) {
	var lang *Language
	if p != nil {
		lang = p.language
	}
	if root == nil || lang == nil {
		return
	}
	runLanguageResultCompatibility(resultCompatibilityContext{
		root:   root,
		source: source,
		parser: p,
		lang:   lang,
	})
	normalizeResultCollapsedNamedLeafChildren(root, lang)
}

func runLanguageResultCompatibility(ctx resultCompatibilityContext) {
	if isCobolLanguage(ctx.lang) {
		normalizeCobolCompatibility(ctx.root, ctx.source, ctx.lang)
		return
	}

	switch ctx.lang.Name {
	case "authzed":
		normalizeAuthzedCompatibility(ctx.root, ctx.source, ctx.lang)
	case "awk":
		normalizeAwkCompatibility(ctx.root, ctx.source, ctx.lang)
	case "bibtex":
		normalizeRootLeadingTriviaStart(ctx.root, ctx.source)
	case "bash":
		normalizeBashProgramVariableAssignments(ctx.root, ctx.lang)
		normalizeBashGeneratedCommandAssignments(ctx.root, ctx.source, ctx.lang)
		normalizeBashCommandNameArguments(ctx.root, ctx.lang)
	case "chatito":
		normalizeChatitoCompatibility(ctx.root, ctx.source, ctx.lang)
	case "arduino":
		normalizeArduinoBuiltinPrimitiveTypes(ctx.root, ctx.source, ctx.lang)
	case "c", "cpp":
		normalizeCCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "c_sharp":
		normalizeCSharpCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "caddy":
		normalizeTopLevelTrailingLineBreakSpan(ctx.root, ctx.source, ctx.lang)
	case "comment":
		normalizeCommentTrailingExtraTrivia(ctx.root, ctx.source, ctx.lang)
	case "cooklang":
		normalizeCooklangTrailingStepTail(ctx.root, ctx.source, ctx.lang)
	case "corn":
		normalizeCornCompatibility(ctx.root, ctx.source, ctx.lang)
	case "cpon":
		normalizeCPONCompatibility(ctx.root, ctx.source, ctx.lang)
	case "cue":
		normalizeCueCompatibility(ctx.root, ctx.source, ctx.lang)
	case "d":
		normalizeDCompatibility(ctx.root, ctx.source, ctx.lang)
	case "dart":
		normalizeDartCompatibility(ctx.root, ctx.source, ctx.lang)
	case "elixir":
		normalizeElixirCompatibility(ctx.root, ctx.source, ctx.lang)
	case "ebnf":
		normalizeEBNFCompatibility(ctx.root, ctx.source, ctx.lang)
	case "erlang":
		normalizeErlangSourceFileForms(ctx.root, ctx.lang)
	case "fortran":
		normalizeFortranStatementLineBreaks(ctx.root, ctx.source, ctx.lang)
		normalizeTopLevelTrailingLineBreakSpan(ctx.root, ctx.source, ctx.lang)
	case "forth":
		normalizeForthCompatibility(ctx.root, ctx.source, ctx.lang)
	case "fidl":
		normalizeFIDLCompatibility(ctx.root, ctx.source, ctx.lang)
	case "go":
		normalizeGoReturnedTreeCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "git_rebase":
		normalizeGitRebaseCompatibility(ctx.root, ctx.source, ctx.lang)
	case "gitcommit":
		normalizeGitcommitCompatibility(ctx.root, ctx.source, ctx.lang)
	case "hack":
		normalizeHackCompatibility(ctx.root, ctx.source, ctx.lang)
	case "haskell":
		normalizeHaskellCompatibility(ctx.root, ctx.source, ctx.lang)
	case "hcl":
		normalizeHCLConfigFileRoot(ctx.root, ctx.lang)
	case "html":
		normalizeHTMLCompatibility(ctx.root, ctx.source, ctx.lang)
	case "http":
		normalizeHTTPCompatibility(ctx.root, ctx.source, ctx.lang)
	case "ini":
		normalizeIniSectionStarts(ctx.root, ctx.lang)
	case "java":
		normalizeJavaCompatibility(ctx.root, ctx.source, ctx.lang)
	case "javascript":
		normalizeJavaScriptCompatibility(ctx.root, ctx.source, ctx.lang)
	case "kotlin":
		normalizeKotlinCompatibility(ctx.root, ctx.source, ctx.lang)
	case "lua":
		normalizeLuaChunkLocalDeclarationFields(ctx.root, ctx.source, ctx.lang)
	case "make":
		normalizeMakeConditionalConsequenceFields(ctx.root, ctx.lang)
	case "nickel":
		normalizeNickelCompatibility(ctx.root, ctx.source, ctx.lang)
	case "nginx":
		normalizeNginxAttributeLineBreaks(ctx.root, ctx.source, ctx.lang)
	case "nim":
		normalizeNimTopLevelCallEnd(ctx.root, ctx.source, ctx.lang)
	case "pascal":
		normalizePascalTopLevelProgramEnd(ctx.root, ctx.source, ctx.lang)
		normalizePascalTrailingExtraTrivia(ctx.root, ctx.source, ctx.lang)
	case "perl":
		normalizePerlCompatibility(ctx.root, ctx.source, ctx.lang)
	case "php":
		normalizePHPCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "powershell":
		normalizePowerShellProgramShape(ctx.root, ctx.source, ctx.lang)
		normalizePowerShellAssignmentOperatorTokens(ctx.root, ctx.source, ctx.lang)
	case "pug":
		normalizeTopLevelTrailingLineBreakSpan(ctx.root, ctx.source, ctx.lang)
	case "ql":
		normalizeQLCompatibility(ctx.root, ctx.source, ctx.lang)
	case "r":
		normalizeRCompatibility(ctx.root, ctx.source, ctx.lang)
	case "python":
		normalizePythonCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "rst":
		normalizeRSTTopLevelSectionEnd(ctx.root, ctx.source, ctx.lang)
	case "rust":
		normalizeRustCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "ruby":
		normalizeRubyThenStarts(ctx.root, ctx.lang)
		normalizeRubyTopLevelModuleBounds(ctx.root, ctx.source, ctx.lang)
	case "scala":
		normalizeScalaCompatibility(ctx.root, ctx.source, ctx.lang)
	case "scheme":
		normalizeSchemeCompatibility(ctx.root, ctx.source, ctx.lang)
	case "solidity":
		normalizeSolidityMemberObjectWrappers(ctx.root, ctx.lang)
	case "sql":
		normalizeSQLRecoveredSelectRoot(ctx.root, ctx.lang)
		normalizeSQLTrailingSelectListError(ctx.root, ctx.lang)
		if ctx.parser != nil && !ctx.parser.skipRecoveryReparse {
			normalizeSQLRecoveredTopLevelSelectStatements(ctx.root, ctx.source, ctx.parser, ctx.lang)
		}
		normalizeSQLSelectClauseBodyIntoFields(ctx.root, ctx.lang)
	case "squirrel":
		normalizeSquirrelCompatibility(ctx.root, ctx.source, ctx.lang)
	case "svelte":
		normalizeSvelteTrailingExtraTrivia(ctx.root, ctx.source, ctx.lang)
	case "swift":
		normalizeSwiftCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "tsx", "typescript":
		normalizeTypeScriptTreeCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "yaml":
		normalizeYAMLRecoveredRoot(ctx.root, ctx.source, ctx.lang)
	case "zig":
		normalizeZigEmptyInitListFields(ctx.root, ctx.lang)
	}
}
