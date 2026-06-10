# Tier classification scan (release gate)

Parity-vs-C is the correctness gate: a grammar is **CLEAN** when every
measured real-corpus file parses byte-identical (type/span/child-count) to
the tree-sitter C oracle; anything below 100% is incorrect-parse. The scan
makes **uncharacterized incorrect parse** the transitory tier we drive to
zero: every non-clean grammar must carry a named, assessed sub-tier in
`tier_classification.tsv`, and the committed ratchet (`clean_grammars.txt`)
makes clean→incorrect regressions release-blocking.

## Tier taxonomy

One tier scale for the whole program (canonical: `docs/reports/tier-ratchet.md`).
**Parity vs C is the hard gate; performance is the sub-rank.** A grammar that
is not byte-clean against the C oracle is **tier IV, full stop** — a fast
wrong parser is worthless. Tiers I–III are reserved for parity-clean grammars,
ranked by performance:

| tier | meaning | rule |
| --- | --- | --- |
| `I` | parity-clean, fast | ≤1.5× C full-parse, cold ≤5ms, blob ≤150KB |
| `II` | parity-clean, ok | ≤8× C full-parse, cold ≤20ms |
| `III` | parity-clean, poor | >8× C, or cold >20ms, or blob >400KB |
| `IV` | **not parity-clean** (any divergence, truncation, or unmeasured parity) | fix parity — see sub-causes below |

### Tier-IV sub-causes (`tier_classification.tsv`)

Every tier-IV grammar carries a named, assessed sub-cause:

| sub-cause | meaning | fix recipe |
| --- | --- | --- |
| `IV-recovery` | both parsers see errors, but C contains damage locally where Go fragments / roots ERROR | faithful C error-cost version competition (see `recovery-cost-competition.md`) |
| `IV-shape` | tree-shape divergence without error nodes | per-grammar diagnosis (`TestFirstDiffDiag`) |
| `IV-scanner` | Go external-scanner port diverges from C (over/under-permissive, token boundaries) | re-port `grammars/<g>_scanner.go` from pinned upstream `src/scanner.c` |
| `IV-version` | most corpus files error in BOTH parsers — corpus uses syntax newer than the embedded grammar | bump the embedded grammar version |
| `IV-stackcap` | divergence/truncation clears or shrinks at `GOT_GLR_MAX_STACKS=2` | add an `effectiveFullParseInitialMaxStacks` cap entry |
| `IV-extmap` | zero/few files measured under the current extension set | add a curated source-extension mapping |
| `IV-perf` | cannot measure within timeout (O(N²) or pathological file) | profile/fix before parity (overlaps the perf push) |
| `IV-unknown` | diagnosed but does not fit a single bucket | deeper single-file diagnosis |
| `IV-unassessed` | **the state we keep empty** — a measured incorrect parse nobody has triaged | run the diagnosis workflow / `TestFirstDiffDiag` |

A `?` suffix (e.g. `IV-recovery?`) marks a *preliminary* classification
inferred from the measure signature (parity%, errTree, trunc) rather than a
per-file diagnosis — these get confirmed when the full diagnosis workflow
re-runs. The scan fails (exit 1) if any tier-IV grammar is `IV-unassessed`
or missing a row, so the uncharacterized count is enforced at zero.

The deep-diagnosis evidence and proposed fixes for the first wave live in
`../../.campaign_state/diagnosis_classified.json`.

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
