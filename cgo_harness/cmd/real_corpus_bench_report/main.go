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

var (
	benchmarkSuffixRE          = regexp.MustCompile(`-\d+$`)
	benchmarkRunRE             = regexp.MustCompile(`^BenchmarkParityRealCorpusParse(Full|IncrementalSingleByteEdit|IncrementalNoEdit)/([^/\s]+)(?:/|\s|$)`)
	benchmarkFailureRE         = regexp.MustCompile(`^--- FAIL: BenchmarkParityRealCorpusParse(?:Full|IncrementalSingleByteEdit|IncrementalNoEdit)/([^\s]+)`)
	structuralParityMismatchRE = regexp.MustCompile(`structural parity mismatch before benchmark for ([^/\s:]+)/([^:\s]+):\s*(.*)$`)
	knownMismatchRE            = regexp.MustCompile(`known mismatch:\s*(.*)$`)
	gotreesitterTruncatedRE    = regexp.MustCompile(`gotreesitter full ([^/\s:]+)/([^:\s]+) truncated:\s*(.*)$`)
	noCorpusFilesRE            = regexp.MustCompile(`real corpus filters selected no files under (.*)$`)
	skippedMismatchFilesRE     = regexp.MustCompile(`GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH skipped \d+/\d+ file\(s\):\s*(.*)$`)
	noParityCleanFilesRE       = regexp.MustCompile(`GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH selected no parity-clean files; skipped=(.*)$`)
	editSiteSkipRE             = regexp.MustCompile(`\bskip\s+(.+?): no single-byte incremental edit site matched benchmark constraints$`)
	editCandidateSkipRE        = regexp.MustCompile(`\bskip\s+(.+?)\s+candidate=(.+)$`)
	noSelectedEditSitesRE      = regexp.MustCompile(`no selected files had a single-byte incremental edit site matching benchmark constraints`)
	attemptedLanguagesRE       = regexp.MustCompile(`(?:^|\s)GTS_(?:REAL_CORPUS_BENCH|PARITY_BENCH)_LANGS=([^\s]+)`)
)

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
	Readiness      string                        `json:"readiness,omitempty"`
	Expectation    string                        `json:"expectation,omitempty"`
	Failure        string                        `json:"failure,omitempty"`
	FailureSource  string                        `json:"failure_source,omitempty"`
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

const (
	fullParseTargetRatio   = 1.0
	incrementalTargetRatio = 1.0
	logScannerMaxTokenSize = 64 * 1024 * 1024

	readinessMeetsTargets          = "meets-current-targets"
	readinessNeedsFullParseWork    = "needs-full-parse-work"
	readinessNeedsIncrementalWork  = "needs-incremental-work"
	readinessNeedsFullAndIncrWork  = "needs-full-and-incremental-work"
	readinessIncompleteMeasurement = "incomplete-measurement"
	readinessParityBlocked         = "parity-blocked"
	readinessBenchmarkRunAborted   = "benchmark-run-aborted"
	readinessCorpusUnavailable     = "corpus-unavailable"
)

type failureSummary struct {
	Language string `json:"language"`
	Kind     string `json:"kind,omitempty"`
	File     string `json:"file,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Source   string `json:"source,omitempty"`
}

type attributionBucket struct {
	Suite string  `json:"suite"`
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type report struct {
	GeneratedAt        string             `json:"generated_at"`
	Inputs             []string           `json:"inputs"`
	Samples            int                `json:"samples"`
	AttemptedLanguages []string           `json:"attempted_languages,omitempty"`
	Failures           []failureSummary   `json:"failures,omitempty"`
	Benchmarks         []benchmarkSummary `json:"benchmarks"`
	Languages          []languageReport   `json:"languages"`
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
	flag.StringVar(&outMD, "out-md", "", "optional Markdown report path")
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
	failures, err := parseFailureFiles(files)
	if err != nil {
		fatalf("parse benchmark failures: %v", err)
	}
	attemptedLanguages, err := parseAttemptedLanguagesFiles(files)
	if err != nil {
		fatalf("parse attempted languages: %v", err)
	}
	if len(samples) == 0 && len(failures) == 0 && len(attemptedLanguages) == 0 {
		fatalf("no BenchmarkParityRealCorpusParse lines or parity failure lines found")
	}

	r := buildReport(files, samples, failures, attemptedLanguages)
	if err := writeJSON(outJSON, r); err != nil {
		fatalf("write %s: %v", outJSON, err)
	}
	if outMD != "" {
		if err := writeMarkdown(outMD, r); err != nil {
			fatalf("write %s: %v", outMD, err)
		}
	}
	fmt.Printf("wrote report json: %s\n", outJSON)
	if outMD != "" {
		fmt.Printf("wrote report markdown: %s\n", outMD)
	}
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

func parseAttemptedLanguagesFiles(files []string) ([]string, error) {
	seen := map[string]bool{}
	for _, path := range files {
		fileLanguages, err := parseAttemptedLanguagesFile(path)
		if err != nil {
			return nil, err
		}
		for _, language := range fileLanguages {
			if isAggregateLanguageSelector(language) {
				continue
			}
			seen[language] = true
		}
	}
	languages := make([]string, 0, len(seen))
	for language := range seen {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages, nil
}

func parseAttemptedLanguagesFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := map[string]bool{}
	scanner := newLogScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		for _, language := range parseAttemptedLanguagesFromLine(line) {
			seen[language] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	languages := make([]string, 0, len(seen))
	for language := range seen {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages, nil
}

func parseAttemptedLanguagesFromLine(line string) []string {
	matches := attemptedLanguagesRE.FindAllStringSubmatch(line, -1)
	var languages []string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		languages = append(languages, parseLanguagesCSV(match[1])...)
	}
	return languages
}

func parseLanguagesCSV(raw string) []string {
	raw = strings.Trim(raw, `"'`)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func isAggregateLanguageSelector(language string) bool {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", "all", "all206", "top50":
		return true
	default:
		return false
	}
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
	scanner := newLogScanner(f)
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

func parseFailureFiles(files []string) ([]failureSummary, error) {
	seen := map[string]bool{}
	var failures []failureSummary
	for _, path := range files {
		fileFailures, err := parseFailureFile(path)
		if err != nil {
			return nil, err
		}
		for _, failure := range fileFailures {
			key := strings.Join([]string{failure.Language, failure.Kind, failure.File, failure.Detail}, "\x00")
			if seen[key] {
				continue
			}
			seen[key] = true
			failures = append(failures, failure)
		}
	}
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Language != failures[j].Language {
			return failures[i].Language < failures[j].Language
		}
		if failures[i].File != failures[j].File {
			return failures[i].File < failures[j].File
		}
		return failures[i].Source < failures[j].Source
	})
	return failures, nil
}

func parseFailureFile(path string) ([]failureSummary, error) {
	if strings.EqualFold(filepath.Base(path), "metadata.txt") {
		return parseMetadataFailureFile(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var failures []failureSummary
	scanner := newLogScanner(f)
	lineNo := 0
	currentSuite := ""
	currentLanguage := ""
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if m := benchmarkRunRE.FindStringSubmatch(line); m != nil {
			currentSuite = m[1]
			currentLanguage = m[2]
			continue
		}
		if m := benchmarkFailureRE.FindStringSubmatch(line); m != nil {
			currentSuite = ""
			currentLanguage = m[1]
			continue
		}
		m := structuralParityMismatchRE.FindStringSubmatch(line)
		if m != nil {
			failures = append(failures, failureSummary{
				Language: m[1],
				Kind:     readinessParityBlocked,
				File:     m[2],
				Detail:   strings.TrimSpace(m[3]),
				Source:   fmt.Sprintf("%s:%d", path, lineNo),
			})
			continue
		}
		m = knownMismatchRE.FindStringSubmatch(line)
		if m != nil && currentLanguage != "" {
			failures = append(failures, failureSummary{
				Language: currentLanguage,
				Kind:     readinessParityBlocked,
				Detail:   "known mismatch: " + strings.TrimSpace(m[1]),
				Source:   fmt.Sprintf("%s:%d", path, lineNo),
			})
			continue
		}
		m = gotreesitterTruncatedRE.FindStringSubmatch(line)
		if m != nil {
			failures = append(failures, failureSummary{
				Language: m[1],
				Kind:     readinessParityBlocked,
				File:     m[2],
				Detail:   "gotreesitter full parse truncated: " + strings.TrimSpace(m[3]),
				Source:   fmt.Sprintf("%s:%d", path, lineNo),
			})
			continue
		}
		m = noCorpusFilesRE.FindStringSubmatch(line)
		if m != nil && currentLanguage != "" {
			failures = append(failures, failureSummary{
				Language: currentLanguage,
				Kind:     readinessCorpusUnavailable,
				Detail:   strings.TrimSpace(m[0]),
				Source:   fmt.Sprintf("%s:%d", path, lineNo),
			})
			continue
		}
		m = skippedMismatchFilesRE.FindStringSubmatch(line)
		if m != nil && currentLanguage != "" {
			detail := "strict parity filtered files; skipped=" + strings.TrimSpace(m[1])
			failures = append(failures, failureSummary{
				Language: currentLanguage,
				Kind:     readinessParityBlocked,
				File:     firstNoParityCleanFailureFile(currentLanguage, m[1]),
				Detail:   detail,
				Source:   fmt.Sprintf("%s:%d", path, lineNo),
			})
			continue
		}
		m = noParityCleanFilesRE.FindStringSubmatch(line)
		if m != nil && currentLanguage != "" {
			detail := "selected no parity-clean files; skipped=" + strings.TrimSpace(m[1])
			failures = append(failures, failureSummary{
				Language: currentLanguage,
				Kind:     readinessParityBlocked,
				File:     firstNoParityCleanFailureFile(currentLanguage, m[1]),
				Detail:   detail,
				Source:   fmt.Sprintf("%s:%d", path, lineNo),
			})
			continue
		}
		if currentSuite != "IncrementalSingleByteEdit" || currentLanguage == "" {
			continue
		}
		if m := editCandidateSkipRE.FindStringSubmatch(line); m != nil {
			detail := "single-byte edit candidate skipped: candidate=" + strings.TrimSpace(m[2])
			kind := readinessIncompleteMeasurement
			if realCorpusEditCandidateParityBlocked(detail) {
				kind = readinessParityBlocked
				detail = "single-byte edit parity failed: candidate=" + strings.TrimSpace(m[2])
			}
			failures = append(failures, failureSummary{
				Language: currentLanguage,
				Kind:     kind,
				File:     normalizeRealCorpusFailureFile(currentLanguage, m[1]),
				Detail:   detail,
				Source:   fmt.Sprintf("%s:%d", path, lineNo),
			})
			continue
		}
		if m := editSiteSkipRE.FindStringSubmatch(line); m != nil {
			failures = append(failures, failureSummary{
				Language: currentLanguage,
				Kind:     readinessIncompleteMeasurement,
				File:     normalizeRealCorpusFailureFile(currentLanguage, m[1]),
				Detail:   "no single-byte incremental edit site matched benchmark constraints",
				Source:   fmt.Sprintf("%s:%d", path, lineNo),
			})
			continue
		}
		if noSelectedEditSitesRE.MatchString(line) {
			failures = append(failures, failureSummary{
				Language: currentLanguage,
				Kind:     readinessIncompleteMeasurement,
				Detail:   strings.TrimSpace(noSelectedEditSitesRE.FindString(line)),
				Source:   fmt.Sprintf("%s:%d", path, lineNo),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return failures, nil
}

func firstNoParityCleanFailureFile(language, skipped string) string {
	skipped = strings.TrimSpace(skipped)
	if skipped == "" {
		return ""
	}
	idx := strings.Index(skipped, "(")
	if idx < 0 {
		return normalizeRealCorpusFailureFile(language, skipped)
	}
	return normalizeRealCorpusFailureFile(language, skipped[:idx])
}

func realCorpusEditCandidateParityBlocked(detail string) bool {
	return strings.Contains(detail, "gotreesitter incremental mismatch against fresh") ||
		strings.Contains(detail, "C incremental mismatch against fresh") ||
		strings.Contains(detail, "fresh structural mismatch")
}

func normalizeRealCorpusFailureFile(language, file string) string {
	file = filepath.ToSlash(strings.TrimSpace(file))
	if file == "" {
		return ""
	}
	if prefix := language + "/"; strings.HasPrefix(file, prefix) {
		return strings.TrimPrefix(file, prefix)
	}
	if marker := "/" + language + "/"; strings.Contains(file, marker) {
		_, after, _ := strings.Cut(file, marker)
		return after
	}
	return file
}

func parseMetadataFailureFile(path string) ([]failureSummary, error) {
	values, err := parseMetadataFile(path)
	if err != nil {
		return nil, err
	}
	exitCode := strings.TrimSpace(values["exit_code"])
	stateError := strings.TrimSpace(values["state_error"])
	if exitCode == "" || exitCode == "0" {
		if stateError == "" {
			return nil, nil
		}
	}
	language := metadataLanguage(values)
	if language == "" {
		return nil, nil
	}
	command := strings.TrimSpace(values["command"])
	detail := fmt.Sprintf("benchmark container exited with code %s", exitCode)
	if exitCode == "" {
		detail = "benchmark container ended without an exit code"
	} else if exitCode == "124" && strings.Contains(command, "timeout ") {
		detail = "benchmark command timed out with exit code 124"
	} else if exitCode == "137" && strings.Contains(command, "timeout ") {
		detail = "benchmark command exceeded timeout kill grace and exited with code 137"
	}
	if oomKilled := strings.TrimSpace(values["oom_killed"]); oomKilled != "" {
		detail += fmt.Sprintf(" (oom_killed=%s)", oomKilled)
	}
	if stateError != "" {
		detail += fmt.Sprintf(": %s", stateError)
	}
	return []failureSummary{{
		Language: language,
		Kind:     readinessBenchmarkRunAborted,
		Detail:   detail,
		Source:   path,
	}}, nil
}

func parseMetadataFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := map[string]string{}
	scanner := newLogScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func newLogScanner(f *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), logScannerMaxTokenSize)
	return scanner
}

func metadataLanguage(values map[string]string) string {
	label := strings.TrimSpace(values["label"])
	if strings.HasPrefix(label, "real-corpus-bench-") {
		return strings.TrimPrefix(label, "real-corpus-bench-")
	}
	for _, language := range parseAttemptedLanguagesFromLine(values["command"]) {
		if !isAggregateLanguageSelector(language) {
			return language
		}
	}
	return ""
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

func buildReport(inputs []string, samples []sample, failures []failureSummary, attemptedLanguages []string) report {
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
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		Inputs:             inputs,
		Samples:            len(samples),
		AttemptedLanguages: attemptedLanguages,
		Failures:           failures,
		Benchmarks:         summaries,
		Languages:          buildLanguageReports(summaries, failures, attemptedLanguages),
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

func buildLanguageReports(summaries []benchmarkSummary, failures []failureSummary, attemptedLanguages []string) []languageReport {
	byKey := make(map[key]benchmarkSummary, len(summaries))
	languages := map[string]bool{}
	for _, s := range summaries {
		byKey[key{suite: s.Suite, language: s.Language, backend: s.Backend}] = s
		languages[s.Language] = true
	}
	attemptedLanguageSet := map[string]bool{}
	for _, language := range attemptedLanguages {
		if !isAggregateLanguageSelector(language) {
			languages[language] = true
			attemptedLanguageSet[language] = true
		}
	}
	firstFailureByLanguage := map[string]failureSummary{}
	for _, failure := range failures {
		if existing, ok := firstFailureByLanguage[failure.Language]; !ok || failurePrecedence(failure) < failurePrecedence(existing) {
			firstFailureByLanguage[failure.Language] = failure
		}
		languages[failure.Language] = true
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
		lr.Readiness, lr.Expectation = classifyReadiness(lr)
		if attemptedLanguageSet[name] && lr.FullRatio <= 0 && lr.EditRatio <= 0 && lr.NoEditRatio <= 0 {
			lr.Expectation = "Benchmark command selected this language but emitted no full, single-byte edit, or no-edit benchmark samples and no parity failure; treat this as a harness coverage gap until benchmark registration or filtering is fixed."
			lr.Failure = "benchmark command selected language but emitted no BenchmarkParityRealCorpusParse samples or parity failure"
		}
		if failure, ok := firstFailureByLanguage[name]; ok && failureShouldOverrideLanguageReport(failure, lr) {
			lr.Readiness, lr.Expectation = classifyFailure(failure)
			lr.Failure = formatFailureSummary(failure)
			lr.FailureSource = failure.Source
		}
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
		if failureRank(reports[i].Readiness) != failureRank(reports[j].Readiness) {
			return failureRank(reports[i].Readiness) < failureRank(reports[j].Readiness)
		}
		if reports[i].Readiness == readinessParityBlocked && reports[j].Readiness != readinessParityBlocked {
			return true
		}
		if reports[i].Readiness != readinessParityBlocked && reports[j].Readiness == readinessParityBlocked {
			return false
		}
		if reports[i].WorstRatio != reports[j].WorstRatio {
			return reports[i].WorstRatio > reports[j].WorstRatio
		}
		return reports[i].Language < reports[j].Language
	})
	return reports
}

func failureShouldOverrideLanguageReport(failure failureSummary, lr languageReport) bool {
	if failure.Kind == readinessParityBlocked {
		if languageReportHasCompleteMeasurements(lr) &&
			strings.Contains(failure.Detail, "single-byte edit parity failed: candidate=") {
			return false
		}
		return true
	}
	return !languageReportHasCompleteMeasurements(lr)
}

func languageReportHasCompleteMeasurements(lr languageReport) bool {
	return lr.FullRatio > 0 && lr.EditRatio > 0 && lr.NoEditRatio > 0
}

func formatFailureSummary(failure failureSummary) string {
	file := strings.TrimSpace(failure.File)
	detail := strings.TrimSpace(failure.Detail)
	switch {
	case file == "":
		return detail
	case detail == "":
		return file
	default:
		return file + ": " + detail
	}
}

func failureRank(readiness string) int {
	switch readiness {
	case readinessParityBlocked:
		return 0
	case readinessBenchmarkRunAborted:
		return 1
	case readinessCorpusUnavailable:
		return 2
	default:
		return 3
	}
}

func failurePrecedence(failure failureSummary) int {
	switch failure.Kind {
	case readinessParityBlocked:
		switch {
		case strings.Contains(failure.Detail, "strict parity filtered files"):
			return 0
		case strings.Contains(failure.Detail, "selected no parity-clean files"):
			return 0
		case strings.Contains(failure.Detail, "single-byte edit parity failed: candidate="):
			return 2
		default:
			return 1
		}
	case readinessCorpusUnavailable:
		return 3
	case readinessBenchmarkRunAborted:
		return 4
	case readinessIncompleteMeasurement:
		if strings.Contains(failure.Detail, "candidate=") {
			return 5
		}
		if strings.TrimSpace(failure.File) != "" {
			return 6
		}
		return 7
	default:
		return 8
	}
}

func classifyFailure(failure failureSummary) (string, string) {
	switch failure.Kind {
	case readinessCorpusUnavailable:
		return readinessCorpusUnavailable,
			"No source files matched the lock-backed corpus filter; do not infer parser readiness until a usable public corpus is selected for this language."
	case readinessBenchmarkRunAborted:
		return readinessBenchmarkRunAborted,
			"The benchmark run ended before the full measurement set completed; treat any partial metrics as diagnostic only."
	case readinessIncompleteMeasurement:
		return readinessIncompleteMeasurement,
			fmt.Sprintf("The single-byte edit benchmark lane was selected but skipped: %s; full/no-edit metrics remain usable, but edit readiness is unknown.", formatFailureSummary(failure))
	default:
		if strings.Contains(failure.Detail, "single-byte edit parity failed") {
			return readinessParityBlocked,
				"Single-byte incremental parity failed during edit-candidate verification; do not infer incremental edit performance readiness until incremental parity matches fresh parse on the real corpus."
		}
		return readinessParityBlocked,
			"Strict fresh parity failed before or during benchmarking; do not infer performance readiness until parity is fixed or a parity-clean corpus subset is selected."
	}
}

func classifyReadiness(lr languageReport) (string, string) {
	var missing []string
	if lr.FullRatio <= 0 {
		missing = append(missing, "full")
	}
	if lr.EditRatio <= 0 {
		missing = append(missing, "single-byte edit")
	}
	if lr.NoEditRatio <= 0 {
		missing = append(missing, "no-edit incremental")
	}
	if len(missing) > 0 {
		return readinessIncompleteMeasurement,
			fmt.Sprintf("Missing %s benchmark data; do not infer readiness from this report.", strings.Join(missing, ", "))
	}

	fullOK := lr.FullRatio <= fullParseTargetRatio
	incrementalOK := lr.EditRatio <= incrementalTargetRatio && lr.NoEditRatio <= incrementalTargetRatio
	switch {
	case fullOK && incrementalOK:
		return readinessMeetsTargets,
			"Full parse is at or faster than C and both incremental lanes are at or faster than C on this real-corpus run."
	case !fullOK && !incrementalOK:
		return readinessNeedsFullAndIncrWork,
			"Full parse is slower than C and at least one incremental lane is slower than C on this real-corpus run."
	case !fullOK:
		return readinessNeedsFullParseWork,
			"Incremental lanes meet target, but full parse is slower than C on this real-corpus run."
	default:
		return readinessNeedsIncrementalWork,
			"Full parse meets target, but at least one incremental lane is slower than C on this real-corpus run."
	}
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
	fmt.Fprintf(&b, "Targets: full parse <= `%.1fx` C, single-byte edit <= `%.1fx` C, no-edit incremental <= `%.1fx` C.\n\n",
		fullParseTargetRatio, incrementalTargetRatio, incrementalTargetRatio)
	fmt.Fprintf(&b, "| Language | Readiness | Full Go | Full C | Full xC | Edit Go | Edit C | Edit xC | No-edit Go | No-edit C | No-edit xC | Top attribution | Failure |\n")
	fmt.Fprintf(&b, "|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|---|\n")
	for _, lr := range r.Languages {
		fmt.Fprintf(
			&b,
			"| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			lr.Language,
			lr.Readiness,
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
			formatFailure(lr.Failure),
		)
	}
	if len(r.Benchmarks) > 0 {
		fmt.Fprintf(&b, "\n## Benchmark Metrics\n\n")
		fmt.Fprintf(&b, "| Language | Suite | Backend | Samples | ns/op | B/op | allocs/op | MB/s | Other medians |\n")
		fmt.Fprintf(&b, "|---|---|---|---:|---:|---:|---:|---:|---|\n")
		for _, summary := range r.Benchmarks {
			fmt.Fprintf(
				&b,
				"| %s | %s | %s | %d | %s | %s | %s | %s | %s |\n",
				summary.Language,
				summary.Suite,
				summary.Backend,
				metricSampleCount(summary.Metrics),
				formatMetric(summary.Metrics, "ns/op"),
				formatMetric(summary.Metrics, "B/op"),
				formatMetric(summary.Metrics, "allocs/op"),
				formatMetric(summary.Metrics, "MB/s"),
				formatOtherMetrics(summary.Metrics),
			)
		}
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

func metricSampleCount(metrics map[string]metricSummary) int {
	maxSamples := 0
	for _, summary := range metrics {
		if summary.Samples > maxSamples {
			maxSamples = summary.Samples
		}
	}
	return maxSamples
}

func formatMetric(metrics map[string]metricSummary, name string) string {
	summary, ok := metrics[name]
	if !ok {
		return ""
	}
	switch name {
	case "ns/op":
		return formatNanos(summary.Median)
	case "B/op", "allocs/op":
		return fmt.Sprintf("%.0f", summary.Median)
	case "MB/s":
		return fmt.Sprintf("%.2f", summary.Median)
	default:
		return formatNumber(summary.Median)
	}
}

func formatOtherMetrics(metrics map[string]metricSummary) string {
	standard := map[string]bool{
		"ns/op":     true,
		"B/op":      true,
		"allocs/op": true,
		"MB/s":      true,
	}
	names := make([]string, 0, len(metrics))
	for name := range metrics {
		if !standard[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%s", name, formatNumber(metrics[name].Median)))
	}
	return strings.Join(parts, "<br>")
}

func formatNumber(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.3f", v)
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

func formatFailure(failure string) string {
	if strings.TrimSpace(failure) == "" {
		return ""
	}
	failure = strings.ReplaceAll(failure, "\n", " ")
	if len(failure) > 160 {
		failure = failure[:157] + "..."
	}
	return failure
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
