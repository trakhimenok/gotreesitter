//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestGrammargenVerify generates a grammar via grammargen WITH LR(1) state
// splitting (the strongest capability) and measures parity vs the C oracle on
// the lock-filtered real corpus — to confirm grammargen beats the ts2go blob.
//
//	REPRO_LANGS  comma list (each needs /tmp/grammar_parity/<lang>/src/grammar.json)
//	REPRO_EXTSMAP  lang=ext;lang=ext;...  (extension per lang for corpus filter)
//	REPRO_DIR    corpus root (per-lang subdir)
//	REPRO_N      files per grammar (default 25)
func TestGrammargenVerify(t *testing.T) {
	langs := strings.Split(os.Getenv("REPRO_LANGS"), ",")
	if len(langs) == 0 || langs[0] == "" {
		t.Skip("set REPRO_LANGS")
	}
	extMap := map[string]string{}
	for _, kv := range strings.Split(os.Getenv("REPRO_EXTSMAP"), ";") {
		if i := strings.Index(kv, "="); i > 0 {
			extMap[kv[:i]] = kv[i+1:]
		}
	}
	root := os.Getenv("REPRO_DIR")
	n := 25
	if v := os.Getenv("REPRO_N"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}

	for _, lang := range langs {
		lang = strings.TrimSpace(lang)
		if lang == "" {
			continue
		}
		jsonPath := filepath.Join("/tmp/grammar_parity", lang, "src", "grammar.json")
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			fmt.Printf("GGVERIFY %s: no grammar.json (%v)\n", lang, err)
			continue
		}
		gram, err := grammargen.ImportGrammarJSON(data)
		if err != nil {
			fmt.Printf("GGVERIFY %s: import failed: %v\n", lang, err)
			continue
		}
		gram.EnableLRSplitting = os.Getenv("REPRO_LRSPLIT") != "0" // latest & greatest (toggle)
		var genLang *gts.Language
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("GGVERIFY %s: generate PANIC: %v\n", lang, r)
				}
			}()
			genLang, err = grammargen.GenerateLanguage(gram)
		}()
		if genLang == nil {
			if err != nil {
				fmt.Printf("GGVERIFY %s: generate failed: %v\n", lang, err)
			}
			continue
		}
		cLang, cerr := ParityCLanguage(lang)
		if cerr != nil {
			fmt.Printf("GGVERIFY %s: no C ref: %v\n", lang, cerr)
			continue
		}
		// adapt external scanner from the ts2go ref blob if grammargen needs one
		if entry, ok := grammarsLangEntry(lang); ok && entry.Language != nil {
			ref := entry.Language()
			if ref != nil && ref.ExternalScanner != nil && len(genLang.ExternalSymbols) > 0 {
				if sc, ok := gts.AdaptExternalScannerByExternalOrder(ref, genLang); ok {
					genLang.ExternalScanner = sc
				}
			}
		}

		exts := strings.Split(extMap[lang], ",")
		var files []string
		dir := filepath.Join(root, lang)
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || strings.Contains(p, "/.git/") {
				return nil
			}
			base := strings.ToLower(filepath.Base(p))
			for _, e := range exts {
				e = strings.ToLower(strings.TrimSpace(e))
				if e != "" && (strings.HasSuffix(base, e) || base == e) && info.Size() >= 32 && info.Size() <= 200_000 {
					files = append(files, p)
					break
				}
			}
			return nil
		})
		sort.Strings(files)
		if n < len(files) {
			files = files[:n]
		}
		match, diverge, errcnt := 0, 0, 0
		for _, f := range files {
			src, rerr := os.ReadFile(f)
			if rerr != nil || len(src) == 0 {
				continue
			}
			var gtree *gts.Tree
			func() {
				defer func() { _ = recover() }()
				gtree, _ = gts.NewParser(genLang).Parse(src)
			}()
			if gtree == nil || gtree.RootNode() == nil {
				errcnt++
				continue
			}
			cp := sitter.NewParser()
			_ = cp.SetLanguage(cLang)
			ct := cp.Parse(src, nil)
			if ct == nil || ct.RootNode() == nil {
				cp.Close()
				gtree.Release()
				continue
			}
			var errs []grammargenCGODivergence
			compareGrammargenVsC(gtree.RootNode(), genLang, ct.RootNode(), "root", &errs)
			if len(errs) == 0 {
				match++
			} else {
				diverge++
			}
			ct.Close()
			cp.Close()
			gtree.Release()
		}
		total := match + diverge
		pct := 0
		if total > 0 {
			pct = 100 * match / total
		}
		fmt.Printf("GGVERIFY %s: grammargen+LRsplit parity=%d/%d(%d%%) parseErr=%d\n", lang, match, total, pct, errcnt)
	}
}

func grammarsLangEntry(name string) (grammars.LangEntry, bool) {
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			return e, true
		}
	}
	return grammars.LangEntry{}, false
}
