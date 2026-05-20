//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type realCorpusBenchmarkCase struct {
	name   string
	path   string
	source []byte
	entry  grammars.LangEntry
	report grammars.ParseSupport
	goLang *gotreesitter.Language
	cLang  *sitter.Language
}

type realCorpusIncrementalCase struct {
	realCorpusBenchmarkCase
	label       string
	offset      int
	original    byte
	replacement byte
	goEdit      gotreesitter.InputEdit
	cEdit       sitter.InputEdit
}

type realCorpusRuntimeTotals struct {
	arenaBytes                        int64
	scratchBytes                      int64
	nodes                             uint64
	iterations                        uint64
	tokens                            uint64
	compactLeafMaterialized           uint64
	pendingParentMaterialized         uint64
	transientParents                  uint64
	transientChildPointers            uint64
	finalChildRefMaterializedChildren uint64
	finalChildRefSingleAccesses       uint64
	finalChildRefSingleMaterialized   uint64
	normalizationRewrites             uint64
}

type realCorpusGoIncrementalState struct {
	tc   realCorpusIncrementalCase
	src  []byte
	tree *gotreesitter.Tree
}

type realCorpusCIncrementalState struct {
	tc   realCorpusIncrementalCase
	src  []byte
	tree *sitter.Tree
}

type realCorpusGoNoEditState struct {
	tc   realCorpusBenchmarkCase
	tree *gotreesitter.Tree
}

type realCorpusCNoEditState struct {
	tc   realCorpusBenchmarkCase
	tree *sitter.Tree
}

func BenchmarkParityRealCorpusParseFull(b *testing.B) {
	for _, name := range realCorpusBenchmarkLanguages(b) {
		name := name
		b.Run(name, func(b *testing.B) {
			cases := prepareRealCorpusBenchmarkCases(b, name)
			if !realCorpusBenchmarkAllowMismatch() {
				verifyRealCorpusBenchmarkFreshParity(b, cases)
			}

			b.Run("gotreesitter", func(b *testing.B) {
				benchmarkRealCorpusGoParseFull(b, cases)
			})
			b.Run("tree-sitter-c", func(b *testing.B) {
				benchmarkRealCorpusCParseFull(b, cases)
			})
		})
	}
}

func BenchmarkParityRealCorpusParseIncrementalSingleByteEdit(b *testing.B) {
	for _, name := range realCorpusBenchmarkLanguages(b) {
		name := name
		b.Run(name, func(b *testing.B) {
			cases := prepareRealCorpusBenchmarkCases(b, name)
			if !realCorpusBenchmarkAllowMismatch() {
				verifyRealCorpusBenchmarkFreshParity(b, cases)
			}
			incrementalCases := prepareRealCorpusIncrementalCases(b, cases)

			b.Run("gotreesitter", func(b *testing.B) {
				benchmarkRealCorpusGoParseIncrementalSingleByteEdit(b, incrementalCases)
			})
			b.Run("tree-sitter-c", func(b *testing.B) {
				benchmarkRealCorpusCParseIncrementalSingleByteEdit(b, incrementalCases)
			})
		})
	}
}

func BenchmarkParityRealCorpusParseIncrementalNoEdit(b *testing.B) {
	for _, name := range realCorpusBenchmarkLanguages(b) {
		name := name
		b.Run(name, func(b *testing.B) {
			cases := prepareRealCorpusBenchmarkCases(b, name)
			if !realCorpusBenchmarkAllowMismatch() {
				verifyRealCorpusBenchmarkFreshParity(b, cases)
			}

			b.Run("gotreesitter", func(b *testing.B) {
				benchmarkRealCorpusGoParseIncrementalNoEdit(b, cases)
			})
			b.Run("tree-sitter-c", func(b *testing.B) {
				benchmarkRealCorpusCParseIncrementalNoEdit(b, cases)
			})
		})
	}
}

func realCorpusBenchmarkLanguages(b *testing.B) []string {
	b.Helper()
	raw := strings.TrimSpace(os.Getenv("GTS_REAL_CORPUS_BENCH_LANGS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("GTS_PARITY_BENCH_LANGS"))
	}
	if raw == "" || strings.EqualFold(raw, "top50") {
		b.Skip("set GTS_REAL_CORPUS_BENCH_LANGS to run real-corpus parity benchmarks")
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name != "" {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		b.Fatalf("GTS_REAL_CORPUS_BENCH_LANGS=%q did not name any languages", raw)
	}
	return out
}

func prepareRealCorpusBenchmarkCases(b *testing.B, name string) []realCorpusBenchmarkCase {
	b.Helper()
	if parityLanguageExcluded(name) {
		b.Skipf("language excluded by GTS_PARITY_SKIP_LANGS: %s", name)
	}
	if reason := paritySkipReason(name); reason != "" {
		b.Skipf("known mismatch: %s", reason)
	}
	entry, ok := parityEntriesByName[name]
	if !ok {
		b.Fatalf("missing registry entry for %q", name)
	}
	report, ok := paritySupportByName[name]
	if !ok {
		b.Fatalf("missing parse support report for %q", name)
	}
	if report.Backend == grammars.ParseBackendUnsupported {
		b.Skipf("unsupported parse backend for %q", name)
	}
	cLang, err := ParityCLanguage(name)
	if err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			b.Skipf("skip C reference parser: %s", skipReason)
		}
		b.Fatalf("load C parser: %v", err)
	}

	root := realCorpusBenchmarkRoot(b)
	langRoot := filepath.Join(root, name)
	stat, err := os.Stat(langRoot)
	if err != nil || !stat.IsDir() {
		b.Skipf("no real corpus directory for %s under %s", name, root)
	}

	files := loadRealCorpusBenchmarkFiles(b, langRoot)
	cases := make([]realCorpusBenchmarkCase, 0, len(files))
	for _, file := range files {
		cases = append(cases, realCorpusBenchmarkCase{
			name:   name,
			path:   file.path,
			source: file.source,
			entry:  entry,
			report: report,
			goLang: entry.Language(),
			cLang:  cLang,
		})
	}
	b.Logf(
		"real corpus %s: files=%d bytes=%d root=%s order=%s cases=%s",
		name,
		len(cases),
		totalRealCorpusBenchmarkBytes(cases),
		langRoot,
		realCorpusBenchmarkOrder(),
		formatRealCorpusCaseList(cases),
	)
	if realCorpusBenchmarkSkipMismatch() && !realCorpusBenchmarkAllowMismatch() {
		cases = filterRealCorpusBenchmarkFreshParity(b, cases)
	}
	if realCorpusBenchmarkAllowMismatch() {
		b.Log("GTS_REAL_CORPUS_BENCH_ALLOW_MISMATCH=1: timing selected files without structural parity precheck")
	}
	return cases
}

type realCorpusBenchmarkFile struct {
	path   string
	source []byte
}

func realCorpusBenchmarkRoot(b *testing.B) string {
	b.Helper()
	if root := strings.TrimSpace(os.Getenv("GTS_REAL_CORPUS_BENCH_ROOT")); root != "" {
		return root
	}
	for _, candidate := range []string{
		"corpus_real",
		filepath.Join("cgo_harness", "corpus_real"),
		filepath.Join("..", "cgo_harness", "corpus_real"),
	} {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	b.Fatal("set GTS_REAL_CORPUS_BENCH_ROOT or run from the repository/cgo_harness root")
	return ""
}

func loadRealCorpusBenchmarkFiles(b *testing.B, root string) []realCorpusBenchmarkFile {
	b.Helper()
	minBytes := realCorpusBenchmarkEnvInt(b, "GTS_REAL_CORPUS_BENCH_MIN_BYTES", 0)
	maxFileBytes := realCorpusBenchmarkEnvInt(b, "GTS_REAL_CORPUS_BENCH_MAX_FILE_BYTES", 0)
	var files []realCorpusBenchmarkFile
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
		files = append(files, realCorpusBenchmarkFile{path: path, source: src})
		return nil
	})
	if err != nil {
		b.Fatalf("load real corpus %s: %v", root, err)
	}
	if len(files) == 0 {
		b.Fatalf("real corpus filters selected no files under %s", root)
	}
	sortRealCorpusBenchmarkFiles(b, files)

	maxFiles := realCorpusBenchmarkEnvInt(b, "GTS_REAL_CORPUS_BENCH_MAX_FILES", 0)
	maxBytes := realCorpusBenchmarkEnvInt(b, "GTS_REAL_CORPUS_BENCH_MAX_BYTES", 0)
	selected := make([]realCorpusBenchmarkFile, 0, len(files))
	selectedBytes := 0
	for _, file := range files {
		if maxFiles > 0 && len(selected) >= maxFiles {
			break
		}
		if maxBytes > 0 && selectedBytes+len(file.source) > maxBytes {
			continue
		}
		selected = append(selected, file)
		selectedBytes += len(file.source)
	}
	if len(selected) == 0 {
		b.Fatalf("real corpus filters selected no files under %s", root)
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].path < selected[j].path
	})
	return selected
}

func sortRealCorpusBenchmarkFiles(b *testing.B, files []realCorpusBenchmarkFile) {
	b.Helper()
	switch realCorpusBenchmarkOrder() {
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
		b.Fatalf("invalid GTS_REAL_CORPUS_BENCH_ORDER=%q; want path, largest, or smallest", realCorpusBenchmarkOrder())
	}
}

func realCorpusBenchmarkOrder() string {
	order := strings.TrimSpace(os.Getenv("GTS_REAL_CORPUS_BENCH_ORDER"))
	if order == "" {
		return "path"
	}
	return order
}

func realCorpusBenchmarkSkipMismatch() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func realCorpusBenchmarkAllowMismatch() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GTS_REAL_CORPUS_BENCH_ALLOW_MISMATCH"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func realCorpusBenchmarkEnvInt(b *testing.B, name string, fallback int) int {
	b.Helper()
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		b.Fatalf("invalid %s=%q", name, raw)
	}
	return n
}

func totalRealCorpusBenchmarkBytes(cases []realCorpusBenchmarkCase) int64 {
	var total int64
	for _, tc := range cases {
		total += int64(len(tc.source))
	}
	return total
}

func verifyRealCorpusBenchmarkFreshParity(b *testing.B, cases []realCorpusBenchmarkCase) {
	b.Helper()
	for _, tc := range cases {
		errs, first := realCorpusBenchmarkFreshParityErrors(b, tc)
		if len(errs) == 0 {
			continue
		}
		b.Fatalf(
			"structural parity mismatch before benchmark for %s/%s: first=%s\n  %s",
			tc.name,
			filepath.Base(tc.path),
			formatRealCorpusDivergence(first),
			firstTop50BenchmarkLines(errs, 12),
		)
	}
}

func filterRealCorpusBenchmarkFreshParity(b *testing.B, cases []realCorpusBenchmarkCase) []realCorpusBenchmarkCase {
	b.Helper()
	out := make([]realCorpusBenchmarkCase, 0, len(cases))
	var skipped []string
	for _, tc := range cases {
		errs, first := realCorpusBenchmarkFreshParityErrors(b, tc)
		if len(errs) == 0 {
			out = append(out, tc)
			continue
		}
		skipped = append(skipped, fmt.Sprintf("%s(%s)", filepath.Base(tc.path), formatRealCorpusDivergence(first)))
	}
	if len(out) == 0 {
		b.Fatalf("GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH selected no parity-clean files; skipped=%s", strings.Join(skipped, ", "))
	}
	if len(skipped) > 0 {
		b.Logf("GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH skipped %d/%d file(s): %s", len(skipped), len(cases), strings.Join(skipped, ", "))
	}
	return out
}

func realCorpusBenchmarkFreshParityErrors(b *testing.B, tc realCorpusBenchmarkCase) ([]string, *DumpV1Divergence) {
	b.Helper()
	goParser := gotreesitter.NewParser(tc.goLang)
	goTree := parseRealCorpusGoFull(b, tc, goParser)
	cParser := newRealCorpusCParser(b, tc)
	cTree := parseRealCorpusCFull(b, cParser, tc.source)

	var errs []string
	compareNodes(goTree.RootNode(), tc.goLang, cTree.RootNode(), "root", &errs)
	first := FirstDivergenceDumpV1(goTree.RootNode(), tc.goLang, cTree.RootNode())
	releaseGoTree(goTree)
	cTree.Close()
	cParser.Close()
	return errs, first
}

func formatRealCorpusDivergence(diff *DumpV1Divergence) string {
	if diff == nil {
		return "(none)"
	}
	return fmt.Sprintf("%s %s go=%s c=%s", diff.Path, diff.Category, diff.GoValue, diff.CValue)
}

func prepareRealCorpusIncrementalCases(b *testing.B, cases []realCorpusBenchmarkCase) []realCorpusIncrementalCase {
	b.Helper()
	out := make([]realCorpusIncrementalCase, 0, len(cases))
	for _, tc := range cases {
		editCase, ok := prepareRealCorpusIncrementalCase(b, tc)
		if !ok {
			b.Logf("skip %s: no single-byte incremental edit site matched benchmark constraints", tc.path)
			continue
		}
		out = append(out, editCase)
	}
	if len(out) == 0 {
		b.Fatalf("no selected files had a single-byte incremental edit site matching benchmark constraints")
	}
	return out
}

func prepareRealCorpusIncrementalCase(b *testing.B, tc realCorpusBenchmarkCase) (realCorpusIncrementalCase, bool) {
	b.Helper()
	candidates := incrementalEditCandidates(tc.source)
	maxCandidates := realCorpusBenchmarkEnvInt(b, "GTS_REAL_CORPUS_BENCH_EDIT_CANDIDATES", 128)
	tried := 0
	for _, candidate := range candidates {
		if candidate.oldEnd != candidate.start+1 || len(candidate.replacement) != 1 {
			continue
		}
		if maxCandidates > 0 && tried >= maxCandidates {
			break
		}
		tried++
		if realCorpusBenchmarkAllowMismatch() {
			return makeRealCorpusIncrementalCase(tc, candidate), true
		}
		editCase, ok := verifyRealCorpusIncrementalCandidate(b, tc, candidate)
		if ok {
			return editCase, true
		}
	}
	return realCorpusIncrementalCase{}, false
}

func makeRealCorpusIncrementalCase(tc realCorpusBenchmarkCase, candidate incrementalEditCandidate) realCorpusIncrementalCase {
	edited := applyEditCandidate(tc.source, candidate)
	goEdit := gotreesitter.InputEdit{
		StartByte:   uint32(candidate.start),
		OldEndByte:  uint32(candidate.oldEnd),
		NewEndByte:  uint32(candidate.newEnd()),
		StartPoint:  pointAtOffset(tc.source, candidate.start),
		OldEndPoint: pointAtOffset(tc.source, candidate.oldEnd),
		NewEndPoint: pointAtOffset(edited, candidate.newEnd()),
	}
	return realCorpusIncrementalCase{
		realCorpusBenchmarkCase: tc,
		label:                   candidate.label,
		offset:                  candidate.start,
		original:                tc.source[candidate.start],
		replacement:             candidate.replacement[0],
		goEdit:                  goEdit,
		cEdit:                   realCorpusCInputEdit(goEdit),
	}
}

func verifyRealCorpusIncrementalCandidate(b *testing.B, tc realCorpusBenchmarkCase, candidate incrementalEditCandidate) (realCorpusIncrementalCase, bool) {
	b.Helper()
	editCase := makeRealCorpusIncrementalCase(tc, candidate)
	edited := applyEditCandidate(tc.source, candidate)

	goParser := gotreesitter.NewParser(tc.goLang)
	goFreshTree := parseRealCorpusGoFull(b, realCorpusCaseWithSource(tc, edited), goParser)
	defer releaseGoTree(goFreshTree)

	cParser := newRealCorpusCParser(b, tc)
	defer cParser.Close()
	cFreshTree := parseRealCorpusCFull(b, cParser, edited)
	defer cFreshTree.Close()
	var freshErrs []string
	compareNodes(goFreshTree.RootNode(), tc.goLang, cFreshTree.RootNode(), "root", &freshErrs)
	if len(freshErrs) > 0 {
		return realCorpusIncrementalCase{}, false
	}

	goOldTree := parseRealCorpusGoFull(b, tc, goParser)
	goOldTree.Edit(editCase.goEdit)
	goIncrTree := parseRealCorpusGoIncremental(b, realCorpusCaseWithSource(tc, edited), goParser, goOldTree)
	releaseGoTree(goOldTree)
	defer releaseGoTree(goIncrTree)
	var goIncrErrs []string
	compareGoNodes(goIncrTree.RootNode(), tc.goLang, goFreshTree.RootNode(), "root", &goIncrErrs)
	if len(goIncrErrs) > 0 {
		return realCorpusIncrementalCase{}, false
	}

	cOldTree := parseRealCorpusCFull(b, cParser, tc.source)
	cOldTree.Edit(&editCase.cEdit)
	cIncrTree := parseRealCorpusCIncremental(b, cParser, edited, cOldTree)
	cOldTree.Close()
	defer cIncrTree.Close()
	var cIncrErrs []string
	compareNodes(goFreshTree.RootNode(), tc.goLang, cIncrTree.RootNode(), "root", &cIncrErrs)
	if len(cIncrErrs) > 0 {
		return realCorpusIncrementalCase{}, false
	}

	return editCase, true
}

func realCorpusCaseWithSource(tc realCorpusBenchmarkCase, source []byte) realCorpusBenchmarkCase {
	tc.source = source
	return tc
}

func realCorpusCInputEdit(edit gotreesitter.InputEdit) sitter.InputEdit {
	return sitter.InputEdit{
		StartByte:      uint(edit.StartByte),
		OldEndByte:     uint(edit.OldEndByte),
		NewEndByte:     uint(edit.NewEndByte),
		StartPosition:  sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column)},
		OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column)},
		NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column)},
	}
}

func benchmarkRealCorpusGoParseFull(b *testing.B, cases []realCorpusBenchmarkCase) {
	parser := gotreesitter.NewParser(cases[0].goLang)
	b.ReportAllocs()
	b.SetBytes(totalRealCorpusBenchmarkBytes(cases))
	b.ResetTimer()

	var totals realCorpusRuntimeTotals
	for i := 0; i < b.N; i++ {
		for _, tc := range cases {
			tree := parseRealCorpusGoFull(b, tc, parser)
			totals.add(tree.ParseRuntime())
			releaseGoTree(tree)
		}
	}
	totals.report(b, cases, b.N)
}

func benchmarkRealCorpusCParseFull(b *testing.B, cases []realCorpusBenchmarkCase) {
	parser := newRealCorpusCParser(b, cases[0])
	defer parser.Close()
	b.ReportAllocs()
	b.SetBytes(totalRealCorpusBenchmarkBytes(cases))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, tc := range cases {
			tree := parseRealCorpusCFull(b, parser, tc.source)
			tree.Close()
		}
	}
	reportRealCorpusCaseMetrics(b, cases)
}

func benchmarkRealCorpusGoParseIncrementalSingleByteEdit(b *testing.B, cases []realCorpusIncrementalCase) {
	parser := gotreesitter.NewParser(cases[0].goLang)
	states := make([]realCorpusGoIncrementalState, 0, len(cases))
	for _, tc := range cases {
		tree := parseRealCorpusGoFull(b, tc.realCorpusBenchmarkCase, parser)
		states = append(states, realCorpusGoIncrementalState{
			tc:   tc,
			src:  append([]byte(nil), tc.source...),
			tree: tree,
		})
	}
	defer func() {
		for _, state := range states {
			releaseGoTree(state.tree)
		}
	}()

	b.ReportAllocs()
	b.SetBytes(totalRealCorpusIncrementalBytes(cases))
	b.ResetTimer()

	var totals realCorpusRuntimeTotals
	for i := 0; i < b.N; i++ {
		for stateIndex := range states {
			state := &states[stateIndex]
			toggleRealCorpusEditByte(state.src, state.tc)
			state.tree.Edit(state.tc.goEdit)
			newTree := parseRealCorpusGoIncremental(b, realCorpusCaseWithSource(state.tc.realCorpusBenchmarkCase, state.src), parser, state.tree)
			totals.add(newTree.ParseRuntime())
			if newTree != state.tree {
				releaseGoTree(state.tree)
			}
			state.tree = newTree
		}
	}
	totals.report(b, realCorpusCasesFromIncremental(cases), b.N)
}

func benchmarkRealCorpusCParseIncrementalSingleByteEdit(b *testing.B, cases []realCorpusIncrementalCase) {
	parser := newRealCorpusCParser(b, cases[0].realCorpusBenchmarkCase)
	defer parser.Close()
	states := make([]realCorpusCIncrementalState, 0, len(cases))
	for _, tc := range cases {
		src := append([]byte(nil), tc.source...)
		tree := parseRealCorpusCFull(b, parser, src)
		states = append(states, realCorpusCIncrementalState{
			tc:   tc,
			src:  src,
			tree: tree,
		})
	}
	defer func() {
		for _, state := range states {
			state.tree.Close()
		}
	}()

	b.ReportAllocs()
	b.SetBytes(totalRealCorpusIncrementalBytes(cases))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for stateIndex := range states {
			state := &states[stateIndex]
			toggleRealCorpusEditByte(state.src, state.tc)
			state.tree.Edit(&state.tc.cEdit)
			newTree := parseRealCorpusCIncremental(b, parser, state.src, state.tree)
			if newTree != state.tree {
				state.tree.Close()
			}
			state.tree = newTree
		}
	}
	reportRealCorpusCaseMetrics(b, realCorpusCasesFromIncremental(cases))
}

func benchmarkRealCorpusGoParseIncrementalNoEdit(b *testing.B, cases []realCorpusBenchmarkCase) {
	parser := gotreesitter.NewParser(cases[0].goLang)
	states := make([]realCorpusGoNoEditState, 0, len(cases))
	for _, tc := range cases {
		states = append(states, realCorpusGoNoEditState{
			tc:   tc,
			tree: parseRealCorpusGoFull(b, tc, parser),
		})
	}
	defer func() {
		for _, state := range states {
			releaseGoTree(state.tree)
		}
	}()

	b.ReportAllocs()
	b.SetBytes(totalRealCorpusBenchmarkBytes(cases))
	b.ResetTimer()

	var totals realCorpusRuntimeTotals
	for i := 0; i < b.N; i++ {
		for stateIndex := range states {
			state := &states[stateIndex]
			newTree := parseRealCorpusGoIncremental(b, state.tc, parser, state.tree)
			totals.add(newTree.ParseRuntime())
			if newTree != state.tree {
				releaseGoTree(state.tree)
			}
			state.tree = newTree
		}
	}
	totals.report(b, cases, b.N)
}

func benchmarkRealCorpusCParseIncrementalNoEdit(b *testing.B, cases []realCorpusBenchmarkCase) {
	parser := newRealCorpusCParser(b, cases[0])
	defer parser.Close()
	states := make([]realCorpusCNoEditState, 0, len(cases))
	for _, tc := range cases {
		states = append(states, realCorpusCNoEditState{
			tc:   tc,
			tree: parseRealCorpusCFull(b, parser, tc.source),
		})
	}
	defer func() {
		for _, state := range states {
			state.tree.Close()
		}
	}()

	b.ReportAllocs()
	b.SetBytes(totalRealCorpusBenchmarkBytes(cases))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for stateIndex := range states {
			state := &states[stateIndex]
			newTree := parseRealCorpusCIncremental(b, parser, state.tc.source, state.tree)
			if newTree != state.tree {
				state.tree.Close()
			}
			state.tree = newTree
		}
	}
	reportRealCorpusCaseMetrics(b, cases)
}

func parseRealCorpusGoFull(tb testing.TB, tc realCorpusBenchmarkCase, parser *gotreesitter.Parser) *gotreesitter.Tree {
	tb.Helper()
	var tree *gotreesitter.Tree
	var err error
	switch tc.report.Backend {
	case grammars.ParseBackendTokenSource:
		if tc.entry.TokenSourceFactory == nil {
			tb.Fatalf("token source backend without factory for %q", tc.name)
		}
		tree, err = parser.ParseWithTokenSource(tc.source, tc.entry.TokenSourceFactory(tc.source, tc.goLang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, err = parser.Parse(tc.source)
	default:
		tb.Fatalf("unsupported parse backend %q for %q", tc.report.Backend, tc.name)
	}
	requireCompleteRealCorpusGoTree(tb, tc, tree, "gotreesitter full", err)
	return tree
}

func parseRealCorpusGoIncremental(tb testing.TB, tc realCorpusBenchmarkCase, parser *gotreesitter.Parser, oldTree *gotreesitter.Tree) *gotreesitter.Tree {
	tb.Helper()
	var tree *gotreesitter.Tree
	var err error
	switch tc.report.Backend {
	case grammars.ParseBackendTokenSource:
		if tc.entry.TokenSourceFactory == nil {
			tb.Fatalf("token source backend without factory for %q", tc.name)
		}
		tree, err = parser.ParseIncrementalWithTokenSource(tc.source, oldTree, tc.entry.TokenSourceFactory(tc.source, tc.goLang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, err = parser.ParseIncremental(tc.source, oldTree)
	default:
		tb.Fatalf("unsupported incremental backend %q for %q", tc.report.Backend, tc.name)
	}
	requireCompleteRealCorpusGoTree(tb, tc, tree, "gotreesitter incremental", err)
	return tree
}

func requireCompleteRealCorpusGoTree(tb testing.TB, tc realCorpusBenchmarkCase, tree *gotreesitter.Tree, phase string, err error) {
	tb.Helper()
	if err != nil {
		if tree != nil {
			releaseGoTree(tree)
		}
		tb.Fatalf("%s %s/%s error: %v", phase, tc.name, filepath.Base(tc.path), err)
	}
	if tree == nil || tree.RootNode() == nil {
		tb.Fatalf("%s %s/%s returned nil tree", phase, tc.name, filepath.Base(tc.path))
	}
	if got, want := tree.RootNode().EndByte(), uint32(len(tc.source)); got != want {
		rt := tree.ParseRuntime()
		releaseGoTree(tree)
		tb.Fatalf("%s %s/%s truncated: root.EndByte=%d want=%d %s", phase, tc.name, filepath.Base(tc.path), got, want, rt.Summary())
	}
}

func newRealCorpusCParser(tb testing.TB, tc realCorpusBenchmarkCase) *sitter.Parser {
	tb.Helper()
	parser := sitter.NewParser()
	if err := parser.SetLanguage(tc.cLang); err != nil {
		parser.Close()
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			tb.Skipf("skip C reference parser SetLanguage: %s", skipReason)
		}
		tb.Fatalf("C SetLanguage %s: %v", tc.name, err)
	}
	return parser
}

func parseRealCorpusCFull(tb testing.TB, parser *sitter.Parser, source []byte) *sitter.Tree {
	tb.Helper()
	tree := parser.Parse(source, nil)
	requireCompleteRealCorpusCTree(tb, tree, source, "C full")
	return tree
}

func parseRealCorpusCIncremental(tb testing.TB, parser *sitter.Parser, source []byte, oldTree *sitter.Tree) *sitter.Tree {
	tb.Helper()
	tree := parser.Parse(source, oldTree)
	requireCompleteRealCorpusCTree(tb, tree, source, "C incremental")
	return tree
}

func requireCompleteRealCorpusCTree(tb testing.TB, tree *sitter.Tree, source []byte, phase string) {
	tb.Helper()
	if tree == nil || tree.RootNode() == nil {
		tb.Fatalf("%s parse returned nil tree", phase)
	}
	if got, want := uint32(tree.RootNode().EndByte()), uint32(len(source)); got != want {
		tree.Close()
		tb.Fatalf("%s parse truncated: root.EndByte=%d want=%d type=%q hasError=%v", phase, got, want, tree.RootNode().Kind(), tree.RootNode().HasError())
	}
}

func toggleRealCorpusEditByte(src []byte, tc realCorpusIncrementalCase) {
	if src[tc.offset] == tc.original {
		src[tc.offset] = tc.replacement
	} else {
		src[tc.offset] = tc.original
	}
}

func totalRealCorpusIncrementalBytes(cases []realCorpusIncrementalCase) int64 {
	var total int64
	for _, tc := range cases {
		total += int64(len(tc.source))
	}
	return total
}

func realCorpusCasesFromIncremental(cases []realCorpusIncrementalCase) []realCorpusBenchmarkCase {
	out := make([]realCorpusBenchmarkCase, 0, len(cases))
	for _, tc := range cases {
		out = append(out, tc.realCorpusBenchmarkCase)
	}
	return out
}

func (t *realCorpusRuntimeTotals) add(rt gotreesitter.ParseRuntime) {
	t.arenaBytes += rt.ArenaBytesAllocated
	t.scratchBytes += rt.ScratchBytesAllocated
	t.nodes += uint64(rt.NodesAllocated)
	t.iterations += uint64(rt.Iterations)
	t.tokens += rt.TokensConsumed
	t.compactLeafMaterialized += rt.CompactFullLeafMaterialized
	t.pendingParentMaterialized += rt.PendingParentMaterialized
	t.transientParents += rt.TransientParentNodesMaterialized
	t.transientChildPointers += rt.TransientChildPointersMaterialized
	t.finalChildRefMaterializedChildren += rt.FinalChildRefMaterializedChildren
	t.finalChildRefSingleAccesses += rt.FinalChildRefSingleChildAccesses
	t.finalChildRefSingleMaterialized += rt.FinalChildRefSingleChildMaterializedChildren
	t.normalizationRewrites += rt.NormalizationNodesRewritten
}

func (t realCorpusRuntimeTotals) report(b *testing.B, cases []realCorpusBenchmarkCase, benchN int) {
	reportRealCorpusCaseMetrics(b, cases)
	if benchN == 0 {
		return
	}
	n := float64(benchN)
	b.ReportMetric(float64(t.arenaBytes)/n, "arena_B/op")
	b.ReportMetric(float64(t.scratchBytes)/n, "scratch_B/op")
	b.ReportMetric(float64(t.nodes)/n, "nodes/op")
	b.ReportMetric(float64(t.iterations)/n, "iterations/op")
	b.ReportMetric(float64(t.tokens)/n, "tokens/op")
	b.ReportMetric(float64(t.compactLeafMaterialized)/n, "compact_leaf_mat/op")
	b.ReportMetric(float64(t.pendingParentMaterialized)/n, "pending_parent_mat/op")
	b.ReportMetric(float64(t.transientParents)/n, "transient_parent_mat/op")
	b.ReportMetric(float64(t.transientChildPointers)/n, "transient_child_ptr_mat/op")
	b.ReportMetric(float64(t.finalChildRefMaterializedChildren)/n, "final_child_ref_mat_children/op")
	b.ReportMetric(float64(t.finalChildRefSingleAccesses)/n, "final_child_ref_single_accesses/op")
	b.ReportMetric(float64(t.finalChildRefSingleMaterialized)/n, "final_child_ref_single_mat/op")
	b.ReportMetric(float64(t.normalizationRewrites)/n, "normalization_rewrites/op")
}

func reportRealCorpusCaseMetrics(b *testing.B, cases []realCorpusBenchmarkCase) {
	b.ReportMetric(float64(len(cases)), "files/op")
}

func formatRealCorpusCaseList(cases []realCorpusBenchmarkCase) string {
	names := make([]string, 0, len(cases))
	for _, tc := range cases {
		names = append(names, fmt.Sprintf("%s:%dB", filepath.Base(tc.path), len(tc.source)))
	}
	return strings.Join(names, ",")
}
