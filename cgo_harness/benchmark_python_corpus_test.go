//go:build cgo && treesitter_c_bench

package cgoharness

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitterpython "github.com/smacker/go-tree-sitter/python"
)

const defaultPythonCorpusPath = "corpus_real/python/large__python3.8_grammar.py"

type pythonCorpusFile struct {
	path   string
	source []byte
}

type pythonCorpusParseMode string

const (
	pythonCorpusParseModeDFA       pythonCorpusParseMode = "dfa"
	pythonCorpusParseModeDFANoTree pythonCorpusParseMode = "dfa_no_tree"
)

type pythonRuntimeBenchStats struct {
	ops                             int
	tokensConsumed                  uint64
	iterations                      uint64
	nodesAllocated                  uint64
	parentNodesAllocated            uint64
	leafNodesAllocated              uint64
	gssNodesAllocated               uint64
	singleStackGSSNodes             uint64
	multiStackGSSNodes              uint64
	arenaBytesAllocated             int64
	scratchBytesAllocated           int64
	entryScratchBytesAllocated      int64
	gssBytesAllocated               int64
	externalCheckpointRecords       uint64
	externalCheckpointSlots         uint64
	externalCheckpointBytes         int64
	externalCheckpointSnapshotBytes uint64
	leafNodesConstructed            uint64
	parentNodesConstructed          uint64
	noTreeReduceNodesConstructed    uint64
	maxStacksSeen                   int
}

func (s *pythonRuntimeBenchStats) add(rt gotreesitter.ParseRuntime) {
	s.ops++
	s.tokensConsumed += rt.TokensConsumed
	s.iterations += uint64(rt.Iterations)
	s.nodesAllocated += uint64(rt.NodesAllocated)
	s.parentNodesAllocated += rt.ParentNodesAllocated
	s.leafNodesAllocated += rt.LeafNodesAllocated
	s.gssNodesAllocated += rt.GSSNodesAllocated
	s.singleStackGSSNodes += rt.SingleStackGSSNodes
	s.multiStackGSSNodes += rt.MultiStackGSSNodes
	s.arenaBytesAllocated += rt.ArenaBytesAllocated
	s.scratchBytesAllocated += rt.ScratchBytesAllocated
	s.entryScratchBytesAllocated += rt.EntryScratchBytesAllocated
	s.gssBytesAllocated += rt.GSSBytesAllocated
	s.externalCheckpointRecords += rt.ExternalScannerCheckpointRecords
	s.externalCheckpointSlots += rt.ExternalScannerCheckpointSlotsAllocated
	s.externalCheckpointBytes += rt.ExternalScannerCheckpointBytesAllocated
	s.externalCheckpointSnapshotBytes += rt.ExternalScannerSnapshotBytesAllocated
	s.leafNodesConstructed += rt.LeafNodesConstructed
	s.parentNodesConstructed += rt.ParentNodesConstructed
	s.noTreeReduceNodesConstructed += rt.NoTreeReduceNodesConstructed
	if rt.MaxStacksSeen > s.maxStacksSeen {
		s.maxStacksSeen = rt.MaxStacksSeen
	}
}

func (s pythonRuntimeBenchStats) report(b *testing.B) {
	if s.ops == 0 {
		return
	}
	b.ReportMetric(float64(s.tokensConsumed)/float64(s.ops), "tokens/op")
	b.ReportMetric(float64(s.maxStacksSeen), "max_stacks")
	if s.tokensConsumed == 0 {
		return
	}
	tokens := float64(s.tokensConsumed)
	gssNodes := s.gssNodesAllocated
	if gssNodes == 0 {
		gssNodes = s.singleStackGSSNodes + s.multiStackGSSNodes
	}
	b.ReportMetric(float64(s.iterations)/tokens, "iters/token")
	b.ReportMetric(float64(s.nodesAllocated)/tokens, "nodes/token")
	b.ReportMetric(float64(s.leafNodesConstructed)/tokens, "leaf_nodes/token")
	b.ReportMetric(float64(s.parentNodesConstructed)/tokens, "parent_nodes/token")
	b.ReportMetric(float64(s.noTreeReduceNodesConstructed)/tokens, "notree_nodes/token")
	if s.parentNodesAllocated != 0 || s.leafNodesAllocated != 0 {
		b.ReportMetric(float64(s.parentNodesAllocated)/tokens, "surv_parent_nodes/token")
		b.ReportMetric(float64(s.leafNodesAllocated)/tokens, "surv_leaf_nodes/token")
	}
	b.ReportMetric(float64(gssNodes)/tokens, "gss_nodes/token")
	b.ReportMetric(float64(s.singleStackGSSNodes)/tokens, "single_gss/token")
	b.ReportMetric(float64(s.multiStackGSSNodes)/tokens, "multi_gss/token")
	b.ReportMetric(float64(s.arenaBytesAllocated)/tokens, "arena_B/token")
	b.ReportMetric(float64(s.scratchBytesAllocated)/tokens, "scratch_B/token")
	b.ReportMetric(float64(s.entryScratchBytesAllocated)/tokens, "entry_B/token")
	b.ReportMetric(float64(s.gssBytesAllocated)/tokens, "gss_B/token")
	b.ReportMetric(float64(s.externalCheckpointRecords)/tokens, "chk_records/token")
	b.ReportMetric(float64(s.externalCheckpointSlots)/tokens, "chk_slots/token")
	b.ReportMetric(float64(s.externalCheckpointBytes)/tokens, "chk_B/token")
	b.ReportMetric(float64(s.externalCheckpointSnapshotBytes)/tokens, "chk_snap_B/token")
}

func loadPythonCorpusFile(tb testing.TB) pythonCorpusFile {
	tb.Helper()

	candidates := []string{strings.TrimSpace(os.Getenv("GOT_PYTHON_CORPUS_FILE"))}
	if candidates[0] == "" {
		candidates = []string{
			defaultPythonCorpusPath,
			filepath.Join("cgo_harness", defaultPythonCorpusPath),
		}
	}

	for _, path := range candidates {
		if path == "" {
			continue
		}
		st, err := os.Stat(path)
		if err != nil || st.IsDir() {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			tb.Fatalf("read python corpus %s: %v", path, err)
		}
		if len(source) == 0 {
			tb.Fatalf("python corpus %s is empty", path)
		}
		tb.Logf("python corpus: path=%s bytes=%d", path, len(source))
		return pythonCorpusFile{path: path, source: source}
	}

	tb.Fatalf("python corpus file not found; tried %s; set GOT_PYTHON_CORPUS_FILE or run from the repository/cgo_harness root", strings.Join(candidates, ", "))
	return pythonCorpusFile{}
}

func pythonCorpusBenchTimeoutMicros(tb testing.TB) uint64 {
	tb.Helper()

	raw := strings.TrimSpace(os.Getenv("GOT_PYTHON_CORPUS_BENCH_TIMEOUT_MICROS"))
	if raw == "" {
		return 0
	}
	timeoutMicros, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		tb.Fatalf("invalid GOT_PYTHON_CORPUS_BENCH_TIMEOUT_MICROS=%q", raw)
	}
	return timeoutMicros
}

func requireCompletePythonCorpusTree(tb testing.TB, lang *gotreesitter.Language, file pythonCorpusFile, tree *gotreesitter.Tree, phase string) {
	tb.Helper()

	if tree == nil {
		tb.Fatalf("%s: parse returned nil tree", phase)
	}
	root := tree.RootNode()
	if root == nil {
		tb.Fatalf("%s: parse returned nil root", phase)
	}
	rt := tree.ParseRuntime()
	if root.HasError() || tree.ParseStoppedEarly() || root.EndByte() != uint32(len(file.source)) || rt.Truncated {
		tb.Fatalf(
			"%s: incomplete parse path=%s type=%q has_error=%v stopped=%v root_end=%d want=%d runtime=%s",
			phase,
			file.path,
			root.Type(lang),
			root.HasError(),
			tree.ParseStoppedEarly(),
			root.EndByte(),
			len(file.source),
			rt.Summary(),
		)
	}
}

func benchmarkPythonCorpusGoDFA(b *testing.B, mode pythonCorpusParseMode) {
	file := loadPythonCorpusFile(b)
	lang := grammars.PythonLanguage()
	pool := gotreesitter.NewParserPool(
		lang,
		gotreesitter.WithParserPoolTimeoutMicros(pythonCorpusBenchTimeoutMicros(b)),
	)

	b.ReportAllocs()
	b.SetBytes(int64(len(file.source)))
	b.ResetTimer()

	var stats pythonRuntimeBenchStats
	for i := 0; i < b.N; i++ {
		var (
			tree *gotreesitter.Tree
			err  error
		)
		switch mode {
		case pythonCorpusParseModeDFA:
			tree, err = pool.Parse(file.source)
		case pythonCorpusParseModeDFANoTree:
			tree, err = pool.ParseNoTreeBenchmarkOnly(file.source)
		default:
			b.Fatalf("unknown python corpus parse mode %q", mode)
		}
		if err != nil {
			if tree != nil {
				tree.Release()
			}
			b.Fatalf("%s: %v", file.path, err)
		}
		requireCompletePythonCorpusTree(b, lang, file, tree, string(mode))
		stats.add(tree.ParseRuntime())
		tree.Release()
	}
	b.StopTimer()
	stats.report(b)
}

func BenchmarkPythonCorpusGoTreeSitterParseDFA(b *testing.B) {
	benchmarkPythonCorpusGoDFA(b, pythonCorpusParseModeDFA)
}

func BenchmarkPythonCorpusGoTreeSitterParseDFANoTree(b *testing.B) {
	benchmarkPythonCorpusGoDFA(b, pythonCorpusParseModeDFANoTree)
}

func BenchmarkPythonCorpusCTreeSitterParseFull(b *testing.B) {
	file := loadPythonCorpusFile(b)
	parser := newCTreeSitterParserWithLanguage(b, sitterpython.GetLanguage)
	defer parser.Close()

	b.ReportAllocs()
	b.SetBytes(int64(len(file.source)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree := parser.Parse(nil, file.source)
		root := requireCompleteCTree(b, tree, file.source, file.path)
		if root.HasError() {
			tree.Close()
			b.Fatalf("%s: cgo parse has errors", file.path)
		}
		tree.Close()
	}
}
