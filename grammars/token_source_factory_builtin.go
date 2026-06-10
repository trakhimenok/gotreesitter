//go:build !grammar_subset

package grammars

func init() {
	registerTokenSourceFactory("authzed", NewAuthzedTokenSourceOrEOF)
	registerTokenSourceFactory("c", NewCTokenSourceOrEOF)
	registerTokenSourceFactory("cpp", NewCTokenSourceOrEOF)
	// Go now ships as a grammargen-compiled blob whose symbol layout differs
	// from ts2go's (auto-semi alternatives are split into `\n`/`;`/`\x00`
	// instead of a single anonymous composite, `blank_identifier` is a
	// non-terminal, etc.). The hand-tuned GoTokenSource in go_lexer.go was
	// calibrated to ts2go's layout; swapping it in for grammargen produces
	// a degraded parse even with soft-fallback lookups. The DFA lexer baked
	// into the grammargen blob parses Go cleanly on its own, so we skip the
	// custom lexer here. GoTokenSource remains usable by downstream callers
	// who carry their own ts2go Go blob via the public API.
	registerTokenSourceFactory("java", NewJavaTokenSourceOrEOF)
	registerTokenSourceFactory("json", NewJSONTokenSourceOrEOF)
	// Lua now parses via the blob's DFA lexer plus LuaExternalScanner (a
	// line-faithful port of upstream scanner.c), which matches the C oracle
	// where the hand-tuned LuaTokenSource diverged (7/40 corpus parity).
	// LuaTokenSource remains available to downstream callers via the public
	// API.
}
