package grammars

import (
	"fmt"
	"sync"

	"github.com/odvcencio/gotreesitter"
)

var (
	poolsMu sync.RWMutex
	pools   = map[string]*gotreesitter.ParserPool{}
)

func getOrCreatePool(name string, lang *gotreesitter.Language) *gotreesitter.ParserPool {
	poolsMu.RLock()
	pp, ok := pools[name]
	poolsMu.RUnlock()
	if ok {
		return pp
	}

	poolsMu.Lock()
	defer poolsMu.Unlock()
	if pp, ok = pools[name]; ok {
		return pp
	}
	pp = gotreesitter.NewParserPool(lang)
	pools[name] = pp
	return pp
}

// ParseFile detects the language from filename, parses source, and returns
// a BoundTree. The caller must call Release() on the returned BoundTree.
func ParseFile(filename string, source []byte) (*gotreesitter.BoundTree, error) {
	entry := DetectLanguage(filename)
	if entry == nil {
		return nil, fmt.Errorf("unsupported file type: %s", filename)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)

	var tree *gotreesitter.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		ts := entry.TokenSourceFactory(source, lang)
		tree, err = parser.ParseWithTokenSource(source, ts)
	} else {
		tree, err = parser.Parse(source)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	return gotreesitter.Bind(tree), nil
}

// ParseFileSource detects the language from filename, parses source, and
// returns a BoundTree. If source can expose a contiguous borrowed slice, the
// returned tree keeps that slice alive until BoundTree.Release.
func ParseFileSource(filename string, source gotreesitter.Source) (*gotreesitter.BoundTree, error) {
	entry := DetectLanguage(filename)
	if entry == nil {
		return nil, fmt.Errorf("unsupported file type: %s", filename)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)

	var tree *gotreesitter.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		tree, err = parser.ParseSourceWithTokenSourceFactory(source, func(src []byte) gotreesitter.TokenSource {
			return entry.TokenSourceFactory(src, lang)
		})
	} else {
		tree, err = parser.ParseSource(source)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	return gotreesitter.Bind(tree), nil
}

// ParseFilePath detects the language from path, parses the file at path, and
// returns a BoundTree.
func ParseFilePath(path string, opts ...gotreesitter.FileSourceOption) (*gotreesitter.BoundTree, error) {
	source, err := gotreesitter.NewFileSource(path, opts...)
	if err != nil {
		return nil, err
	}
	return ParseFileSource(path, source)
}

// ParseFilePooled is like ParseFile but reuses a per-language ParserPool
// to avoid allocating a new parser on every call. It is safe for concurrent use.
// The caller must call Release() on the returned BoundTree.
func ParseFilePooled(filename string, source []byte) (*gotreesitter.BoundTree, error) {
	entry := DetectLanguage(filename)
	if entry == nil {
		return nil, fmt.Errorf("unsupported file type: %s", filename)
	}

	lang := entry.Language()
	pp := getOrCreatePool(entry.Name, lang)

	var tree *gotreesitter.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		ts := entry.TokenSourceFactory(source, lang)
		tree, err = pp.ParseWithTokenSource(source, ts)
	} else {
		tree, err = pp.Parse(source)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	return gotreesitter.Bind(tree), nil
}

// ParseFilePathPooled is like ParseFilePath but reuses a per-language
// ParserPool.
func ParseFilePathPooled(path string, opts ...gotreesitter.FileSourceOption) (*gotreesitter.BoundTree, error) {
	source, err := gotreesitter.NewFileSource(path, opts...)
	if err != nil {
		return nil, err
	}
	return ParseFileSourcePooled(path, source)
}

// ParseFileSourcePooled is like ParseFileSource but reuses a per-language
// ParserPool to avoid allocating a new parser on every call. It is safe for
// concurrent use.
func ParseFileSourcePooled(filename string, source gotreesitter.Source) (*gotreesitter.BoundTree, error) {
	entry := DetectLanguage(filename)
	if entry == nil {
		return nil, fmt.Errorf("unsupported file type: %s", filename)
	}

	lang := entry.Language()
	pp := getOrCreatePool(entry.Name, lang)

	var tree *gotreesitter.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		tree, err = pp.ParseSourceWithTokenSourceFactory(source, func(src []byte) gotreesitter.TokenSource {
			return entry.TokenSourceFactory(src, lang)
		})
	} else {
		tree, err = pp.ParseSource(source)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	return gotreesitter.Bind(tree), nil
}
