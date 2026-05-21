package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var benchmarkSuffixRE = regexp.MustCompile(`-\d+$`)

type sample struct {
	Suite    string             `json:"suite"`
	Language string             `json:"language"`
	Backend  string             `json:"backend"`
	Metrics  map[string]float64 `json:"metrics"`
	Source   string             `json:"source,omitempty"`
}

type metricSummary struct {
	Samples int     `json:"samples"`
	Median  float64 `json:"median"`
	Mean    float64 `json:"mean"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}

type benchmarkSummary struct {
	Suite    string                   `json:"suite"`
	Language string                   `json:"language"`
	Backend  string                   `json:"backend"`
	Metrics  map[string]metricSummary `json:"metrics"`
}

type languageReport struct {
	Language       string                        `json:"language"`
	FullRatio      float64                       `json:"full_ratio,omitempty"`
	EditRatio      float64                       `json:"edit_ratio,omitempty"`
	NoEditRatio    float64                       `json:"noedit_ratio,omitempty"`
	WorstRatio     float64                       `json:"worst_ratio,omitempty"`
	FullGoNanos    float64                       `json:"full_go_ns,omitempty"`
	FullCNanos     float64                       `json:"full_c_ns,omitempty"`
	EditGoNanos    float64                       `json:"edit_go_ns,omitempty"`
	EditCNanos     float64                       `json:"edit_c_ns,omitempty"`
	NoEditGoNanos  float64                       `json:"noedit_go_ns,omitempty"`
	NoEditCNanos   float64                       `json:"noedit_c_ns,omitempty"`
	TopAttribution []attributionBucket           `json:"top_attribution,omitempty"`
	GoSuiteMetrics map[string]map[string]float64 `json:"go_suite_metrics,omitempty"`
	CSuiteMetrics  map[string]map[string]float64 `json:"c_suite_metrics,omitempty"`
}

type attributionBucket struct {
	Suite string  `json:"suite"`
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type report struct {
	GeneratedAt string             `json:"generated_at"`
	Inputs      []string           `json:"inputs"`
	Samples     int                `json:"samples"`
	Benchmarks  []benchmarkSummary `json:"benchmarks"`
	Languages   []languageReport   `json:"languages"`
}

type key struct {
	suite    string
	language string
	backend  string
}

func main() {
	var (
		inputsCSV string
		outJSON   string
		outMD     string
	)
	flag.StringVar(&inputsCSV, "input", "", "comma-separated benchmark log files or directories; positional args are also accepted")
	flag.StringVar(&outJSON, "out-json", "real_corpus_bench_report.json", "JSON report path")
	flag.StringVar(&outMD, "out-md", "REAL_CORPUS_BENCH_REPORT.md", "Markdown report path")
	flag.Parse()

	inputs := parseInputList(inputsCSV)
	inputs = append(inputs, flag.Args()...)
	if len(inputs) == 0 {
		fatalf("provide at least one benchmark log via -input or positional args")
	}
	files, err := expandInputs(inputs)
	if err != nil {
		fatalf("expand inputs: %v", err)
	}
	if len(files) == 0 {
		fatalf("no benchmark log files selected")
	}

	samples, err := parseBenchmarkFiles(files)
	if err != nil {
		fatalf("parse benchmark logs: %v", err)
	}
	if len(samples) == 0 {
		fatalf("no BenchmarkParityRealCorpusParse lines found")
	}

	r := buildReport(files, samples)
	if err := writeJSON(outJSON, r); err != nil {
		fatalf("write %s: %v", outJSON, err)
	}
	if err := writeMarkdown(outMD, r); err != nil {
		fatalf("write %s: %v", outMD, err)
	}
	fmt.Printf("wrote report json: %s\n", outJSON)
	fmt.Printf("wrote report markdown: %s\n", outMD)
	fmt.Printf("samples: %d languages: %d\n", r.Samples, len(r.Languages))
}

func parseInputList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func expandInputs(inputs []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, input := range inputs {
		st, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if !seen[input] {
				seen[input] = true
				files = append(files, input)
			}
			continue
		}
		err = filepath.WalkDir(input, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".log", ".txt", ".out":
			default:
				return nil
			}
			if !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func parseBenchmarkFiles(files []string) ([]sample, error) {
	var samples []sample
	for _, path := range files {
		fileSamples, err := parseBenchmarkFile(path)
		if err != nil {
			return nil, err
		}
		samples = append(samples, fileSamples...)
	}
	return samples, nil
}

func parseBenchmarkFile(path string) ([]sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var samples []sample
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "BenchmarkParityRealCorpusParse") {
			continue
		}
		s, ok, err := parseBenchmarkLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		if !ok {
			continue
		}
		s.Source = path
		samples = append(samples, s)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func parseBenchmarkLine(line string) (sample, bool, error) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return sample{}, false, nil
	}
	suite, lang, backend, ok := parseBenchmarkName(fields[0])
	if !ok {
		return sample{}, false, nil
	}
	if _, err := strconv.ParseInt(fields[1], 10, 64); err != nil {
		return sample{}, false, fmt.Errorf("parse iteration count %q: %w", fields[1], err)
	}
	metrics := make(map[string]float64)
	for i := 2; i+1 < len(fields); i += 2 {
		value, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			continue
		}
		unit := fields[i+1]
		metrics[unit] = value
	}
	return sample{Suite: suite, Language: lang, Backend: backend, Metrics: metrics}, true, nil
}

func parseBenchmarkName(name string) (suite, language, backend string, ok bool) {
	parts := strings.Split(name, "/")
	if len(parts) != 3 {
		return "", "", "", false
	}
	const prefix = "BenchmarkParityRealCorpusParse"
	if !strings.HasPrefix(parts[0], prefix) {
		return "", "", "", false
	}
	suite = strings.TrimPrefix(parts[0], prefix)
	backend = benchmarkSuffixRE.ReplaceAllString(parts[2], "")
	return suite, parts[1], backend, true
}

func buildReport(inputs []string, samples []sample) report {
	grouped := make(map[key][]sample)
	for _, s := range samples {
		k := key{suite: s.Suite, language: s.Language, backend: s.Backend}
		grouped[k] = append(grouped[k], s)
	}

	summaries := make([]benchmarkSummary, 0, len(grouped))
	for k, group := range grouped {
		summaries = append(summaries, benchmarkSummary{
			Suite:    k.suite,
			Language: k.language,
			Backend:  k.backend,
			Metrics:  summarizeMetrics(group),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		a, b := summaries[i], summaries[j]
		if a.Language != b.Language {
			return a.Language < b.Language
		}
		if a.Suite != b.Suite {
			return a.Suite < b.Suite
		}
		return a.Backend < b.Backend
	})

	return report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Inputs:      inputs,
		Samples:     len(samples),
		Benchmarks:  summaries,
		Languages:   buildLanguageReports(summaries),
	}
}

func summarizeMetrics(samples []sample) map[string]metricSummary {
	valuesByName := make(map[string][]float64)
	for _, s := range samples {
		for name, value := range s.Metrics {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}
			valuesByName[name] = append(valuesByName[name], value)
		}
	}
	out := make(map[string]metricSummary, len(valuesByName))
	for name, values := range valuesByName {
		out[name] = summarizeValues(values)
	}
	return out
}

func summarizeValues(values []float64) metricSummary {
	sort.Float64s(values)
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	median := values[len(values)/2]
	if len(values)%2 == 0 {
		median = (values[len(values)/2-1] + values[len(values)/2]) / 2
	}
	return metricSummary{
		Samples: len(values),
		Median:  median,
		Mean:    sum / float64(len(values)),
		Min:     values[0],
		Max:     values[len(values)-1],
	}
}

func buildLanguageReports(summaries []benchmarkSummary) []languageReport {
	byKey := make(map[key]benchmarkSummary, len(summaries))
	languages := map[string]bool{}
	for _, s := range summaries {
		byKey[key{suite: s.Suite, language: s.Language, backend: s.Backend}] = s
		languages[s.Language] = true
	}
	names := make([]string, 0, len(languages))
	for name := range languages {
		names = append(names, name)
	}
	sort.Strings(names)

	reports := make([]languageReport, 0, len(names))
	for _, name := range names {
		lr := languageReport{
			Language:       name,
			GoSuiteMetrics: map[string]map[string]float64{},
			CSuiteMetrics:  map[string]map[string]float64{},
		}
		lr.FullGoNanos = medianMetric(byKey, "Full", name, "gotreesitter", "ns/op")
		lr.FullCNanos = medianMetric(byKey, "Full", name, "tree-sitter-c", "ns/op")
		lr.EditGoNanos = medianMetric(byKey, "IncrementalSingleByteEdit", name, "gotreesitter", "ns/op")
		lr.EditCNanos = medianMetric(byKey, "IncrementalSingleByteEdit", name, "tree-sitter-c", "ns/op")
		lr.NoEditGoNanos = medianMetric(byKey, "IncrementalNoEdit", name, "gotreesitter", "ns/op")
		lr.NoEditCNanos = medianMetric(byKey, "IncrementalNoEdit", name, "tree-sitter-c", "ns/op")
		lr.FullRatio = ratio(lr.FullGoNanos, lr.FullCNanos)
		lr.EditRatio = ratio(lr.EditGoNanos, lr.EditCNanos)
		lr.NoEditRatio = ratio(lr.NoEditGoNanos, lr.NoEditCNanos)
		lr.WorstRatio = maxFloat(lr.FullRatio, lr.EditRatio, lr.NoEditRatio)
		for _, suite := range []string{"Full", "IncrementalSingleByteEdit", "IncrementalNoEdit"} {
			if metrics := medianMetrics(byKey, suite, name, "gotreesitter"); len(metrics) > 0 {
				lr.GoSuiteMetrics[suite] = metrics
			}
			if metrics := medianMetrics(byKey, suite, name, "tree-sitter-c"); len(metrics) > 0 {
				lr.CSuiteMetrics[suite] = metrics
			}
		}
		lr.TopAttribution = topAttribution(lr.GoSuiteMetrics, 6)
		reports = append(reports, lr)
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].WorstRatio != reports[j].WorstRatio {
			return reports[i].WorstRatio > reports[j].WorstRatio
		}
		return reports[i].Language < reports[j].Language
	})
	return reports
}

func medianMetric(byKey map[key]benchmarkSummary, suite, language, backend, metric string) float64 {
	s, ok := byKey[key{suite: suite, language: language, backend: backend}]
	if !ok {
		return 0
	}
	m, ok := s.Metrics[metric]
	if !ok {
		return 0
	}
	return m.Median
}

func medianMetrics(byKey map[key]benchmarkSummary, suite, language, backend string) map[string]float64 {
	s, ok := byKey[key{suite: suite, language: language, backend: backend}]
	if !ok {
		return nil
	}
	out := make(map[string]float64, len(s.Metrics))
	for name, m := range s.Metrics {
		out[name] = m.Median
	}
	return out
}

func ratio(goNanos, cNanos float64) float64 {
	if goNanos <= 0 || cNanos <= 0 {
		return 0
	}
	return goNanos / cNanos
}

func maxFloat(values ...float64) float64 {
	maxValue := 0.0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func topAttribution(suiteMetrics map[string]map[string]float64, limit int) []attributionBucket {
	names := []string{
		"parse_wall_ns/op",
		"reparse_ns/op",
		"reuse_ns/op",
		"unattributed_ns/op",
		"parser_loop_ns/op",
		"parser_accounted_ns/op",
		"parser_unattributed_ns/op",
		"token_next_ns/op",
		"action_dispatch_ns/op",
		"action_apply_ns/op",
		"action_lookup_ns/op",
		"glr_merge_ns/op",
		"glr_cull_ns/op",
		"result_accounted_ns/op",
		"result_select_ns/op",
		"result_tree_build_ns/op",
		"result_finalize_root_ns/op",
		"result_compatibility_ns/op",
		"result_parent_link_ns/op",
		"normalization_ns/op",
	}
	var buckets []attributionBucket
	for suite, metrics := range suiteMetrics {
		for _, name := range names {
			value := metrics[name]
			if value <= 0 {
				continue
			}
			buckets = append(buckets, attributionBucket{Suite: suite, Name: name, Value: value})
		}
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Value != buckets[j].Value {
			return buckets[i].Value > buckets[j].Value
		}
		if buckets[i].Suite != buckets[j].Suite {
			return buckets[i].Suite < buckets[j].Suite
		}
		return buckets[i].Name < buckets[j].Name
	})
	if len(buckets) > limit {
		buckets = buckets[:limit]
	}
	return buckets
}

func writeJSON(path string, r report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func writeMarkdown(path string, r report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Real Corpus Bench Report\n\n")
	fmt.Fprintf(&b, "Generated: `%s`\n\n", r.GeneratedAt)
	fmt.Fprintf(&b, "Samples: `%d`\n\n", r.Samples)
	fmt.Fprintf(&b, "| Language | Full Go | Full C | Full xC | Edit Go | Edit C | Edit xC | No-edit Go | No-edit C | No-edit xC | Top attribution |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
	for _, lr := range r.Languages {
		fmt.Fprintf(
			&b,
			"| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			lr.Language,
			formatNanos(lr.FullGoNanos),
			formatNanos(lr.FullCNanos),
			formatRatio(lr.FullRatio),
			formatNanos(lr.EditGoNanos),
			formatNanos(lr.EditCNanos),
			formatRatio(lr.EditRatio),
			formatNanos(lr.NoEditGoNanos),
			formatNanos(lr.NoEditCNanos),
			formatRatio(lr.NoEditRatio),
			formatAttribution(lr.TopAttribution),
		)
	}
	fmt.Fprintf(&b, "\n## Inputs\n\n")
	for _, input := range r.Inputs {
		fmt.Fprintf(&b, "- `%s`\n", input)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func formatNanos(v float64) string {
	if v <= 0 {
		return ""
	}
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.3fs", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.3fms", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.3fus", v/1e3)
	default:
		return fmt.Sprintf("%.0fns", v)
	}
}

func formatRatio(v float64) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2fx", v)
}

func formatAttribution(buckets []attributionBucket) string {
	if len(buckets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		parts = append(parts, fmt.Sprintf("%s/%s=%s", bucket.Suite, bucket.Name, formatNanos(bucket.Value)))
	}
	return strings.Join(parts, "<br>")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
