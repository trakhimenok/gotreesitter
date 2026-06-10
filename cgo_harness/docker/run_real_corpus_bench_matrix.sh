#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RUNNER="$SCRIPT_DIR/run_parity_in_docker.sh"

LANGS_CSV="go,python,rust,java,javascript,typescript,c"
LANGS_FILE=""
OUT_ROOT="$REPO_ROOT/harness_out/real_corpus_bench_matrix"
# Default to higher count + longer benchtime than the legacy values
# (5×750ms). The legacy defaults produced 28% wall-time variance between
# back-to-back runs on the same code, which made tracking <5% perf
# improvements impossible. 10×5s gives ~6× more parse iterations per
# bench line, which tightens variance well below 5%.
COUNT="10"
BENCHTIME="5s"
MEMORY_LIMIT="8g"
CPUS_LIMIT="1"
# Pin the container to a specific physical CPU (one for Go bench, the
# adjacent for CGo bench would be ideal but Docker can't switch
# per-invocation, so use a single pinned CPU for both backends in the
# same matrix run). CPU 18 mirrors the host-side ab_pinned.sh default;
# it's near the end of the high-frequency cores so the kernel scheduler
# is less likely to want it for other work. Empty = no pinning.
CPUSET_CPUS="18"
PIDS_LIMIT="4096"
GOMAXPROCS_VALUE="1"
ALLOW_MISMATCH="0"
SKIP_MISMATCH="0"
PHASE_TIMING="1"
MAX_FILES=""
MAX_BYTES=""
MAX_FILE_BYTES=""
MIN_BYTES=""
SHARDS=""
SHARD=""
ORDER="path"
BENCH_TIMEOUT=""
CORPUS_ROOT=""
CORPUS_SOURCES_ROOT=""
CORPUS_SOURCE_LOCK=""
BUILD_IMAGE=1
EXTRA_ENV=()
KEEP_GOING=1

usage() {
  cat <<'EOF'
Usage: run_real_corpus_bench_matrix.sh [options]

Run real-corpus Go-vs-C parse benchmarks one language per Docker container,
then build a ranked markdown/json report from the benchmark logs.

Options:
  --langs <list>          Comma-separated languages, or "all" for the lock-backed language set
                          (default: go,python,rust,java,javascript,typescript,c)
  --langs-file <path>     Newline- or comma-separated language list, such as output from
                          real_corpus_inventory -select ready-to-benchmark -out-langs
  --out-root <path>       Output root (default: harness_out/real_corpus_bench_matrix)
  --count <n>             go test benchmark count (default: 10)
  --benchtime <dur>       go test benchmark benchtime (default: 5s)
  --memory <limit>        Docker memory limit (default: 8g)
  --cpus <count>          Docker CPU limit (default: 1)
  --cpuset-cpus <list>    Pin container to specific CPUs via --cpuset-cpus
                          (e.g. "18" or "16-19"). Default: "18". Empty
                          disables pinning, but ratio comparisons across
                          back-to-back runs become unreliable without it.
  --pids <count>          Docker PID limit (default: 4096)
  --gomaxprocs <n>        GOMAXPROCS inside container (default: 1)
  --allow-mismatch        Skip strict fresh parity precheck and time selected files
  --skip-mismatch         Filter out files that fail fresh parity precheck
  --phase-timing <0|1>    Export GOT_PARSE_PHASE_TIMING (default: 1)
  --max-files <n>         Export GTS_REAL_CORPUS_BENCH_MAX_FILES
  --max-bytes <n>         Export GTS_REAL_CORPUS_BENCH_MAX_BYTES
  --max-file-bytes <n>    Export GTS_REAL_CORPUS_BENCH_MAX_FILE_BYTES
  --min-bytes <n>         Export GTS_REAL_CORPUS_BENCH_MIN_BYTES
  --shards <n>            Export GTS_REAL_CORPUS_BENCH_SHARDS for deterministic corpus sharding
  --shard <n>             Export GTS_REAL_CORPUS_BENCH_SHARD, 1-based within --shards
  --order <mode>          path|largest|smallest (default: path)
  --timeout <duration>    Bound each per-language go test inside the container
                          with coreutils timeout, e.g. 2m or 30s. Timed-out
                          languages are reported as benchmark-run-aborted.
  --corpus-root <path>    Export GTS_REAL_CORPUS_BENCH_ROOT. Path is resolved
                          inside /workspace/cgo_harness unless absolute.
  --corpus-sources-root <path>
                          Host path to external per-language corpus source
                          checkouts. Mounted read-only at /corpus-sources and
                          benchmarked as /corpus-sources/<language>.
  --corpus-source-lock <path>
                          Optional lock file for --corpus-sources-root
                          filtering. Mounted read-only at /corpus-source.lock.
  --stop-on-failure       Stop after the first language failure
  --no-build              Skip Docker image build in underlying runner
  --extra-env <K=V>       Append KEY=VALUE to the env prefix passed to go test
                          inside the container. May be supplied multiple times.
  -h, --help              Show this help

The generated report is written to:
  <out-root>/<timestamp>/REAL_CORPUS_BENCH_REPORT.md
  <out-root>/<timestamp>/real_corpus_bench_report.json
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --langs) LANGS_CSV="$2"; shift 2 ;;
    --langs-file) LANGS_FILE="$2"; shift 2 ;;
    --out-root) OUT_ROOT="$2"; shift 2 ;;
    --count) COUNT="$2"; shift 2 ;;
    --benchtime) BENCHTIME="$2"; shift 2 ;;
    --memory) MEMORY_LIMIT="$2"; shift 2 ;;
    --cpus) CPUS_LIMIT="$2"; shift 2 ;;
    --cpuset-cpus) CPUSET_CPUS="$2"; shift 2 ;;
    --pids) PIDS_LIMIT="$2"; shift 2 ;;
    --gomaxprocs) GOMAXPROCS_VALUE="$2"; shift 2 ;;
    --allow-mismatch) ALLOW_MISMATCH="1"; shift ;;
    --skip-mismatch) SKIP_MISMATCH="1"; shift ;;
    --phase-timing) PHASE_TIMING="$2"; shift 2 ;;
    --max-files) MAX_FILES="$2"; shift 2 ;;
    --max-bytes) MAX_BYTES="$2"; shift 2 ;;
    --max-file-bytes) MAX_FILE_BYTES="$2"; shift 2 ;;
    --min-bytes) MIN_BYTES="$2"; shift 2 ;;
    --shards) SHARDS="$2"; shift 2 ;;
    --shard) SHARD="$2"; shift 2 ;;
    --order) ORDER="$2"; shift 2 ;;
    --timeout) BENCH_TIMEOUT="$2"; shift 2 ;;
    --corpus-root) CORPUS_ROOT="$2"; shift 2 ;;
    --corpus-sources-root) CORPUS_SOURCES_ROOT="$2"; shift 2 ;;
    --corpus-source-lock) CORPUS_SOURCE_LOCK="$2"; shift 2 ;;
    --stop-on-failure) KEEP_GOING=0; shift ;;
    --no-build) BUILD_IMAGE=0; shift ;;
    --extra-env) EXTRA_ENV+=("$2"); shift 2 ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$ORDER" in
  path|largest|smallest) ;;
  *)
    echo "invalid --order: $ORDER" >&2
    exit 2
    ;;
esac

OUT_ROOT="${OUT_ROOT/#\~/$HOME}"
if [[ "$OUT_ROOT" != /* ]]; then
  OUT_ROOT="$PWD/$OUT_ROOT"
fi
mkdir -p "$OUT_ROOT"
OUT_ROOT="$(cd "$OUT_ROOT" && pwd -P)"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="$OUT_ROOT/$STAMP"
DOCKER_OUT="$RUN_DIR/docker"
mkdir -p "$DOCKER_OUT"

RAW_LANGS=()
if [[ -n "$LANGS_FILE" ]]; then
  if [[ ! -f "$LANGS_FILE" ]]; then
    echo "--langs-file not found: $LANGS_FILE" >&2
    exit 2
  fi
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="${line//,/ }"
    for token in $line; do
      RAW_LANGS+=("$token")
    done
  done <"$LANGS_FILE"
elif [[ "${LANGS_CSV,,}" == "all" || "${LANGS_CSV,,}" == "all206" ]]; then
  while IFS= read -r line; do
    RAW_LANGS+=("$line")
  done < <(
    cd "$REPO_ROOT/cgo_harness"
    go run ./cmd/build_real_corpus -langs all -print-langs
  )
else
  IFS=',' read -r -a RAW_LANGS <<< "$LANGS_CSV"
fi
LANGS=()
for raw in "${RAW_LANGS[@]}"; do
  lang="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"
  lang="${lang//[[:space:]]/}"
  if [[ -n "$lang" ]]; then
    LANGS+=("$lang")
  fi
done
if [[ ${#LANGS[@]} -eq 0 ]]; then
  echo "no languages selected" >&2
  exit 2
fi
if [[ -n "$CORPUS_SOURCES_ROOT" ]]; then
  if [[ -n "$CORPUS_ROOT" ]]; then
    echo "set only one of --corpus-root or --corpus-sources-root" >&2
    exit 2
  fi
  CORPUS_SOURCES_ROOT="${CORPUS_SOURCES_ROOT/#\~/$HOME}"
  if [[ "$CORPUS_SOURCES_ROOT" != /* ]]; then
    CORPUS_SOURCES_ROOT="$PWD/$CORPUS_SOURCES_ROOT"
  fi
  if [[ ! -d "$CORPUS_SOURCES_ROOT" ]]; then
    echo "--corpus-sources-root does not exist: $CORPUS_SOURCES_ROOT" >&2
    exit 2
  fi
  CORPUS_SOURCES_ROOT="$(cd "$CORPUS_SOURCES_ROOT" && pwd -P)"
  CORPUS_ROOT="/corpus-sources"
fi
if [[ -n "$CORPUS_SOURCE_LOCK" ]]; then
  if [[ -z "$CORPUS_SOURCES_ROOT" ]]; then
    echo "--corpus-source-lock requires --corpus-sources-root" >&2
    exit 2
  fi
  CORPUS_SOURCE_LOCK="${CORPUS_SOURCE_LOCK/#\~/$HOME}"
  if [[ "$CORPUS_SOURCE_LOCK" != /* ]]; then
    CORPUS_SOURCE_LOCK="$PWD/$CORPUS_SOURCE_LOCK"
  fi
  if [[ ! -f "$CORPUS_SOURCE_LOCK" ]]; then
    echo "--corpus-source-lock does not exist: $CORPUS_SOURCE_LOCK" >&2
    exit 2
  fi
  CORPUS_SOURCE_LOCK="$(cd "$(dirname "$CORPUS_SOURCE_LOCK")" && pwd -P)/$(basename "$CORPUS_SOURCE_LOCK")"
fi

bench_env_prefix() {
  local lang="$1"
  local envs=(
    "GOMAXPROCS=$GOMAXPROCS_VALUE"
    "GOT_PARSE_PHASE_TIMING=$PHASE_TIMING"
    "GTS_REAL_CORPUS_BENCH_LANGS=$lang"
    "GTS_REAL_CORPUS_BENCH_ALLOW_MISMATCH=$ALLOW_MISMATCH"
    "GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH=$SKIP_MISMATCH"
    "GTS_REAL_CORPUS_BENCH_ORDER=$ORDER"
  )
  if [[ -n "$CORPUS_ROOT" ]]; then envs+=("GTS_REAL_CORPUS_BENCH_ROOT=$CORPUS_ROOT"); fi
  if [[ -n "$CORPUS_SOURCES_ROOT" ]]; then envs+=("GTS_REAL_CORPUS_BENCH_LOCK_FILTER=1"); fi
  if [[ -n "$CORPUS_SOURCE_LOCK" ]]; then envs+=("GTS_REAL_CORPUS_BENCH_LOCK=/corpus-source.lock"); fi
  if [[ -n "$MAX_FILES" ]]; then envs+=("GTS_REAL_CORPUS_BENCH_MAX_FILES=$MAX_FILES"); fi
  if [[ -n "$MAX_BYTES" ]]; then envs+=("GTS_REAL_CORPUS_BENCH_MAX_BYTES=$MAX_BYTES"); fi
  if [[ -n "$MAX_FILE_BYTES" ]]; then envs+=("GTS_REAL_CORPUS_BENCH_MAX_FILE_BYTES=$MAX_FILE_BYTES"); fi
  if [[ -n "$MIN_BYTES" ]]; then envs+=("GTS_REAL_CORPUS_BENCH_MIN_BYTES=$MIN_BYTES"); fi
  if [[ -n "$SHARDS" ]]; then envs+=("GTS_REAL_CORPUS_BENCH_SHARDS=$SHARDS"); fi
  if [[ -n "$SHARD" ]]; then envs+=("GTS_REAL_CORPUS_BENCH_SHARD=$SHARD"); fi
  for kv in "${EXTRA_ENV[@]}"; do envs+=("$kv"); done
  printf 'env'
  for env_kv in "${envs[@]}"; do
    printf ' %q' "$env_kv"
  done
}

failures=()
build_flag=()
if [[ "$BUILD_IMAGE" == "0" ]]; then
  build_flag=(--no-build)
fi

for lang in "${LANGS[@]}"; do
  echo "==> real-corpus bench: $lang"
  env_prefix="$(bench_env_prefix "$lang")"
  bench_cmd="$env_prefix go test . -tags treesitter_c_parity -run '^$' -bench 'BenchmarkParityRealCorpusParse(Full|IncrementalSingleByteEdit|IncrementalNoEdit)$' -benchmem -count=$COUNT -benchtime=$BENCHTIME -v"
  if [[ -n "$BENCH_TIMEOUT" ]]; then
    inner_cmd="cd /workspace/cgo_harness && timeout --foreground --kill-after=30s $BENCH_TIMEOUT $bench_cmd"
  else
    inner_cmd="cd /workspace/cgo_harness && $bench_cmd"
  fi
  runner_args=(
    --out-root "$DOCKER_OUT"
    --label "real-corpus-bench-$lang"
    --memory "$MEMORY_LIMIT"
    --cpus "$CPUS_LIMIT"
    --pids "$PIDS_LIMIT"
  )
  if [[ -n "$CPUSET_CPUS" ]]; then
    runner_args+=(--cpuset-cpus "$CPUSET_CPUS")
  fi
  if [[ -n "$CORPUS_SOURCES_ROOT" ]]; then
    runner_args+=(--mount "$CORPUS_SOURCES_ROOT:/corpus-sources:ro")
  fi
  if [[ -n "$CORPUS_SOURCE_LOCK" ]]; then
    runner_args+=(--mount "$CORPUS_SOURCE_LOCK:/corpus-source.lock:ro")
  fi
  if [[ ${#build_flag[@]} -gt 0 ]]; then
    runner_args+=("${build_flag[@]}")
  fi
  if "$RUNNER" "${runner_args[@]}" -- "$inner_cmd" 2>&1 | tee "$RUN_DIR/$lang.runner.log"; then
    :
  else
    failures+=("$lang")
    if [[ "$KEEP_GOING" == "0" ]]; then
      break
    fi
  fi
  build_flag=(--no-build)
done

if find "$DOCKER_OUT" -name container.log -type f | grep -q .; then
  if (
    cd "$REPO_ROOT/cgo_harness"
    go run ./cmd/real_corpus_bench_report \
      -input "$DOCKER_OUT" \
      -out-json "$RUN_DIR/real_corpus_bench_report.json" \
      -out-md "$RUN_DIR/REAL_CORPUS_BENCH_REPORT.md"
  ); then
    echo "report: $RUN_DIR/REAL_CORPUS_BENCH_REPORT.md"
  else
    echo "report generation failed; inspect logs under $DOCKER_OUT" >&2
  fi
else
  echo "no container logs found under $DOCKER_OUT" >&2
fi

if [[ ${#failures[@]} -gt 0 ]]; then
  printf '%s\n' "${failures[@]}" >"$RUN_DIR/failed_languages.txt"
  echo "failed languages: ${failures[*]}" >&2
  exit 1
fi

echo "real-corpus bench matrix complete"
echo "artifacts: $RUN_DIR"
