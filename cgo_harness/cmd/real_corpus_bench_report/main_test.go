package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBenchmarkLine(t *testing.T) {
	line := "BenchmarkParityRealCorpusParseIncrementalSingleByteEdit/rust/tree-sitter-c-16 123 225000 ns/op 2048 B/op 3 allocs/op 2 files/op 100000 parse_wall_ns/op"
	s, ok, err := parseBenchmarkLine(line)
	if err != nil {
		t.Fatalf("parseBenchmarkLine error: %v", err)
	}
	if !ok {
		t.Fatal("parseBenchmarkLine did not match")
	}
	if s.Suite != "IncrementalSingleByteEdit" || s.Language != "rust" || s.Backend != "tree-sitter-c" {
		t.Fatalf("unexpected identity: %#v", s)
	}
	if got := s.Metrics["ns/op"]; got != 225000 {
		t.Fatalf("ns/op=%v", got)
	}
	if got := s.Metrics["parse_wall_ns/op"]; got != 100000 {
		t.Fatalf("parse_wall_ns/op=%v", got)
	}
}

func TestBuildReportRanksWorstRatio(t *testing.T) {
	samples := []sample{
		{Suite: "Full", Language: "go", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "Full", Language: "go", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalSingleByteEdit", Language: "go", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 300, "parse_wall_ns/op": 250}},
		{Suite: "IncrementalSingleByteEdit", Language: "go", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalNoEdit", Language: "go", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 50}},
		{Suite: "IncrementalNoEdit", Language: "go", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "Full", Language: "rust", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 600, "result_tree_build_ns/op": 400}},
		{Suite: "Full", Language: "rust", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalSingleByteEdit", Language: "rust", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 80}},
		{Suite: "IncrementalSingleByteEdit", Language: "rust", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalNoEdit", Language: "rust", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 50}},
		{Suite: "IncrementalNoEdit", Language: "rust", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
	}
	r := buildReport([]string{"bench.txt"}, samples, nil, nil)
	if len(r.Languages) != 2 {
		t.Fatalf("languages=%d", len(r.Languages))
	}
	if r.Languages[0].Language != "rust" {
		t.Fatalf("first language=%s", r.Languages[0].Language)
	}
	if got := r.Languages[0].WorstRatio; got != 6 {
		t.Fatalf("rust worst ratio=%v", got)
	}
	if len(r.Languages[0].TopAttribution) == 0 || r.Languages[0].TopAttribution[0].Name != "result_tree_build_ns/op" {
		t.Fatalf("unexpected attribution: %#v", r.Languages[0].TopAttribution)
	}
	if got := r.Languages[0].Readiness; got != readinessNeedsFullParseWork {
		t.Fatalf("rust readiness=%q", got)
	}
	if got := r.Languages[1].Readiness; got != readinessNeedsIncrementalWork {
		t.Fatalf("go readiness=%q", got)
	}
}

func TestBuildReportIncludesParityBlockedFailureOnlyLanguage(t *testing.T) {
	failures := []failureSummary{{
		Language: "hcl",
		Kind:     readinessParityBlocked,
		File:     "large.tf",
		Detail:   "first=/config_file/body[0]/block[0]/block_start[2] shape go=children=0 c=children=1",
		Source:   "container.log:5",
	}}

	r := buildReport([]string{"container.log"}, nil, failures, nil)
	if r.Samples != 0 {
		t.Fatalf("samples=%d, want 0", r.Samples)
	}
	if len(r.Failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(r.Failures))
	}
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Language != "hcl" {
		t.Fatalf("language=%q, want hcl", lang.Language)
	}
	if lang.Readiness != readinessParityBlocked {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessParityBlocked)
	}
	if lang.Failure == "" || lang.FailureSource != "container.log:5" {
		t.Fatalf("failure fields missing: %#v", lang)
	}
}

func TestBuildReportParityFailureOverridesSamples(t *testing.T) {
	samples := []sample{
		{Suite: "Full", Language: "yaml", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 200}},
		{Suite: "Full", Language: "yaml", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalSingleByteEdit", Language: "yaml", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 80}},
		{Suite: "IncrementalSingleByteEdit", Language: "yaml", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalNoEdit", Language: "yaml", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 50}},
		{Suite: "IncrementalNoEdit", Language: "yaml", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
	}
	failures := []failureSummary{{
		Language: "yaml",
		Kind:     readinessParityBlocked,
		File:     "in.yaml",
		Detail:   "first=/stream/document[0]/block_sequence[0] type go=block_sequence c=block_node",
		Source:   "container.log:12",
	}}

	r := buildReport([]string{"sampled.log", "full-source.log"}, samples, failures, nil)
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Readiness != readinessParityBlocked {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessParityBlocked)
	}
	if lang.FailureSource != "container.log:12" || !strings.Contains(lang.Failure, "in.yaml") {
		t.Fatalf("failure fields missing: %#v", lang)
	}
	if lang.FullRatio != 2 {
		t.Fatalf("full ratio=%v, want metrics preserved", lang.FullRatio)
	}
	if len(r.Benchmarks) == 0 {
		t.Fatal("benchmark metric rows were dropped")
	}
}

func TestBuildReportParityFailureOverridesRunAbort(t *testing.T) {
	failures := []failureSummary{
		{
			Language: "bitbake",
			Kind:     readinessBenchmarkRunAborted,
			Detail:   "benchmark container exited with code 1 (oom_killed=false)",
			Source:   "metadata.txt",
		},
		{
			Language: "bitbake",
			Kind:     readinessParityBlocked,
			File:     "3.2.bb",
			Detail:   "gotreesitter full parse truncated: root.EndByte=142 want=146",
			Source:   "container.log:27",
		},
	}

	r := buildReport([]string{"metadata.txt", "container.log"}, nil, failures, nil)
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Readiness != readinessParityBlocked {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessParityBlocked)
	}
	if lang.FailureSource != "container.log:27" || !strings.Contains(lang.Failure, "3.2.bb") {
		t.Fatalf("wrong failure selected: %#v", lang)
	}
}

func TestBuildReportIncludesBenchmarkRunAbortedLanguage(t *testing.T) {
	failures := []failureSummary{{
		Language: "bibtex",
		Kind:     readinessBenchmarkRunAborted,
		Detail:   "benchmark container exited with code 137 (oom_killed=false)",
		Source:   "metadata.txt",
	}}

	r := buildReport([]string{"metadata.txt"}, nil, failures, nil)
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Readiness != readinessBenchmarkRunAborted {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessBenchmarkRunAborted)
	}
	if !strings.Contains(lang.Expectation, "partial metrics") {
		t.Fatalf("expectation=%q", lang.Expectation)
	}
	if !strings.Contains(lang.Failure, "137") {
		t.Fatalf("failure=%q", lang.Failure)
	}
	if strings.HasPrefix(lang.Failure, ":") {
		t.Fatalf("failure has leading separator: %q", lang.Failure)
	}
}

func TestBuildReportIncludesCorpusUnavailableLanguage(t *testing.T) {
	failures := []failureSummary{{
		Language: "ada",
		Kind:     readinessCorpusUnavailable,
		Detail:   "real corpus filters selected no files under /corpus-sources/ada",
		Source:   "container.log:7",
	}}

	r := buildReport([]string{"container.log"}, nil, failures, nil)
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Readiness != readinessCorpusUnavailable {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessCorpusUnavailable)
	}
	if !strings.Contains(lang.Expectation, "No source files matched") {
		t.Fatalf("expectation=%q", lang.Expectation)
	}
	if !strings.Contains(lang.Failure, "/corpus-sources/ada") {
		t.Fatalf("failure=%q", lang.Failure)
	}
}

func TestBuildReportCompleteMeasurementsOverrideStaleCorpusUnavailableFailure(t *testing.T) {
	samples := []sample{
		{Suite: "Full", Language: "gomod", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 300}},
		{Suite: "Full", Language: "gomod", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalSingleByteEdit", Language: "gomod", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 80}},
		{Suite: "IncrementalSingleByteEdit", Language: "gomod", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalNoEdit", Language: "gomod", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 50}},
		{Suite: "IncrementalNoEdit", Language: "gomod", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
	}
	failures := []failureSummary{{
		Language: "gomod",
		Kind:     readinessCorpusUnavailable,
		Detail:   "real corpus filters selected no files under /corpus-sources/gomod",
		Source:   "old/container.log:7",
	}}

	r := buildReport([]string{"old/container.log", "new/container.log"}, samples, failures, nil)
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Readiness != readinessNeedsFullParseWork {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessNeedsFullParseWork)
	}
	if lang.Failure != "" || lang.FailureSource != "" {
		t.Fatalf("stale failure should not override complete measurements: %#v", lang)
	}
	if lang.FullRatio != 3 || lang.EditRatio != 0.8 || lang.NoEditRatio != 0.5 {
		t.Fatalf("ratios not preserved: full=%v edit=%v noedit=%v", lang.FullRatio, lang.EditRatio, lang.NoEditRatio)
	}
}

func TestBuildReportCompleteMeasurementsOverrideEditCandidateParityFailure(t *testing.T) {
	samples := []sample{
		{Suite: "Full", Language: "authzed", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 1200}},
		{Suite: "Full", Language: "authzed", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalSingleByteEdit", Language: "authzed", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 50}},
		{Suite: "IncrementalSingleByteEdit", Language: "authzed", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalNoEdit", Language: "authzed", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 40}},
		{Suite: "IncrementalNoEdit", Language: "authzed", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
	}
	failures := []failureSummary{{
		Language: "authzed",
		Kind:     readinessParityBlocked,
		File:     "associativity.zed",
		Detail:   "single-byte edit parity failed: candidate=replace@156:'1'->'2': gotreesitter incremental mismatch against fresh",
		Source:   "container.log:42",
	}}

	r := buildReport([]string{"container.log"}, samples, failures, []string{"authzed"})
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Readiness != readinessNeedsFullParseWork {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessNeedsFullParseWork)
	}
	if lang.Failure != "" || lang.FailureSource != "" {
		t.Fatalf("edit candidate rejection should not override complete measurements: %#v", lang)
	}
	if lang.FullRatio != 12 || lang.EditRatio != 0.5 || lang.NoEditRatio != 0.4 {
		t.Fatalf("ratios not preserved: full=%v edit=%v noedit=%v", lang.FullRatio, lang.EditRatio, lang.NoEditRatio)
	}
}

func TestBuildReportIncludesAttemptedOnlyLanguage(t *testing.T) {
	r := buildReport([]string{"metadata.txt"}, nil, nil, []string{"agda"})
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Language != "agda" {
		t.Fatalf("language=%q, want agda", lang.Language)
	}
	if lang.Readiness != readinessIncompleteMeasurement {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessIncompleteMeasurement)
	}
	if !strings.Contains(lang.Expectation, "emitted no full") {
		t.Fatalf("expectation=%q", lang.Expectation)
	}
	if !strings.Contains(lang.Failure, "emitted no BenchmarkParityRealCorpusParse samples") {
		t.Fatalf("failure=%q", lang.Failure)
	}
}

func TestParseBenchmarkFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bench.log")
	err := os.WriteFile(path, []byte(`
BenchmarkParityRealCorpusParseFull/python/gotreesitter-16 10 1000 ns/op 1 files/op
BenchmarkParityRealCorpusParseFull/python/tree-sitter-c-16 10 500 ns/op 1 files/op
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	samples, err := parseBenchmarkFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples=%d", len(samples))
	}
}

func TestParseBenchmarkFilesAllowsLargeCorpusLogLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container.log")
	data := strings.Join([]string{
		"    benchmark_real_corpus_parity_test.go:167: real corpus glsl: files=583 cases=" + strings.Repeat("shader.glsl:1024B,", 5000),
		"--- FAIL: BenchmarkParityRealCorpusParseFull/glsl",
		"    benchmark_real_corpus_parity_test.go:169: structural parity mismatch before benchmark for glsl/shader.glsl: first=/ERROR type go=ERROR c=translation_unit",
		"BenchmarkParityRealCorpusParseFull/groovy/gotreesitter-16 10 1000 ns/op 1 files/op",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	samples, err := parseBenchmarkFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples=%d, want 1", len(samples))
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(failures))
	}
	if failures[0].Language != "glsl" || failures[0].Kind != readinessParityBlocked {
		t.Fatalf("unexpected failure: %#v", failures[0])
	}
}

func TestParseAttemptedLanguagesFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.txt")
	err := os.WriteFile(path, []byte(`
command=env GTS_REAL_CORPUS_BENCH_LANGS=agda,apex go test .
other=env GTS_PARITY_BENCH_LANGS=top50 go test .
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	languages, err := parseAttemptedLanguagesFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(languages, ",")
	if got != "agda,apex" {
		t.Fatalf("languages=%q", got)
	}
}

func TestParseFailureFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container.log")
	err := os.WriteFile(path, []byte(`
--- FAIL: BenchmarkParityRealCorpusParseFull/hcl
    benchmark_real_corpus_parity_test.go:168: structural parity mismatch before benchmark for hcl/large.tf: first=/config_file/body[0]/block[0]/block_start[2] shape go=children=0 c=children=1
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(failures))
	}
	if failures[0].Language != "hcl" || failures[0].File != "large.tf" {
		t.Fatalf("unexpected failure: %#v", failures[0])
	}
	if failures[0].Kind != readinessParityBlocked {
		t.Fatalf("failure kind=%q", failures[0].Kind)
	}
	if failures[0].Source == "" {
		t.Fatalf("missing source: %#v", failures[0])
	}
}

func TestParseFailureFilesKnownMismatchSkip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container.log")
	err := os.WriteFile(path, []byte(`
BenchmarkParityRealCorpusParseFull
BenchmarkParityRealCorpusParseFull/cue
    benchmark_real_corpus_parity_test.go:167: known mismatch: known degraded structural parity: named wrapper/runtime alias shape still diverges from C reference
--- SKIP: BenchmarkParityRealCorpusParseFull/cue
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(failures))
	}
	if failures[0].Language != "cue" || failures[0].Kind != readinessParityBlocked {
		t.Fatalf("unexpected failure: %#v", failures[0])
	}
	if !strings.Contains(failures[0].Detail, "known degraded structural parity") {
		t.Fatalf("detail=%q", failures[0].Detail)
	}
}

func TestParseFailureFilesGotreesitterTruncated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container.log")
	err := os.WriteFile(path, []byte(`
--- FAIL: BenchmarkParityRealCorpusParseIncrementalSingleByteEdit/bitbake
    benchmark_real_corpus_parity_test.go:190: gotreesitter full bitbake/3.2.bb truncated: root.EndByte=142 want=146 truncated=true stopReason=accepted
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(failures))
	}
	if failures[0].Language != "bitbake" || failures[0].File != "3.2.bb" || failures[0].Kind != readinessParityBlocked {
		t.Fatalf("unexpected failure: %#v", failures[0])
	}
	if !strings.Contains(failures[0].Detail, "root.EndByte=142") {
		t.Fatalf("detail=%q", failures[0].Detail)
	}
}

func TestParseFailureFilesEditLaneSkipDiagnostics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container.log")
	err := os.WriteFile(path, []byte(`
BenchmarkParityRealCorpusParseIncrementalSingleByteEdit
BenchmarkParityRealCorpusParseIncrementalSingleByteEdit/json
    benchmark_real_corpus_parity_test.go:190: skip /corpus-sources/json/package-lock.json: no single-byte incremental edit site matched benchmark constraints
    benchmark_real_corpus_parity_test.go:190: skip json/node-types.json candidate=replace@41:'t'->'u': truncated: root.EndByte=2686 want=2698
    benchmark_real_corpus_parity_test.go:190: no selected files had a single-byte incremental edit site matching benchmark constraints
--- SKIP: BenchmarkParityRealCorpusParseIncrementalSingleByteEdit/json
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	var candidate failureSummary
	for _, failure := range failures {
		if strings.Contains(failure.Detail, "candidate=") {
			candidate = failure
			break
		}
	}
	if candidate.Language != "json" || candidate.Kind != readinessIncompleteMeasurement {
		t.Fatalf("missing candidate diagnostic: %#v", failures)
	}
	if candidate.File != "node-types.json" {
		t.Fatalf("file=%q", candidate.File)
	}
	if !strings.Contains(candidate.Detail, "root.EndByte=2686") {
		t.Fatalf("detail=%q", candidate.Detail)
	}
}

func TestBuildReportUsesEditLaneSkipDiagnostic(t *testing.T) {
	samples := []sample{
		{Suite: "Full", Language: "json", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 200}},
		{Suite: "Full", Language: "json", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
		{Suite: "IncrementalNoEdit", Language: "json", Backend: "gotreesitter", Metrics: map[string]float64{"ns/op": 50}},
		{Suite: "IncrementalNoEdit", Language: "json", Backend: "tree-sitter-c", Metrics: map[string]float64{"ns/op": 100}},
	}
	failures := []failureSummary{
		{
			Language: "json",
			Kind:     readinessIncompleteMeasurement,
			File:     "package-lock.json",
			Detail:   "no single-byte incremental edit site matched benchmark constraints",
			Source:   "container.log:3",
		},
		{
			Language: "json",
			Kind:     readinessIncompleteMeasurement,
			File:     "node-types.json",
			Detail:   "single-byte edit candidate skipped: candidate=replace@41:'t'->'u': truncated: root.EndByte=2686 want=2698",
			Source:   "container.log:4",
		},
	}

	r := buildReport([]string{"container.log"}, samples, failures, []string{"json"})
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Readiness != readinessIncompleteMeasurement {
		t.Fatalf("readiness=%q", lang.Readiness)
	}
	if lang.FullRatio != 2 || lang.NoEditRatio != 0.5 {
		t.Fatalf("ratios not preserved: full=%v noedit=%v", lang.FullRatio, lang.NoEditRatio)
	}
	if !strings.Contains(lang.Failure, "node-types.json") || !strings.Contains(lang.Failure, "root.EndByte=2686") {
		t.Fatalf("failure=%q", lang.Failure)
	}
	if !strings.Contains(lang.Expectation, "selected but skipped") || !strings.Contains(lang.Expectation, "edit readiness is unknown") {
		t.Fatalf("expectation=%q", lang.Expectation)
	}
}

func TestBuildReportClassifiesEditCandidateParityFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container.log")
	err := os.WriteFile(path, []byte(`
BenchmarkParityRealCorpusParseIncrementalSingleByteEdit
BenchmarkParityRealCorpusParseIncrementalSingleByteEdit/json
    benchmark_real_corpus_parity_test.go:190: skip json/package-lock.json candidate=replace@46:'0'->'1': gotreesitter incremental mismatch against fresh: root: Type left="ERROR" right="document"
--- SKIP: BenchmarkParityRealCorpusParseIncrementalSingleByteEdit/json
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(failures))
	}
	if failures[0].Kind != readinessParityBlocked {
		t.Fatalf("kind=%q, want %q: %#v", failures[0].Kind, readinessParityBlocked, failures[0])
	}

	r := buildReport([]string{path}, nil, failures, []string{"json"})
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Readiness != readinessParityBlocked {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessParityBlocked)
	}
	if !strings.Contains(lang.Expectation, "Single-byte incremental parity failed") {
		t.Fatalf("expectation=%q", lang.Expectation)
	}
	if !strings.Contains(lang.Failure, "package-lock.json") || !strings.Contains(lang.Failure, "ERROR") {
		t.Fatalf("failure=%q", lang.Failure)
	}
}

func TestParseFailureFilesMetadataExit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.txt")
	err := os.WriteFile(path, []byte(`
label=real-corpus-bench-bibtex
exit_code=137
oom_killed=false
state_error=
command=env GTS_REAL_CORPUS_BENCH_LANGS=bibtex go test .
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(failures))
	}
	if failures[0].Language != "bibtex" || failures[0].Kind != readinessBenchmarkRunAborted {
		t.Fatalf("unexpected failure: %#v", failures[0])
	}
	if !strings.Contains(failures[0].Detail, "137") || !strings.Contains(failures[0].Detail, "oom_killed=false") {
		t.Fatalf("detail=%q", failures[0].Detail)
	}
}

func TestParseFailureFilesMetadataTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.txt")
	err := os.WriteFile(path, []byte(`
label=real-corpus-bench-bibtex
exit_code=124
oom_killed=false
state_error=
command=timeout --foreground --kill-after=30s 2m env GTS_REAL_CORPUS_BENCH_LANGS=bibtex go test .
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(failures))
	}
	if failures[0].Language != "bibtex" || failures[0].Kind != readinessBenchmarkRunAborted {
		t.Fatalf("unexpected failure: %#v", failures[0])
	}
	if !strings.Contains(failures[0].Detail, "timed out") || !strings.Contains(failures[0].Detail, "124") {
		t.Fatalf("detail=%q", failures[0].Detail)
	}
}

func TestParseFailureFilesNoCorpusFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container.log")
	err := os.WriteFile(path, []byte(`
--- FAIL: BenchmarkParityRealCorpusParseFull/ada
    benchmark_real_corpus_parity_test.go:354: real corpus filters selected no files under /corpus-sources/ada
--- FAIL: BenchmarkParityRealCorpusParseFull
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(failures))
	}
	if failures[0].Language != "ada" || failures[0].Kind != readinessCorpusUnavailable {
		t.Fatalf("unexpected failure: %#v", failures[0])
	}
	if !strings.Contains(failures[0].Detail, "/corpus-sources/ada") {
		t.Fatalf("detail=%q", failures[0].Detail)
	}
}

func TestParseFailureFilesNoParityCleanFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container.log")
	err := os.WriteFile(path, []byte(`
BenchmarkParityRealCorpusParseFull
BenchmarkParityRealCorpusParseFull/ron
    benchmark_real_corpus_parity_test.go:167: GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH selected no parity-clean files; skipped=example.ron(/source_file/struct[0]/([0] field go= c=body), preserve_sequence_ex1.ron(/source_file/struct[0]/([0] field go= c=body)
--- FAIL: BenchmarkParityRealCorpusParseFull/ron
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%d, want 1", len(failures))
	}
	if failures[0].Language != "ron" || failures[0].File != "example.ron" || failures[0].Kind != readinessParityBlocked {
		t.Fatalf("unexpected failure: %#v", failures[0])
	}
	if !strings.Contains(failures[0].Detail, "selected no parity-clean files") {
		t.Fatalf("detail=%q", failures[0].Detail)
	}
}

func TestParseFailureFilesPartialSkipMismatchOverridesCompleteMetrics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container.log")
	err := os.WriteFile(path, []byte(`
BenchmarkParityRealCorpusParseFull
BenchmarkParityRealCorpusParseFull/twig
    benchmark_real_corpus_parity_test.go:167: GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH skipped 1/13 file(s): no_line.twig(/template/output_directive[0] shape go=children=3 c=children=1)
BenchmarkParityRealCorpusParseFull/twig/gotreesitter-16 10 200 ns/op
BenchmarkParityRealCorpusParseFull/twig/tree-sitter-c-16 10 100 ns/op
BenchmarkParityRealCorpusParseIncrementalSingleByteEdit/twig
    benchmark_real_corpus_parity_test.go:190: skip twig/markdown_to_html.html.twig candidate=replace@3:'a'->'b': gotreesitter incremental mismatch against fresh: root: ChildCount left=3 right=4
BenchmarkParityRealCorpusParseIncrementalSingleByteEdit/twig/gotreesitter-16 10 50 ns/op
BenchmarkParityRealCorpusParseIncrementalSingleByteEdit/twig/tree-sitter-c-16 10 100 ns/op
BenchmarkParityRealCorpusParseIncrementalNoEdit/twig/gotreesitter-16 10 40 ns/op
BenchmarkParityRealCorpusParseIncrementalNoEdit/twig/tree-sitter-c-16 10 100 ns/op
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	files, err := expandInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	samples, err := parseBenchmarkFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	failures, err := parseFailureFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	r := buildReport([]string{path}, samples, failures, []string{"twig"})
	if len(r.Languages) != 1 {
		t.Fatalf("languages=%d, want 1", len(r.Languages))
	}
	lang := r.Languages[0]
	if lang.Readiness != readinessParityBlocked {
		t.Fatalf("readiness=%q, want %q", lang.Readiness, readinessParityBlocked)
	}
	if lang.FullRatio != 2 || lang.EditRatio != 0.5 || lang.NoEditRatio != 0.4 {
		t.Fatalf("ratios not preserved: full=%v edit=%v noedit=%v", lang.FullRatio, lang.EditRatio, lang.NoEditRatio)
	}
	if !strings.Contains(lang.Failure, "no_line.twig") || !strings.Contains(lang.Failure, "strict parity filtered files") {
		t.Fatalf("failure=%q", lang.Failure)
	}
}

func TestWriteMarkdownIncludesBenchmarkMetrics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	r := report{
		GeneratedAt: "2026-06-07T00:00:00Z",
		Samples:     2,
		Languages: []languageReport{
			{
				Language:  "go",
				Readiness: readinessMeetsTargets,
				FullRatio: 1.2,
			},
		},
		Benchmarks: []benchmarkSummary{
			{
				Language: "go",
				Suite:    "Full",
				Backend:  "gotreesitter",
				Metrics: map[string]metricSummary{
					"ns/op":            {Samples: 2, Median: 1200},
					"B/op":             {Samples: 2, Median: 64},
					"allocs/op":        {Samples: 2, Median: 3},
					"parse_wall_ns/op": {Samples: 2, Median: 1000},
				},
			},
		},
	}
	if err := writeMarkdown(path, r); err != nil {
		t.Fatalf("writeMarkdown: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"## Benchmark Metrics",
		"| go | Full | gotreesitter | 2 | 1.200us | 64 | 3 |  | parse_wall_ns/op=1000 |",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("markdown missing %q:\n%s", want, text)
		}
	}
}

func TestClassifyReadiness(t *testing.T) {
	tests := []struct {
		name string
		lr   languageReport
		want string
	}{
		{
			name: "meets targets",
			lr:   languageReport{FullRatio: 0.9, EditRatio: 0.5, NoEditRatio: 0.1},
			want: readinessMeetsTargets,
		},
		{
			name: "needs full",
			lr:   languageReport{FullRatio: 1.1, EditRatio: 0.5, NoEditRatio: 0.1},
			want: readinessNeedsFullParseWork,
		},
		{
			name: "needs incremental",
			lr:   languageReport{FullRatio: 0.9, EditRatio: 1.1, NoEditRatio: 0.1},
			want: readinessNeedsIncrementalWork,
		},
		{
			name: "needs both",
			lr:   languageReport{FullRatio: 1.1, EditRatio: 1.1, NoEditRatio: 0.1},
			want: readinessNeedsFullAndIncrWork,
		},
		{
			name: "incomplete",
			lr:   languageReport{FullRatio: 0.9, EditRatio: 0.5},
			want: readinessIncompleteMeasurement,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, expectation := classifyReadiness(tt.lr)
			if got != tt.want {
				t.Fatalf("readiness=%q want=%q", got, tt.want)
			}
			if expectation == "" {
				t.Fatal("empty expectation")
			}
		})
	}
}
