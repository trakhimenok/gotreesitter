//go:build cgo && treesitter_c_parity

package cgoharness

// THROWAWAY diagnostic: evaluate candidate C grammars (alternative repos/pins)
// against a corpus file list, counting how many files parse without errors.
// Used to pick a bump target when the pinned repo's HEAD predates the corpus
// syntax.
//
//	CAND_SPECS="move|https://github.com/x/y|<commit>|src;..." \
//	CAND_FILES=/tmp/list.txt CAND_ROOT=/corpus/move \
//	  go test . -tags treesitter_c_parity -run TestCandidateGrammarEval -v
import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCandidateGrammarEval(t *testing.T) {
	specs := os.Getenv("CAND_SPECS")
	listPath := os.Getenv("CAND_FILES")
	root := os.Getenv("CAND_ROOT")
	if specs == "" || listPath == "" || root == "" {
		t.Skip("set CAND_SPECS, CAND_FILES, CAND_ROOT")
	}

	var files []string
	lf, err := os.Open(listPath)
	if err != nil {
		t.Fatalf("open file list: %v", err)
	}
	sc := bufio.NewScanner(lf)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			files = append(files, filepath.Join(root, line))
		}
	}
	lf.Close()

	tmpRoot, err := os.MkdirTemp("", "cand-eval-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}

	for _, spec := range strings.Split(specs, ";") {
		parts := strings.Split(spec, "|")
		if len(parts) != 4 {
			t.Fatalf("bad spec %q", spec)
		}
		entry := parityLockEntry{
			Name:    parts[0],
			RepoURL: parts[1],
			Commit:  parts[2],
			Subdir:  parts[3],
		}
		label := fmt.Sprintf("%s@%s (%s)", entry.Name, entry.Commit[:12], entry.RepoURL)
		ref, err := buildParityCRef(filepath.Join(tmpRoot, entry.Commit[:12]), entry)
		if err != nil {
			t.Logf("CAND %s: BUILD FAILED: %v", label, err)
			continue
		}
		clean, errored, failed := 0, 0, 0
		for _, f := range files {
			src, rerr := os.ReadFile(f)
			if rerr != nil || len(src) == 0 {
				failed++
				continue
			}
			p := sitter.NewParser()
			_ = p.SetLanguage(ref.lang)
			tree := p.Parse(src, nil)
			if tree == nil || tree.RootNode() == nil {
				failed++
				p.Close()
				continue
			}
			if tree.RootNode().HasError() {
				errored++
			} else {
				clean++
			}
			tree.Close()
			p.Close()
		}
		t.Logf("CAND %s: clean=%d errored=%d failed=%d of %d", label, clean, errored, failed, len(files))
	}
}
