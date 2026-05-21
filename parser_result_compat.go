package gotreesitter

// resultCompatibilityStrut is a language-specific post-build tree rewrite.
//
// These are deliberately named as struts: they hold up parity while the parser,
// scanner, or grammargen output is not yet producing the final C tree-sitter
// shape directly. New parser work should prefer fixing the underlying runtime
// or grammar generation path. When that happens, remove the corresponding strut
// and its regression tests instead of letting this registry become permanent
// architecture.
type resultCompatibilityStrut func(resultCompatibilityContext)

type resultCompatibilityContext struct {
	root   *Node
	source []byte
	parser *Parser
	lang   *Language
}

var resultCompatibilityStrutLanguageNames = []string{
	"bash",
	"c",
	"c_sharp",
	"caddy",
	"cobol",
	"COBOL",
	"comment",
	"cooklang",
	"d",
	"dart",
	"elixir",
	"erlang",
	"fortran",
	"go",
	"haskell",
	"hcl",
	"html",
	"ini",
	"java",
	"javascript",
	"lua",
	"make",
	"nginx",
	"nim",
	"pascal",
	"perl",
	"php",
	"powershell",
	"pug",
	"python",
	"rst",
	"rust",
	"ruby",
	"scala",
	"sql",
	"svelte",
	"tsx",
	"typescript",
	"yaml",
	"zig",
}

func resultCompatibilityStrutForLanguage(name string) resultCompatibilityStrut {
	switch name {
	case "bash":
		return normalizeBashResultStrut
	case "c":
		return normalizeCResultStrut
	case "c_sharp":
		return normalizeCSharpResultStrut
	case "caddy":
		return normalizeCaddyResultStrut
	case "cobol", "COBOL":
		return normalizeCobolResultStrut
	case "comment":
		return normalizeCommentResultStrut
	case "cooklang":
		return normalizeCooklangResultStrut
	case "d":
		return normalizeDResultStrut
	case "dart":
		return normalizeDartResultStrut
	case "elixir":
		return normalizeElixirResultStrut
	case "erlang":
		return normalizeErlangResultStrut
	case "fortran":
		return normalizeFortranResultStrut
	case "go":
		return normalizeGoResultStrut
	case "haskell":
		return normalizeHaskellResultStrut
	case "hcl":
		return normalizeHCLResultStrut
	case "html":
		return normalizeHTMLResultStrut
	case "ini":
		return normalizeIniResultStrut
	case "java":
		return normalizeJavaResultStrut
	case "javascript":
		return normalizeJavaScriptResultStrut
	case "lua":
		return normalizeLuaResultStrut
	case "make":
		return normalizeMakeResultStrut
	case "nginx":
		return normalizeNginxResultStrut
	case "nim":
		return normalizeNimResultStrut
	case "pascal":
		return normalizePascalResultStrut
	case "perl":
		return normalizePerlResultStrut
	case "php":
		return normalizePHPResultStrut
	case "powershell":
		return normalizePowerShellResultStrut
	case "pug":
		return normalizePugResultStrut
	case "python":
		return normalizePythonResultStrut
	case "rst":
		return normalizeRSTResultStrut
	case "rust":
		return normalizeRustResultStrut
	case "ruby":
		return normalizeRubyResultStrut
	case "scala":
		return normalizeScalaResultStrut
	case "sql":
		return normalizeSQLResultStrut
	case "svelte":
		return normalizeSvelteResultStrut
	case "tsx", "typescript":
		return normalizeTypeScriptResultStrut
	case "yaml":
		return normalizeYAMLResultStrut
	case "zig":
		return normalizeZigResultStrut
	default:
		return nil
	}
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
	if strut := resultCompatibilityStrutForLanguage(lang.Name); strut != nil {
		strut(resultCompatibilityContext{
			root:   root,
			source: source,
			parser: p,
			lang:   lang,
		})
	}
}

func normalizeBashResultStrut(ctx resultCompatibilityContext) {
	normalizeBashProgramVariableAssignments(ctx.root, ctx.lang)
	normalizeBashGeneratedCommandAssignments(ctx.root, ctx.source, ctx.lang)
	normalizeBashCommandNameArguments(ctx.root, ctx.lang)
}

func normalizeCResultStrut(ctx resultCompatibilityContext) {
	normalizeCCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeCSharpResultStrut(ctx resultCompatibilityContext) {
	normalizeCSharpCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
}

func normalizeCaddyResultStrut(ctx resultCompatibilityContext) {
	normalizeTopLevelTrailingLineBreakSpan(ctx.root, ctx.source, ctx.lang)
}

func normalizeCobolResultStrut(ctx resultCompatibilityContext) {
	normalizeCobolCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeCommentResultStrut(ctx resultCompatibilityContext) {
	normalizeCommentTrailingExtraTrivia(ctx.root, ctx.source, ctx.lang)
}

func normalizeCooklangResultStrut(ctx resultCompatibilityContext) {
	normalizeCooklangTrailingStepTail(ctx.root, ctx.source, ctx.lang)
}

func normalizeDResultStrut(ctx resultCompatibilityContext) {
	normalizeDCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeDartResultStrut(ctx resultCompatibilityContext) {
	normalizeDartCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeElixirResultStrut(ctx resultCompatibilityContext) {
	normalizeElixirNestedCallTargetFields(ctx.root, ctx.lang)
}

func normalizeErlangResultStrut(ctx resultCompatibilityContext) {
	normalizeErlangSourceFileForms(ctx.root, ctx.lang)
}

func normalizeFortranResultStrut(ctx resultCompatibilityContext) {
	normalizeFortranStatementLineBreaks(ctx.root, ctx.source, ctx.lang)
	normalizeTopLevelTrailingLineBreakSpan(ctx.root, ctx.source, ctx.lang)
}

func normalizeGoResultStrut(ctx resultCompatibilityContext) {
	normalizeGoReturnedTreeCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
}

func normalizeHaskellResultStrut(ctx resultCompatibilityContext) {
	normalizeHaskellCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeHCLResultStrut(ctx resultCompatibilityContext) {
	normalizeHCLConfigFileRoot(ctx.root, ctx.lang)
}

func normalizeHTMLResultStrut(ctx resultCompatibilityContext) {
	normalizeHTMLCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeIniResultStrut(ctx resultCompatibilityContext) {
	normalizeIniSectionStarts(ctx.root, ctx.lang)
}

func normalizeJavaResultStrut(ctx resultCompatibilityContext) {
	normalizeJavaCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeJavaScriptResultStrut(ctx resultCompatibilityContext) {
	normalizeJavaScriptCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeLuaResultStrut(ctx resultCompatibilityContext) {
	normalizeLuaChunkLocalDeclarationFields(ctx.root, ctx.source, ctx.lang)
}

func normalizeMakeResultStrut(ctx resultCompatibilityContext) {
	normalizeMakeConditionalConsequenceFields(ctx.root, ctx.lang)
}

func normalizeNginxResultStrut(ctx resultCompatibilityContext) {
	normalizeNginxAttributeLineBreaks(ctx.root, ctx.source, ctx.lang)
}

func normalizeNimResultStrut(ctx resultCompatibilityContext) {
	normalizeNimTopLevelCallEnd(ctx.root, ctx.source, ctx.lang)
}

func normalizePascalResultStrut(ctx resultCompatibilityContext) {
	normalizePascalTopLevelProgramEnd(ctx.root, ctx.source, ctx.lang)
	normalizePascalTrailingExtraTrivia(ctx.root, ctx.source, ctx.lang)
}

func normalizePerlResultStrut(ctx resultCompatibilityContext) {
	normalizePerlCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizePHPResultStrut(ctx resultCompatibilityContext) {
	normalizePHPCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizePowerShellResultStrut(ctx resultCompatibilityContext) {
	normalizePowerShellProgramShape(ctx.root, ctx.source, ctx.lang)
}

func normalizePugResultStrut(ctx resultCompatibilityContext) {
	normalizeTopLevelTrailingLineBreakSpan(ctx.root, ctx.source, ctx.lang)
}

func normalizePythonResultStrut(ctx resultCompatibilityContext) {
	normalizePythonCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang)
}

func normalizeRSTResultStrut(ctx resultCompatibilityContext) {
	normalizeRSTTopLevelSectionEnd(ctx.root, ctx.source, ctx.lang)
}

func normalizeRustResultStrut(ctx resultCompatibilityContext) {
	normalizeRustCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
}

func normalizeRubyResultStrut(ctx resultCompatibilityContext) {
	normalizeRubyThenStarts(ctx.root, ctx.lang)
	normalizeRubyTopLevelModuleBounds(ctx.root, ctx.source, ctx.lang)
}

func normalizeScalaResultStrut(ctx resultCompatibilityContext) {
	normalizeScalaCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeSQLResultStrut(ctx resultCompatibilityContext) {
	normalizeSQLRecoveredSelectRoot(ctx.root, ctx.lang)
}

func normalizeSvelteResultStrut(ctx resultCompatibilityContext) {
	normalizeSvelteTrailingExtraTrivia(ctx.root, ctx.source, ctx.lang)
}

func normalizeTypeScriptResultStrut(ctx resultCompatibilityContext) {
	normalizeTypeScriptTreeCompatibility(ctx.root, ctx.source, ctx.lang)
}

func normalizeYAMLResultStrut(ctx resultCompatibilityContext) {
	normalizeYAMLRecoveredRoot(ctx.root, ctx.source, ctx.lang)
}

func normalizeZigResultStrut(ctx resultCompatibilityContext) {
	normalizeZigEmptyInitListFields(ctx.root, ctx.lang)
}
