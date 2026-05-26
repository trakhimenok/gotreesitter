package grammars

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

type countingSource struct {
	data     []byte
	releases int
}

func (s *countingSource) Len() uint64 {
	return uint64(len(s.data))
}

func (s *countingSource) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, gotreesitter.ErrInvalidSourceRange
	}
	if off >= int64(len(s.data)) {
		return 0, io.EOF
	}
	n := copy(p, s.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (s *countingSource) Slice(start, end uint64) (gotreesitter.SourceSlice, bool, error) {
	if end < start || end > uint64(len(s.data)) {
		return gotreesitter.SourceSlice{}, false, gotreesitter.ErrInvalidSourceRange
	}
	return gotreesitter.SourceSlice{
		Bytes: s.data[int(start):int(end)],
		Release: func() {
			s.releases++
		},
	}, true, nil
}

func TestParseFile(t *testing.T) {
	bt, err := ParseFile("main.go", []byte("package main\n\nfunc main() {}\n"))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFile returned nil root")
	}
	if got := bt.NodeType(root); got != "source_file" {
		t.Errorf("root type = %q, want %q", got, "source_file")
	}
}

func TestParseFileUnknownExtension(t *testing.T) {
	_, err := ParseFile("file.xyz", []byte("hello"))
	if err == nil {
		t.Fatal("expected error for unknown extension")
	}
}

func TestParseFileEmptySource(t *testing.T) {
	bt, err := ParseFile("main.go", []byte{})
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()
}

func TestParseFilePython(t *testing.T) {
	bt, err := ParseFile("script.py", []byte("def hello():\n    pass\n"))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	if bt.RootNode() == nil {
		t.Fatal("ParseFile returned nil root for Python")
	}

	found := false
	gotreesitter.Walk(bt.RootNode(), func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if bt.NodeType(node) == "function_definition" {
			found = true
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if !found {
		t.Error("expected to find function_definition in Python parse tree")
	}
}

func TestParseFileSource(t *testing.T) {
	source := &countingSource{data: []byte("package main\n\nfunc main() {}\n")}
	bt, err := ParseFileSource("main.go", source)
	if err != nil {
		t.Fatalf("ParseFileSource error: %v", err)
	}
	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFileSource returned nil root")
	}
	if got := bt.NodeType(root); got != "source_file" {
		t.Errorf("root type = %q, want %q", got, "source_file")
	}
	if source.releases != 0 {
		t.Fatalf("source released before BoundTree.Release: got %d, want 0", source.releases)
	}
	bt.Release()
	if source.releases != 1 {
		t.Fatalf("source releases after BoundTree.Release = %d, want 1", source.releases)
	}
}

func TestParseFileSourceUnknownExtensionDoesNotBorrow(t *testing.T) {
	source := &countingSource{data: []byte("hello")}
	_, err := ParseFileSource("file.xyz", source)
	if err == nil {
		t.Fatal("expected error for unknown extension")
	}
	if source.releases != 0 {
		t.Fatalf("unknown extension should not borrow source; release count = %d", source.releases)
	}
}

func TestParseFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	bt, err := ParseFilePath(path)
	if err != nil {
		t.Fatalf("ParseFilePath error: %v", err)
	}
	defer bt.Release()
	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFilePath returned nil root")
	}
	if got := bt.NodeType(root); got != "source_file" {
		t.Errorf("root type = %q, want %q", got, "source_file")
	}
}

func TestParseFilePooled(t *testing.T) {
	bt, err := ParseFilePooled("main.go", []byte("package main\n\nfunc main() {}\n"))
	if err != nil {
		t.Fatalf("ParseFilePooled error: %v", err)
	}
	defer bt.Release()
	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFilePooled returned nil root")
	}
	if got := bt.NodeType(root); got != "source_file" {
		t.Errorf("root type = %q, want %q", got, "source_file")
	}
}

func TestParseFileSourcePooled(t *testing.T) {
	source := &countingSource{data: []byte("package main\n\nfunc main() {}\n")}
	bt, err := ParseFileSourcePooled("main.go", source)
	if err != nil {
		t.Fatalf("ParseFileSourcePooled error: %v", err)
	}
	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFileSourcePooled returned nil root")
	}
	if got := bt.NodeType(root); got != "source_file" {
		t.Errorf("root type = %q, want %q", got, "source_file")
	}
	bt.Release()
	if source.releases != 1 {
		t.Fatalf("source releases after BoundTree.Release = %d, want 1", source.releases)
	}
}

func TestParseFilePathPooled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	bt, err := ParseFilePathPooled(path, gotreesitter.WithFileSourceMmap(false))
	if err != nil {
		t.Fatalf("ParseFilePathPooled error: %v", err)
	}
	defer bt.Release()
	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFilePathPooled returned nil root")
	}
	if got := bt.NodeType(root); got != "source_file" {
		t.Errorf("root type = %q, want %q", got, "source_file")
	}
}

func TestParseFilePooledUnknownExtension(t *testing.T) {
	_, err := ParseFilePooled("file.xyz", []byte("hello"))
	if err == nil {
		t.Fatal("expected error for unknown extension")
	}
}

func TestParseFilePooledReusesPool(t *testing.T) {
	bt1, err := ParseFilePooled("a.go", []byte("package a\n"))
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	bt1.Release()
	bt2, err := ParseFilePooled("b.go", []byte("package b\n"))
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	bt2.Release()
}

func TestParseFilePooledConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			src := []byte(fmt.Sprintf("package p%d\n\nfunc f%d() {}\n", n, n))
			bt, err := ParseFilePooled(fmt.Sprintf("f%d.go", n), src)
			if err != nil {
				t.Errorf("concurrent parse %d: %v", n, err)
				return
			}
			bt.Release()
		}(i)
	}
	wg.Wait()
}
