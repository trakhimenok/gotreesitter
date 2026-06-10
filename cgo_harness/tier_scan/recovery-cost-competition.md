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

## Minimal reproducer: why "force-reduce-before-wrap" is the crux

The simplest recovery-shape failure is `requirements` on
`pkg ; python_version >= '3.13'  # note` (a trailing comment after an
environment marker — unparseable in BOTH parsers). The C oracle produces:

```
file
  requirement [0:33]   "pkg ; python_version >= '3.13'"
  ERROR [33:38]         "  # note"
```

Go shatters the line: `pkg` → ERROR, the whole line fragments, root → ERROR.

Two narrow fixes were tried and REVERTED because they did not clear it:
- Adding `requirements` to `resyncTopLevelLanguage` + a `file` root-label
  carve-out in `resultRootBuild.syntheticRootSymbol` fixed the ROOT label and
  preserved *most* structure, but Go still wrapped `requirement[0:33]` inside
  an `ERROR[0:35]` instead of keeping it whole.

Root cause, precisely: at the no-action point the `requirement` production is
**in progress on the stack, not yet reduced**. The resync preservation loop
keeps *completed* top-level siblings (those with a GOTO that reaches an
end-accepting state) — it cannot preserve a partial one. The comment
lookahead has NO action, so no reduce fires to complete the requirement.

C completes it via `ts_parser__do_all_potential_reductions`, which applies
reduces reachable on ANY symbol (including the end symbol) to close
in-progress productions BEFORE wrapping the error. **This force-reduce step is
the missing primitive** — it is what both the resync path and the absorbing
cost-competition version need, and it is why no simple tweak clears the
recovery bucket. Implement `forceReduceTowardCompletion(stack)` (apply
available reduces for the end symbol until none remain, completing
`requirement` / `function_definition` / `_sexp` / etc.), then preserve the
now-complete sibling and wrap only the genuinely-failed suffix. Validate it on
`requirements` (cleanest), then `c`/`jq` (already on resync — must not
regress), then widen.

## Why targeted primitives are insufficient (proven by implementation)

Four narrowing attempts on the `requirements` minimal case, each reverted:

1. **resync + root carve-out** — fixed the root label, but the in-progress
   `requirement` was wrapped in ERROR (not preserved): resync keeps *completed*
   siblings, can't *complete* a partial one.
2. **force-reduce on the lookahead** — no reduce action exists for the comment
   at the stuck state.
3. **force-reduce on the production's terminator + missing-terminator insertion**
   — at the stuck state (requirements state 31) there is **neither a unique
   reduce NOR a unique shift terminal** (`reduceOK=false shiftOK=false`). The
   `requirement` does not complete via any single in-place action; C reaches its
   completion through full recover candidate-generation (pop to a recoverable
   ancestor, THEN do_all_potential_reductions there, THEN missing insertion),
   not a local step.
4. Compounding this, the fragmented `bcrypt`→ERROR tree comes from the **retry
   full-parse** (`parser_retry.go`), not the first pass — so the recover fix
   must also produce a tree the retry-selection prefers (lower error cost).

Conclusion: the recovery bucket is **not assemblable from targeted primitives**.
It requires a faithful port of C's complete `ts_parser__recover` — candidate
generation across all stack ancestors + `do_all_potential_reductions` +
missing-token insertion + error-cost version competition + retry-selection
integration, working together. That is the dedicated engine project this spec
scopes; partial shortcuts either regress (scheme 4.ss) or no-op (requirements).

## The lexer half (already shipped, keep)

The skipped-error lexing (`Lexer.NextWithErrorRuns` + `errorRunToken` +
`pushLexErrorRunLeaf`, commits 96f488db/625d2114) is the orthogonal, shipped
half: it surfaces unlexable byte runs as ERROR subtrees the way C's
`ts_parser__lex` does, gated to diff/elisp/jq. Extending error-runs to a new
grammar (e.g. scheme) needs the error-mode-retry refinement (retry the
failed lex in `LexModes[0]`, return its token before forming a skipped run)
plus the invisible-token hidden-leaf shape — both prototyped and preserved in
the same branch history; re-land them WITH the cost competition, not before.
