package gotreesitter

import (
	"sort"
	"sync"
)

// AmbiguityProfile aggregates parser states/lookaheads that contribute to GLR
// fanout. It is intended for diagnostics and benchmark runs, not normal API use.
type AmbiguityProfile struct {
	mu            sync.Mutex
	entries       map[AmbiguityKey]*AmbiguityStat
	reduceEntries map[AmbiguityKey]*AmbiguityStat
	mergeEntries  map[StateID]*AmbiguityStat
}

// AmbiguityKey identifies one parse-table ambiguity bucket.
type AmbiguityKey struct {
	State       StateID
	Lookahead   Symbol
	ActionCount uint8
	ShiftCount  uint8
	ReduceCount uint8
}

// AmbiguityStat is a snapshot row from AmbiguityProfile.
type AmbiguityStat struct {
	State             StateID
	Lookahead         Symbol
	ActionCount       uint8
	ShiftCount        uint8
	ReduceCount       uint8
	Actions           []ParseAction
	Hits              uint64
	Forks             uint64
	MultiStackHits    uint64
	StackInTotal      uint64
	StackInMax        int
	ReduceChainHits   uint64
	ReduceChainSteps  uint64
	ReduceChainMaxLen int
	MergeCalls        uint64
	MergeStacksIn     uint64
	MergeStacksOut    uint64
	MergeStacksInMax  int
	MergeStacksOutMax int
}

// NewAmbiguityProfile creates an empty GLR ambiguity profile.
func NewAmbiguityProfile() *AmbiguityProfile {
	return &AmbiguityProfile{
		entries:       make(map[AmbiguityKey]*AmbiguityStat),
		reduceEntries: make(map[AmbiguityKey]*AmbiguityStat),
		mergeEntries:  make(map[StateID]*AmbiguityStat),
	}
}

// Reset clears all accumulated ambiguity counters.
func (p *AmbiguityProfile) Reset() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	clear(p.entries)
	clear(p.reduceEntries)
	clear(p.mergeEntries)
}

// SnapshotTop returns the highest-impact ambiguity buckets ordered by stack
// pressure, then hit count.
func (p *AmbiguityProfile) SnapshotTop(limit int) []AmbiguityStat {
	if p == nil || limit == 0 {
		return nil
	}
	p.mu.Lock()
	out := make([]AmbiguityStat, 0, len(p.entries))
	for _, stat := range p.entries {
		copied := *stat
		if len(stat.Actions) > 0 {
			copied.Actions = append([]ParseAction(nil), stat.Actions...)
		}
		out = append(out, copied)
	}
	p.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].StackInTotal != out[j].StackInTotal {
			return out[i].StackInTotal > out[j].StackInTotal
		}
		if out[i].Hits != out[j].Hits {
			return out[i].Hits > out[j].Hits
		}
		if out[i].StackInMax != out[j].StackInMax {
			return out[i].StackInMax > out[j].StackInMax
		}
		if out[i].State != out[j].State {
			return out[i].State < out[j].State
		}
		return out[i].Lookahead < out[j].Lookahead
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SnapshotTopReduceChains returns the parser states/lookaheads that spent the
// most time in deterministic reduce-chain fusion.
func (p *AmbiguityProfile) SnapshotTopReduceChains(limit int) []AmbiguityStat {
	if p == nil || limit == 0 {
		return nil
	}
	p.mu.Lock()
	out := make([]AmbiguityStat, 0, len(p.reduceEntries))
	for _, stat := range p.reduceEntries {
		out = append(out, *stat)
	}
	p.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].ReduceChainSteps != out[j].ReduceChainSteps {
			return out[i].ReduceChainSteps > out[j].ReduceChainSteps
		}
		if out[i].ReduceChainHits != out[j].ReduceChainHits {
			return out[i].ReduceChainHits > out[j].ReduceChainHits
		}
		if out[i].ReduceChainMaxLen != out[j].ReduceChainMaxLen {
			return out[i].ReduceChainMaxLen > out[j].ReduceChainMaxLen
		}
		if out[i].State != out[j].State {
			return out[i].State < out[j].State
		}
		return out[i].Lookahead < out[j].Lookahead
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SnapshotTopMergeStates returns parser states that most often participate in
// multi-stack merge passes. These rows are keyed by state only, because merge
// happens before the next lookahead dispatch.
func (p *AmbiguityProfile) SnapshotTopMergeStates(limit int) []AmbiguityStat {
	if p == nil || limit == 0 {
		return nil
	}
	p.mu.Lock()
	out := make([]AmbiguityStat, 0, len(p.mergeEntries))
	for _, stat := range p.mergeEntries {
		out = append(out, *stat)
	}
	p.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].MergeStacksIn != out[j].MergeStacksIn {
			return out[i].MergeStacksIn > out[j].MergeStacksIn
		}
		if out[i].MergeCalls != out[j].MergeCalls {
			return out[i].MergeCalls > out[j].MergeCalls
		}
		if out[i].MergeStacksOut != out[j].MergeStacksOut {
			return out[i].MergeStacksOut > out[j].MergeStacksOut
		}
		return out[i].State < out[j].State
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (p *AmbiguityProfile) record(state StateID, lookahead Symbol, actions []ParseAction, stackCount int) {
	if p == nil || (len(actions) <= 1 && stackCount <= 1) {
		return
	}
	var shifts, reduces int
	for _, action := range actions {
		switch action.Type {
		case ParseActionShift:
			shifts++
		case ParseActionReduce:
			reduces++
		}
	}
	key := AmbiguityKey{
		State:       state,
		Lookahead:   lookahead,
		ActionCount: saturatingUint8(len(actions)),
		ShiftCount:  saturatingUint8(shifts),
		ReduceCount: saturatingUint8(reduces),
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.entries == nil {
		p.entries = make(map[AmbiguityKey]*AmbiguityStat)
	}
	stat := p.entries[key]
	if stat == nil {
		stat = &AmbiguityStat{
			State:       state,
			Lookahead:   lookahead,
			ActionCount: key.ActionCount,
			ShiftCount:  key.ShiftCount,
			ReduceCount: key.ReduceCount,
			Actions:     append([]ParseAction(nil), actions...),
		}
		p.entries[key] = stat
	}
	stat.Hits++
	if len(actions) > 1 {
		stat.Forks++
	}
	if stackCount > 1 {
		stat.MultiStackHits++
	}
	if stackCount > stat.StackInMax {
		stat.StackInMax = stackCount
	}
	if stackCount > 0 {
		stat.StackInTotal += uint64(stackCount)
	}
}

func (p *AmbiguityProfile) recordReduceChainStep(state StateID, lookahead Symbol, chainLen int) {
	if p == nil {
		return
	}
	key := AmbiguityKey{State: state, Lookahead: lookahead}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reduceEntries == nil {
		p.reduceEntries = make(map[AmbiguityKey]*AmbiguityStat)
	}
	stat := p.reduceEntries[key]
	if stat == nil {
		stat = &AmbiguityStat{State: state, Lookahead: lookahead}
		p.reduceEntries[key] = stat
	}
	stat.ReduceChainHits++
	stat.ReduceChainSteps++
	if chainLen > stat.ReduceChainMaxLen {
		stat.ReduceChainMaxLen = chainLen
	}
}

func (p *AmbiguityProfile) recordMergeBefore(stacks []glrStack) {
	if p == nil || len(stacks) <= 1 {
		return
	}
	counts := map[StateID]int{}
	for i := range stacks {
		if stacks[i].dead || len(stacks[i].entries) == 0 {
			continue
		}
		counts[stacks[i].top().state]++
	}
	if len(counts) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mergeEntries == nil {
		p.mergeEntries = make(map[StateID]*AmbiguityStat)
	}
	for state, count := range counts {
		stat := p.mergeEntries[state]
		if stat == nil {
			stat = &AmbiguityStat{State: state}
			p.mergeEntries[state] = stat
		}
		stat.MergeCalls++
		stat.MergeStacksIn += uint64(count)
		if count > stat.MergeStacksInMax {
			stat.MergeStacksInMax = count
		}
	}
}

func (p *AmbiguityProfile) recordMergeAfter(stacks []glrStack) {
	if p == nil || len(stacks) == 0 {
		return
	}
	counts := map[StateID]int{}
	for i := range stacks {
		if stacks[i].dead || len(stacks[i].entries) == 0 {
			continue
		}
		counts[stacks[i].top().state]++
	}
	if len(counts) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mergeEntries == nil {
		p.mergeEntries = make(map[StateID]*AmbiguityStat)
	}
	for state, count := range counts {
		stat := p.mergeEntries[state]
		if stat == nil {
			stat = &AmbiguityStat{State: state}
			p.mergeEntries[state] = stat
		}
		stat.MergeStacksOut += uint64(count)
		if count > stat.MergeStacksOutMax {
			stat.MergeStacksOutMax = count
		}
	}
}

func saturatingUint8(n int) uint8 {
	if n <= 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return uint8(n)
}
