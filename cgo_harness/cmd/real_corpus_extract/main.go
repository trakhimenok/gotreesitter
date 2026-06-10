package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const defaultCorpusSourceRoot = "../gotreesitter-corpora/corpus_sources"

type extractionSummary struct {
	Language       string          `json:"language"`
	Recipe         string          `json:"recipe"`
	RepoPath       string          `json:"repo_path"`
	OutputDir      string          `json:"output_dir"`
	SourceFiles    int             `json:"source_files"`
	ExtractedFiles int             `json:"extracted_files"`
	ExtractedBytes int64           `json:"extracted_bytes"`
	Commit         string          `json:"commit,omitempty"`
	Snippets       []snippetRecord `json:"snippets,omitempty"`
}

type snippetRecord struct {
	SourcePath string `json:"source_path"`
	OutputPath string `json:"output_path"`
	SHA256     string `json:"sha256"`
	Bytes      int    `json:"bytes"`
}

type snippet struct {
	sourcePath string
	sourceID   string
	content    []byte
}

func main() {
	var (
		rootRaw string
		langs   string
	)
	flag.StringVar(&rootRaw, "root", "", "root containing external per-language corpus checkouts")
	flag.StringVar(&langs, "langs", "comment,doxygen,gitcommit,jsdoc,markdown_inline", "comma-separated languages to extract")
	flag.Parse()

	root, err := resolveCorpusSourceRoot(rootRaw)
	if err != nil {
		fatalf("resolve corpus source root: %v", err)
	}
	selected := parseLangs(langs)
	if len(selected) == 0 {
		fatalf("no languages selected")
	}

	var summaries []extractionSummary
	for _, lang := range selected {
		summary, err := extractLanguage(root, lang)
		if err != nil {
			fatalf("%s: %v", lang, err)
		}
		summaries = append(summaries, summary)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summaries); err != nil {
		fatalf("write summary: %v", err)
	}
}

func parseLangs(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		field = strings.ToLower(strings.TrimSpace(field))
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

func resolveCorpusSourceRoot(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = os.Getenv("GTS_CORPUS_SOURCES_ROOT")
	}
	if raw == "" {
		for _, candidate := range []string{
			defaultCorpusSourceRoot,
			filepath.Join("..", "..", "gotreesitter-corpora", "corpus_sources"),
		} {
			if _, err := os.Stat(candidate); err == nil {
				raw = candidate
				break
			}
		}
	}
	if raw == "" {
		raw = defaultCorpusSourceRoot
	}
	if strings.HasPrefix(raw, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			raw = filepath.Join(home, strings.TrimPrefix(raw, "~/"))
		}
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func extractLanguage(root, lang string) (extractionSummary, error) {
	repo := filepath.Join(root, lang)
	if st, err := os.Stat(repo); err != nil || !st.IsDir() {
		return extractionSummary{}, fmt.Errorf("missing checkout %s", repo)
	}
	outDir := filepath.Join(repo, ".gts-extracted", lang)
	if err := os.RemoveAll(outDir); err != nil {
		return extractionSummary{}, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return extractionSummary{}, err
	}

	var snippets []snippet
	var sourceFiles int
	var err error
	recipe := lang
	switch lang {
	case "comment":
		sourceFiles, snippets, err = extractCommentCorpus(repo)
	case "doxygen":
		sourceFiles, snippets, err = extractDoxygenCorpus(repo)
	case "gitcommit":
		sourceFiles, snippets, err = extractGitCommitCorpus(repo)
	case "git_rebase":
		sourceFiles, snippets, err = extractGitRebaseCorpus(repo)
	case "jsdoc":
		sourceFiles, snippets, err = extractJSDocCorpus(repo)
	case "markdown_inline":
		sourceFiles, snippets, err = extractMarkdownInlineCorpus(repo)
	default:
		return extractionSummary{}, fmt.Errorf("no extraction recipe")
	}
	if err != nil {
		return extractionSummary{}, err
	}
	if len(snippets) == 0 {
		return extractionSummary{}, fmt.Errorf("no snippets extracted")
	}
	sort.Slice(snippets, func(i, j int) bool {
		if snippets[i].sourcePath != snippets[j].sourcePath {
			return snippets[i].sourcePath < snippets[j].sourcePath
		}
		return snippets[i].sourceID < snippets[j].sourceID
	})

	summary := extractionSummary{
		Language:    lang,
		Recipe:      recipe,
		RepoPath:    repo,
		OutputDir:   outDir,
		SourceFiles: sourceFiles,
		Commit:      gitOutput(repo, "rev-parse", "HEAD"),
	}
	for i, snip := range snippets {
		ext := extractedExtension(lang)
		name := fmt.Sprintf("%05d-%s%s", i+1, shortHash(snip.content), ext)
		outPath := filepath.Join(outDir, name)
		content := normalizeSnippetBytes(snip.content)
		if err := os.WriteFile(outPath, content, 0o644); err != nil {
			return extractionSummary{}, err
		}
		relOut, _ := filepath.Rel(repo, outPath)
		sum := sha256.Sum256(content)
		summary.ExtractedFiles++
		summary.ExtractedBytes += int64(len(content))
		summary.Snippets = append(summary.Snippets, snippetRecord{
			SourcePath: snip.sourcePath,
			OutputPath: filepath.ToSlash(relOut),
			SHA256:     hex.EncodeToString(sum[:]),
			Bytes:      len(content),
		})
	}
	if err := writeExtractionManifest(outDir, summary); err != nil {
		return extractionSummary{}, err
	}
	return summary, nil
}

func extractedExtension(lang string) string {
	switch lang {
	case "comment":
		return ".comment"
	case "doxygen":
		return ".doxygen"
	case "gitcommit":
		return ".gitcommit"
	case "git_rebase":
		return ".git-rebase-todo"
	case "jsdoc":
		return ".jsdoc"
	case "markdown_inline":
		return ".mdinline"
	default:
		return ".txt"
	}
}

func writeExtractionManifest(outDir string, summary extractionSummary) error {
	path := filepath.Join(outDir, "manifest.json")
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func normalizeSnippetBytes(src []byte) []byte {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	src = bytes.TrimSpace(src)
	if len(src) == 0 || src[len(src)-1] != '\n' {
		src = append(src, '\n')
	}
	return src
}

func shortHash(src []byte) string {
	sum := sha256.Sum256(src)
	return hex.EncodeToString(sum[:])[:12]
}

func extractCommentCorpus(repo string) (int, []snippet, error) {
	return extractCodeCommentCorpus(repo, []string{".c", ".cc", ".cpp", ".h", ".hpp", ".js", ".jsx", ".ts", ".tsx", ".rs", ".go"}, func(rel string, comments []commentBlock) []snippet {
		out := make([]snippet, 0, len(comments))
		for _, c := range comments {
			body := cleanPlainComment(c.text, c.kind)
			if len(bytes.TrimSpace(body)) < 8 {
				continue
			}
			out = append(out, snippet{sourcePath: rel, sourceID: fmt.Sprintf("%08d", c.start), content: body})
		}
		return out
	})
}

func extractJSDocCorpus(repo string) (int, []snippet, error) {
	return extractCodeCommentCorpus(repo, []string{".js", ".jsx", ".ts", ".tsx"}, func(rel string, comments []commentBlock) []snippet {
		out := make([]snippet, 0, len(comments))
		for _, c := range comments {
			trimmed := bytes.TrimSpace(c.text)
			if c.kind != "block" || !bytes.HasPrefix(trimmed, []byte("/**")) || len(trimmed) < 12 {
				continue
			}
			out = append(out, snippet{sourcePath: rel, sourceID: fmt.Sprintf("%08d", c.start), content: c.text})
		}
		return out
	})
}

func extractDoxygenCorpus(repo string) (int, []snippet, error) {
	return extractCodeCommentCorpus(repo, []string{".c", ".cc", ".cpp", ".h", ".hh", ".hpp"}, func(rel string, comments []commentBlock) []snippet {
		out := make([]snippet, 0, len(comments))
		for _, c := range comments {
			trimmed := bytes.TrimSpace(c.text)
			if c.kind != "block" || !(bytes.HasPrefix(trimmed, []byte("/**")) || bytes.HasPrefix(trimmed, []byte("/*!"))) || len(trimmed) < 12 {
				continue
			}
			out = append(out, snippet{sourcePath: rel, sourceID: fmt.Sprintf("%08d", c.start), content: c.text})
		}
		return out
	})
}

type commentBlock struct {
	start int
	kind  string
	text  []byte
}

func extractCodeCommentCorpus(repo string, exts []string, convert func(string, []commentBlock) []snippet) (int, []snippet, error) {
	files, err := walkFiles(repo, exts)
	if err != nil {
		return 0, nil, err
	}
	var out []snippet
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, nil, err
		}
		rel, _ := filepath.Rel(repo, path)
		out = append(out, convert(filepath.ToSlash(rel), scanCStyleComments(data))...)
	}
	return len(files), out, nil
}

func scanCStyleComments(src []byte) []commentBlock {
	var out []commentBlock
	for i := 0; i < len(src); {
		switch src[i] {
		case '\'', '"':
			i = skipQuoted(src, i, src[i])
		case '`':
			i = skipTemplate(src, i)
		case '/':
			if i+1 >= len(src) {
				i++
				continue
			}
			switch src[i+1] {
			case '/':
				start := i
				i += 2
				for i < len(src) && src[i] != '\n' {
					i++
				}
				out = append(out, commentBlock{start: start, kind: "line", text: append([]byte(nil), src[start:i]...)})
			case '*':
				start := i
				i += 2
				for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
					i++
				}
				if i+1 < len(src) {
					i += 2
					out = append(out, commentBlock{start: start, kind: "block", text: append([]byte(nil), src[start:i]...)})
				}
			default:
				i++
			}
		default:
			i++
		}
	}
	return out
}

func skipQuoted(src []byte, i int, quote byte) int {
	i++
	for i < len(src) {
		if src[i] == '\\' {
			i += 2
			continue
		}
		if src[i] == quote {
			return i + 1
		}
		if src[i] == '\n' && quote != '`' {
			return i + 1
		}
		i++
	}
	return i
}

func skipTemplate(src []byte, i int) int {
	i++
	for i < len(src) {
		if src[i] == '\\' {
			i += 2
			continue
		}
		if src[i] == '`' {
			return i + 1
		}
		i++
	}
	return i
}

func cleanPlainComment(raw []byte, kind string) []byte {
	text := string(raw)
	if kind == "line" {
		text = strings.TrimPrefix(text, "//")
		return []byte(strings.TrimSpace(text))
	}
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		lines[i] = strings.TrimSpace(line)
	}
	return []byte(strings.TrimSpace(strings.Join(lines, "\n")))
}

func extractMarkdownInlineCorpus(repo string) (int, []snippet, error) {
	files, err := walkFiles(repo, []string{".md", ".markdown"})
	if err != nil {
		return 0, nil, err
	}
	var out []snippet
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, nil, err
		}
		rel, _ := filepath.Rel(repo, path)
		paragraphs := markdownInlineParagraphs(string(data))
		for i, para := range paragraphs {
			out = append(out, snippet{
				sourcePath: filepath.ToSlash(rel),
				sourceID:   fmt.Sprintf("%05d", i),
				content:    []byte(para),
			})
		}
	}
	return len(files), out, nil
}

var (
	unorderedListRE = regexp.MustCompile(`^[-+*]\s+`)
	orderedListRE   = regexp.MustCompile(`^\d+[.)]\s+`)
	taskMarkerRE    = regexp.MustCompile(`^\[[ xX]\]\s+`)
	tableRuleRE     = regexp.MustCompile(`^[\s|:-]+$`)
)

func markdownInlineParagraphs(src string) []string {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var out []string
	var current []string
	inFence := false
	flush := func() {
		if len(current) == 0 {
			return
		}
		para := strings.Join(current, " ")
		para = strings.Join(strings.Fields(para), " ")
		if len(para) >= 3 {
			out = append(out, para)
		}
		current = current[:0]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			flush()
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" || tableRuleRE.MatchString(trimmed) {
			flush()
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			flush()
			continue
		}
		trimmed = stripMarkdownBlockPrefix(trimmed)
		if trimmed == "" {
			flush()
			continue
		}
		current = append(current, trimmed)
	}
	flush()
	return out
}

func stripMarkdownBlockPrefix(line string) string {
	for strings.HasPrefix(line, ">") {
		line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
	}
	for strings.HasPrefix(line, "#") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	}
	line = unorderedListRE.ReplaceAllString(line, "")
	line = orderedListRE.ReplaceAllString(line, "")
	line = taskMarkerRE.ReplaceAllString(line, "")
	return strings.TrimSpace(line)
}

func extractGitCommitCorpus(repo string) (int, []snippet, error) {
	if gitOutput(repo, "rev-parse", "--is-shallow-repository") == "true" {
		cmd := exec.Command("git", "-C", repo, "fetch", "--unshallow", "--filter=blob:none", "origin")
		if out, err := cmd.CombinedOutput(); err != nil {
			return 0, nil, fmt.Errorf("fetch git history: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	cmd := exec.Command("git", "-C", repo, "log", "--format=%H%x1f%B%x1e")
	data, err := cmd.Output()
	if err != nil {
		return 0, nil, err
	}
	records := bytes.Split(data, []byte{0x1e})
	out := make([]snippet, 0, len(records))
	for _, record := range records {
		record = bytes.TrimSpace(record)
		if len(record) == 0 {
			continue
		}
		parts := bytes.SplitN(record, []byte{0x1f}, 2)
		if len(parts) != 2 {
			continue
		}
		hash := string(bytes.TrimSpace(parts[0]))
		body := bytes.TrimSpace(parts[1])
		if len(body) < 3 {
			continue
		}
		out = append(out, snippet{
			sourcePath: "git-log:" + hash,
			sourceID:   hash,
			content:    append([]byte(nil), body...),
		})
	}
	return len(out), out, nil
}

func extractGitRebaseCorpus(repo string) (int, []snippet, error) {
	root := filepath.Join(repo, "t")
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		root = repo
	}
	files, err := walkFiles(root, []string{".sh"})
	if err != nil {
		return 0, nil, err
	}
	var out []snippet
	for _, path := range files {
		rel, _ := filepath.Rel(repo, path)
		relSlash := filepath.ToSlash(rel)
		if !gitRebaseCorpusSourceFile(relSlash) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, nil, err
		}
		out = append(out, extractGitRebaseHeredocs(relSlash, data)...)
	}
	return len(files), out, nil
}

func gitRebaseCorpusSourceFile(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.FromSlash(path)))
	return strings.Contains(base, "rebase")
}

func extractGitRebaseHeredocs(sourcePath string, src []byte) []snippet {
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	var out []snippet
	for i := 0; i < len(lines); i++ {
		marker, stripTabs, ok := shellHeredocMarker(lines[i])
		if !ok {
			continue
		}
		start := i + 1
		var body []string
		for i++; i < len(lines); i++ {
			line := lines[i]
			cmp := line
			if stripTabs {
				cmp = strings.TrimLeft(cmp, "\t")
				line = strings.TrimLeft(line, "\t")
			}
			if strings.TrimSpace(cmp) == marker {
				break
			}
			body = append(body, line)
		}
		content := []byte(strings.Join(body, "\n"))
		if !looksLikeGitRebaseTodo(content) {
			continue
		}
		out = append(out, snippet{
			sourcePath: sourcePath,
			sourceID:   fmt.Sprintf("%08d", start+1),
			content:    content,
		})
	}
	return out
}

func shellHeredocMarker(line string) (string, bool, bool) {
	idx := strings.Index(line, "<<")
	if idx < 0 {
		return "", false, false
	}
	rest := strings.TrimSpace(line[idx+2:])
	stripTabs := false
	if strings.HasPrefix(rest, "-") {
		stripTabs = true
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "-"))
	}
	if rest == "" {
		return "", false, false
	}
	if rest[0] == '\\' {
		rest = rest[1:]
	}
	if rest == "" {
		return "", false, false
	}
	if rest[0] == '\'' || rest[0] == '"' {
		quote := rest[0]
		rest = rest[1:]
		end := strings.IndexByte(rest, quote)
		if end < 0 {
			return "", false, false
		}
		rest = rest[:end]
	} else {
		rest = strings.Fields(rest)[0]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false, false
	}
	return rest, stripTabs, true
}

func looksLikeGitRebaseTodo(src []byte) bool {
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	commands := map[string]struct{}{
		"b": {}, "break": {},
		"d": {}, "drop": {},
		"e": {}, "edit": {},
		"exec": {},
		"f":    {}, "fixup": {},
		"l": {}, "label": {},
		"m": {}, "merge": {},
		"p": {}, "pick": {},
		"r": {}, "reword": {},
		"reset": {},
		"s":     {}, "squash": {},
		"t": {}, "u": {},
		"update-ref": {},
		"x":          {},
	}
	commandLines := 0
	invalidLines := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field := strings.Fields(line)[0]
		if _, ok := commands[field]; ok {
			commandLines++
			continue
		}
		invalidLines++
	}
	return commandLines > 0 && invalidLines == 0
}

func walkFiles(root string, exts []string) ([]string, error) {
	extSet := map[string]struct{}{}
	for _, ext := range exts {
		extSet[strings.ToLower(ext)] = struct{}{}
	}
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch strings.ToLower(d.Name()) {
			case ".git", ".gts-extracted", ".gradle", "bazel-bin", "bazel-out", "bazel-testlogs", "build", "dist", "node_modules", "target", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if _, ok := extSet[strings.ToLower(filepath.Ext(path))]; ok {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func gitOutput(repo string, args ...string) string {
	cmdArgs := append([]string{"-C", repo}, args...)
	out, err := exec.Command("git", cmdArgs...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "real_corpus_extract: "+format+"\n", args...)
	os.Exit(1)
}
