package gotreesitter_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Warm-parse benchmark: reuse one Parser to parse many files. This mirrors
// the indexer workload (canopy, IDE). If the arena pool works correctly,
// the per-parse allocation should drop sharply after the first parse.
func BenchmarkSelfParseWarmReuse(b *testing.B) {
	files := []string{
		"parser.go",
		"parser_reduce.go",
		"parser_test.go",
		"query_test.go",
		"parser_result_csharp_query.go",
		"parser_result_scala_template.go",
	}
	srcs := make([][]byte, len(files))
	var totalBytes int
	for i, n := range files {
		data, err := os.ReadFile(n)
		if err != nil {
			b.Fatalf("read %s: %v", n, err)
		}
		srcs[i] = data
		totalBytes += len(data)
	}

	b.Run("cold_fresh_parser_each_time", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(totalBytes))
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, src := range srcs {
				parser := gotreesitter.NewParser(grammars.GoLanguage())
				tree, _ := parser.Parse(src)
				_ = tree
			}
		}
		b.StopTimer()
		runtime.ReadMemStats(&after)
		totalAlloc := after.TotalAlloc - before.TotalAlloc
		b.ReportMetric(float64(totalAlloc)/float64(b.N), "heap_bytes/iter")
		b.ReportMetric(float64(totalAlloc)/float64(b.N*int(len(srcs))*int(totalBytes/len(srcs))), "heap_ratio")
	})

	b.Run("warm_one_parser_reused", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(totalBytes))
		parser := gotreesitter.NewParser(grammars.GoLanguage())
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, src := range srcs {
				tree, _ := parser.Parse(src)
				_ = tree
			}
		}
		b.StopTimer()
		runtime.ReadMemStats(&after)
		totalAlloc := after.TotalAlloc - before.TotalAlloc
		b.ReportMetric(float64(totalAlloc)/float64(b.N), "heap_bytes/iter")
	})

	b.Run("warm_with_drain_between_rounds", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(totalBytes))
		parser := gotreesitter.NewParser(grammars.GoLanguage())
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, src := range srcs {
				tree, _ := parser.Parse(src)
				_ = tree
			}
			// Note: DrainArenaPools is from PR #25; this benchmark intentionally
			// calls something equivalent if we've merged it. Skip-check if not.
			// For now we rely on automatic pool churn.
			runtime.GC()
		}
		b.StopTimer()
		runtime.ReadMemStats(&after)
		totalAlloc := after.TotalAlloc - before.TotalAlloc
		b.ReportMetric(float64(totalAlloc)/float64(b.N), "heap_bytes/iter")
	})
}
