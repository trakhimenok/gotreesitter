#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

IMAGE_TAG="gotreesitter/cgo-harness:go1.25-local"
CORPUS_ROOT="${GOT_JAVA_CORPUS_ROOT:-/tmp/gotreesitter-java-corpus/apache-lucene}"
OUT_ROOT=""
LABEL="java-corpus"
MODE="timeout-sweep"
MEMORY_LIMIT="12g"
CPUS_LIMIT="4"
PIDS_LIMIT="4096"
WALL_TIMEOUT="90m"
KILL_GRACE="30s"
GO_TEST_TIMEOUT="75m"
BUILD_IMAGE=1
DRY_RUN=0

CORPUS_ORDER="${GOT_JAVA_CORPUS_ORDER:-}"
CORPUS_MAX_FILES="${GOT_JAVA_CORPUS_MAX_FILES:-}"
CORPUS_MAX_BYTES="${GOT_JAVA_CORPUS_MAX_BYTES:-}"
CORPUS_MIN_BYTES="${GOT_JAVA_CORPUS_MIN_BYTES:-}"
CORPUS_MAX_FILE_BYTES="${GOT_JAVA_CORPUS_MAX_FILE_BYTES:-}"
TIMEOUT_SWEEP="${GOT_JAVA_TIMEOUT_SWEEP:-}"
PARSE_MODES="${GOT_JAVA_PARSE_MODES:-}"
BENCH_TIMEOUT="${GOT_JAVA_BENCH_TIMEOUT:-}"
BENCH_REGEX='BenchmarkJavaCorpus(GoTreeSitterParseDFA|GoTreeSitterParseTokenSource|GoTreeSitterParseAspectFallback|CTreeSitterParseFull)$'
BENCH_COUNT="10"
BENCH_TIME="750ms"
GOMAXPROCS_VALUE="1"

usage() {
  cat <<'EOF'
Usage: run_java_corpus_probe.sh [options]

Run the Java corpus timeout probe or benchmark in a bounded Docker container.
The repository is mounted at /workspace and the external Java corpus is mounted
read-only at /java-corpus. Corpus data is not vendored into the repository.

Options:
  --mode <name>              Probe mode: timeout-sweep|benchmark (default: timeout-sweep)
  --repo-root <path>         Repository/worktree root mounted at /workspace
  --corpus-root <path>       Host Java corpus root mounted at /java-corpus
                              (default: /tmp/gotreesitter-java-corpus/apache-lucene)
  --image <tag>              Docker image tag (default: gotreesitter/cgo-harness:go1.25-local)
  --memory <limit>           Container memory limit (default: 12g)
  --cpus <count>             CPU limit passed to Docker (default: 4)
  --pids <count>             PID limit passed to Docker (default: 4096)
  --wall-timeout <duration>  Host-side wall deadline (default: 90m)
  --kill-grace <duration>    Grace period after wall timeout before SIGKILL (default: 30s)
  --go-timeout <duration>    go test -timeout value inside the container (default: 75m)
  --out-root <path>          Artifact output root (default: <repo-root>/harness_out/docker-java-corpus)
  --label <name>             Optional run label suffix (default: java-corpus)
  --no-build                 Skip docker build step
  --dry-run                  Print the Docker/test command summary without running it
  -h, --help                 Show this help

Java corpus knobs:
  --order <path|largest|smallest>
                              GOT_JAVA_CORPUS_ORDER
  --max-files <n>             GOT_JAVA_CORPUS_MAX_FILES
  --max-bytes <n>             GOT_JAVA_CORPUS_MAX_BYTES
  --min-bytes <n>             GOT_JAVA_CORPUS_MIN_BYTES
  --max-file-bytes <n>        GOT_JAVA_CORPUS_MAX_FILE_BYTES
  --timeout-sweep <list>      GOT_JAVA_TIMEOUT_SWEEP, e.g. 100ms,500ms,2s,0
  --parse-modes <list>        GOT_JAVA_PARSE_MODES, e.g. dfa or dfa,aspect_fallback
  --bench-timeout <duration>  GOT_JAVA_BENCH_TIMEOUT

Benchmark knobs:
  --bench <regex>             Benchmark regex for --mode benchmark
  --bench-count <n>           go test benchmark -count (default: 10)
  --benchtime <duration>      go test -benchtime (default: 750ms)
  --gomaxprocs <n>            GOMAXPROCS inside the container (default: 1)

Environment passthrough:
  GOTOOLCHAIN
  GOT_PARSE_MEMORY_BUDGET_MB
  GOT_GLR_MAX_MERGE_PER_KEY
  GOT_GLR_MAX_STACKS
  GOT_PARSE_NODE_LIMIT_SCALE
  GOT_GLR_FORCE_CONFLICT_WIDTH

Examples:
  cgo_harness/docker/run_java_corpus_probe.sh --mode timeout-sweep --max-files 50
  cgo_harness/docker/run_java_corpus_probe.sh --mode benchmark --bench-timeout 2s --max-files 25
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="$2"; shift 2 ;;
    --repo-root) REPO_ROOT="$2"; shift 2 ;;
    --corpus-root) CORPUS_ROOT="$2"; shift 2 ;;
    --image) IMAGE_TAG="$2"; shift 2 ;;
    --memory) MEMORY_LIMIT="$2"; shift 2 ;;
    --cpus) CPUS_LIMIT="$2"; shift 2 ;;
    --pids) PIDS_LIMIT="$2"; shift 2 ;;
    --wall-timeout) WALL_TIMEOUT="$2"; shift 2 ;;
    --kill-grace) KILL_GRACE="$2"; shift 2 ;;
    --go-timeout) GO_TEST_TIMEOUT="$2"; shift 2 ;;
    --out-root) OUT_ROOT="$2"; shift 2 ;;
    --label) LABEL="$2"; shift 2 ;;
    --order) CORPUS_ORDER="$2"; shift 2 ;;
    --max-files) CORPUS_MAX_FILES="$2"; shift 2 ;;
    --max-bytes) CORPUS_MAX_BYTES="$2"; shift 2 ;;
    --min-bytes) CORPUS_MIN_BYTES="$2"; shift 2 ;;
    --max-file-bytes) CORPUS_MAX_FILE_BYTES="$2"; shift 2 ;;
    --timeout-sweep) TIMEOUT_SWEEP="$2"; shift 2 ;;
    --parse-modes) PARSE_MODES="$2"; shift 2 ;;
    --bench-timeout) BENCH_TIMEOUT="$2"; shift 2 ;;
    --bench) BENCH_REGEX="$2"; shift 2 ;;
    --bench-count) BENCH_COUNT="$2"; shift 2 ;;
    --benchtime) BENCH_TIME="$2"; shift 2 ;;
    --gomaxprocs) GOMAXPROCS_VALUE="$2"; shift 2 ;;
    --no-build) BUILD_IMAGE=0; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

sanitize_label() {
  local in="$1"
  in="${in,,}"
  in="$(echo "$in" | sed -E 's/[^a-z0-9_.-]+/-/g; s/^-+//; s/-+$//; s/-+/-/g')"
  if [[ -z "$in" ]]; then
    in="run"
  fi
  echo "$in"
}

duration_to_seconds() {
  local d="$1"
  case "$d" in
    *h) echo $(( ${d%h} * 3600 )) ;;
    *m) echo $(( ${d%m} * 60 )) ;;
    *s) echo "${d%s}" ;;
    *) echo "$d" ;;
  esac
}

require_non_negative_int() {
  local name="$1"
  local value="$2"
  if [[ -n "$value" && ! "$value" =~ ^[0-9]+$ ]]; then
    echo "invalid $name: $value (expected non-negative integer)" >&2
    exit 2
  fi
}

case "$MODE" in
  timeout-sweep|benchmark) ;;
  *)
    echo "invalid --mode: $MODE (expected timeout-sweep|benchmark)" >&2
    exit 2
    ;;
esac

if [[ -n "$CORPUS_ORDER" ]]; then
  case "$CORPUS_ORDER" in
    path|largest|smallest) ;;
    *)
      echo "invalid --order: $CORPUS_ORDER (expected path|largest|smallest)" >&2
      exit 2
      ;;
  esac
fi

require_non_negative_int "--max-files" "$CORPUS_MAX_FILES"
require_non_negative_int "--max-bytes" "$CORPUS_MAX_BYTES"
require_non_negative_int "--min-bytes" "$CORPUS_MIN_BYTES"
require_non_negative_int "--max-file-bytes" "$CORPUS_MAX_FILE_BYTES"
require_non_negative_int "--bench-count" "$BENCH_COUNT"
require_non_negative_int "--gomaxprocs" "$GOMAXPROCS_VALUE"

REPO_ROOT="${REPO_ROOT/#\~/$HOME}"
CORPUS_ROOT="${CORPUS_ROOT/#\~/$HOME}"
if [[ -z "$OUT_ROOT" ]]; then
  OUT_ROOT="$REPO_ROOT/harness_out/docker-java-corpus"
fi
OUT_ROOT="${OUT_ROOT/#\~/$HOME}"

if [[ ! -d "$REPO_ROOT" ]]; then
  echo "repo root does not exist: $REPO_ROOT" >&2
  exit 2
fi
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
if [[ ! -d "$CORPUS_ROOT" ]]; then
  echo "java corpus root does not exist: $CORPUS_ROOT" >&2
  echo "seed it with: cgo_harness/seed_java_corpus.sh --dest '$CORPUS_ROOT'" >&2
  exit 2
fi
CORPUS_ROOT="$(cd "$CORPUS_ROOT" && pwd)"
mkdir -p "$OUT_ROOT"

LABEL_SLUG="$(sanitize_label "$LABEL")"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="$OUT_ROOT/$STAMP-$LABEL_SLUG-$MODE"
CONTAINER_NAME="gts-java-corpus-${STAMP,,}-${LABEL_SLUG}-${MODE}"

ENV_ARGS=(
  -e "GOT_JAVA_CORPUS_ROOT=/java-corpus"
)
[[ -n "$CORPUS_ORDER" ]] && ENV_ARGS+=(-e "GOT_JAVA_CORPUS_ORDER=$CORPUS_ORDER")
[[ -n "$CORPUS_MAX_FILES" ]] && ENV_ARGS+=(-e "GOT_JAVA_CORPUS_MAX_FILES=$CORPUS_MAX_FILES")
[[ -n "$CORPUS_MAX_BYTES" ]] && ENV_ARGS+=(-e "GOT_JAVA_CORPUS_MAX_BYTES=$CORPUS_MAX_BYTES")
[[ -n "$CORPUS_MIN_BYTES" ]] && ENV_ARGS+=(-e "GOT_JAVA_CORPUS_MIN_BYTES=$CORPUS_MIN_BYTES")
[[ -n "$CORPUS_MAX_FILE_BYTES" ]] && ENV_ARGS+=(-e "GOT_JAVA_CORPUS_MAX_FILE_BYTES=$CORPUS_MAX_FILE_BYTES")
[[ -n "$TIMEOUT_SWEEP" ]] && ENV_ARGS+=(-e "GOT_JAVA_TIMEOUT_SWEEP=$TIMEOUT_SWEEP")
[[ -n "$PARSE_MODES" ]] && ENV_ARGS+=(-e "GOT_JAVA_PARSE_MODES=$PARSE_MODES")
[[ -n "$BENCH_TIMEOUT" ]] && ENV_ARGS+=(-e "GOT_JAVA_BENCH_TIMEOUT=$BENCH_TIMEOUT")

ENV_ARGS+=(-e "GOMAXPROCS=$GOMAXPROCS_VALUE")
for var in GOTOOLCHAIN GOT_PARSE_MEMORY_BUDGET_MB GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_FORCE_CONFLICT_WIDTH; do
  if [[ -n "${!var:-}" ]]; then
    ENV_ARGS+=("-e" "$var=${!var}")
  fi
done

case "$MODE" in
  timeout-sweep)
    INNER_CMD="cd /workspace/cgo_harness && /usr/bin/time -v go test . -tags treesitter_c_bench -run '^TestJavaCorpusTimeoutSweep$' -count=1 -v -timeout '$GO_TEST_TIMEOUT'"
    ;;
  benchmark)
    INNER_CMD="cd /workspace/cgo_harness && /usr/bin/time -v go test . -tags treesitter_c_bench -run '^$' -bench '$BENCH_REGEX' -benchmem -count '$BENCH_COUNT' -benchtime '$BENCH_TIME' -timeout '$GO_TEST_TIMEOUT'"
    ;;
esac
INNER_CMD="export PATH=/usr/local/go/bin:\$PATH; $INNER_CMD"

echo "java corpus probe:"
echo "  mode:         $MODE"
echo "  repo:         $REPO_ROOT -> /workspace (read-only)"
echo "  corpus:       $CORPUS_ROOT -> /java-corpus (read-only)"
echo "  image:        $IMAGE_TAG"
echo "  resources:    memory=$MEMORY_LIMIT cpus=$CPUS_LIMIT pids=$PIDS_LIMIT"
echo "  timeouts:     wall=$WALL_TIMEOUT kill_grace=$KILL_GRACE go_test=$GO_TEST_TIMEOUT"
echo "  artifacts:    $OUT_DIR"
echo "  command:      $INNER_CMD"
echo "  env:"
printf '    %q\n' "${ENV_ARGS[@]}"

if [[ "$DRY_RUN" == "1" ]]; then
  echo "dry-run: not building image or starting container"
  exit 0
fi

mkdir -p "$OUT_DIR"

if [[ "$BUILD_IMAGE" == "1" ]]; then
  docker build -t "$IMAGE_TAG" "$SCRIPT_DIR"
fi

CID=""
cleanup() {
  if [[ -n "$CID" ]]; then
    docker rm -f "$CID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

CID="$(docker create \
  --name "$CONTAINER_NAME" \
  --init \
  --memory "$MEMORY_LIMIT" \
  --memory-swap "$MEMORY_LIMIT" \
  --cpus "$CPUS_LIMIT" \
  --pids-limit "$PIDS_LIMIT" \
  --mount "type=bind,src=$REPO_ROOT,dst=/workspace,readonly" \
  --mount "type=bind,src=$CORPUS_ROOT,dst=/java-corpus,readonly" \
  --mount "type=volume,src=gotreesitter-go-mod-cache,dst=/go/pkg/mod" \
  --mount "type=volume,src=gotreesitter-go-build-cache,dst=/root/.cache/go-build" \
  "${ENV_ARGS[@]}" \
  "$IMAGE_TAG" \
  bash -c "$INNER_CMD")"

START_EPOCH="$(date +%s)"
docker start "$CID" >/dev/null

(
  sleep "$(duration_to_seconds "$WALL_TIMEOUT")"
  if docker inspect -f '{{.State.Running}}' "$CID" 2>/dev/null | grep -q true; then
    echo "[watchdog] wall timeout after ${WALL_TIMEOUT}, sending SIGTERM to $CONTAINER_NAME" >&2
    docker kill --signal=SIGTERM "$CID" >/dev/null 2>&1 || true
    sleep "$(duration_to_seconds "$KILL_GRACE")"
    if docker inspect -f '{{.State.Running}}' "$CID" 2>/dev/null | grep -q true; then
      echo "[watchdog] still running after ${KILL_GRACE}, sending SIGKILL" >&2
      docker kill --signal=SIGKILL "$CID" >/dev/null 2>&1 || true
    fi
  fi
) &
WATCHDOG_PID=$!

docker logs -f "$CID" 2>&1 | tee "$OUT_DIR/container.log" || true
EXIT_CODE="$(docker wait "$CID")"
kill "$WATCHDOG_PID" 2>/dev/null || true
wait "$WATCHDOG_PID" 2>/dev/null || true

END_EPOCH="$(date +%s)"
WALL_SEC=$(( END_EPOCH - START_EPOCH ))

docker inspect "$CID" >"$OUT_DIR/inspect.json"
OOM_KILLED="$(docker inspect -f '{{.State.OOMKilled}}' "$CID")"
STATE_ERROR="$(docker inspect -f '{{.State.Error}}' "$CID")"
FINISHED_AT="$(docker inspect -f '{{.State.FinishedAt}}' "$CID")"
PEAK_RSS_KB="$(grep -Eo 'Maximum resident set size \(kbytes\): [0-9]+' "$OUT_DIR/container.log" 2>/dev/null | tail -1 | awk '{print $NF}')"
PEAK_RSS_KB="${PEAK_RSS_KB:-unknown}"

{
  echo "container_name=$CONTAINER_NAME"
  echo "container_id=$CID"
  echo "image=$IMAGE_TAG"
  echo "mode=$MODE"
  echo "repo_root=$REPO_ROOT"
  echo "corpus_root=$CORPUS_ROOT"
  echo "memory=$MEMORY_LIMIT"
  echo "cpus=$CPUS_LIMIT"
  echo "pids=$PIDS_LIMIT"
  echo "wall_timeout=$WALL_TIMEOUT"
  echo "kill_grace=$KILL_GRACE"
  echo "go_test_timeout=$GO_TEST_TIMEOUT"
  echo "exit_code=$EXIT_CODE"
  echo "oom_killed=$OOM_KILLED"
  echo "state_error=$STATE_ERROR"
  echo "wall_seconds=$WALL_SEC"
  echo "peak_rss_kb=$PEAK_RSS_KB"
  echo "finished_at=$FINISHED_AT"
  echo "command=$INNER_CMD"
  echo "corpus_order=${CORPUS_ORDER:-inherit}"
  echo "corpus_max_files=${CORPUS_MAX_FILES:-inherit}"
  echo "corpus_max_bytes=${CORPUS_MAX_BYTES:-inherit}"
  echo "corpus_min_bytes=${CORPUS_MIN_BYTES:-inherit}"
  echo "corpus_max_file_bytes=${CORPUS_MAX_FILE_BYTES:-inherit}"
  echo "timeout_sweep=${TIMEOUT_SWEEP:-inherit}"
  echo "parse_modes=${PARSE_MODES:-inherit}"
  echo "bench_timeout=${BENCH_TIMEOUT:-inherit}"
  echo "bench_regex=$BENCH_REGEX"
  echo "bench_count=$BENCH_COUNT"
  echo "benchtime=$BENCH_TIME"
  echo "gomaxprocs=$GOMAXPROCS_VALUE"
} >"$OUT_DIR/metadata.txt"

echo "java corpus probe complete"
echo "artifacts: $OUT_DIR"
echo "exit_code: $EXIT_CODE"
echo "oom_killed: $OOM_KILLED"
echo "wall_seconds: $WALL_SEC"
echo "peak_rss_kb: $PEAK_RSS_KB"
if [[ -n "$STATE_ERROR" ]]; then
  echo "docker_state_error: $STATE_ERROR"
fi

if [[ "$EXIT_CODE" != "0" ]]; then
  exit "$EXIT_CODE"
fi
