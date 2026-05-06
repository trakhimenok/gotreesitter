package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestRepairNoLookaheadLexModes(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	// The no-lookahead repair logic writes ^uint32(0) sentinel LexStateIndex
	// values into the last few LexModes entries (one per repaired state).
	// Use a tail-relative offset so the fixture survives blob regens that
	// add new states ahead of the sentinels. Negative `state` means
	// "len(LexModes) + state" — i.e. -4 is the fourth-from-last entry,
	// which is the first repaired sentinel slot for grammars that repair
	// four no-lookahead states.
	tests := []struct {
		name  string
		load  func() []gotreesitter.LexMode
		state int
	}{
		{
			name:  "scala",
			load:  func() []gotreesitter.LexMode { return ScalaLanguage().LexModes },
			state: -4,
		},
		{
			name:  "rust",
			load:  func() []gotreesitter.LexMode { return RustLanguage().LexModes },
			state: 3820,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lexModes := tc.load()
			idx := tc.state
			if idx < 0 {
				idx = len(lexModes) + idx
			}
			if idx < 0 || idx >= len(lexModes) {
				t.Fatalf("state %d (resolved %d) out of range for %s (len=%d)",
					tc.state, idx, tc.name, len(lexModes))
			}
			if got := lexModes[idx].LexStateIndex(); got != ^uint32(0) {
				t.Fatalf("LexModes[%d].LexStateIndex = %d, want %d", idx, got, ^uint32(0))
			}
		})
	}
}
