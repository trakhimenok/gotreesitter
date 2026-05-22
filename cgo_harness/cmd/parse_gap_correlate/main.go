package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type reportRow struct {
	Language string        `json:"language"`
	Sample   string        `json:"sample"`
	Mode     string        `json:"mode"`
	Bytes    int           `json:"bytes"`
	MedianNS int64         `json:"median_ns"`
	BOp      int64         `json:"b_op"`
	AllocsOp float64       `json:"allocs_op"`
	Parity   paritySummary `json:"parity"`
	Runtime  runtimeStats  `json:"runtime"`
	Error    string        `json:"error"`
	Blocker  string        `json:"blocker"`
}

type paritySummary struct {
	Highlight *bool  `json:"highlight,omitempty"`
	Query     *bool  `json:"query,omitempty"`
	Deep      bool   `json:"deep"`
	Error     string `json:"error,omitempty"`
}

type runtimeStats struct {
	Tokens                  uint64        `json:"tokens,omitempty"`
	NodesAllocated          int           `json:"nodes_allocated,omitempty"`
	FinalNodes              uint64        `json:"final_nodes,omitempty"`
	GSSNodes                uint64        `json:"gss_nodes,omitempty"`
	MaxStacksSeen           int           `json:"max_stacks_seen,omitempty"`
	SingleStackTokens       uint64        `json:"single_stack_tokens,omitempty"`
	MultiStackTokens        uint64        `json:"multi_stack_tokens,omitempty"`
	MergeStacksIn           uint64        `json:"merge_stacks_in,omitempty"`
	MergeStacksOut          uint64        `json:"merge_stacks_out,omitempty"`
	ArenaLiveB              int64         `json:"arena_live_b,omitempty"`
	ArenaCapacityB          int64         `json:"arena_capacity_b,omitempty"`
	ArenaCapacityWaste      uint64        `json:"arena_capacity_waste,omitempty"`
	FinalChildRangeDrains   uint64        `json:"final_child_range_drains,omitempty"`
	PublicNodesMaterialized uint64        `json:"public_nodes_materialized,omitempty"`
	DenseFallbacks          uint64        `json:"dense_fallbacks,omitempty"`
	ResultSelectionNS       int64         `json:"result_selection_ns,omitempty"`
	ResultBuildNS           int64         `json:"result_build_ns,omitempty"`
	ResultCompatibilityNS   int64         `json:"result_compatibility_ns,omitempty"`
	ResultParentLinkNS      int64         `json:"result_parent_link_ns,omitempty"`
	ResultFinalizeRootNS    int64         `json:"result_finalize_root_ns,omitempty"`
	ResultExtendTrailingNS  int64         `json:"result_extend_trailing_ns,omitempty"`
	NormalizationNS         int64         `json:"normalization_ns,omitempty"`
	NormalizationNodes      uint64        `json:"normalization_nodes_visited,omitempty"`
	NormalizationRewrites   uint64        `json:"normalization_nodes_rewritten,omitempty"`
	ParseWallNS             int64         `json:"parse_wall_ns,omitempty"`
	ParserLoopNS            int64         `json:"parser_loop_ns,omitempty"`
	TokenNextNS             int64         `json:"token_next_ns,omitempty"`
	ActionDispatchNS        int64         `json:"action_dispatch_ns,omitempty"`
	ActionLookupNS          int64         `json:"action_lookup_ns,omitempty"`
	GLRMergeNS              int64         `json:"glr_merge_ns,omitempty"`
	GLRCullNS               int64         `json:"glr_cull_ns,omitempty"`
	MergeCalls              uint64        `json:"merge_calls,omitempty"`
	ForkCount               uint64        `json:"fork_count,omitempty"`
	ConflictRR              uint64        `json:"conflict_rr,omitempty"`
	ConflictRS              uint64        `json:"conflict_rs,omitempty"`
	ConflictOther           uint64        `json:"conflict_other,omitempty"`
	LexBytes                uint64        `json:"lex_bytes,omitempty"`
	LexTokens               uint64        `json:"lex_tokens,omitempty"`
	ReduceChainSteps        uint64        `json:"reduce_chain_steps,omitempty"`
	ReduceChainMaxLen       uint64        `json:"reduce_chain_max_len,omitempty"`
	NoTreeReduceNodes       uint64        `json:"notree_reduce_nodes,omitempty"`
	NoTreeLeafNodes         uint64        `json:"notree_leaf_nodes,omitempty"`
	HotAmbiguities          []hotGLRState `json:"hot_ambiguities,omitempty"`
	HotReduceChains         []hotGLRState `json:"hot_reduce_chains,omitempty"`
	HotMergeStates          []hotGLRState `json:"hot_merge_states,omitempty"`
}

type hotGLRState struct {
	State             uint32 `json:"state"`
	Lookahead         uint16 `json:"lookahead,omitempty"`
	ActionCount       uint8  `json:"action_count,omitempty"`
	ShiftCount        uint8  `json:"shift_count,omitempty"`
	ReduceCount       uint8  `json:"reduce_count,omitempty"`
	Hits              uint64 `json:"hits,omitempty"`
	Forks             uint64 `json:"forks,omitempty"`
	MultiStackHits    uint64 `json:"multi_stack_hits,omitempty"`
	StackInTotal      uint64 `json:"stack_in_total,omitempty"`
	StackInMax        int    `json:"stack_in_max,omitempty"`
	ReduceChainHits   uint64 `json:"reduce_chain_hits,omitempty"`
	ReduceChainSteps  uint64 `json:"reduce_chain_steps,omitempty"`
	ReduceChainMaxLen int    `json:"reduce_chain_max_len,omitempty"`
	MergeCalls        uint64 `json:"merge_calls,omitempty"`
	MergeStacksIn     uint64 `json:"merge_stacks_in,omitempty"`
	MergeStacksOut    uint64 `json:"merge_stacks_out,omitempty"`
	MergeStacksInMax  int    `json:"merge_stacks_in_max,omitempty"`
	MergeStacksOutMax int    `json:"merge_stacks_out_max,omitempty"`
}

type modeAgg struct {
	rows      int
	errors    int
	ns        int64
	bytes     int64
	bop       int64
	allocs    float64
	runtime   runtimeStats
	maxStacks int
}

type langAgg struct {
	language       string
	samples        map[string]struct{}
	bytesBySample  map[string]int
	parityFailures int
	queryFailures  int
	highFailures   int
	modes          map[string]*modeAgg
}

type langScore struct {
	lang              string
	samples           int
	parityFailures    int
	queryFailures     int
	highlightFailures int
	bytes             int64
	cgoFull           int64
	goFull            int64
	goNoTree          int64
	goQuery           int64
	goCursor          int64
	goEdit            int64
	goNoop            int64
	fullRatio         float64
	noTreeRatio       float64
	queryRatio        float64
	queryOverFull     float64
	fullOverNoTree    float64
	attrs             map[string]float64
	hotAmbiguities    []hotGLRState
	hotReduceChains   []hotGLRState
	hotMergeStates    []hotGLRState
	bucket            string
}

func main() {
	var resultsPath string
	flag.StringVar(&resultsPath, "results", "", "parse_gap_report results.jsonl path")
	flag.Parse()
	if strings.TrimSpace(resultsPath) == "" {
		fmt.Fprintln(os.Stderr, "--results is required")
		os.Exit(2)
	}
	rows, err := readRows(resultsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read results: %v\n", err)
		os.Exit(1)
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no rows")
		os.Exit(1)
	}
	scores := scoreRows(rows)
	render(scores)
}

func readRows(path string) ([]reportRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []reportRow
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 64*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row reportRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("decode line %d: %w", len(rows)+1, err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func scoreRows(rows []reportRow) []langScore {
	langs := map[string]*langAgg{}
	for _, row := range rows {
		agg := langs[row.Language]
		if agg == nil {
			agg = &langAgg{
				language:      row.Language,
				samples:       map[string]struct{}{},
				bytesBySample: map[string]int{},
				modes:         map[string]*modeAgg{},
			}
			langs[row.Language] = agg
		}
		agg.samples[row.Sample] = struct{}{}
		if row.Bytes > agg.bytesBySample[row.Sample] {
			agg.bytesBySample[row.Sample] = row.Bytes
		}
		if row.Parity.Error != "" {
			agg.parityFailures++
		}
		if row.Parity.Query != nil && !*row.Parity.Query {
			agg.queryFailures++
		}
		if row.Parity.Highlight != nil && !*row.Parity.Highlight {
			agg.highFailures++
		}
		m := agg.modes[row.Mode]
		if m == nil {
			m = &modeAgg{}
			agg.modes[row.Mode] = m
		}
		if row.Error != "" {
			m.errors++
		}
		if row.MedianNS > 0 && row.Error == "" {
			m.rows++
			m.ns += row.MedianNS
			m.bytes += int64(row.Bytes)
			m.bop += row.BOp
			m.allocs += row.AllocsOp
			m.runtime.add(row.Runtime)
			if row.Runtime.MaxStacksSeen > m.maxStacks {
				m.maxStacks = row.Runtime.MaxStacksSeen
			}
		}
	}
	scores := make([]langScore, 0, len(langs))
	for _, agg := range langs {
		var bytes int64
		for _, n := range agg.bytesBySample {
			bytes += int64(n)
		}
		s := langScore{
			lang:              agg.language,
			samples:           len(agg.samples),
			parityFailures:    agg.parityFailures,
			queryFailures:     agg.queryFailures,
			highlightFailures: agg.highFailures,
			bytes:             bytes,
			cgoFull:           modeNS(agg, "cgo_full"),
			goFull:            modeNS(agg, "go_full"),
			goNoTree:          modeNS(agg, "go_no_tree"),
			goQuery:           modeNS(agg, "go_parse_query"),
			goCursor:          modeNS(agg, "go_cursor_walk"),
			goEdit:            modeNS(agg, "go_edit"),
			goNoop:            modeNS(agg, "go_noop_incremental"),
			attrs:             map[string]float64{},
		}
		s.fullRatio = ratio(s.goFull, s.cgoFull)
		s.noTreeRatio = ratio(s.goNoTree, s.cgoFull)
		s.queryRatio = ratio(s.goQuery, s.cgoFull)
		s.queryOverFull = ratio(s.goQuery, s.goFull)
		s.fullOverNoTree = ratio(s.goFull, s.goNoTree)
		if gf := agg.modes["go_full"]; gf != nil {
			r := gf.runtime
			tokens := float64(max64(r.Tokens, 1))
			bytesDen := float64(maxInt64(gf.bytes, 1))
			wall := float64(maxInt64(r.ParseWallNS, gf.ns))
			s.attrs["no_tree_ratio"] = s.noTreeRatio
			s.attrs["full_over_no_tree"] = s.fullOverNoTree
			s.attrs["query_over_full"] = s.queryOverFull
			s.attrs["bytes"] = float64(bytes)
			s.attrs["tokens_per_kb"] = tokens / (bytesDen / 1024.0)
			s.attrs["gss_per_token"] = float64(r.GSSNodes) / tokens
			s.attrs["final_nodes_per_token"] = float64(r.FinalNodes) / tokens
			s.attrs["arena_live_per_byte"] = float64(r.ArenaLiveB) / bytesDen
			s.attrs["arena_capacity_waste_per_byte"] = float64(r.ArenaCapacityWaste) / bytesDen
			s.attrs["public_materialized_per_token"] = float64(r.PublicNodesMaterialized) / tokens
			s.attrs["final_child_drains_per_token"] = float64(r.FinalChildRangeDrains) / tokens
			s.attrs["dense_fallbacks_per_token"] = float64(r.DenseFallbacks) / tokens
			s.attrs["merge_calls_per_token"] = float64(r.MergeCalls) / tokens
			s.attrs["forks_per_token"] = float64(r.ForkCount) / tokens
			s.attrs["merge_stack_in_per_token"] = float64(r.MergeStacksIn) / tokens
			s.attrs["multi_stack_token_share"] = float64(r.MultiStackTokens) / tokens
			s.attrs["reduce_steps_per_token"] = float64(r.ReduceChainSteps) / tokens
			s.attrs["normalization_nodes_per_token"] = float64(r.NormalizationNodes) / tokens
			s.attrs["normalization_rewrites_per_token"] = float64(r.NormalizationRewrites) / tokens
			s.attrs["parser_loop_share"] = float64(r.ParserLoopNS) / wall
			s.attrs["token_next_share"] = float64(r.TokenNextNS) / wall
			s.attrs["action_lookup_share"] = float64(r.ActionLookupNS) / wall
			s.attrs["action_dispatch_share"] = float64(r.ActionDispatchNS) / wall
			s.attrs["glr_merge_share"] = float64(r.GLRMergeNS) / wall
			s.attrs["result_build_share"] = float64(r.ResultBuildNS) / wall
			s.attrs["result_compat_share"] = float64(r.ResultCompatibilityNS) / wall
			s.attrs["normalization_share"] = float64(r.NormalizationNS) / wall
			s.hotAmbiguities = topHotStates(r.HotAmbiguities, func(h hotGLRState) uint64 {
				if h.StackInTotal > 0 {
					return h.StackInTotal
				}
				return h.Hits
			}, 5)
			s.hotReduceChains = topHotStates(r.HotReduceChains, func(h hotGLRState) uint64 {
				return h.ReduceChainSteps
			}, 5)
			s.hotMergeStates = topHotStates(r.HotMergeStates, func(h hotGLRState) uint64 {
				return h.MergeStacksIn
			}, 5)
		}
		s.bucket = classify(s)
		scores = append(scores, s)
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].fullRatio == scores[j].fullRatio {
			return scores[i].lang < scores[j].lang
		}
		return scores[i].fullRatio > scores[j].fullRatio
	})
	return scores
}

func modeNS(agg *langAgg, mode string) int64 {
	if agg == nil || agg.modes[mode] == nil {
		return 0
	}
	return agg.modes[mode].ns
}

func classify(s langScore) string {
	if s.parityFailures > 0 {
		return "parity_blocked"
	}
	coreGap := s.noTreeRatio > 2.0
	resultGap := s.fullOverNoTree > 2.0
	switch {
	case s.fullRatio == 0:
		return "missing_full_timing"
	case coreGap && resultGap:
		return "parser_core_plus_result_gap"
	case coreGap:
		return "parser_core_gap"
	case resultGap:
		return "result_or_materialization_gap"
	case s.queryOverFull > 1.5:
		return "query_payback_gap"
	case s.fullRatio <= 1.15:
		return "cgo_class"
	case s.fullRatio <= 2.0:
		return "sub_2x_followup"
	default:
		return "mixed_gap"
	}
}

func render(scores []langScore) {
	fmt.Println("# Parse Gap Correlation")
	fmt.Println()
	fmt.Println("Ratios use summed per-sample medians, so large and small files both contribute their measured lane cost. Correlations are directional; top-12 rings are intentionally small.")
	fmt.Println()
	fmt.Println("| lang | samples | parity_fail | full/cgo | no_tree/cgo | full/no_tree | query/full | bytes | bucket |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |")
	for _, s := range scores {
		fmt.Printf("| %s | %d | %d | %s | %s | %s | %s | %d | %s |\n",
			s.lang,
			s.samples,
			s.parityFailures,
			ratioText(s.fullRatio),
			ratioText(s.noTreeRatio),
			ratioText(s.fullOverNoTree),
			ratioText(s.queryOverFull),
			s.bytes,
			s.bucket,
		)
	}

	fmt.Println()
	fmt.Println("## Go Full Attribution")
	fmt.Println()
	fmt.Println("| lang | go_full | cgo_full | go_no_tree | parser_loop | token_next | action_lookup | glr_merge | result_build | compat | gss/token | merge/token | public_mat/token | drains/token |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, s := range scores {
		fmt.Printf("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %.3f | %.3f | %.3f | %.3f |\n",
			s.lang,
			nsText(s.goFull),
			nsText(s.cgoFull),
			nsText(s.goNoTree),
			pctText(s.attrs["parser_loop_share"]),
			pctText(s.attrs["token_next_share"]),
			pctText(s.attrs["action_lookup_share"]),
			pctText(s.attrs["glr_merge_share"]),
			pctText(s.attrs["result_build_share"]),
			pctText(s.attrs["result_compat_share"]),
			s.attrs["gss_per_token"],
			s.attrs["merge_calls_per_token"],
			s.attrs["public_materialized_per_token"],
			s.attrs["final_child_drains_per_token"],
		)
	}

	type corr struct {
		name string
		r    float64
		n    int
	}
	var corrs []corr
	names := attrNames(scores)
	for _, name := range names {
		var xs, ys []float64
		for _, s := range scores {
			x, ok := s.attrs[name]
			if !ok || !finite(x) || !finite(s.fullRatio) || s.fullRatio == 0 {
				continue
			}
			xs = append(xs, x)
			ys = append(ys, s.fullRatio)
		}
		if len(xs) < 3 {
			continue
		}
		corrs = append(corrs, corr{name: name, r: pearson(xs, ys), n: len(xs)})
	}
	sort.Slice(corrs, func(i, j int) bool {
		ai, aj := math.Abs(corrs[i].r), math.Abs(corrs[j].r)
		if ai == aj {
			return corrs[i].name < corrs[j].name
		}
		return ai > aj
	})
	fmt.Println()
	fmt.Println("## Strongest Directional Correlations")
	fmt.Println()
	fmt.Println("| attribute | r(full/cgo) | n |")
	fmt.Println("| --- | ---: | ---: |")
	limit := min(len(corrs), 16)
	for i := 0; i < limit; i++ {
		fmt.Printf("| %s | %.3f | %d |\n", corrs[i].name, corrs[i].r, corrs[i].n)
	}

	if hasHotShapes(scores) {
		fmt.Println()
		fmt.Println("## Hot GLR Shapes")
		fmt.Println()
		for _, s := range scores {
			if len(s.hotAmbiguities) == 0 && len(s.hotReduceChains) == 0 && len(s.hotMergeStates) == 0 {
				continue
			}
			fmt.Printf("### %s\n\n", s.lang)
			printHotTable("fork/action buckets", s.hotAmbiguities, "stack_in")
			printHotTable("reduce-chain buckets", s.hotReduceChains, "reduce_steps")
			printHotTable("merge-state buckets", s.hotMergeStates, "merge_in")
		}
	}

	fmt.Println()
	fmt.Println("## Attention Buckets")
	fmt.Println()
	buckets := map[string][]string{}
	for _, s := range scores {
		buckets[s.bucket] = append(buckets[s.bucket], s.lang)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sort.Strings(buckets[key])
		fmt.Printf("- `%s`: %s\n", key, strings.Join(buckets[key], ", "))
	}
}

func topHotStates(in []hotGLRState, score func(h hotGLRState) uint64, limit int) []hotGLRState {
	if len(in) == 0 || limit == 0 {
		return nil
	}
	out := aggregateHotStates(in)
	sort.Slice(out, func(i, j int) bool {
		di, dj := score(out[i]), score(out[j])
		if di == dj {
			if out[i].State == out[j].State {
				return out[i].Lookahead < out[j].Lookahead
			}
			return out[i].State < out[j].State
		}
		return di > dj
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func aggregateHotStates(in []hotGLRState) []hotGLRState {
	type key struct {
		state       uint32
		lookahead   uint16
		actionCount uint8
		shiftCount  uint8
		reduceCount uint8
	}
	byKey := map[key]*hotGLRState{}
	for _, h := range in {
		k := key{h.State, h.Lookahead, h.ActionCount, h.ShiftCount, h.ReduceCount}
		dst := byKey[k]
		if dst == nil {
			copied := h
			byKey[k] = &copied
			continue
		}
		dst.Hits += h.Hits
		dst.Forks += h.Forks
		dst.MultiStackHits += h.MultiStackHits
		dst.StackInTotal += h.StackInTotal
		if h.StackInMax > dst.StackInMax {
			dst.StackInMax = h.StackInMax
		}
		dst.ReduceChainHits += h.ReduceChainHits
		dst.ReduceChainSteps += h.ReduceChainSteps
		if h.ReduceChainMaxLen > dst.ReduceChainMaxLen {
			dst.ReduceChainMaxLen = h.ReduceChainMaxLen
		}
		dst.MergeCalls += h.MergeCalls
		dst.MergeStacksIn += h.MergeStacksIn
		dst.MergeStacksOut += h.MergeStacksOut
		if h.MergeStacksInMax > dst.MergeStacksInMax {
			dst.MergeStacksInMax = h.MergeStacksInMax
		}
		if h.MergeStacksOutMax > dst.MergeStacksOutMax {
			dst.MergeStacksOutMax = h.MergeStacksOutMax
		}
	}
	out := make([]hotGLRState, 0, len(byKey))
	for _, h := range byKey {
		out = append(out, *h)
	}
	return out
}

func hasHotShapes(scores []langScore) bool {
	for _, s := range scores {
		if len(s.hotAmbiguities) > 0 || len(s.hotReduceChains) > 0 || len(s.hotMergeStates) > 0 {
			return true
		}
	}
	return false
}

func printHotTable(label string, rows []hotGLRState, metric string) {
	if len(rows) == 0 {
		return
	}
	fmt.Printf("`%s`\n\n", label)
	fmt.Println("| state | lookahead | actions | shifts | reduces | forks | multi_stack | stack_in | reduce_steps | merge_in | merge_out | metric |")
	fmt.Println("| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, h := range rows {
		var score uint64
		switch metric {
		case "reduce_steps":
			score = h.ReduceChainSteps
		case "merge_in":
			score = h.MergeStacksIn
		default:
			score = h.StackInTotal
		}
		fmt.Printf("| %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			h.State,
			h.Lookahead,
			h.ActionCount,
			h.ShiftCount,
			h.ReduceCount,
			h.Forks,
			h.MultiStackHits,
			h.StackInTotal,
			h.ReduceChainSteps,
			h.MergeStacksIn,
			h.MergeStacksOut,
			score,
		)
	}
	fmt.Println()
}

func attrNames(scores []langScore) []string {
	seen := map[string]struct{}{}
	for _, s := range scores {
		for name := range s.attrs {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func pearson(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < 2 {
		return 0
	}
	var sx, sy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
	}
	mx := sx / float64(len(xs))
	my := sy / float64(len(ys))
	var num, dx, dy float64
	for i := range xs {
		x := xs[i] - mx
		y := ys[i] - my
		num += x * y
		dx += x * x
		dy += y * y
	}
	if dx == 0 || dy == 0 {
		return 0
	}
	return num / math.Sqrt(dx*dy)
}

func (r *runtimeStats) add(o runtimeStats) {
	r.Tokens += o.Tokens
	r.NodesAllocated += o.NodesAllocated
	r.FinalNodes += o.FinalNodes
	r.GSSNodes += o.GSSNodes
	if o.MaxStacksSeen > r.MaxStacksSeen {
		r.MaxStacksSeen = o.MaxStacksSeen
	}
	r.SingleStackTokens += o.SingleStackTokens
	r.MultiStackTokens += o.MultiStackTokens
	r.MergeStacksIn += o.MergeStacksIn
	r.MergeStacksOut += o.MergeStacksOut
	r.ArenaLiveB += o.ArenaLiveB
	r.ArenaCapacityB += o.ArenaCapacityB
	r.ArenaCapacityWaste += o.ArenaCapacityWaste
	r.FinalChildRangeDrains += o.FinalChildRangeDrains
	r.PublicNodesMaterialized += o.PublicNodesMaterialized
	r.DenseFallbacks += o.DenseFallbacks
	r.ResultSelectionNS += o.ResultSelectionNS
	r.ResultBuildNS += o.ResultBuildNS
	r.ResultCompatibilityNS += o.ResultCompatibilityNS
	r.ResultParentLinkNS += o.ResultParentLinkNS
	r.ResultFinalizeRootNS += o.ResultFinalizeRootNS
	r.ResultExtendTrailingNS += o.ResultExtendTrailingNS
	r.NormalizationNS += o.NormalizationNS
	r.NormalizationNodes += o.NormalizationNodes
	r.NormalizationRewrites += o.NormalizationRewrites
	r.ParseWallNS += o.ParseWallNS
	r.ParserLoopNS += o.ParserLoopNS
	r.TokenNextNS += o.TokenNextNS
	r.ActionDispatchNS += o.ActionDispatchNS
	r.ActionLookupNS += o.ActionLookupNS
	r.GLRMergeNS += o.GLRMergeNS
	r.GLRCullNS += o.GLRCullNS
	r.MergeCalls += o.MergeCalls
	r.ForkCount += o.ForkCount
	r.ConflictRR += o.ConflictRR
	r.ConflictRS += o.ConflictRS
	r.ConflictOther += o.ConflictOther
	r.LexBytes += o.LexBytes
	r.LexTokens += o.LexTokens
	r.ReduceChainSteps += o.ReduceChainSteps
	if o.ReduceChainMaxLen > r.ReduceChainMaxLen {
		r.ReduceChainMaxLen = o.ReduceChainMaxLen
	}
	r.NoTreeReduceNodes += o.NoTreeReduceNodes
	r.NoTreeLeafNodes += o.NoTreeLeafNodes
	r.HotAmbiguities = append(r.HotAmbiguities, o.HotAmbiguities...)
	r.HotReduceChains = append(r.HotReduceChains, o.HotReduceChains...)
	r.HotMergeStates = append(r.HotMergeStates, o.HotMergeStates...)
}

func ratio(num, den int64) float64 {
	if num <= 0 || den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func ratioText(v float64) string {
	if v <= 0 || !finite(v) {
		return "n/a"
	}
	return fmt.Sprintf("%.2fx", v)
}

func nsText(ns int64) string {
	if ns <= 0 {
		return "n/a"
	}
	if ns >= 1_000_000 {
		return fmt.Sprintf("%.2fms", float64(ns)/1_000_000)
	}
	if ns >= 1_000 {
		return fmt.Sprintf("%.2fus", float64(ns)/1_000)
	}
	return fmt.Sprintf("%dns", ns)
}

func pctText(v float64) string {
	if v <= 0 || !finite(v) {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", v*100)
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func max64(v, floor uint64) uint64 {
	if v < floor {
		return floor
	}
	return v
}

func maxInt64(v, floor int64) int64 {
	if v < floor {
		return floor
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
