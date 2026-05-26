package gotreesitter

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func buildSourceBenchmarkInput(terms int) []byte {
	var b strings.Builder
	b.Grow(terms * 3)
	for i := 0; i < terms; i++ {
		if i > 0 {
			b.WriteByte('+')
		}
		b.WriteString(strconv.Itoa(i % 10))
	}
	return []byte(b.String())
}

func BenchmarkParseSourceBytesArithmetic(b *testing.B) {
	parser := NewParser(buildArithmeticLanguage())
	source := BytesSource(buildSourceBenchmarkInput(2048))

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tree, err := parser.ParseSource(source)
		if err != nil {
			b.Fatalf("ParseSource failed: %v", err)
		}
		tree.Release()
	}
}

func BenchmarkParseSourceFileArithmetic(b *testing.B) {
	parser := NewParser(buildArithmeticLanguage())
	path := filepath.Join(b.TempDir(), "input.txt")
	if err := os.WriteFile(path, buildSourceBenchmarkInput(2048), 0o644); err != nil {
		b.Fatalf("WriteFile failed: %v", err)
	}
	source, err := NewFileSource(path)
	if err != nil {
		b.Fatalf("NewFileSource failed: %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tree, err := parser.ParseSource(source)
		if err != nil {
			b.Fatalf("ParseSource failed: %v", err)
		}
		tree.Release()
	}
}

func BenchmarkParseSourceFileArithmeticWithTokenSourceFactory(b *testing.B) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	path := filepath.Join(b.TempDir(), "input.txt")
	if err := os.WriteFile(path, buildSourceBenchmarkInput(2048), 0o644); err != nil {
		b.Fatalf("WriteFile failed: %v", err)
	}
	source, err := NewFileSource(path)
	if err != nil {
		b.Fatalf("NewFileSource failed: %v", err)
	}
	factory := arithmeticTokenSourceFactory(lang)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tree, err := parser.ParseSourceWithTokenSourceFactory(source, factory)
		if err != nil {
			b.Fatalf("ParseSourceWithTokenSourceFactory failed: %v", err)
		}
		tree.Release()
	}
}
