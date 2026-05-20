//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type exactQueryCase struct {
	name   string
	lang   string
	source string
	query  string
}

type exactQueryMatch struct {
	PatternIndex int
	Captures     []exactQueryCapture
}

type exactQueryCapture struct {
	Name      string
	Type      string
	Named     bool
	StartByte uint32
	EndByte   uint32
	Text      string
}

func TestParityQueryExactJavaScriptEcosystemPatterns(t *testing.T) {
	cases := []exactQueryCase{
		{
			name: "top_level_comments_are_separate_matches",
			lang: "javascript",
			source: `// one
// two
const value = 1;
// three
`,
			query: `(program (comment) @comment)`,
		},
		{
			name: "anchor_after_zero_or_more_comments",
			lang: "javascript",
			source: `const one = require("one");
const two = require(
  // keep this with the dependency
  "two"
);
const three = require(
  /* leading block */
  // leading line
  "three"
);
`,
			query: `
(call_expression
  function: (identifier) @fn
  arguments: (arguments . (comment)* . (string (string_fragment) @from))
  (#eq? @fn "require"))
`,
		},
		{
			name:   "repeated_capture_grouping",
			lang:   "javascript",
			source: `const values = [alpha, beta, gamma];`,
			query:  `(array (identifier) @item)`,
		},
		{
			name: "top_level_repetition_groups_adjacent_comments",
			lang: "javascript",
			source: `// a
// b
// c

call();

// d
`,
			query: `(comment)+ @doc`,
		},
		{
			name: "parent_repetition_uses_first_contiguous_comment_run",
			lang: "javascript",
			source: `// a
// b
// c

call();

// d
`,
			query: `(program (comment)+ @doc)`,
		},
		{
			name: "parent_star_repetition_uses_first_contiguous_comment_run",
			lang: "javascript",
			source: `// a
// b
// c

call();

// d
`,
			query: `(program (comment)* @doc)`,
		},
		{
			name:   "parent_star_repetition_without_child",
			lang:   "javascript",
			source: `call();`,
			query:  `(program (comment)* @doc)`,
		},
		{
			name:   "parent_optional_repetition_without_child",
			lang:   "javascript",
			source: `call();`,
			query:  `(program (comment)? @doc)`,
		},
		{
			name: "top_level_star_repetition_groups_adjacent_comments",
			lang: "javascript",
			source: `// a
// b
// c

call();

// d
`,
			query: `(comment)* @doc`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertExactQueryParity(t, tc)
		})
	}
}

func TestParityQueryExactKeywordStringPatterns(t *testing.T) {
	cases := []exactQueryCase{
		{
			name:   "kotlin_val_keyword_alias",
			lang:   "kotlin",
			source: grammars.ParseSmokeSample("kotlin"),
			query:  `"val" @keyword`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertExactQueryParity(t, tc)
		})
	}
}

func assertExactQueryParity(t *testing.T, tc exactQueryCase) {
	t.Helper()

	src := []byte(tc.source)
	goTree, goLang, err := parseWithGo(parityCase{name: tc.lang, source: tc.source}, src, nil)
	if err != nil {
		t.Fatalf("Go parse error: %v", err)
	}
	defer releaseGoTree(goTree)

	cLang, err := ParityCLanguage(tc.lang)
	if err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("skip C reference parser: %s", skipReason)
		}
		t.Fatalf("load C parser: %v", err)
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("skip C reference parser SetLanguage: %s", skipReason)
		}
		t.Fatalf("C SetLanguage: %v", err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C parser returned nil tree")
	}
	defer cTree.Close()

	var structuralErrs []string
	compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &structuralErrs)

	goMatches := collectGoExactQueryMatches(t, goLang, goTree, tc.query, src)
	cMatches := collectCExactQueryMatches(t, cLang, cTree, tc.query, src)

	queryMatches := reflect.DeepEqual(goMatches, cMatches)
	if len(structuralErrs) == 0 && queryMatches {
		return
	}

	var structuralReport string
	if len(structuralErrs) > 0 {
		structuralReport = fmt.Sprintf("\nStructural divergence:\n%s\nGo root: %s\nC root:  %s\nGo children:\n%s\nC children:\n%s",
			firstLines(structuralErrs, 12),
			goTree.RootNode().SExpr(goLang),
			cTree.RootNode().ToSexp(),
			formatGoRootChildren(goTree.RootNode(), goLang, src),
			formatCRootChildren(cTree.RootNode(), src))
	}

	var queryReport string
	if !queryMatches {
		queryReport = fmt.Sprintf("\nQuery divergence:\nGo:\n%s\nC:\n%s",
			formatExactQueryMatches(goMatches),
			formatExactQueryMatches(cMatches))
	}

	t.Fatalf("exact query parity mismatch for %s/%s\nQuery:\n%s%s%s",
		tc.lang, tc.name,
		strings.TrimSpace(tc.query),
		structuralReport,
		queryReport)
}

func collectGoExactQueryMatches(t *testing.T, lang *gotreesitter.Language, tree *gotreesitter.Tree, queryStr string, source []byte) []exactQueryMatch {
	t.Helper()

	q, err := gotreesitter.NewQuery(queryStr, lang)
	if err != nil {
		t.Fatalf("Go NewQuery error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), lang, source)
	var matches []exactQueryMatch
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		snap := exactQueryMatch{PatternIndex: m.PatternIndex}
		for _, c := range m.Captures {
			if c.Node == nil {
				continue
			}
			snap.Captures = append(snap.Captures, exactQueryCapture{
				Name:      c.Name,
				Type:      c.Node.Type(lang),
				Named:     c.Node.IsNamed(),
				StartByte: c.Node.StartByte(),
				EndByte:   c.Node.EndByte(),
				Text:      c.Text(source),
			})
		}
		matches = append(matches, snap)
	}
	return matches
}

func collectCExactQueryMatches(t *testing.T, lang *sitter.Language, tree *sitter.Tree, queryStr string, source []byte) []exactQueryMatch {
	t.Helper()

	query, err := sitter.NewQuery(lang, queryStr)
	if err != nil {
		t.Fatalf("C NewQuery error: %v", err)
	}
	defer query.Close()

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()

	names := query.CaptureNames()
	iter := cursor.Matches(query, tree.RootNode(), source)

	var matches []exactQueryMatch
	for {
		m := iter.Next()
		if m == nil {
			break
		}
		snap := exactQueryMatch{PatternIndex: int(m.PatternIndex)}
		for _, c := range m.Captures {
			name := ""
			if int(c.Index) < len(names) {
				name = names[c.Index]
			}
			start := uint32(c.Node.StartByte())
			end := uint32(c.Node.EndByte())
			snap.Captures = append(snap.Captures, exactQueryCapture{
				Name:      name,
				Type:      c.Node.Kind(),
				Named:     c.Node.IsNamed(),
				StartByte: start,
				EndByte:   end,
				Text:      string(source[start:end]),
			})
		}
		matches = append(matches, snap)
	}
	return matches
}

func formatExactQueryMatches(matches []exactQueryMatch) string {
	if len(matches) == 0 {
		return "  <no matches>"
	}
	var b strings.Builder
	for i, m := range matches {
		fmt.Fprintf(&b, "  match[%d] pattern=%d captures=%d\n", i, m.PatternIndex, len(m.Captures))
		for j, c := range m.Captures {
			fmt.Fprintf(&b, "    capture[%d] @%s %s named=%v [%d-%d] %q\n",
				j, c.Name, c.Type, c.Named, c.StartByte, c.EndByte, c.Text)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func firstLines(lines []string, limit int) string {
	if len(lines) <= limit {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:limit], "\n") + fmt.Sprintf("\n... and %d more", len(lines)-limit)
}

func formatGoRootChildren(root *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if root == nil {
		return "  <nil>"
	}
	var b strings.Builder
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child == nil {
			fmt.Fprintf(&b, "  [%d] <nil>\n", i)
			continue
		}
		fmt.Fprintf(&b, "  [%d] %s named=%v [%d-%d] %q\n",
			i, child.Type(lang), child.IsNamed(), child.StartByte(), child.EndByte(), child.Text(source))
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatCRootChildren(root *sitter.Node, source []byte) string {
	if root == nil {
		return "  <nil>"
	}
	var b strings.Builder
	for i := uint(0); i < uint(root.ChildCount()); i++ {
		child := root.Child(i)
		if child == nil {
			fmt.Fprintf(&b, "  [%d] <nil>\n", i)
			continue
		}
		start := uint32(child.StartByte())
		end := uint32(child.EndByte())
		fmt.Fprintf(&b, "  [%d] %s named=%v [%d-%d] %q\n",
			i, child.Kind(), child.IsNamed(), start, end, string(source[start:end]))
	}
	return strings.TrimRight(b.String(), "\n")
}
