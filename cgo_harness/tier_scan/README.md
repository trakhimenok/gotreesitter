# Tier classification scan (release gate)

Parity-vs-C is the correctness gate: a grammar is **CLEAN** when every
measured real-corpus file parses byte-identical (type/span/child-count) to
the tree-sitter C oracle; anything below 100% is **TIER IV** (incorrect
parse). Tier IV is a transitory tier — the goal is zero members, and the
committed ratchet makes regressions release-blocking.

## Run on every release

```sh
GTS_CORPUS_DIR=/path/to/gotreesitter-corpora/corpus_sources \
  cgo_harness/docker/run_tier_scan.sh
```

- Exit 0: no previously-clean grammar regressed. The summary lists any
  NEWLY CLEAN grammars — advance `clean_grammars.txt` in the same release
  PR so the ratchet only ever grows.
- Exit 1: a grammar in `clean_grammars.txt` fell below 100% parity. The
  release must not ship until it is restored (fix the engine/grammar or
  revert the regressing change).

The corpus (~33GB of real source repos) is not vendored; the scan runs on a
host that has it (developer machine or a self-hosted runner). Hosted CI
covers the smoke/canary parity gates in `.github/workflows/ci.yml`; this
scan is the full-breadth release gate.

## Files

- `exts.tsv` — grammar → comma-separated source extensions measured for it
  (the lock-filter used by `TestMeasureDtierVsC`).
- `clean_grammars.txt` — the ratchet: grammars that must stay at 100%
  corpus parity. Append newly-clean grammars; never remove one without an
  accepted engineering decision.

## Measurement contract

`TestMeasureDtierVsC` picks the first `GTS_TIER_SCAN_N` (default 40)
path-sorted files between 32B and 200KB per grammar and compares production
`Parser.Parse` output against the linked tree-sitter C grammar node-by-node.
Grammars with no corpus dir, zero eligible files, or a per-grammar timeout
(default 600s) are reported as UNMEASURED — they are not silently dropped.
