// Package taproot is the common front-end harness shared by M31 DSLs that use
// the gotreesitter runtime. It provides:
//
//   - Language: grammar generation + per-name caching (sync.Mutex + map).
//   - Walker: a bundle of *Language + source bytes with CST cursor helpers.
//   - Parse: the one-stop flow (Language → parse → error check).
//
// The error-leaf finder in Walker.SyntaxError is lifted from
// ~/work/elio/parse/tree.go (treeWalker.syntaxError): it collects every
// ERROR/MISSING leaf, picks the latest-positioned one — preferring MISSING
// nodes because they pinpoint expected-but-absent tokens — and formats either
// "expected <type>" (MISSING) or "near <text>" (ERROR).
package taproot

import (
	"fmt"
	"strings"
	"sync"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
)

// ── Language cache ────────────────────────────────────────────────────────────

type langEntry struct {
	lang *gts.Language
	err  error
}

var (
	cacheMu sync.Mutex
	cache   = map[string]langEntry{}
)

// Language generates and caches (once per name) the tree-sitter Language for a
// grammar. build is called only on a cache miss; subsequent calls for the same
// name return the cached result without calling build again.
func Language(name string, build func() *grammargen.Grammar) (*gts.Language, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	if e, ok := cache[name]; ok {
		return e.lang, e.err
	}

	lang, _, err := grammargen.GenerateLanguageAndBlob(build())
	e := langEntry{lang: lang, err: err}
	cache[name] = e
	return e.lang, e.err
}

// ── Walker ────────────────────────────────────────────────────────────────────

// Walker bundles a parse's language and source bytes with CST cursor helpers.
// Lifted from elio/parse/tree.go (treeWalker) and confirmed against
// selena/parse/parse.go (walker) and eos/syntax/cst.go (cstLowerer).
type Walker struct {
	Lang *gts.Language
	Src  []byte
}

// NewWalker constructs a Walker for the given language and source.
func NewWalker(lang *gts.Language, src []byte) *Walker {
	return &Walker{Lang: lang, Src: src}
}

// Type returns the grammar type name of n.
func (w *Walker) Type(n *gts.Node) string {
	if n == nil {
		return ""
	}
	return n.Type(w.Lang)
}

// Text returns the source text spanned by n.
func (w *Walker) Text(n *gts.Node) string {
	if n == nil {
		return ""
	}
	return n.Text(w.Src)
}

// Field returns the child of n that is bound to the named field, or nil.
func (w *Walker) Field(n *gts.Node, field string) *gts.Node {
	if n == nil {
		return nil
	}
	return n.ChildByFieldName(field, w.Lang)
}

// Pos returns the 1-based line and column where n begins.
func (w *Walker) Pos(n *gts.Node) (line, col int) {
	if n == nil {
		return 1, 1
	}
	pt := n.StartPoint()
	return int(pt.Row) + 1, int(pt.Column) + 1
}

// SyntaxError walks the subtree rooted at root to find the best error or
// missing leaf and returns a formatted error.
//
// Strategy (lifted from elio/parse/tree.go treeWalker.syntaxError):
//
//  1. Collect every leaf that is an ERROR node or a MISSING node.
//  2. Among those, prefer MISSING (which pinpoints an expected-but-absent
//     token). Among peers of the same kind, pick the latest start byte
//     (where the LR parser got stuck).
//  3. Format as "line:col: syntax error: expected <type>" for MISSING, or
//     "line:col: syntax error near <text>" for ERROR.
func (w *Walker) SyntaxError(root *gts.Node) error {
	var best *gts.Node
	bestMissing := false

	var walk func(n *gts.Node)
	walk = func(n *gts.Node) {
		bad := n.Type(w.Lang) == "ERROR" || n.IsError() || n.IsMissing()
		childBad := false
		for i := 0; i < n.ChildCount(); i++ {
			c := n.Child(i)
			if c == nil {
				continue
			}
			if c.Type(w.Lang) == "ERROR" || c.IsError() || c.IsMissing() {
				childBad = true
			}
			walk(c)
		}
		if bad && !childBad { // leaf error/missing node
			miss := n.IsMissing()
			switch {
			case best == nil:
			case miss && !bestMissing:
			case miss == bestMissing && n.StartByte() > best.StartByte():
			default:
				return
			}
			best, bestMissing = n, miss
		}
	}
	walk(root)

	if best == nil {
		return fmt.Errorf("syntax error")
	}
	pt := best.StartPoint()
	line, col := int(pt.Row)+1, int(pt.Column)+1
	if best.IsMissing() {
		return fmt.Errorf("%d:%d: syntax error: expected %s", line, col, best.Type(w.Lang))
	}
	near := strings.TrimSpace(w.Text(best))
	if len(near) > 30 {
		near = near[:30] + "…"
	}
	return fmt.Errorf("%d:%d: syntax error near %q", line, col, near)
}

// ── Parse ─────────────────────────────────────────────────────────────────────

// Parse runs the full common DSL parse flow:
//  1. Obtain (or generate+cache) the Language for name.
//  2. Parse src with a new Parser.
//  3. If the root HasError, return (root, walker, walker.SyntaxError(root)).
//  4. Otherwise return (root, walker, nil).
//
// root and walker are always non-nil when the language step and parse step
// succeed (even when a syntax error is present), so callers can inspect the
// partial tree.
func Parse(name string, build func() *grammargen.Grammar, src []byte) (*gts.Node, *Walker, error) {
	lang, err := Language(name, build)
	if err != nil {
		return nil, nil, fmt.Errorf("generate language %q: %w", name, err)
	}

	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		return nil, nil, fmt.Errorf("parse: %w", err)
	}

	root := tree.RootNode()
	w := NewWalker(lang, src)

	if root.HasError() {
		return root, w, w.SyntaxError(root)
	}
	return root, w, nil
}
