package gotreesitter

import (
	"errors"
	"io"
	"os"
	"sync"
)

const maxSourceInt = int(^uint(0) >> 1)

var (
	// ErrNoSource is returned when a source-backed parse receives a nil Source.
	ErrNoSource = errors.New("source: nil source")

	// ErrSourceTooLarge is returned when a Source exceeds the parser's uint32
	// byte-coordinate space.
	ErrSourceTooLarge = errors.New("source: length exceeds uint32 byte coordinate space")

	// ErrInvalidSourceRange is returned when a Source cannot provide the
	// requested range.
	ErrInvalidSourceRange = errors.New("source: invalid range")

	// ErrNoTokenSourceFactory is returned when a source-backed token-source
	// parse receives a nil factory.
	ErrNoTokenSourceFactory = errors.New("source: nil token source factory")

	// ErrFileSourceMmapUnsupported is returned when a FileSource is configured
	// to require mmap but file-backed mmap is unavailable.
	ErrFileSourceMmapUnsupported = errors.New("source: file-backed mmap is unavailable")
)

// Source provides parser input without requiring callers to hand over an owned
// contiguous byte slice. Implementations may expose contiguous ranges directly
// through Slice, or fall back to ReadAt for materialization.
type Source interface {
	// Len returns the source length in bytes.
	Len() uint64

	// ReadAt reads from a byte offset using io.ReaderAt semantics.
	ReadAt(p []byte, off int64) (int, error)

	// Slice returns a contiguous byte window for [start, end). Returning ok=false
	// with a nil error asks the parser to materialize through ReadAt instead.
	Slice(start, end uint64) (SourceSlice, bool, error)
}

// SourceSlice is a contiguous source window borrowed from a Source.
//
// Release is called by Tree.Release for parser-owned source windows, so the
// returned bytes must remain valid until Release runs.
type SourceSlice struct {
	Bytes   []byte
	Release func()
}

func (s SourceSlice) release() {
	if s.Release != nil {
		s.Release()
	}
}

// SourceSlicer can expose borrowed contiguous source windows.
type SourceSlicer interface {
	Slice(start, end uint64) (SourceSlice, bool, error)
}

type sourceLease struct {
	mu        sync.Mutex
	refs      int
	releaseFn func()
}

func newSourceLease(release func()) *sourceLease {
	if release == nil {
		return nil
	}
	return &sourceLease{
		refs:      1,
		releaseFn: release,
	}
}

func (l *sourceLease) retain() *sourceLease {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.releaseFn == nil {
		return nil
	}
	l.refs++
	return l
}

func (l *sourceLease) releaseOne() func() {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.refs <= 0 {
		return nil
	}
	l.refs--
	if l.refs != 0 {
		return nil
	}
	release := l.releaseFn
	l.releaseFn = nil
	return release
}

func (l *sourceLease) release() {
	if release := l.releaseOne(); release != nil {
		release()
	}
}

// BytesSource adapts an existing byte slice to Source.
type BytesSource []byte

func (s BytesSource) Len() uint64 {
	return uint64(len(s))
}

func (s BytesSource) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, ErrInvalidSourceRange
	}
	if off >= int64(len(s)) {
		return 0, io.EOF
	}
	n := copy(p, []byte(s)[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (s BytesSource) Slice(start, end uint64) (SourceSlice, bool, error) {
	if end < start || end > uint64(len(s)) {
		return SourceSlice{}, false, ErrInvalidSourceRange
	}
	return SourceSlice{Bytes: []byte(s)[int(start):int(end)]}, true, nil
}

// ReaderAtSource adapts an io.ReaderAt with a fixed byte length to Source.
type ReaderAtSource struct {
	reader io.ReaderAt
	size   uint64
	slicer SourceSlicer
}

// ReaderAtSourceOption configures a ReaderAtSource.
type ReaderAtSourceOption func(*ReaderAtSource)

// WithReaderAtSourceSlicer lets ReaderAtSource borrow contiguous windows from
// another object. If the slicer returns ok=false, parsing falls back to ReadAt.
func WithReaderAtSourceSlicer(slicer SourceSlicer) ReaderAtSourceOption {
	return func(s *ReaderAtSource) {
		if s != nil {
			s.slicer = slicer
		}
	}
}

// NewReaderAtSource creates a Source from r and a known byte length.
func NewReaderAtSource(r io.ReaderAt, size uint64, opts ...ReaderAtSourceOption) (*ReaderAtSource, error) {
	if r == nil {
		return nil, ErrNoSource
	}
	s := &ReaderAtSource{reader: r, size: size}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s, nil
}

func (s *ReaderAtSource) Len() uint64 {
	if s == nil {
		return 0
	}
	return s.size
}

func (s *ReaderAtSource) ReadAt(p []byte, off int64) (int, error) {
	if s == nil || s.reader == nil {
		return 0, ErrNoSource
	}
	return s.reader.ReadAt(p, off)
}

func (s *ReaderAtSource) Slice(start, end uint64) (SourceSlice, bool, error) {
	if s == nil {
		return SourceSlice{}, false, ErrNoSource
	}
	if end < start || end > s.size {
		return SourceSlice{}, false, ErrInvalidSourceRange
	}
	if s.slicer == nil {
		return SourceSlice{}, false, nil
	}
	return s.slicer.Slice(start, end)
}

type fileSourceConfig struct {
	mmap         bool
	mmapFallback bool
	requireMmap  bool
}

// FileSourceOption configures FileSource behavior.
type FileSourceOption func(*fileSourceConfig)

func defaultFileSourceConfig() fileSourceConfig {
	return fileSourceConfig{mmap: true}
}

// WithFileSourceMmap enables or disables mmap-backed slicing.
func WithFileSourceMmap(enabled bool) FileSourceOption {
	return func(cfg *fileSourceConfig) {
		if cfg != nil {
			cfg.mmap = enabled
		}
	}
}

// WithFileSourceMmapFallback allows FileSource to materialize through ReadAt if
// mmap fails. By default mmap errors are returned to avoid accidental large-file
// materialization.
func WithFileSourceMmapFallback(enabled bool) FileSourceOption {
	return func(cfg *fileSourceConfig) {
		if cfg != nil {
			cfg.mmapFallback = enabled
		}
	}
}

// WithFileSourceRequireMmap makes FileSource fail instead of materializing on
// platforms without mmap support.
func WithFileSourceRequireMmap(enabled bool) FileSourceOption {
	return func(cfg *fileSourceConfig) {
		if cfg != nil {
			cfg.requireMmap = enabled
		}
	}
}

// FileSource adapts a regular file to Source. On platforms with mmap support,
// full-file Slice calls are backed by a read-only memory map.
type FileSource struct {
	path   string
	size   uint64
	config fileSourceConfig
}

// NewFileSource creates a source for a regular file at path.
func NewFileSource(path string, opts ...FileSourceOption) (*FileSource, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 {
		return nil, ErrInvalidSourceRange
	}
	cfg := defaultFileSourceConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &FileSource{path: path, size: uint64(info.Size()), config: cfg}, nil
}

func (s *FileSource) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *FileSource) Len() uint64 {
	if s == nil {
		return 0
	}
	return s.size
}

func (s *FileSource) ReadAt(p []byte, off int64) (int, error) {
	if s == nil {
		return 0, ErrNoSource
	}
	f, err := os.Open(s.path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.ReadAt(p, off)
}

func sourceBytes(src Source) ([]byte, func(), error) {
	if src == nil {
		return nil, nil, ErrNoSource
	}

	sourceLen := src.Len()
	if sourceLen > uint64(^uint32(0)) {
		return nil, nil, ErrSourceTooLarge
	}
	if sourceLen > uint64(maxSourceInt) {
		return nil, nil, ErrSourceTooLarge
	}

	if borrowed, ok, err := src.Slice(0, sourceLen); err != nil {
		return nil, nil, err
	} else if ok {
		if uint64(len(borrowed.Bytes)) != sourceLen {
			borrowed.release()
			return nil, nil, ErrInvalidSourceRange
		}
		if borrowed.Release == nil {
			return borrowed.Bytes, nil, nil
		}
		return borrowed.Bytes, borrowed.release, nil
	}

	out := make([]byte, int(sourceLen))
	for read := 0; read < len(out); {
		remaining := len(out) - read
		n, err := src.ReadAt(out[read:], int64(read))
		if n < 0 || n > remaining {
			return nil, nil, ErrInvalidSourceRange
		}
		read += n
		if n == 0 && err == nil {
			return nil, nil, io.ErrNoProgress
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) && read == len(out) {
			break
		}
		if errors.Is(err, io.EOF) {
			return nil, nil, io.ErrUnexpectedEOF
		}
		return nil, nil, err
	}
	return out, nil, nil
}

func attachSourceRelease(tree *Tree, release func()) *Tree {
	if tree != nil && release != nil {
		tree.sourceLease = newSourceLease(release)
	}
	return tree
}
