//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package gotreesitter

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

func (s *FileSource) Slice(start, end uint64) (SourceSlice, bool, error) {
	if s == nil {
		return SourceSlice{}, false, ErrNoSource
	}
	if end < start || end > s.size {
		return SourceSlice{}, false, ErrInvalidSourceRange
	}
	if s.size > uint64(maxSourceInt) {
		return SourceSlice{}, false, ErrSourceTooLarge
	}
	if !s.config.mmap {
		if s.config.requireMmap {
			return SourceSlice{}, false, ErrFileSourceMmapUnsupported
		}
		return SourceSlice{}, false, nil
	}
	if s.size == 0 {
		return SourceSlice{Bytes: nil}, true, nil
	}

	f, err := os.Open(s.path)
	if err != nil {
		return s.mmapFallbackOrError(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return s.mmapFallbackOrError(err)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || uint64(info.Size()) < s.size {
		return SourceSlice{}, false, ErrInvalidSourceRange
	}

	data, err := unix.Mmap(int(f.Fd()), 0, int(s.size), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return s.mmapFallbackOrError(err)
	}

	var once sync.Once
	return SourceSlice{
		Bytes: data[int(start):int(end)],
		Release: func() {
			once.Do(func() {
				_ = unix.Munmap(data)
			})
		},
	}, true, nil
}

func (s *FileSource) mmapFallbackOrError(err error) (SourceSlice, bool, error) {
	if s != nil && s.config.mmapFallback && !s.config.requireMmap {
		return SourceSlice{}, false, nil
	}
	return SourceSlice{}, false, err
}
