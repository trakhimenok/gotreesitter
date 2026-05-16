//go:build cgo && treesitter_c_bench

package cgoharness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/smacker/go-tree-sitter"
	sitterjava "github.com/smacker/go-tree-sitter/java"
)

type javaCorpusFile struct {
	path   string
	source []byte
}

type javaParseMode string

const (
	javaParseModeDFA            javaParseMode = "dfa"
	javaParseModeTokenSource    javaParseMode = "token_source"
	javaParseModeAspectFallback javaParseMode = "aspect_fallback"
)

func loadJavaCorpus(tb testing.TB) []javaCorpusFile {
	tb.Helper()

	root := strings.TrimSpace(os.Getenv("GOT_JAVA_CORPUS_ROOT"))
	if root == "" {
		for _, candidate := range []string{"corpus_real/java", filepath.Join("cgo_harness", "corpus_real", "java")} {
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				root = candidate
				break
			}
		}
	}
	if root == "" {
		tb.Fatal("set GOT_JAVA_CORPUS_ROOT or run from the repository/cgo_harness root")
	}

	maxFiles := javaEnvInt(tb, "GOT_JAVA_CORPUS_MAX_FILES", 0)
	maxBytes := int64(javaEnvInt(tb, "GOT_JAVA_CORPUS_MAX_BYTES", 0))
	minBytes := javaEnvInt(tb, "GOT_JAVA_CORPUS_MIN_BYTES", 0)
	maxFileBytes := javaEnvInt(tb, "GOT_JAVA_CORPUS_MAX_FILE_BYTES", 0)
	order := strings.TrimSpace(os.Getenv("GOT_JAVA_CORPUS_ORDER"))
	if order == "" {
		order = "path"
	}

	var files []javaCorpusFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".gradle", "bazel-bin", "bazel-out", "bazel-testlogs", "build", "node_modules", "target":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".java" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(src) < minBytes {
			return nil
		}
		if maxFileBytes > 0 && len(src) > maxFileBytes {
			return nil
		}
		files = append(files, javaCorpusFile{path: path, source: src})
		return nil
	})
	if err != nil {
		tb.Fatalf("load java corpus %s: %v", root, err)
	}
	if len(files) == 0 {
		tb.Fatalf("no .java files under %s", root)
	}
	switch order {
	case "path":
		sort.Slice(files, func(i, j int) bool {
			return files[i].path < files[j].path
		})
	case "largest":
		sort.Slice(files, func(i, j int) bool {
			if len(files[i].source) != len(files[j].source) {
				return len(files[i].source) > len(files[j].source)
			}
			return files[i].path < files[j].path
		})
	case "smallest":
		sort.Slice(files, func(i, j int) bool {
			if len(files[i].source) != len(files[j].source) {
				return len(files[i].source) < len(files[j].source)
			}
			return files[i].path < files[j].path
		})
	default:
		tb.Fatalf("invalid GOT_JAVA_CORPUS_ORDER=%q; want path, largest, or smallest", order)
	}

	availableFiles := len(files)
	availableBytes := totalJavaCorpusBytes(files)
	selected := make([]javaCorpusFile, 0, len(files))
	var selectedBytes int64
	for _, file := range files {
		if maxFiles > 0 && len(selected) >= maxFiles {
			break
		}
		fileBytes := int64(len(file.source))
		if maxBytes > 0 && selectedBytes+fileBytes > maxBytes {
			continue
		}
		selected = append(selected, file)
		selectedBytes += fileBytes
	}
	if len(selected) == 0 {
		tb.Fatalf("java corpus filters selected no files under %s", root)
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].path < selected[j].path
	})
	tb.Logf(
		"java corpus: root=%s order=%s files=%d/%d bytes=%d/%d min_bytes=%d max_file_bytes=%d max_files=%d max_bytes=%d",
		root,
		order,
		len(selected),
		availableFiles,
		selectedBytes,
		availableBytes,
		minBytes,
		maxFileBytes,
		maxFiles,
		maxBytes,
	)
	return selected
}

func totalJavaCorpusBytes(files []javaCorpusFile) int64 {
	var total int64
	for _, file := range files {
		total += int64(len(file.source))
	}
	return total
}

func javaEnvInt(tb testing.TB, name string, fallback int) int {
	tb.Helper()
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		tb.Fatalf("invalid %s=%q", name, raw)
	}
	return n
}

func javaTimeoutSweep(tb testing.TB) []uint64 {
	tb.Helper()
	raw := strings.TrimSpace(os.Getenv("GOT_JAVA_TIMEOUT_SWEEP"))
	if raw == "" {
		raw = "100ms,500ms,2s,0"
	}
	parts := strings.Split(raw, ",")
	out := make([]uint64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		timeout, err := parseJavaTimeout(part)
		if err != nil {
			tb.Fatalf("invalid GOT_JAVA_TIMEOUT_SWEEP value %q: %v", part, err)
		}
		out = append(out, timeout)
	}
	if len(out) == 0 {
		tb.Fatal("GOT_JAVA_TIMEOUT_SWEEP produced no timeout values")
	}
	return out
}

func javaBenchTimeout(tb testing.TB) uint64 {
	tb.Helper()
	raw := strings.TrimSpace(os.Getenv("GOT_JAVA_BENCH_TIMEOUT"))
	if raw == "" {
		return 0
	}
	timeout, err := parseJavaTimeout(raw)
	if err != nil {
		tb.Fatalf("invalid GOT_JAVA_BENCH_TIMEOUT=%q: %v", raw, err)
	}
	return timeout
}

func parseJavaTimeout(raw string) (uint64, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "0", "none", "off":
		return 0, nil
	}
	if n, err := strconv.ParseUint(raw, 10, 64); err == nil {
		return n, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("duration must be non-negative")
	}
	return uint64(d / time.Microsecond), nil
}

func formatJavaTimeout(timeoutMicros uint64) string {
	if timeoutMicros == 0 {
		return "none"
	}
	return (time.Duration(timeoutMicros) * time.Microsecond).String()
}

func newCTreeSitterJavaParser(tb testing.TB) *sitter.Parser {
	tb.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(sitterjava.GetLanguage())
	return parser
}

type javaCorpusStats struct {
	files             int
	bytes             int64
	duration          time.Duration
	ok                int
	hasError          int
	incomplete        int
	stopped           int
	timeout           int
	fallback          int
	maxDuration       time.Duration
	maxFile           string
	firstIssueFile    string
	firstIssueSummary string
	firstIssueHasErr  bool
	stopReasons       map[gotreesitter.ParseStopReason]int
}

type javaParseResult struct {
	tree      *gotreesitter.Tree
	fallback  bool
	duration  time.Duration
	parseMode javaParseMode
}

func parseJavaWithMode(pool *gotreesitter.ParserPool, lang *gotreesitter.Language, mode javaParseMode, source []byte) (javaParseResult, error) {
	start := time.Now()
	switch mode {
	case javaParseModeDFA:
		tree, err := pool.Parse(source)
		return javaParseResult{tree: tree, duration: time.Since(start), parseMode: mode}, err
	case javaParseModeTokenSource:
		tree, err := pool.ParseWithTokenSourceFactory(source, func(src []byte) (gotreesitter.TokenSource, error) {
			return grammars.NewJavaTokenSource(src, lang)
		})
		return javaParseResult{tree: tree, duration: time.Since(start), parseMode: mode}, err
	case javaParseModeAspectFallback:
		tree, err := pool.Parse(source)
		if err != nil {
			return javaParseResult{tree: tree, duration: time.Since(start), parseMode: mode}, err
		}
		if tree == nil || tree.RootNode() == nil || !tree.RootNode().HasError() {
			return javaParseResult{tree: tree, duration: time.Since(start), parseMode: mode}, nil
		}
		tree.Release()
		fallbackTree, err := pool.ParseWithTokenSourceFactory(source, func(src []byte) (gotreesitter.TokenSource, error) {
			return grammars.NewJavaTokenSource(src, lang)
		})
		return javaParseResult{tree: fallbackTree, fallback: true, duration: time.Since(start), parseMode: mode}, err
	default:
		return javaParseResult{duration: time.Since(start), parseMode: mode}, fmt.Errorf("unknown java parse mode %q", mode)
	}
}

func javaParseModes(tb testing.TB) []javaParseMode {
	tb.Helper()
	raw := strings.TrimSpace(os.Getenv("GOT_JAVA_PARSE_MODES"))
	if raw == "" {
		raw = "dfa,token_source,aspect_fallback"
	}
	parts := strings.Split(raw, ",")
	modes := make([]javaParseMode, 0, len(parts))
	for _, part := range parts {
		mode := javaParseMode(strings.TrimSpace(part))
		switch mode {
		case "":
			continue
		case javaParseModeDFA, javaParseModeTokenSource, javaParseModeAspectFallback:
			modes = append(modes, mode)
		default:
			tb.Fatalf("invalid GOT_JAVA_PARSE_MODES value %q; want dfa, token_source, or aspect_fallback", part)
		}
	}
	if len(modes) == 0 {
		tb.Fatal("GOT_JAVA_PARSE_MODES produced no parse modes")
	}
	return modes
}

func runJavaCorpus(files []javaCorpusFile, mode javaParseMode, timeoutMicros uint64) (javaCorpusStats, error) {
	lang := grammars.JavaLanguage()
	pool := gotreesitter.NewParserPool(lang, gotreesitter.WithParserPoolTimeoutMicros(timeoutMicros))
	stats := javaCorpusStats{stopReasons: make(map[gotreesitter.ParseStopReason]int)}

	for _, file := range files {
		result, err := parseJavaWithMode(pool, lang, mode, file.source)
		if err != nil {
			if result.tree != nil {
				result.tree.Release()
			}
			return stats, fmt.Errorf("%s: %w", file.path, err)
		}
		tree := result.tree
		stats.files++
		stats.bytes += int64(len(file.source))
		stats.duration += result.duration
		if result.fallback {
			stats.fallback++
		}
		if result.duration > stats.maxDuration {
			stats.maxDuration = result.duration
			stats.maxFile = file.path
		}
		if tree == nil || tree.RootNode() == nil {
			stats.incomplete++
			if tree != nil {
				tree.Release()
			}
			continue
		}
		root := tree.RootNode()
		rt := tree.ParseRuntime()
		stats.stopReasons[rt.StopReason]++
		if root.HasError() {
			stats.hasError++
		}
		if tree.ParseStoppedEarly() {
			stats.stopped++
		}
		if rt.StopReason == gotreesitter.ParseStopTimeout {
			stats.timeout++
		}
		if root.EndByte() != uint32(len(file.source)) || rt.Truncated {
			stats.incomplete++
		}
		if stats.firstIssueFile == "" && (root.HasError() || tree.ParseStoppedEarly() || root.EndByte() != uint32(len(file.source)) || rt.Truncated) {
			stats.firstIssueFile = file.path
			stats.firstIssueSummary = rt.Summary()
			stats.firstIssueHasErr = root.HasError()
		}
		if !root.HasError() && !tree.ParseStoppedEarly() && root.EndByte() == uint32(len(file.source)) && !rt.Truncated {
			stats.ok++
		}
		tree.Release()
	}
	return stats, nil
}

func formatJavaStopReasons(counts map[gotreesitter.ParseStopReason]int) string {
	if len(counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(counts))
	for reason := range counts {
		keys = append(keys, string(reason))
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, key := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(key)
		sb.WriteByte('=')
		sb.WriteString(strconv.Itoa(counts[gotreesitter.ParseStopReason(key)]))
	}
	return sb.String()
}

func TestJavaCorpusTimeoutSweep(t *testing.T) {
	files := loadJavaCorpus(t)
	modes := javaParseModes(t)
	for _, timeoutMicros := range javaTimeoutSweep(t) {
		for _, mode := range modes {
			stats, err := runJavaCorpus(files, mode, timeoutMicros)
			if err != nil {
				t.Fatalf("java corpus timeout=%s mode=%s: %v", formatJavaTimeout(timeoutMicros), mode, err)
			}
			nsPerByte := float64(0)
			if stats.bytes > 0 {
				nsPerByte = float64(stats.duration.Nanoseconds()) / float64(stats.bytes)
			}
			t.Logf(
				"java-timeout-sweep timeout=%s mode=%s files=%d bytes=%d total=%s ns_per_byte=%.2f ok=%d has_error=%d incomplete=%d stopped=%d timeouts=%d fallback=%d max=%s max_file=%s stops=%s first_issue=%s first_issue_has_error=%v first_issue_runtime=%q",
				formatJavaTimeout(timeoutMicros),
				mode,
				stats.files,
				stats.bytes,
				stats.duration,
				nsPerByte,
				stats.ok,
				stats.hasError,
				stats.incomplete,
				stats.stopped,
				stats.timeout,
				stats.fallback,
				stats.maxDuration,
				stats.maxFile,
				formatJavaStopReasons(stats.stopReasons),
				stats.firstIssueFile,
				stats.firstIssueHasErr,
				stats.firstIssueSummary,
			)
		}
	}
}

func benchmarkJavaCorpusGoTreeSitter(b *testing.B, mode javaParseMode) {
	files := loadJavaCorpus(b)
	timeoutMicros := javaBenchTimeout(b)
	totalBytes := totalJavaCorpusBytes(files)
	lang := grammars.JavaLanguage()
	pool := gotreesitter.NewParserPool(lang, gotreesitter.WithParserPoolTimeoutMicros(timeoutMicros))

	b.ReportAllocs()
	b.SetBytes(totalBytes)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, file := range files {
			result, err := parseJavaWithMode(pool, lang, mode, file.source)
			if err != nil {
				if result.tree != nil {
					result.tree.Release()
				}
				b.Fatalf("%s: %v", file.path, err)
			}
			tree := result.tree
			if tree == nil || tree.RootNode() == nil {
				b.Fatalf("%s: parse returned nil root", file.path)
			}
			root := tree.RootNode()
			rt := tree.ParseRuntime()
			if root.HasError() || tree.ParseStoppedEarly() || root.EndByte() != uint32(len(file.source)) || rt.Truncated {
				tree.Release()
				b.Fatalf("%s: incomplete parse mode=%s timeout=%s has_error=%v runtime=%s", file.path, mode, formatJavaTimeout(timeoutMicros), root.HasError(), rt.Summary())
			}
			tree.Release()
		}
	}
}

func BenchmarkJavaCorpusGoTreeSitterParseDFA(b *testing.B) {
	benchmarkJavaCorpusGoTreeSitter(b, javaParseModeDFA)
}

func BenchmarkJavaCorpusGoTreeSitterParseTokenSource(b *testing.B) {
	benchmarkJavaCorpusGoTreeSitter(b, javaParseModeTokenSource)
}

func BenchmarkJavaCorpusGoTreeSitterParseAspectFallback(b *testing.B) {
	benchmarkJavaCorpusGoTreeSitter(b, javaParseModeAspectFallback)
}

func BenchmarkJavaCorpusCTreeSitterParseFull(b *testing.B) {
	files := loadJavaCorpus(b)
	totalBytes := totalJavaCorpusBytes(files)
	parser := newCTreeSitterJavaParser(b)
	defer parser.Close()

	b.ReportAllocs()
	b.SetBytes(totalBytes)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, file := range files {
			tree := parser.Parse(nil, file.source)
			root := requireCompleteCTree(b, tree, file.source, file.path)
			if root.HasError() {
				tree.Close()
				b.Fatalf("%s: cgo parse has errors", file.path)
			}
			tree.Close()
		}
	}
}
