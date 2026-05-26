// Diagnostic: parse one JS file with GLR equiv audit on, print counters.
// Run: go run ./cgo_harness/cmd/equiv_audit_oneshot <jsfile>
package main

import (
	"fmt"
	"os"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: equiv_audit_oneshot <jsfile>")
		os.Exit(2)
	}
	path := os.Args[1]
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}

	gotreesitter.EnableGLREquivAudit(true)
	defer gotreesitter.EnableGLREquivAudit(false)

	lang := grammars.JavascriptLanguage()
	parser := gotreesitter.NewParser(lang)

	// Warm up once.
	if _, err := parser.Parse(src); err != nil {
		fmt.Fprintf(os.Stderr, "warm parse: %v\n", err)
		os.Exit(1)
	}

	start := time.Now()
	tree, err := parser.Parse(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}
	elapsed := time.Since(start)

	rt := tree.ParseRuntime()
	lookups := rt.EquivCacheLookups
	hits := rt.EquivCacheHits
	frontier := rt.EquivFrontierCalls
	childScans := rt.EquivFrontierChildScans
	candidate := rt.EquivFrontierCandidateCompares
	keyMisses := rt.EquivCacheKeyMisses
	epochMisses := rt.EquivCacheEpochMisses
	versionMisses := rt.EquivCacheVersionMisses

	hitPct := 0.0
	if lookups > 0 {
		hitPct = 100.0 * float64(hits) / float64(lookups)
	}

	fmt.Printf("File:               %s (%d bytes)\n", path, len(src))
	fmt.Printf("Parse wall:         %v\n", elapsed)
	fmt.Printf("\n=== Equiv cache ===\n")
	fmt.Printf("Cache lookups:      %d\n", lookups)
	fmt.Printf("Cache hits:         %d  (%.1f%% hit rate)\n", hits, hitPct)
	fmt.Printf("Cache key misses:   %d\n", keyMisses)
	fmt.Printf("Cache epoch misses: %d\n", epochMisses)
	fmt.Printf("Cache version misses:%d\n", versionMisses)
	fmt.Printf("\n=== Frontier walk ===\n")
	fmt.Printf("Frontier calls:     %d\n", frontier)
	fmt.Printf("Child scans:        %d  (%.1f per call)\n", childScans, ratio(childScans, frontier))
	fmt.Printf("Candidate compares: %d  (%.2f per call)\n", candidate, ratio(candidate, frontier))

	fmt.Printf("\n=== Stack-pair audit (the real lever) ===\n")
	keyed := rt.StackEquivPairKeyed
	repeats := rt.StackEquivPairRepeats
	repeatTrue := rt.StackEquivPairRepeatTrue
	repeatFalse := rt.StackEquivPairRepeatFalse
	mismatch := rt.StackEquivPairRepeatMismatch
	stores := rt.StackEquivPairStores
	fmt.Printf("Pair lookups (keyed): %d\n", keyed)
	fmt.Printf("Pair repeats:         %d  (%.1f%% would short-circuit)\n", repeats, 100.0*float64(repeats)/maxF(float64(keyed)))
	fmt.Printf("  → repeat true:      %d\n", repeatTrue)
	fmt.Printf("  → repeat false:     %d\n", repeatFalse)
	fmt.Printf("  → repeat MISMATCH:  %d  (must be 0 for cache to be correctness-safe)\n", mismatch)
	fmt.Printf("Pair stores:          %d  (live map size)\n", stores)

	fmt.Printf("\n=== Stack-pair CONTENT-HASH variant (the experiment) ===\n")
	ckeyed := rt.StackEquivContentPairKeyed
	crepeats := rt.StackEquivContentPairRepeats
	crepeatTrue := rt.StackEquivContentPairRepeatTrue
	crepeatFalse := rt.StackEquivContentPairRepeatFalse
	cmismatch := rt.StackEquivContentPairRepeatMismatch
	cstores := rt.StackEquivContentPairStores
	fmt.Printf("Content-pair lookups: %d\n", ckeyed)
	fmt.Printf("Content-pair repeats: %d  (%.1f%% would short-circuit)\n", crepeats, 100.0*float64(crepeats)/maxF(float64(ckeyed)))
	fmt.Printf("  → repeat true:      %d\n", crepeatTrue)
	fmt.Printf("  → repeat false:     %d\n", crepeatFalse)
	fmt.Printf("  → repeat MISMATCH:  %d  (must be 0 for cache to be correctness-safe)\n", cmismatch)
	fmt.Printf("Content-pair stores:  %d  (live map size)\n", cstores)

	if repeats > 0 && crepeats > repeats {
		fmt.Printf("\n*** Content-hash variant captures %.1fx more repeats (%d vs %d).\n",
			float64(crepeats)/float64(repeats), crepeats, repeats)
	}
}

func maxF(x float64) float64 {
	if x < 1 {
		return 1
	}
	return x
}

func ratio(num, den uint64) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}
