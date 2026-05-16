package gotreesitter

import (
	"sort"
	"sync"
)

// AmbiguityProfile aggregates parser states/lookaheads that contribute to GLR
// fanout. It is intended for diagnostics and benchmark runs, not normal API use.
type AmbiguityProfile struct {
	mu      sync.Mutex
	entries map[AmbiguityKey]*AmbiguityStat
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
	State        StateID
	Lookahead    Symbol
	ActionCount  uint8
	ShiftCount   uint8
	ReduceCount  uint8
	Actions      []ParseAction
	Hits         uint64
	StackInTotal uint64
	StackInMax   int
}

// NewAmbiguityProfile creates an empty GLR ambiguity profile.
func NewAmbiguityProfile() *AmbiguityProfile {
	return &AmbiguityProfile{entries: make(map[AmbiguityKey]*AmbiguityStat)}
}

// Reset clears all accumulated ambiguity counters.
func (p *AmbiguityProfile) Reset() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	clear(p.entries)
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
	if stackCount > stat.StackInMax {
		stat.StackInMax = stackCount
	}
	if stackCount > 0 {
		stat.StackInTotal += uint64(stackCount)
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
