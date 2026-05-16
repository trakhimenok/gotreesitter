//go:build cgo && treesitter_c_bench

package cgoharness

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type javaLoggedToken struct {
	symbol    gotreesitter.Symbol
	name      string
	startByte uint32
	endByte   uint32
	text      string
}

type javaParseDiag struct {
	label       string
	tree        *gotreesitter.Tree
	segments    [][]javaLoggedToken
	firstIssue  *gotreesitter.Node
	issueKind   string
	issueParent *gotreesitter.Node
}

var javaLogTokenPattern = regexp.MustCompile(`^token sym=([0-9]+) start=([0-9]+) end=([0-9]+)$`)

func TestJavaCorpusTokenDiagnostics(t *testing.T) {
	files := loadJavaCorpus(t)
	if len(files) == 0 {
		t.Fatal("no java corpus files selected")
	}
	file := files[0]
	lang := grammars.JavaLanguage()

	dfa := parseJavaDiag(t, "dfa", lang, file.source, func(parser *gotreesitter.Parser) (*gotreesitter.Tree, error) {
		return parser.Parse(file.source)
	})
	defer dfa.tree.Release()

	tokenSource := parseJavaDiag(t, "token_source", lang, file.source, func(parser *gotreesitter.Parser) (*gotreesitter.Tree, error) {
		return parser.ParseWithTokenSourceFactory(file.source, func(src []byte) (gotreesitter.TokenSource, error) {
			return grammars.NewJavaTokenSource(src, lang)
		})
	})
	defer tokenSource.tree.Release()

	t.Logf("java-token-diag file=%s bytes=%d", file.path, len(file.source))
	logJavaParseDiag(t, lang, file.source, dfa)
	logJavaParseDiag(t, lang, file.source, tokenSource)
	logJavaTokenDivergence(t, file.source, lastJavaTokenSegment(dfa), lastJavaTokenSegment(tokenSource))
}

func parseJavaDiag(t *testing.T, label string, lang *gotreesitter.Language, src []byte, parse func(*gotreesitter.Parser) (*gotreesitter.Tree, error)) javaParseDiag {
	t.Helper()
	parser := gotreesitter.NewParser(lang)
	var segments [][]javaLoggedToken
	parser.SetLogger(func(kind gotreesitter.ParserLogType, msg string) {
		switch kind {
		case gotreesitter.ParserLogParse:
			if strings.HasPrefix(msg, "start ") {
				segments = append(segments, nil)
			}
		case gotreesitter.ParserLogLex:
			tok, ok := parseJavaLoggedToken(msg, lang, src)
			if !ok {
				return
			}
			if len(segments) == 0 {
				segments = append(segments, nil)
			}
			segments[len(segments)-1] = append(segments[len(segments)-1], tok)
		}
	})

	tree, err := parse(parser)
	if err != nil {
		t.Fatalf("%s parse failed: %v", label, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s parse returned nil root", label)
	}
	issue, kind, parent := firstJavaIssueNode(tree.RootNode())
	return javaParseDiag{
		label:       label,
		tree:        tree,
		segments:    segments,
		firstIssue:  issue,
		issueKind:   kind,
		issueParent: parent,
	}
}

func parseJavaLoggedToken(msg string, lang *gotreesitter.Language, src []byte) (javaLoggedToken, bool) {
	m := javaLogTokenPattern.FindStringSubmatch(msg)
	if len(m) != 4 {
		return javaLoggedToken{}, false
	}
	sym64, err := strconv.ParseUint(m[1], 10, 16)
	if err != nil {
		return javaLoggedToken{}, false
	}
	start64, err := strconv.ParseUint(m[2], 10, 32)
	if err != nil {
		return javaLoggedToken{}, false
	}
	end64, err := strconv.ParseUint(m[3], 10, 32)
	if err != nil {
		return javaLoggedToken{}, false
	}
	tok := javaLoggedToken{
		symbol:    gotreesitter.Symbol(sym64),
		startByte: uint32(start64),
		endByte:   uint32(end64),
	}
	if int(tok.symbol) < len(lang.SymbolNames) {
		tok.name = lang.SymbolNames[tok.symbol]
	} else {
		tok.name = "?"
	}
	if int(tok.startByte) <= len(src) && int(tok.endByte) <= len(src) && tok.startByte <= tok.endByte {
		tok.text = string(src[tok.startByte:tok.endByte])
	}
	return tok, true
}

func firstJavaIssueNode(root *gotreesitter.Node) (*gotreesitter.Node, string, *gotreesitter.Node) {
	type candidate struct {
		node   *gotreesitter.Node
		parent *gotreesitter.Node
		kind   string
		rank   int
		span   uint32
	}

	var best *candidate
	better := func(next candidate) bool {
		if best == nil {
			return true
		}
		if next.span != best.span {
			return next.span < best.span
		}
		if next.rank != best.rank {
			return next.rank < best.rank
		}
		return next.node.StartByte() < best.node.StartByte()
	}
	consider := func(node, parent *gotreesitter.Node, kind string, rank int) {
		span := node.EndByte() - node.StartByte()
		next := candidate{node: node, parent: parent, kind: kind, rank: rank, span: span}
		if better(next) {
			best = &next
		}
	}

	var walk func(node, parent *gotreesitter.Node)
	walk = func(node, parent *gotreesitter.Node) {
		if node == nil {
			return
		}
		if node.IsError() {
			consider(node, parent, "ERROR", 0)
		}
		if node.IsMissing() {
			consider(node, parent, "missing", 0)
		}
		for i := 0; i < node.ChildCount(); i++ {
			walk(node.Child(i), node)
		}
		if node.HasError() {
			consider(node, parent, "has_error", 1)
		}
	}
	walk(root, nil)
	if best == nil {
		return nil, "", nil
	}
	return best.node, best.kind, best.parent
}

func logJavaParseDiag(t *testing.T, lang *gotreesitter.Language, src []byte, diag javaParseDiag) {
	t.Helper()
	root := diag.tree.RootNode()
	seg := lastJavaTokenSegment(diag)
	t.Logf("%s runtime=%s has_error=%v segments=%d last_tokens=%d", diag.label, diag.tree.ParseRuntime().Summary(), root.HasError(), len(diag.segments), len(seg))
	if diag.firstIssue == nil {
		t.Logf("%s first_issue=<none>", diag.label)
		return
	}
	t.Logf(
		"%s first_issue kind=%s type=%s bytes=%d..%d parent=%s snippet=%q",
		diag.label,
		diag.issueKind,
		diag.firstIssue.Type(lang),
		diag.firstIssue.StartByte(),
		diag.firstIssue.EndByte(),
		javaNodeType(diag.issueParent, lang),
		javaSnippet(src, diag.firstIssue.StartByte(), diag.firstIssue.EndByte(), 96),
	)
	logJavaNodePath(t, diag.label+" issue_path", lang, diag.firstIssue, 10)
	logJavaSiblingWindow(t, diag.label+" issue_siblings", lang, src, diag.firstIssue, 4)
	logJavaTokenWindow(t, diag.label+" tokens_before_issue", src, seg, diag.firstIssue.StartByte(), 8)
}

func lastJavaTokenSegment(diag javaParseDiag) []javaLoggedToken {
	if len(diag.segments) == 0 {
		return nil
	}
	return diag.segments[len(diag.segments)-1]
}

func logJavaTokenDivergence(t *testing.T, src []byte, dfa, tokenSource []javaLoggedToken) {
	t.Helper()
	limit := len(dfa)
	if len(tokenSource) < limit {
		limit = len(tokenSource)
	}
	for i := 0; i < limit; i++ {
		if dfa[i].symbol == tokenSource[i].symbol && dfa[i].startByte == tokenSource[i].startByte && dfa[i].endByte == tokenSource[i].endByte {
			continue
		}
		t.Logf("java-token-divergence index=%d dfa=%s token_source=%s", i, formatJavaLoggedToken(dfa[i]), formatJavaLoggedToken(tokenSource[i]))
		logJavaTokenWindowAtIndex(t, "dfa divergence_window", src, dfa, i, 5)
		logJavaTokenWindowAtIndex(t, "token_source divergence_window", src, tokenSource, i, 5)
		return
	}
	if len(dfa) != len(tokenSource) {
		t.Logf("java-token-divergence length dfa=%d token_source=%d", len(dfa), len(tokenSource))
		return
	}
	t.Logf("java-token-divergence none tokens=%d", len(dfa))
}

func logJavaTokenWindow(t *testing.T, label string, src []byte, toks []javaLoggedToken, offset uint32, radius int) {
	t.Helper()
	idx := 0
	for idx < len(toks) && toks[idx].endByte <= offset {
		idx++
	}
	logJavaTokenWindowAtIndex(t, label, src, toks, idx, radius)
}

func logJavaTokenWindowAtIndex(t *testing.T, label string, src []byte, toks []javaLoggedToken, idx, radius int) {
	t.Helper()
	if len(toks) == 0 {
		t.Logf("%s: <no tokens>", label)
		return
	}
	start := idx - radius
	if start < 0 {
		start = 0
	}
	end := idx + radius + 1
	if end > len(toks) {
		end = len(toks)
	}
	var parts []string
	for i := start; i < end; i++ {
		marker := " "
		if i == idx {
			marker = "*"
		}
		parts = append(parts, fmt.Sprintf("%s%d:%s", marker, i, formatJavaLoggedToken(toks[i])))
	}
	t.Logf("%s: %s", label, strings.Join(parts, " | "))
}

func formatJavaLoggedToken(tok javaLoggedToken) string {
	text := strings.ReplaceAll(tok.text, "\n", `\n`)
	text = strings.ReplaceAll(text, "\t", `\t`)
	if len(text) > 48 {
		text = text[:48] + "..."
	}
	return fmt.Sprintf("%s(%d)[%d..%d]=%q", tok.name, tok.symbol, tok.startByte, tok.endByte, text)
}

func logJavaNodePath(t *testing.T, label string, lang *gotreesitter.Language, node *gotreesitter.Node, limit int) {
	t.Helper()
	if node == nil {
		t.Logf("%s: <nil>", label)
		return
	}
	if limit <= 0 {
		limit = 1
	}
	var path []string
	for cur := node; cur != nil && len(path) < limit; cur = cur.Parent() {
		path = append(path, formatJavaNodeSummary(cur, lang, nil))
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	if node.Parent() != nil && len(path) == limit {
		path = append([]string{"..."}, path...)
	}
	t.Logf("%s: %s", label, strings.Join(path, " -> "))
}

func logJavaSiblingWindow(t *testing.T, label string, lang *gotreesitter.Language, src []byte, node *gotreesitter.Node, radius int) {
	t.Helper()
	if node == nil || node.Parent() == nil {
		t.Logf("%s: <no siblings>", label)
		return
	}
	parent := node.Parent()
	idx := -1
	for i := 0; i < parent.ChildCount(); i++ {
		if parent.Child(i) == node {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Logf("%s: <sibling index unknown>", label)
		return
	}
	start := idx - radius
	if start < 0 {
		start = 0
	}
	end := idx + radius + 1
	if end > parent.ChildCount() {
		end = parent.ChildCount()
	}
	var parts []string
	for i := start; i < end; i++ {
		child := parent.Child(i)
		marker := " "
		if i == idx {
			marker = "*"
		}
		parts = append(parts, fmt.Sprintf("%s%d:%s", marker, i, formatJavaNodeSummary(child, lang, src)))
	}
	t.Logf("%s: parent=%s %s", label, formatJavaNodeSummary(parent, lang, nil), strings.Join(parts, " | "))
}

func formatJavaNodeSummary(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte) string {
	if node == nil {
		return "<nil>"
	}
	flags := ""
	if node.IsError() {
		flags += " ERROR"
	}
	if node.IsMissing() {
		flags += " missing"
	}
	if node.HasError() {
		flags += " has_error"
	}
	text := ""
	if src != nil && node.ChildCount() == 0 && node.EndByte() > node.StartByte() && int(node.EndByte()) <= len(src) {
		nodeText := string(src[node.StartByte():node.EndByte()])
		nodeText = strings.ReplaceAll(nodeText, "\n", `\n`)
		nodeText = strings.ReplaceAll(nodeText, "\t", `\t`)
		if len(nodeText) > 48 {
			nodeText = nodeText[:48] + "..."
		}
		text = "=" + strconv.Quote(nodeText)
	}
	return fmt.Sprintf("%s[%d..%d]%s%s", node.Type(lang), node.StartByte(), node.EndByte(), flags, text)
}

func javaNodeType(node *gotreesitter.Node, lang *gotreesitter.Language) string {
	if node == nil {
		return "<nil>"
	}
	return node.Type(lang)
}

func javaSnippet(src []byte, start, end uint32, radius int) string {
	if start > uint32(len(src)) || end > uint32(len(src)) || start > end {
		return ""
	}
	if radius < 0 {
		radius = 0
	}
	lo := int(start)
	if lo > radius {
		lo -= radius
	} else {
		lo = 0
	}
	hi := int(end)
	if hi-int(start) > radius {
		hi = int(start) + radius
	}
	hi += radius
	if hi > len(src) {
		hi = len(src)
	}
	s := string(src[lo:hi])
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
