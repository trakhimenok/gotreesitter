//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !solaris

package gotreesitter

func (s *FileSource) Slice(start, end uint64) (SourceSlice, bool, error) {
	if s == nil {
		return SourceSlice{}, false, ErrNoSource
	}
	if end < start || end > s.size {
		return SourceSlice{}, false, ErrInvalidSourceRange
	}
	if s.config.requireMmap {
		return SourceSlice{}, false, ErrFileSourceMmapUnsupported
	}
	return SourceSlice{}, false, nil
}
