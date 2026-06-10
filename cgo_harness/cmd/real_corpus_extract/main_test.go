package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanCStyleCommentsSkipsStrings(t *testing.T) {
	src := []byte(`const url = "https://example.invalid//not-comment";
// TODO: real line
const s = "/* no */";
/**
 * @param {string} name
 */
`)
	comments := scanCStyleComments(src)
	if len(comments) != 2 {
		t.Fatalf("comments=%d %#v", len(comments), comments)
	}
	if got := string(cleanPlainComment(comments[0].text, comments[0].kind)); got != "TODO: real line" {
		t.Fatalf("line comment body=%q", got)
	}
	if !strings.Contains(string(comments[1].text), "@param") {
		t.Fatalf("block comment missing jsdoc content: %q", comments[1].text)
	}
}

func TestCleanPlainBlockCommentStripsDecorators(t *testing.T) {
	got := string(cleanPlainComment([]byte("/*\n * FIXME: first\n * https://example.invalid\n */"), "block"))
	want := "FIXME: first\nhttps://example.invalid"
	if got != want {
		t.Fatalf("body=%q want %q", got, want)
	}
}

func TestMarkdownInlineParagraphsStripsBlockSyntax(t *testing.T) {
	src := strings.Join([]string{
		"# Heading *with emphasis*",
		"",
		"- [x] item with [link](https://example.invalid)",
		"  continuation",
		"",
		"```",
		"*not inline corpus*",
		"```",
		"> quoted **text**",
	}, "\n")
	got := markdownInlineParagraphs(src)
	want := []string{
		"Heading *with emphasis*",
		"item with [link](https://example.invalid) continuation",
		"quoted **text**",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("paragraphs=%#v want %#v", got, want)
	}
}

func TestExtractGitRebaseHeredocsKeepsTodoLists(t *testing.T) {
	src := []byte(`cat >todo <<-EOF &&
	pick $(git log -1 --format=%h J) # J
	fixup $(git log -1 --format=%h update-refs) # fixup! J # empty
	update-ref refs/heads/topic
	EOF

cat >script <<-\EOS &&
echo "pick $(git rev-parse HEAD)" >>"$GIT_REBASE_TODO"
EOS

cat >todo2 <<'EOF'
label onto
reset onto
merge -C $(git rev-parse HEAD) onto side branch
EOF
`)
	got := extractGitRebaseHeredocs("t/t3404-rebase-interactive.sh", src)
	if len(got) != 2 {
		t.Fatalf("snippets=%d %#v", len(got), got)
	}
	if !strings.Contains(string(got[0].content), "update-ref refs/heads/topic") {
		t.Fatalf("first snippet missing update-ref: %q", got[0].content)
	}
	if !strings.Contains(string(got[1].content), "merge -C") {
		t.Fatalf("second snippet missing merge command: %q", got[1].content)
	}
}

func TestLooksLikeGitRebaseTodoRejectsMixedShell(t *testing.T) {
	if looksLikeGitRebaseTodo([]byte("pick abc123 subject\nexec make test\n")) != true {
		t.Fatalf("valid todo was rejected")
	}
	if looksLikeGitRebaseTodo([]byte("echo pick abc123\npick def456\n")) {
		t.Fatalf("mixed shell was accepted")
	}
	if looksLikeGitRebaseTodo([]byte("Revert \"topic\"\nsecond\nfirst\n")) {
		t.Fatalf("commit-log expectation was accepted")
	}
}

func TestGitRebaseCorpusSourceFileKeepsRebaseTestsOnly(t *testing.T) {
	if !gitRebaseCorpusSourceFile("t/t3430-rebase-merges.sh") {
		t.Fatalf("rebase test file was rejected")
	}
	if gitRebaseCorpusSourceFile("t/t7064-wtstatus-pv2.sh") {
		t.Fatalf("non-rebase status test file was accepted")
	}
}

func TestNormalizeSnippetBytes(t *testing.T) {
	got := string(normalizeSnippetBytes([]byte("\r\n hello\r\n")))
	if got != "hello\n" {
		t.Fatalf("normalized=%q", got)
	}
}

func TestResolveCorpusSourceRootFindsSiblingWorkspaceFromHarness(t *testing.T) {
	tmp := t.TempDir()
	harnessDir := filepath.Join(tmp, "gotreesitter", "cgo_harness")
	corpusRoot := filepath.Join(tmp, "gotreesitter-corpora", "corpus_sources")
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(corpusRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	t.Setenv("GTS_CORPUS_SOURCES_ROOT", "")
	if err := os.Chdir(harnessDir); err != nil {
		t.Fatal(err)
	}
	got, err := resolveCorpusSourceRoot("")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(corpusRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("root=%q want %q", got, want)
	}
}
