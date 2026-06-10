//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestMeasureDtierVsC times the production (or forest) parser against the C
// oracle over a lock-filtered corpus dir and reports full_ratio + parity +
// truncation, so an unmeasured ("D") grammar can be classified into a tier.
//
//	REPRO_LANG   grammar name (must exist in grammars.AllLanguages)
//	REPRO_DIR    corpus root (per-lang subdir REPRO_DIR/REPRO_LANG)
//	REPRO_EXTS   comma list of extensions to keep (lock-filter; e.g. .agda)
//	REPRO_N      max files (default 40)
//	REPRO_FOREST =1 measure the forest path (recovery on) instead of production
//
// Per-file panic recovery + the caller-set GOT_PARSE_MEMORY_BUDGET_MB keep a
// pathological blowup file from crashing the run (it yields a truncated tree).
func TestMeasureDtierVsC(t *testing.T) {
	if os.Getenv("REPRO_DIR") == "" {
		t.Skip("set REPRO_DIR")
	}
	name := os.Getenv("REPRO_LANG")
	root := os.Getenv("REPRO_DIR")
	dir := filepath.Join(root, name)
	exts := strings.Split(os.Getenv("REPRO_EXTS"), ",")
	forest := os.Getenv("REPRO_FOREST") == "1"

	var goLang *gts.Language
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			goLang = e.Language()
			break
		}
	}
	if goLang == nil {
		t.Fatalf("%s: not in grammars.AllLanguages", name)
	}
	cLang, err := ParityCLanguage(name)
	if err != nil {
		t.Skipf("%s: no C reference: %v", name, err)
	}
	if forest {
		gts.SetGLRForestRecover(true)
		defer gts.SetGLRForestRecover(false)
	}

	var files []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(p, "/.git/") {
			return nil
		}
		base := strings.ToLower(filepath.Base(p))
		for _, e := range exts {
			e = strings.ToLower(strings.TrimSpace(e))
			if e == "" {
				continue
			}
			if (strings.HasPrefix(e, ".") && strings.HasSuffix(base, e)) || base == e || strings.HasSuffix(base, "."+e) {
				if info.Size() >= 32 && info.Size() <= 200_000 {
					files = append(files, p)
				}
				break
			}
		}
		return nil
	})
	sort.Strings(files)
	n := 40
	if v := os.Getenv("REPRO_N"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}
	if n < len(files) {
		files = files[:n]
	}

	// minParse runs the parse up to 3x and returns the min wall time; recovers
	// panics (returns panicked=true) so one bad file cannot kill the run.
	// notAccepted == the parser stopped early (memory/no-stacks/node-limit) —
	// the REAL truncation signal (endByte<len is a false positive: trailing
	// comments/extras legitimately aren't covered by the root, same as C).
	minParse := func(src []byte) (dur time.Duration, endByte uint32, hasErr, notAccepted, panicked bool) {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		best := time.Duration(1<<62 - 1)
		for i := 0; i < 3; i++ {
			p := gts.NewParser(goLang)
			t0 := time.Now()
			var tr *gts.Tree
			if forest {
				ft, _ := p.ParseForestExperimental(src)
				tr = ft
			} else {
				tr, _ = p.Parse(src)
			}
			d := time.Since(t0)
			if d < best {
				best = d
			}
			if i == 2 && tr != nil && tr.RootNode() != nil {
				endByte = tr.RootNode().EndByte()
				hasErr = tr.RootNode().HasError()
				notAccepted = tr.ParseStopReason() != gts.ParseStopAccepted
			}
			if tr != nil {
				tr.Release()
			}
		}
		return best, endByte, hasErr, notAccepted, false
	}

	var totGo, totC time.Duration
	var ratios []float64
	dispatched, matchC, divergeC, trunc, panics, errTree := 0, 0, 0, 0, 0, 0
	for _, f := range files {
		src, rerr := os.ReadFile(f)
		if rerr != nil || len(src) == 0 {
			continue
		}
		goDur, endByte, hasErr, notAccepted, panicked := minParse(src)
		if panicked {
			panics++
			continue
		}
		_ = endByte
		// C oracle timing (min of 3)
		cBest := time.Duration(1<<62 - 1)
		var cTree *sitter.Tree
		var cParser *sitter.Parser
		for i := 0; i < 3; i++ {
			cParser = sitter.NewParser()
			_ = cParser.SetLanguage(cLang)
			t0 := time.Now()
			ct := cParser.Parse(src, nil)
			d := time.Since(t0)
			if d < cBest {
				cBest = d
			}
			if i == 2 {
				cTree = ct
			} else if ct != nil {
				ct.Close()
			}
			if i < 2 {
				cParser.Close()
			}
		}
		if cTree == nil || cTree.RootNode() == nil {
			if cParser != nil {
				cParser.Close()
			}
			continue
		}
		dispatched++
		if notAccepted {
			trunc++
		}
		if hasErr {
			errTree++
		}
		// re-parse once for the comparison tree (timing runs released theirs)
		gp := gts.NewParser(goLang)
		var gtree *gts.Tree
		func() {
			defer func() { _ = recover() }()
			if forest {
				gtree, _ = gp.ParseForestExperimental(src)
			} else {
				gtree, _ = gp.Parse(src)
			}
		}()
		if gtree != nil && gtree.RootNode() != nil {
			var errs []string
			compareNodes(gtree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
			if len(errs) == 0 {
				matchC++
			} else {
				divergeC++
				if os.Getenv("REPRO_DUMP_DIVERGENCE") == "1" && divergeC <= 6 {
					fmt.Printf("DIVERGE %s %s: %s\n", name, filepath.Base(f),
						strings.Join(errs[:min(2, len(errs))], " || "))
				}
			}
			gtree.Release()
		}
		if cBest > 0 {
			ratios = append(ratios, float64(goDur)/float64(cBest))
		}
		totGo += goDur
		totC += cBest
		cTree.Close()
		cParser.Close()
	}

	median := 0.0
	if len(ratios) > 0 {
		sort.Float64s(ratios)
		median = ratios[len(ratios)/2]
	}
	agg := 0.0
	if totC > 0 {
		agg = float64(totGo) / float64(totC)
	}
	mode := "prod"
	if forest {
		mode = "forest"
	}
	parityPct := 0.0
	if dispatched > 0 {
		parityPct = 100 * float64(matchC) / float64(dispatched)
	}
	fmt.Printf("MEASURE-DTIER %s mode=%s files=%d medianRatio=%.2fx aggRatio=%.2fx parityMatch=%d/%d(%.0f%%) diverge=%d trunc=%d errTree=%d panics=%d\n",
		name, mode, dispatched, median, agg, matchC, dispatched, parityPct, divergeC, trunc, errTree, panics)
}
