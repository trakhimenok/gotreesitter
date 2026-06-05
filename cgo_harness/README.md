# cgo_harness

This module contains CGo-only parity and baseline benchmark harnesses used to compare `gotreesitter` against native C tree-sitter parsers.

## Unified Harness Gate

Do not start local OOM diagnosis with the unified gate runner. It aggregates
broad correctness, parity, and perf work and makes it harder to identify which
language is responsible for a memory spike.

For local work, start with one language per container:

```sh
bash cgo_harness/docker/run_single_grammar_parity.sh typescript
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs typescript
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs typescript
```

Use the unified gate runner only in CI or deliberate lab-style sweeps:

```sh
go run ./cmd/harnessgate -mode all
```

This executes:

- root correctness (`go test ./... -count=1`)
- curated cgo parity suites
- stable perf trio (optionally benchgate-compared to a baseline)

Artifacts are written under `harness_out/`.

Optional weighted confidence scoring can be enabled from `harnessgate` using
either a built-in profile (`top50`, `core90`) or a custom manifest JSON:

```sh
go run ./cmd/harnessgate -mode correctness \
  -real-corpus-dir cgo_harness/corpus_real \
  -real-corpus-langs top10 \
  -confidence-profile core90 \
  -confidence-min 0.90
```

Framework details (oracles, corpus tiers, gate policy):

- `cgo_harness/HARNESS_FRAMEWORK.md`

## Run Parity Tests

Default parity runs use `smoke` mode: a small representative subset that is
fast enough for CI. For local OOM diagnosis, prefer the single-language Docker
commands above instead of broad host-side sweeps. The direct `go test` examples
below are best treated as CI/lab references, not the default local workflow.

```sh
go test . -tags treesitter_c_parity \
  -run '^TestParityFreshParse$|^TestParityIncrementalParse$|^TestParityHasNoErrors$|^TestParityIssue3Repros$|^TestParityGLRCanaryGo$|^TestParityGLRCanarySet$|^TestParityGLRCapPressureTopLanguages$|^TestParityGateCoverageRatchet$|^TestParityHighlight$' \
  -count=1 -v
```

Set `GTS_PARITY_MODE=top50` for the top-50 correctness lock set, or
`GTS_PARITY_MODE=exhaustive` for the full curated sweep and the larger
diagnostic suites:

```sh
GTS_PARITY_MODE=top50 \
go test . -tags treesitter_c_parity \
  -run '^TestParityFreshParse$|^TestParityIncrementalParse$|^TestParityHasNoErrors$|^TestParityTop50ParseSmoke$|^TestParityTop50ParseMaterializationTrends$' \
  -count=1 -v

GTS_PARITY_MODE=exhaustive \
go test . -tags treesitter_c_parity \
  -run '^TestParityFreshParse$|^TestParityIncrementalParse$|^TestParityHasNoErrors$|^TestParityIssue3Repros$|^TestParityGLRCanaryGo$|^TestParityGLRCanarySet$|^TestParityGLRCapPressureTopLanguages$|^TestParityGateCoverageRatchet$|^TestParityHighlight$|^TestParityHighlightAllGrammars$' \
  -count=1 -v

GTS_PARITY_MODE=exhaustive \
go test . -tags treesitter_c_parity \
  -run '^TestParityCorpusFreshParse$' \
  -count=1 -v

GTS_PARITY_MODE=exhaustive \
go test . -tags treesitter_c_parity \
  -run '^TestParityYAMLCorpus$|^TestParityYAMLCorpusStructural$|^TestParityYAMLCorpusSummary$' \
  -count=1 -v
```

## Run Top-50 Parity Benchmarks

`BenchmarkParityTop50ParseFull` prechecks gotreesitter-vs-C structural parity
for each selected language, then benchmarks both parsers side by side. Keep
local diagnosis narrow with `GTS_PARITY_BENCH_LANGS`; omit it only for CI/lab
top-50 sweeps.

```sh
GOMAXPROCS=1 GTS_PARITY_MODE=top50 GTS_PARITY_BENCH_LANGS=java,python,rust \
go test . -tags treesitter_c_parity -run '^$' \
  -bench '^BenchmarkParityTop50ParseFull/' \
  -benchmem -count=10 -benchtime=750ms
```

## Run Real-Corpus Parser Benchmarks

`BenchmarkParityRealCorpusParse*` uses `cgo_harness/corpus_real/<language>`
fixtures and compares gotreesitter against the C tree-sitter runtime for full
parse, single-byte incremental edit, and no-edit incremental parse. Strict
structural parity is the default precheck.

```sh
GOMAXPROCS=1 GTS_REAL_CORPUS_BENCH_LANGS=go \
go test . -tags treesitter_c_parity -run '^$' \
  -bench '^BenchmarkParityRealCorpusParse(Full|IncrementalSingleByteEdit|IncrementalNoEdit)/go/' \
  -benchmem -count=10 -benchtime=750ms
```

Useful narrow-run knobs:

- `GTS_REAL_CORPUS_BENCH_LANGS=c,cpp,c_sharp`
- `GTS_REAL_CORPUS_BENCH_ORDER=path|largest|smallest`
- `GTS_REAL_CORPUS_BENCH_MAX_FILES=1`
- `GTS_REAL_CORPUS_BENCH_MAX_FILE_BYTES=20000`
- `GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH=1` to benchmark only parity-clean files.
- `GTS_REAL_CORPUS_BENCH_ALLOW_MISMATCH=1` for timing-only diagnosis when the
  selected corpus exposes a known structural mismatch.
- `GOT_PARSE_PHASE_TIMING=1` to enable parser-loop, token, action, GLR stack,
  and result-selection/tree-build/finalization phase timing for full parses
  beyond the default large-Python diagnostic lane.

The gotreesitter incremental lanes also report parser attribution counters:
`edit_ns/op`, `parse_wall_ns/op`, `reuse_ns/op`, `reparse_ns/op`,
`unattributed_ns/op`, parser buckets such as `parser_loop_ns/op`,
`token_next_ns/op`, `action_dispatch_ns/op`, `action_lookup_ns/op`,
`action_apply_ns/op`, `glr_merge_ns/op`, and `glr_cull_ns/op`, reused
subtree/byte counts, reuse rejection counts, GLR stack iteration counts,
recovery counts, survivor-node counts, and result phase buckets such as
`result_tree_build_ns/op`, `result_compatibility_ns/op`,
`result_parent_link_ns/op`, and `normalization_ns/op`. The no-edit lane
reports zero parser work when the unchanged-tree fast path returns the previous
tree.

For cross-language optimization sweeps, use the Docker matrix runner. It runs
one language per container, enables phase timing by default, keeps the raw logs,
and writes a ranked JSON/Markdown report with Go/C ratios and top attribution
buckets:

```sh
bash cgo_harness/docker/run_real_corpus_bench_matrix.sh \
  --langs go,python,rust,java,javascript,typescript,c \
  --count 5 \
  --benchtime 750ms
```

Useful matrix diagnosis presets:

```sh
# Time only parity-clean files when a language has known corpus mismatches.
bash cgo_harness/docker/run_real_corpus_bench_matrix.sh \
  --langs rust,javascript \
  --skip-mismatch

# Timing-only probe for a bounded C-family lane.
bash cgo_harness/docker/run_real_corpus_bench_matrix.sh \
  --langs c \
  --allow-mismatch \
  --max-file-bytes 20000
```

To rebuild a report from existing logs:

```sh
cd cgo_harness
go run ./cmd/real_corpus_bench_report \
  -input ../harness_out/real_corpus_bench_matrix/<run>/docker \
  -out-md ../harness_out/real_corpus_bench_matrix/<run>/REAL_CORPUS_BENCH_REPORT.md
```

## Run Parity Tests In Docker Sandbox

This keeps heavy parity runs isolated from your host/WSL memory space and
captures container failure metadata (`OOMKilled`, exit code, state error).

```sh
chmod +x cgo_harness/docker/run_parity_in_docker.sh
cgo_harness/docker/run_parity_in_docker.sh \
  --memory 8g \
  --cpus 4

# Optional: exclude one or more languages from parity loops in this run.
GTS_PARITY_SKIP_LANGS=scala \
  cgo_harness/docker/run_parity_in_docker.sh \
  --memory 8g \
  --cpus 4

# Optional: force the full exhaustive sweep inside the container.
GTS_PARITY_MODE=exhaustive \
  cgo_harness/docker/run_parity_in_docker.sh \
  --memory 8g \
  --cpus 4
```

Run strict Scala real-world parity in the same sandbox:

```sh
cgo_harness/docker/run_parity_in_docker.sh \
  --memory 8g \
  --cpus 4 \
  --strict-scala
```

Run against a specific worktree/repo root:

```sh
cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /path/to/worktree \
  --label glr-exp-a \
  --memory 8g \
  --cpus 4
```

Artifacts are written to `<out-root>/<timestamp>[-label]/` (default out-root is
`<repo-root>/harness_out/docker`):

- `container.log`
- `inspect.json`
- `metadata.txt`

## Run Multiple Worktree Experiments In Parallel

Use the experiment runner to fan out 2-3 bounded containers across different
worktrees while preserving per-experiment artifacts/metadata.

```sh
chmod +x cgo_harness/docker/run_parity_experiments.sh
cgo_harness/docker/run_parity_experiments.sh \
  --experiment main=/home/me/work/gotreesitter \
  --experiment glr-a=/home/me/work/gts-glr-a \
  --experiment glr-b=/home/me/work/gts-glr-b \
  --max-parallel 2 \
  --memory 6g \
  --cpus 2
```

You can also provide a custom command (applied to each experiment):

```sh
cgo_harness/docker/run_parity_experiments.sh \
  --experiment scala=/home/me/work/gts-scala \
  --max-parallel 1 \
  -- "cd /workspace/cgo_harness && GTS_PARITY_SCALA_REALWORLD_STRICT=1 go test . -tags treesitter_c_parity -run '^TestParityScalaRealWorldCorpus$' -count=1 -v"
```

Optional Scala real-world structural parity probe:

```sh
go test . -tags treesitter_c_parity \
  -run '^TestParityScalaRealWorldCorpus$' \
  -count=1 -v
```

Scala real-world probe modes:

- default: regression ratchet against pinned divergence baselines with stable budgets (`GOT_PARSE_NODE_LIMIT_SCALE=3`, `GOT_GLR_MAX_STACKS=8` unless already set)
- strict: exact parity required (zero divergences + no Go error nodes)

Strict mode command:

```sh
GTS_PARITY_SCALA_REALWORLD_STRICT=1 \
  go test . -tags treesitter_c_parity \
  -run '^TestParityScalaRealWorldCorpus$' \
  -count=1 -v
```

## Run Parity Breaker Sweeps (Opt-In)

Use breaker sweeps to aggressively search for structural/highlight parity
regressions via deterministic source mutations and optional real-corpus runs.
These tests are disabled by default.
They are discovery-oriented and may intentionally fail until divergences are
burned down.

```sh
cd cgo_harness
GTS_PARITY_BREAKER=1 \
GTS_PARITY_BREAKER_MAX_LANGS=50 \
GTS_PARITY_BREAKER_MAX_MUTATIONS=12 \
go test . -tags treesitter_c_parity \
  -run '^TestParityMutationSweepStructural$|^TestParityMutationSweepHighlight$' \
  -count=1 -v
```

Common controls:

- `GTS_PARITY_BREAKER_LANGS=go,scala,...` explicit language allow-list.
- `GTS_PARITY_BREAKER_PIN_LANGS=scala,...` force-priority languages into capped runs.
- `GTS_PARITY_BREAKER_MAX_LANGS=<n>` cap selected language count after filters.
- `GTS_PARITY_BREAKER_SHARDS=<n>` and `GTS_PARITY_BREAKER_SHARD_INDEX=<0..n-1>` deterministic sharding for parallel matrix runs.
- `GTS_PARITY_BREAKER_INCLUDE_DEGRADED=1` include known degraded languages in sweep selection.
- `GTS_PARITY_SKIP_LANGS=scala,...` exclude languages from parity loops (fresh/incremental/highlight/breaker).

Example: 3-way sharded matrix with Scala pinned across shards:

```sh
cd cgo_harness
for i in 0 1 2; do
  GTS_PARITY_BREAKER=1 \
  GTS_PARITY_BREAKER_SHARDS=3 \
  GTS_PARITY_BREAKER_SHARD_INDEX="$i" \
  GTS_PARITY_BREAKER_PIN_LANGS=scala \
  GTS_PARITY_BREAKER_MAX_LANGS=40 \
  GTS_PARITY_BREAKER_MAX_MUTATIONS=12 \
  go test . -tags treesitter_c_parity \
    -run '^TestParityMutationSweepStructural$' \
    -count=1 -v &
done
wait
```

Optional real-corpus structural sweep from a generated manifest:

```sh
cd cgo_harness
GTS_PARITY_BREAKER=1 \
GTS_PARITY_BREAKER_CORPUS_MANIFEST=../harness_out/corpus_degraded7/manifest.json \
GTS_PARITY_BREAKER_CORPUS_MAX_FILES=2 \
GTS_PARITY_BREAKER_CORPUS_MAX_BYTES=65536 \
go test . -tags treesitter_c_parity \
  -run '^TestParityBreakerRealCorpusStructural$' \
  -count=1 -v
```

## Run Corpus Parity (`dump.v1`)

This command compares `gotreesitter` vs the native C oracle, emits `dump.v1`
artifacts for both runtimes, writes JSONL results, and updates `PARITY.md`.

```sh
go run -tags treesitter_c_parity ./cmd/corpus_parity \
  --lang top10 \
  --corpus ./corpus \
  --out ./parity_out/results.jsonl \
  --artifact-dir ./parity_out/dump_v1 \
  --artifact-mode failures \
  --scoreboard ./PARITY.md
```

Notes:

- `--lang` accepts `top10` (default), a single language (`go`), or a comma-separated list.
- For multiple languages, corpus layout is `--corpus/<language>/**`.
- For a single language (`--lang go`), `--corpus` can point directly at that language directory.
- `--artifact-mode failures` is recommended for large real-corpus sweeps; it keeps dump artifacts only for failing files.
- `--fail-on-mismatch` is recommended for gate runs; it still writes JSONL and artifacts, then exits non-zero if any row has `pass=false`.
- `--workers N` parallelizes files within each language with one Go parser and one C parser per worker. The default is `1`; use higher values only inside a memory-bounded container and pair them with matching CPU/GOMAXPROCS limits. JSONL output remains sorted by input file order.

## Build Real Corpus (Lock-Pinned)

Use the corpus builder to materialize production-grade real corpus fixtures from
`grammars/languages.lock` pinned upstream commits:

```sh
go run ./cgo_harness/cmd/build_real_corpus \
  -profile cgo_harness/testdata/top50_manifest.json \
  -out cgo_harness/corpus_real
```

Notes:

- Selection is deterministic and bucketed (`small`, `medium`, `large`) per language.
- Selection targets `small`/`medium`/`large` buckets per language, with deterministic fallback when one bucket has no candidates.
- Source files are pulled from pinned upstream commits and recorded in
  `cgo_harness/corpus_real/manifest.json` with SHA256 + source path metadata.
- Validate corpus quality bar:

```sh
cd cgo_harness
GTS_REAL_CORPUS_MANIFEST=corpus_real/manifest.json \
  go test . -run TestRealCorpusManifestQuality -count=1
```

- Use this corpus with the parity runner:

```sh
go run ./cmd/harnessgate -mode correctness \
  -real-corpus-dir cgo_harness/corpus_real \
  -real-corpus-langs top50
```

- Produce an explicit L3/L4 board from the manifest + parity results:

```sh
go run ./cmd/real_corpus_board \
  --manifest cgo_harness/corpus_real/manifest.json \
  --results harness_out/03_real_corpus_results.jsonl \
  --out-json harness_out/03_real_corpus_board.json \
  --out-md harness_out/03_real_corpus_board.md \
  --l4-limit 20
```

Notes:

- `L3` is all `medium` entries present in the built manifest.
- `L4` is all `large` entries by default, or the top `N` heavy-duty languages by
  max large-file bytes when `--l4-limit` is set.
- `cmd/harnessgate` can generate the same board directly when passed
  `-real-corpus-manifest` and optional `-real-corpus-l4-limit`.

## Focused Grammargen Targets

For the current high-value grammargen lane, use the focused Docker runner:

```sh
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs typescript
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs typescript
```

It limits work to `javascript`, `typescript`, `tsx`, `c`, `cpp`, `c_sharp`,
`cobol`, and `fortran`; keep local diagnosis to one `--langs` value at a time.
Each real-corpus grammar runs in its own container, and the direct
grammargen-vs-C oracle also runs one language at a time. That keeps OOMs
contained to a single target instead of killing the host. `fortran` is
currently real-corpus-only because the direct C-oracle harness does not expose
it yet.

For Fortran real-corpus work, the single-grammar runner and focused target lane
now default to a tighter bounded preset unless you explicitly override it or
pass `--unsafe-fortran-defaults`: `--memory 3g`, `--cpus 1`, `--pids 512`,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, `GOT_LALR_LR0_CORE_BUDGET=160000000`, and
`GTS_GRAMMARGEN_REAL_CORPUS_GENERATE_TIMEOUT=15m`.

## Run C Baseline Benchmarks

```sh
GOMAXPROCS=1 go test . -run '^$' -tags treesitter_c_bench \
  -bench 'BenchmarkCTreeSitterGoParseFull|BenchmarkCTreeSitterGoParseIncrementalSingleByteEdit|BenchmarkCTreeSitterGoParseIncrementalNoEdit' \
  -benchmem -count=10 -benchtime=750ms
```

These harnesses are intentionally split into a separate module so the root `gotreesitter` module remains pure-Go in dependency metadata.

## Run Pure-C Runtime Matrix (No CGo)

This compares against the tree-sitter C runtime compiled directly with `gcc`/`g++` and does not execute through Go cgo bindings.

```sh
./pure_c/run_matrix.sh
```

The matrix currently runs full-parse benchmarks for:

- `c`
- `go`
- `java`
- `html`
- `lua`
- `toml`
- `yaml`

## Run Pure-C Go Incremental Benchmark (No CGo)

This reproduces full parse, incremental single-byte edit, and incremental
random-edit incremental, and no-edit numbers against the native C runtime:

```sh
./pure_c/run_go_benchmark.sh
```

Optional arguments:

```sh
./pure_c/run_go_benchmark.sh <func_count> <full_iters> <inc_iters>
```

Example:

```sh
./pure_c/run_go_benchmark.sh 500 2000 20000
```

Optional compiler tuning flags:

```sh
CFLAGS_EXTRA="-march=native -flto" ./pure_c/run_go_benchmark.sh
```

## Run Go Head-to-Head Comparison

This runs both:

- `gotreesitter` Go benchmarks
- pure-C runtime benchmark (no cgo)

```sh
./pure_c/run_go_head_to_head.sh
```

## Run Multi-Language Head-to-Head Matrix

This runs:

- pure-C runtime matrix (`c`, `go`, `java`, `html`, `lua`, `toml`, `yaml`)
- matching `gotreesitter` benchmarks
- a combined summary table with per-language speedup ratios

```sh
./pure_c/run_matrix_head_to_head.sh
```

## Run Full Claim Suite (3-way, multi-size, repeated)

This runs repeated benchmarks across:

- `gotreesitter` (pure Go)
- tree-sitter C runtime via cgo bindings
- tree-sitter C runtime compiled directly with GCC (no cgo)

and generates a median-based report.

```sh
./pure_c/run_claim_suite.sh
```

Tunable inputs:

```sh
RUNS=7 SIZES="500 2000 10000" CFLAGS_EXTRA="-march=native -flto" ./pure_c/run_claim_suite.sh
```
