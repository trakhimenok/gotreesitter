package gotreesitter

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type countingSliceSource struct {
	data     []byte
	releases int
}

func (s *countingSliceSource) Len() uint64 {
	return uint64(len(s.data))
}

func (s *countingSliceSource) ReadAt(p []byte, off int64) (int, error) {
	return readAtBytes(s.data, p, off)
}

func (s *countingSliceSource) Slice(start, end uint64) (SourceSlice, bool, error) {
	if end < start || end > uint64(len(s.data)) {
		return SourceSlice{}, false, ErrInvalidSourceRange
	}
	return SourceSlice{
		Bytes: s.data[int(start):int(end)],
		Release: func() {
			s.releases++
		},
	}, true, nil
}

type readAtOnlySource struct {
	data        []byte
	sliceCalled bool
	readCalls   int
}

func (s *readAtOnlySource) Len() uint64 {
	return uint64(len(s.data))
}

func (s *readAtOnlySource) ReadAt(p []byte, off int64) (int, error) {
	s.readCalls++
	return readAtBytes(s.data, p, off)
}

func (s *readAtOnlySource) Slice(start, end uint64) (SourceSlice, bool, error) {
	s.sliceCalled = true
	if end < start || end > uint64(len(s.data)) {
		return SourceSlice{}, false, ErrInvalidSourceRange
	}
	return SourceSlice{}, false, nil
}

type mismatchedSliceSource struct {
	data     []byte
	releases int
}

func (s *mismatchedSliceSource) Len() uint64 {
	return uint64(len(s.data))
}

func (s *mismatchedSliceSource) ReadAt(p []byte, off int64) (int, error) {
	return readAtBytes(s.data, p, off)
}

func (s *mismatchedSliceSource) Slice(start, end uint64) (SourceSlice, bool, error) {
	return SourceSlice{
		Bytes: s.data[:len(s.data)-1],
		Release: func() {
			s.releases++
		},
	}, true, nil
}

type hugeSource struct {
	sliceCalled bool
	readCalled  bool
}

func (s *hugeSource) Len() uint64 {
	return uint64(^uint32(0)) + 1
}

func (s *hugeSource) ReadAt(p []byte, off int64) (int, error) {
	s.readCalled = true
	return 0, io.EOF
}

func (s *hugeSource) Slice(start, end uint64) (SourceSlice, bool, error) {
	s.sliceCalled = true
	return SourceSlice{}, false, nil
}

type stalledSource struct{}

func (s stalledSource) Len() uint64 {
	return 1
}

func (s stalledSource) ReadAt(p []byte, off int64) (int, error) {
	return 0, nil
}

func (s stalledSource) Slice(start, end uint64) (SourceSlice, bool, error) {
	return SourceSlice{}, false, nil
}

func readAtBytes(data []byte, p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, ErrInvalidSourceRange
	}
	if off >= int64(len(data)) {
		return 0, io.EOF
	}
	n := copy(p, data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func requireArithmeticTree(t *testing.T, tree *Tree, lang *Language) *Node {
	t.Helper()
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("expected root node, got nil")
	}
	if root.HasError() {
		t.Fatalf("unexpected parse error: %s", root.SExpr(lang))
	}
	return root
}

func supportsMmapFileSource() bool {
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "netbsd", "openbsd", "dragonfly", "solaris":
		return true
	default:
		return false
	}
}

func arithmeticTokenSourceFactory(lang *Language) func([]byte) TokenSource {
	seed := NewParser(lang)
	return func(src []byte) TokenSource {
		lexer := NewLexer(lang.LexStates, src)
		return acquireDFATokenSource(lexer, lang, seed.lookupActionIndex, seed.hasKeywordState)
	}
}

func TestBytesSourceSliceAndReadAt(t *testing.T) {
	src := BytesSource([]byte("abc"))
	if got, want := src.Len(), uint64(3); got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	slice, ok, err := src.Slice(1, 3)
	if err != nil {
		t.Fatalf("Slice failed: %v", err)
	}
	if !ok {
		t.Fatal("Slice returned ok=false")
	}
	if got, want := string(slice.Bytes), "bc"; got != want {
		t.Fatalf("Slice bytes = %q, want %q", got, want)
	}

	buf := make([]byte, 2)
	n, err := src.ReadAt(buf, 1)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if got, want := n, 2; got != want {
		t.Fatalf("ReadAt n = %d, want %d", got, want)
	}
	if got, want := string(buf), "bc"; got != want {
		t.Fatalf("ReadAt bytes = %q, want %q", got, want)
	}
}

func TestReaderAtSourceMaterializes(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source, err := NewReaderAtSource(bytes.NewReader([]byte("1+2+3")), uint64(len("1+2+3")))
	if err != nil {
		t.Fatalf("NewReaderAtSource failed: %v", err)
	}

	tree, err := parser.ParseSource(source)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}
	defer tree.Release()
	requireArithmeticTree(t, tree, lang)
	if tree.sourceLease != nil {
		t.Fatal("ReaderAtSource without slicer should not install a source lease")
	}
}

func TestReaderAtSourceUsesOptionalSlicer(t *testing.T) {
	backing := &countingSliceSource{data: []byte("1+2")}
	source, err := NewReaderAtSource(bytes.NewReader(backing.data), uint64(len(backing.data)), WithReaderAtSourceSlicer(backing))
	if err != nil {
		t.Fatalf("NewReaderAtSource failed: %v", err)
	}

	data, release, err := sourceBytes(source)
	if err != nil {
		t.Fatalf("sourceBytes failed: %v", err)
	}
	if release == nil {
		t.Fatal("expected borrowed ReaderAtSource slicer to return a release hook")
	}
	if got, want := string(data), "1+2"; got != want {
		t.Fatalf("sourceBytes data = %q, want %q", got, want)
	}
	release()
	if backing.releases != 1 {
		t.Fatalf("slicer release count = %d, want 1", backing.releases)
	}
}

func TestNewReaderAtSourceRejectsNilReader(t *testing.T) {
	_, err := NewReaderAtSource(nil, 1)
	if !errors.Is(err, ErrNoSource) {
		t.Fatalf("NewReaderAtSource error = %v, want ErrNoSource", err)
	}
}

func TestParseSourceMatchesParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := []byte("1+2+3")

	gotTree, err := parser.ParseSource(BytesSource(source))
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}
	defer gotTree.Release()

	wantTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	defer wantTree.Release()

	got := requireArithmeticTree(t, gotTree, lang).SExpr(lang)
	want := requireArithmeticTree(t, wantTree, lang).SExpr(lang)
	if got != want {
		t.Fatalf("SExpr mismatch\n got: %s\nwant: %s", got, want)
	}
	if gotTree.sourceLease != nil {
		t.Fatal("BytesSource should not install a release hook")
	}
}

func TestParseSourceRetainsBorrowedSourceUntilRelease(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := &countingSliceSource{data: []byte("1+2")}

	tree, err := parser.ParseSource(source)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}
	requireArithmeticTree(t, tree, lang)
	if source.releases != 0 {
		t.Fatalf("release count before Tree.Release = %d, want 0", source.releases)
	}

	tree.Release()
	if source.releases != 1 {
		t.Fatalf("release count after Tree.Release = %d, want 1", source.releases)
	}

	tree.Release()
	if source.releases != 1 {
		t.Fatalf("release count after second Tree.Release = %d, want 1", source.releases)
	}
}

func TestParseSourceWithTokenSourceFactoryBorrowsSourceUntilRelease(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := &countingSliceSource{data: []byte("1+2+3")}
	baseFactory := arithmeticTokenSourceFactory(lang)
	var factorySawBorrowedBytes bool
	factory := func(src []byte) TokenSource {
		factorySawBorrowedBytes = len(src) > 0 && &src[0] == &source.data[0]
		return baseFactory(src)
	}

	tree, err := parser.ParseSourceWithTokenSourceFactory(source, factory)
	if err != nil {
		t.Fatalf("ParseSourceWithTokenSourceFactory failed: %v", err)
	}
	requireArithmeticTree(t, tree, lang)
	if !factorySawBorrowedBytes {
		t.Fatal("token source factory did not receive borrowed source bytes")
	}
	if source.releases != 0 {
		t.Fatalf("release count before Tree.Release = %d, want 0", source.releases)
	}

	tree.Release()
	if source.releases != 1 {
		t.Fatalf("release count after Tree.Release = %d, want 1", source.releases)
	}
}

func TestParseSourceWithTokenSourceFactoryReleasesOnNilFactory(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	source := &countingSliceSource{data: []byte("1+2")}

	_, err := parser.ParseSourceWithTokenSourceFactory(source, nil)
	if !errors.Is(err, ErrNoTokenSourceFactory) {
		t.Fatalf("ParseSourceWithTokenSourceFactory error = %v, want ErrNoTokenSourceFactory", err)
	}
	if source.releases != 0 {
		t.Fatalf("source should not be borrowed when factory is nil; release count = %d", source.releases)
	}
}

func TestParseSourceRejectsInvalidParserBeforeBorrowingSource(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.LexStates = nil
	parser := NewParser(lang)
	source := &countingSliceSource{data: []byte("1+2")}

	_, err := parser.ParseSource(source)
	if err == nil {
		t.Fatal("expected ParseSource to fail without DFA lexer")
	}
	if source.releases != 0 {
		t.Fatalf("source should not be borrowed for invalid parser setup; release count = %d", source.releases)
	}
}

func TestParseIncrementalSourceReleasesUnusedNoEditBorrow(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	first := &countingSliceSource{data: []byte("1+2")}

	oldTree, err := parser.ParseSource(first)
	if err != nil {
		t.Fatalf("initial ParseSource failed: %v", err)
	}
	defer oldTree.Release()

	second := &countingSliceSource{data: []byte("1+2")}
	tree, err := parser.ParseIncrementalSource(second, oldTree)
	if err != nil {
		t.Fatalf("ParseIncrementalSource failed: %v", err)
	}
	if tree != oldTree {
		t.Fatalf("ParseIncrementalSource returned %p, want old tree %p", tree, oldTree)
	}
	if second.releases != 1 {
		t.Fatalf("unused no-edit source release count = %d, want 1", second.releases)
	}
	if first.releases != 0 {
		t.Fatalf("old tree source release count before Tree.Release = %d, want 0", first.releases)
	}

	oldTree.Release()
	if first.releases != 1 {
		t.Fatalf("old tree source release count after Tree.Release = %d, want 1", first.releases)
	}
}

func TestParseIncrementalSourceWithTokenSourceFactoryReleasesUnusedNoEditBorrow(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	factory := arithmeticTokenSourceFactory(lang)
	first := &countingSliceSource{data: []byte("1+2")}

	oldTree, err := parser.ParseSourceWithTokenSourceFactory(first, factory)
	if err != nil {
		t.Fatalf("initial ParseSourceWithTokenSourceFactory failed: %v", err)
	}
	defer oldTree.Release()

	second := &countingSliceSource{data: []byte("1+2")}
	tree, err := parser.ParseIncrementalSourceWithTokenSourceFactory(second, oldTree, factory)
	if err != nil {
		t.Fatalf("ParseIncrementalSourceWithTokenSourceFactory failed: %v", err)
	}
	if tree != oldTree {
		t.Fatalf("ParseIncrementalSourceWithTokenSourceFactory returned %p, want old tree %p", tree, oldTree)
	}
	if second.releases != 1 {
		t.Fatalf("unused no-edit source release count = %d, want 1", second.releases)
	}
	if first.releases != 0 {
		t.Fatalf("old tree source release count before Tree.Release = %d, want 0", first.releases)
	}
}

func TestParseSourceMaterializesReadAtFallback(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := &readAtOnlySource{data: []byte("1+2+3")}

	tree, err := parser.ParseSource(source)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}
	defer tree.Release()
	requireArithmeticTree(t, tree, lang)
	if !source.sliceCalled {
		t.Fatal("expected Slice to be attempted")
	}
	if source.readCalls == 0 {
		t.Fatal("expected ReadAt fallback to be used")
	}
	if tree.sourceLease != nil {
		t.Fatal("materialized source should not install a release hook")
	}
}

func TestParseSourceRejectsHugeSourceBeforeReading(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	source := &hugeSource{}

	_, err := parser.ParseSource(source)
	if !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("ParseSource error = %v, want ErrSourceTooLarge", err)
	}
	if source.sliceCalled {
		t.Fatal("Slice should not be called for oversized sources")
	}
	if source.readCalled {
		t.Fatal("ReadAt should not be called for oversized sources")
	}
}

func TestSourceBytesReleasesMismatchedSlice(t *testing.T) {
	source := &mismatchedSliceSource{data: []byte("1+2")}

	_, _, err := sourceBytes(source)
	if !errors.Is(err, ErrInvalidSourceRange) {
		t.Fatalf("sourceBytes error = %v, want ErrInvalidSourceRange", err)
	}
	if source.releases != 1 {
		t.Fatalf("mismatched slice release count = %d, want 1", source.releases)
	}
}

func TestSourceBytesRejectsReadAtNoProgress(t *testing.T) {
	_, _, err := sourceBytes(stalledSource{})
	if !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("sourceBytes error = %v, want io.ErrNoProgress", err)
	}
}

func TestParseSourceFileSource(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	path := filepath.Join(t.TempDir(), "input.txt")
	sourceBytes := []byte("1+2+3+4")
	if err := os.WriteFile(path, sourceBytes, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	fileSource, err := NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource failed: %v", err)
	}
	tree, err := parser.ParseSource(fileSource)
	if err != nil {
		t.Fatalf("ParseSource(FileSource) failed: %v", err)
	}
	defer tree.Release()

	got := requireArithmeticTree(t, tree, lang).SExpr(lang)
	wantTree, err := parser.Parse(sourceBytes)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	defer wantTree.Release()
	want := requireArithmeticTree(t, wantTree, lang).SExpr(lang)
	if got != want {
		t.Fatalf("SExpr mismatch\n got: %s\nwant: %s", got, want)
	}
	if supportsMmapFileSource() && tree.sourceLease == nil {
		t.Fatal("FileSource should install a release hook on mmap-backed platforms")
	}
}

func TestParseSourceCopyRetainsBorrowedSourceUntilCopiesRelease(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := &countingSliceSource{data: []byte("1+2")}

	tree, err := parser.ParseSource(source)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}
	copyTree := tree.Copy()
	if copyTree == nil || copyTree.RootNode() == nil {
		t.Fatal("Copy returned nil tree/root")
	}

	tree.Release()
	if source.releases != 0 {
		t.Fatalf("source release count after original Release = %d, want 0", source.releases)
	}
	if got, want := string(copyTree.Source()), "1+2"; got != want {
		t.Fatalf("copy source = %q, want %q", got, want)
	}
	requireArithmeticTree(t, copyTree, lang)

	copyTree.Release()
	if source.releases != 1 {
		t.Fatalf("source release count after copy Release = %d, want 1", source.releases)
	}
}

func TestParseSourceFileSourceCanDisableMmap(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("1+2+3"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	fileSource, err := NewFileSource(path, WithFileSourceMmap(false))
	if err != nil {
		t.Fatalf("NewFileSource failed: %v", err)
	}
	tree, err := parser.ParseSource(fileSource)
	if err != nil {
		t.Fatalf("ParseSource(FileSource) failed: %v", err)
	}
	defer tree.Release()
	requireArithmeticTree(t, tree, lang)
	if tree.sourceLease != nil {
		t.Fatal("FileSource with mmap disabled should materialize without a source lease")
	}
}

func TestFileSourceRequireMmapDoesNotSilentlyMaterializeWhenMmapDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("1+2+3"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	fileSource, err := NewFileSource(path, WithFileSourceMmap(false), WithFileSourceRequireMmap(true))
	if err != nil {
		t.Fatalf("NewFileSource failed: %v", err)
	}
	_, _, err = sourceBytes(fileSource)
	if !errors.Is(err, ErrFileSourceMmapUnsupported) {
		t.Fatalf("sourceBytes error = %v, want ErrFileSourceMmapUnsupported", err)
	}
}

func TestParserParseFile(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("1+2+3"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	tree, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	defer tree.Release()
	requireArithmeticTree(t, tree, lang)
}

func TestParserParseFileWithTokenSourceFactory(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("1+2+3"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	tree, err := parser.ParseFileWithTokenSourceFactory(path, arithmeticTokenSourceFactory(lang))
	if err != nil {
		t.Fatalf("ParseFileWithTokenSourceFactory failed: %v", err)
	}
	defer tree.Release()
	requireArithmeticTree(t, tree, lang)
}

func TestParserParseIncrementalFileReleasesUnusedNoEditBorrow(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("1+2"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	oldTree, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	defer oldTree.Release()

	tree, err := parser.ParseIncrementalFile(path, oldTree)
	if err != nil {
		t.Fatalf("ParseIncrementalFile failed: %v", err)
	}
	if tree != oldTree {
		t.Fatalf("ParseIncrementalFile returned %p, want old tree %p", tree, oldTree)
	}
}

func TestFileSourceSourceBytesUsesBorrowedMmap(t *testing.T) {
	if !supportsMmapFileSource() {
		t.Skip("mmap-backed FileSource slicing is unsupported on this platform")
	}
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	fileSource, err := NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource failed: %v", err)
	}

	data, release, err := sourceBytes(fileSource)
	if err != nil {
		t.Fatalf("sourceBytes failed: %v", err)
	}
	if release == nil {
		t.Fatal("expected mmap-backed sourceBytes to return a release hook")
	}
	defer release()
	if got, want := string(data), "abc"; got != want {
		t.Fatalf("sourceBytes data = %q, want %q", got, want)
	}
}

func TestParserPoolParseSource(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)

	tree, err := pool.ParseSource(BytesSource([]byte("1+2+3")))
	if err != nil {
		t.Fatalf("ParserPool.ParseSource failed: %v", err)
	}
	defer tree.Release()
	requireArithmeticTree(t, tree, lang)
}

func TestParserPoolParseFile(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("1+2+3"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	tree, err := pool.ParseFile(path)
	if err != nil {
		t.Fatalf("ParserPool.ParseFile failed: %v", err)
	}
	defer tree.Release()
	requireArithmeticTree(t, tree, lang)
}

func TestParserPoolParseSourceWithTokenSourceFactory(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)

	tree, err := pool.ParseSourceWithTokenSourceFactory(
		BytesSource([]byte("1+2+3")),
		arithmeticTokenSourceFactory(lang),
	)
	if err != nil {
		t.Fatalf("ParserPool.ParseSourceWithTokenSourceFactory failed: %v", err)
	}
	defer tree.Release()
	requireArithmeticTree(t, tree, lang)
}

func TestParserPoolParseIncrementalSource(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	first := &countingSliceSource{data: []byte("1+2")}

	oldTree, err := pool.ParseSource(first)
	if err != nil {
		t.Fatalf("ParserPool.ParseSource failed: %v", err)
	}
	defer oldTree.Release()

	second := &countingSliceSource{data: []byte("1+2")}
	tree, err := pool.ParseIncrementalSource(second, oldTree)
	if err != nil {
		t.Fatalf("ParserPool.ParseIncrementalSource failed: %v", err)
	}
	if tree != oldTree {
		t.Fatalf("ParserPool.ParseIncrementalSource returned %p, want old tree %p", tree, oldTree)
	}
	if second.releases != 1 {
		t.Fatalf("unused no-edit source release count = %d, want 1", second.releases)
	}
}

func TestParserPoolParseIncrementalSourceWithTokenSourceFactory(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	factory := arithmeticTokenSourceFactory(lang)
	first := &countingSliceSource{data: []byte("1+2")}

	oldTree, err := pool.ParseSourceWithTokenSourceFactory(first, factory)
	if err != nil {
		t.Fatalf("ParserPool.ParseSourceWithTokenSourceFactory failed: %v", err)
	}
	defer oldTree.Release()

	second := &countingSliceSource{data: []byte("1+2")}
	tree, err := pool.ParseIncrementalSourceWithTokenSourceFactory(second, oldTree, factory)
	if err != nil {
		t.Fatalf("ParserPool.ParseIncrementalSourceWithTokenSourceFactory failed: %v", err)
	}
	if tree != oldTree {
		t.Fatalf("ParserPool.ParseIncrementalSourceWithTokenSourceFactory returned %p, want old tree %p", tree, oldTree)
	}
	if second.releases != 1 {
		t.Fatalf("unused no-edit source release count = %d, want 1", second.releases)
	}
}
