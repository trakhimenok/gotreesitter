# Spec: faithful C error-cost version competition (III-recovery unlock)

The largest tier-IV bucket — ~90 grammars classed `III-recovery` /
`III-recovery?` (asm, c, cpp, c_sharp, glsl, graphql, groovy, hare, html,
julia, luau, ninja, scheme, zig, typst, …) — all share one root cause and
one fix. This is the highest-leverage remaining correctness lever.

## Root cause

When a token has no parse action, tree-sitter C (`ts_parser__recover`) does
**version competition**: it pauses the erroring stack version in
`ERROR_STATE` (which keeps absorbing tokens into one open error subtree) and,
at each subsequent token, forks *resume candidates* that pop/wrap/resume. All
versions then compete by **error cost** (`ts_subtree_error_cost`):

```
cost = ERROR_COST_PER_RECOVERY(500) * num_error_regions
     + ERROR_COST_PER_SKIPPED_CHAR(1) * skipped_bytes
     + ERROR_COST_PER_SKIPPED_LINE(2) * skipped_lines
     + ERROR_COST_PER_SKIPPED_TREE(100) * skipped_visible_trees
     + ERROR_COST_PER_MISSING_TREE(110) * missing_leaves
```

The cheapest version wins, so C reliably prefers **one locally-contained
error region under a named root** over many fragments or an ERROR root. Go's
production GLR commits to a single recovery strategy at the failure point
(`tryResyncErrorRecovery` / `pushOrExtendErrorNode`), so it fragments where C
contains, and roots ERROR where C keeps the start symbol.

## What was prototyped (and why it was backed out)

A prototype (`parser_recovery_competition.go`, preserved in the
`spore.2026-06-09.local.tier-iv-burndown` attachment / git history of branch
`parity/c-oracle-clears`) added:

- `openErr *Node` on `glrStack` — the C `ERROR_STATE` analogue (absorbing
  version accumulates tokens into one open ERROR wrapper).
- `spawnAbsorbingErrorFork` at the no-action site + `absorbingStackIteration`
  that forks a *close candidate* when a simulated `[push ERROR + all
  potential reductions]` makes the lookahead shiftable.
- `stackResultErrorCost` (the formula above) folded into
  `stackCompareForResultSelection` as a tiebreak among equal error-rank
  versions.

**Result:** it cleared scheme's `ez-grammar-test.ss` and the gensym
`'#{name uid}` shape, but **regressed `s/4.ss`** from one localized list
child-count diff to a root ERROR (cc=55), and was **neutral on zig/typst**.
Verdict: the concept is correct but the implementation is not faithful
enough — the absorbing version wins selection on `4.ss` despite producing a
worse tree, meaning either the close-candidate never fires there or the cost
comparison is mis-scoped (it must compete the absorbing version against the
legacy resume version, and the absorbing version must keep reducing toward
the start symbol, not accumulate flat children). Per the campaign's
no-regressions rule it was backed out of the shipped engine.

## What "faithful" requires (next session)

1. **Version set, not single strategy.** On no-action, keep BOTH the
   absorbing version and the resume version(s) alive as real GLR stacks; let
   `stackCompareForResultSelection` (error-cost tiebreak) pick at the end —
   do not let the absorbing path replace the resume path.
2. **Absorbing version must reduce.** The open ERROR must be periodically
   closed-and-reduced so the surrounding production (translation_unit /
   program / source_file) still completes; otherwise it accumulates flat
   children and roots ERROR (the `4.ss` failure).
3. **Bound version explosion.** C caps to `MAX_VERSION_COUNT` (6) and prunes
   by cost each token. The prototype's single absorbing fork is too coarse;
   real per-token candidate generation with cost pruning is needed.
4. **Validate per-grammar against the real corpus**, not synthetic repros —
   `4.ss` cleared in isolation (`/tmp/gensym.ss`) but regressed on the full
   file. Gate `errorCostCompetitionLanguage()` on one grammar at a time,
   each verified to net-improve its full corpus with zero clean-grammar
   regressions (full ratchet sweep, all non-timing fields).

## The lexer half (already shipped, keep)

The skipped-error lexing (`Lexer.NextWithErrorRuns` + `errorRunToken` +
`pushLexErrorRunLeaf`, commits 96f488db/625d2114) is the orthogonal, shipped
half: it surfaces unlexable byte runs as ERROR subtrees the way C's
`ts_parser__lex` does, gated to diff/elisp/jq. Extending error-runs to a new
grammar (e.g. scheme) needs the error-mode-retry refinement (retry the
failed lex in `LexModes[0]`, return its token before forming a skipped run)
plus the invisible-token hidden-leaf shape — both prototyped and preserved in
the same branch history; re-land them WITH the cost competition, not before.
