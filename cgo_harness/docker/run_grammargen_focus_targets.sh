#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
REAL_RUNNER="$SCRIPT_DIR/run_single_grammar_parity.sh"
CGO_RUNNER="$SCRIPT_DIR/run_grammargen_c_parity.sh"

DEFAULT_MEMORY_LIMIT="8g"
DEFAULT_CPUS_LIMIT="1"
DEFAULT_PIDS_LIMIT="512"
DEFAULT_GOMAXPROCS_VALUE="1"
DEFAULT_GOFLAGS_VALUE="-p=1"
DEFAULT_REAL_TIMEOUT="15m"
DEFAULT_REAL_MAX_CASES="25"
DEFAULT_REAL_PROFILE="aggressive"
DEFAULT_JOBS="1"

FORTRAN_SAFE_MEMORY_LIMIT="3g"
FORTRAN_SAFE_CPUS_LIMIT="1"
FORTRAN_SAFE_PIDS_LIMIT="512"
FORTRAN_SAFE_GOMAXPROCS_VALUE="1"
FORTRAN_SAFE_GOFLAGS_VALUE="-p=1"
FORTRAN_SAFE_LR0_CORE_BUDGET="160000000"
FORTRAN_SAFE_GENERATE_TIMEOUT="15m"

MODE="all"
LANGS_CSV="css,javascript,typescript,tsx,c,c_sharp,cobol,fortran"
MEMORY_LIMIT="$DEFAULT_MEMORY_LIMIT"
CPUS_LIMIT="$DEFAULT_CPUS_LIMIT"
PIDS_LIMIT="$DEFAULT_PIDS_LIMIT"
GOMAXPROCS_VALUE="$DEFAULT_GOMAXPROCS_VALUE"
GOFLAGS_VALUE="$DEFAULT_GOFLAGS_VALUE"
REAL_TIMEOUT="$DEFAULT_REAL_TIMEOUT"
REAL_MAX_CASES="$DEFAULT_REAL_MAX_CASES"
REAL_PROFILE="$DEFAULT_REAL_PROFILE"
CGO_TIMEOUT_MINS="45"
CGO_MAX_CASES="20"
CGO_MAX_BYTES="262144"
REPORT_ROOT="$REPO_ROOT/cgo_harness/reports/focus_targets"
SEED_DIR=""
OFFLINE=0
BUILD_IMAGE=1
LR_SPLIT=0
REAL_LR0_CORE_BUDGET=""
REAL_GENERATE_TIMEOUT=""
FORTRAN_SAFE_DEFAULTS=1
JOBS="$DEFAULT_JOBS"
ALLOW_HOST_OVERSUBSCRIBE=0

MEMORY_SET=0
CPUS_SET=0
PIDS_SET=0
GOMAXPROCS_SET=0
GOFLAGS_SET=0
REAL_LR0_CORE_BUDGET_SET=0
REAL_GENERATE_TIMEOUT_SET=0

usage() {
  cat <<'EOF'
Usage: run_grammargen_focus_targets.sh [options]

Run the high-value grammargen targets only, with safe isolation by default:
  css, javascript, typescript, tsx, c, c_sharp, cobol, fortran

Modes:
  all          Run real-corpus parity and direct grammargen-vs-C parity
  real-corpus  Run only per-grammar real-corpus parity
  cgo          Run only direct grammargen-vs-C parity

Options:
  --mode <m>             all|real-corpus|cgo (default: all)
  --langs <list>         Comma-separated subset of target languages
  --memory <limit>       Docker memory limit for both paths (default: 8g)
  --cpus <count>         Docker CPU limit for both paths (default: 1)
  --pids <count>         Docker PID limit for both paths (default: 512)
  --gomaxprocs <n>       Export GOMAXPROCS inside both containers (default: 1)
  --goflags <value>      Export GOFLAGS inside both containers (default: -p=1)
  --lr0-core-budget <n>  Export GOT_LALR_LR0_CORE_BUDGET for real-corpus runs
  --generate-timeout <d> Export GTS_GRAMMARGEN_REAL_CORPUS_GENERATE_TIMEOUT
                         for real-corpus runs
  --unsafe-fortran-defaults
                         Disable the default bounded Fortran preset for
                         real-corpus runs. By default, Fortran uses
                         memory=3g, cpus=1, pids=512, GOMAXPROCS=1,
                         GOFLAGS=-p=1, lr0_core_budget=160000000, and
                         generate_timeout=15m unless you override them.
  --real-timeout <dur>   Real-corpus timeout per grammar (default: 15m)
  --real-max-cases <n>   Real-corpus max cases per grammar (default: 25)
  --profile <name>       smoke|balanced|aggressive (default: aggressive)
  --cgo-timeout <mins>   Direct C parity timeout minutes (default: 45)
  --cgo-max-cases <n>    Direct C parity max cases (default: 20)
  --cgo-max-bytes <n>    Direct C parity max sample bytes (default: 262144)
  --report-root <path>   Real-corpus report root (default: cgo_harness/reports/focus_targets)
  --seed-dir <path>      Seed grammar repos from a dir under repo root
  --offline              Do not clone missing grammar repos in containers
  --lr-split             Enable GTS_GRAMMARGEN_LR_SPLIT for real-corpus runs
  --jobs <n>             Concurrent per-language containers (default: 1).
                         Each language still runs in its own memory/pid-limited
                         container. Aggregate container memory is guarded
                         against host MemAvailable by default.
  --allow-host-oversubscribe
                         Allow --jobs * --memory to exceed the host memory
                         guard. Intended only for dedicated CI hosts.
  --no-build             Skip Docker image builds in the underlying runners
  --list                 Print canonical target languages and exit
  -h, --help             Show this help

Notes:
  - Real-corpus parity runs one grammar per container via run_single_grammar_parity.sh.
  - Direct C parity also runs one language per container via run_grammargen_c_parity.sh.
  - The default lane is single-worker by design: one grammar, one container,
    cpus=1, GOMAXPROCS=1, GOFLAGS=-p=1.
  - cpp remains supported via --langs cpp, but stays off the default lane
    until its focused perf signal is trustworthy again.
  - fortran real-corpus runs also default to a tighter bounded envelope
    unless you override it or pass --unsafe-fortran-defaults.
  - fortran is currently real-corpus-only; the direct grammargen-vs-C harness
    does not expose it yet.
EOF
}

canonical_lang() {
  local lang
  lang="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  lang="${lang//[[:space:]]/}"
  case "$lang" in
    js) echo "javascript" ;;
    ts) echo "typescript" ;;
    c++|cplusplus) echo "cpp" ;;
    c#|csharp) echo "c_sharp" ;;
    c_lang) echo "c" ;;
    *) echo "$lang" ;;
  esac
}

is_supported_focus_lang() {
  case "$1" in
    css|javascript|typescript|tsx|c|cpp|cuda|c_sharp|cobol|fortran) return 0 ;;
    *) return 1 ;;
  esac
}

real_corpus_lang() {
  case "$1" in
    c) echo "c_lang" ;;
    *) echo "$1" ;;
  esac
}

supports_cgo_parity() {
  case "$1" in
    css|javascript|typescript|tsx|c|cpp|cuda|c_sharp|cobol) return 0 ;;
    *) return 1 ;;
  esac
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="$2"; shift 2 ;;
    --langs) LANGS_CSV="$2"; shift 2 ;;
    --memory) MEMORY_LIMIT="$2"; MEMORY_SET=1; shift 2 ;;
    --cpus) CPUS_LIMIT="$2"; CPUS_SET=1; shift 2 ;;
    --pids) PIDS_LIMIT="$2"; PIDS_SET=1; shift 2 ;;
    --gomaxprocs) GOMAXPROCS_VALUE="$2"; GOMAXPROCS_SET=1; shift 2 ;;
    --goflags) GOFLAGS_VALUE="$2"; GOFLAGS_SET=1; shift 2 ;;
    --lr0-core-budget) REAL_LR0_CORE_BUDGET="$2"; REAL_LR0_CORE_BUDGET_SET=1; shift 2 ;;
    --generate-timeout) REAL_GENERATE_TIMEOUT="$2"; REAL_GENERATE_TIMEOUT_SET=1; shift 2 ;;
    --unsafe-fortran-defaults) FORTRAN_SAFE_DEFAULTS=0; shift ;;
    --real-timeout) REAL_TIMEOUT="$2"; shift 2 ;;
    --real-max-cases) REAL_MAX_CASES="$2"; shift 2 ;;
    --profile) REAL_PROFILE="$2"; shift 2 ;;
    --cgo-timeout) CGO_TIMEOUT_MINS="$2"; shift 2 ;;
    --cgo-max-cases) CGO_MAX_CASES="$2"; shift 2 ;;
    --cgo-max-bytes) CGO_MAX_BYTES="$2"; shift 2 ;;
    --report-root) REPORT_ROOT="$2"; shift 2 ;;
    --seed-dir) SEED_DIR="$2"; shift 2 ;;
    --offline) OFFLINE=1; shift ;;
    --lr-split) LR_SPLIT=1; shift ;;
    --jobs) JOBS="$2"; shift 2 ;;
    --allow-host-oversubscribe) ALLOW_HOST_OVERSUBSCRIBE=1; shift ;;
    --no-build) BUILD_IMAGE=0; shift ;;
    --list)
      printf '%s\n' css javascript typescript tsx c cpp c_sharp cobol fortran
      exit 0
      ;;
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

case "$MODE" in
  all|real-corpus|cgo) ;;
  *)
    echo "invalid --mode: $MODE" >&2
    exit 2
    ;;
esac

declare -a TARGET_LANGS=()
declare -A seen_langs=()
IFS=',' read -r -a raw_langs <<< "$LANGS_CSV"
for raw_lang in "${raw_langs[@]}"; do
  lang="$(canonical_lang "$raw_lang")"
  if [[ -z "$lang" ]]; then
    continue
  fi
  if ! is_supported_focus_lang "$lang"; then
    echo "unsupported focus language: $raw_lang" >&2
    exit 2
  fi
  if [[ -n "${seen_langs[$lang]:-}" ]]; then
    continue
  fi
  seen_langs[$lang]=1
  TARGET_LANGS+=("$lang")
done

if [[ ${#TARGET_LANGS[@]} -eq 0 ]]; then
  echo "no target languages selected" >&2
  exit 2
fi

require_positive_int() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[1-9][0-9]*$ ]]; then
    echo "$name must be a positive integer, got: $value" >&2
    exit 2
  fi
}

docker_memory_limit_to_bytes() {
  local value="$1"
  local number unit
  value="${value//[[:space:]]/}"
  if [[ "$value" =~ ^([0-9]+)([bBkKmMgG]?)$ ]]; then
    number="${BASH_REMATCH[1]}"
    unit="${BASH_REMATCH[2],,}"
  else
    return 1
  fi
  case "$unit" in
    ""|b) printf '%s\n' "$number" ;;
    k) printf '%s\n' "$((number * 1024))" ;;
    m) printf '%s\n' "$((number * 1024 * 1024))" ;;
    g) printf '%s\n' "$((number * 1024 * 1024 * 1024))" ;;
    *) return 1 ;;
  esac
}

host_mem_available_bytes() {
  awk '/^MemAvailable:/ { printf "%.0f\n", $2 * 1024 }' /proc/meminfo 2>/dev/null
}

resolve_fortran_real_corpus_setting() {
  local lang="$1"
  local current="$2"
  local was_set="$3"
  local safe_default="$4"

  if [[ "$lang" == "fortran" && "$FORTRAN_SAFE_DEFAULTS" == "1" && "$was_set" == "0" ]]; then
    echo "$safe_default"
    return
  fi

  echo "$current"
}

effective_real_corpus_memory_limit() {
  local lang="$1"
  resolve_fortran_real_corpus_setting "$lang" "$MEMORY_LIMIT" "$MEMORY_SET" "$FORTRAN_SAFE_MEMORY_LIMIT"
}

max_parallel_memory_budget_bytes() {
  local effective_jobs="$1"
  local phase="$2"
  shift 2

  local lang limit limit_bytes
  local -a ranked_limits=()
  for lang in "$@"; do
    case "$phase" in
      real-corpus)
        limit="$(effective_real_corpus_memory_limit "$lang")"
        ;;
      cgo)
        if ! supports_cgo_parity "$lang"; then
          continue
        fi
        limit="$MEMORY_LIMIT"
        ;;
      *)
        echo "internal error: unknown memory guard phase: $phase" >&2
        exit 2
        ;;
    esac
    limit_bytes="$(docker_memory_limit_to_bytes "$limit" || true)"
    if [[ -z "$limit_bytes" ]]; then
      echo "warning: could not parse $phase memory limit for $lang: $limit" >&2
      return 1
    fi
    ranked_limits+=("$limit_bytes|$lang|$limit")
  done

  if [[ "${#ranked_limits[@]}" -eq 0 ]]; then
    printf '0|\n'
    return 0
  fi

  local aggregate_bytes=0
  local count=0
  local entry detail lang_name limit_text
  local -a selected_limits=()
  while IFS= read -r entry; do
    [[ -n "$entry" ]] || continue
    limit_bytes="${entry%%|*}"
    detail="${entry#*|}"
    lang_name="${detail%%|*}"
    limit_text="${detail#*|}"
    aggregate_bytes="$((aggregate_bytes + limit_bytes))"
    selected_limits+=("$lang_name=$limit_text")
    ((count+=1))
    if [[ "$count" -ge "$effective_jobs" ]]; then
      break
    fi
  done < <(printf '%s\n' "${ranked_limits[@]}" | sort -t '|' -k1,1nr)

  printf '%s|%s\n' "$aggregate_bytes" "${selected_limits[*]}"
}

guard_parallel_memory_budget() {
  local target_count="${1:-0}"
  local effective_jobs="$JOBS"
  if [[ "$target_count" =~ ^[1-9][0-9]*$ && "$effective_jobs" -gt "$target_count" ]]; then
    effective_jobs="$target_count"
  fi
  if [[ "$effective_jobs" -le 1 || "$ALLOW_HOST_OVERSUBSCRIBE" == "1" ]]; then
    return 0
  fi
  local available_bytes aggregate_bytes guard_bytes
  available_bytes="$(host_mem_available_bytes || true)"
  if [[ -z "$available_bytes" ]]; then
    echo "warning: could not read host MemAvailable; proceeding with --jobs=$JOBS memory=$MEMORY_LIMIT" >&2
    return 0
  fi

  local phase_result phase_bytes phase_details
  local max_phase="selected"
  local selected_limits=""
  aggregate_bytes=0
  if [[ "$MODE" == "all" || "$MODE" == "real-corpus" ]]; then
    phase_result="$(max_parallel_memory_budget_bytes "$effective_jobs" real-corpus "${TARGET_LANGS[@]}" || true)"
    if [[ -z "$phase_result" ]]; then
      echo "warning: could not calculate real-corpus memory guard; proceeding with --jobs=$JOBS" >&2
      return 0
    fi
    phase_bytes="${phase_result%%|*}"
    phase_details="${phase_result#*|}"
    if [[ "$phase_bytes" -gt "$aggregate_bytes" ]]; then
      aggregate_bytes="$phase_bytes"
      max_phase="real-corpus"
      selected_limits="$phase_details"
    fi
  fi
  if [[ "$MODE" == "all" || "$MODE" == "cgo" ]]; then
    phase_result="$(max_parallel_memory_budget_bytes "$effective_jobs" cgo "${TARGET_LANGS[@]}" || true)"
    if [[ -z "$phase_result" ]]; then
      echo "warning: could not calculate cgo memory guard; proceeding with --jobs=$JOBS" >&2
      return 0
    fi
    phase_bytes="${phase_result%%|*}"
    phase_details="${phase_result#*|}"
    if [[ "$phase_bytes" -gt "$aggregate_bytes" ]]; then
      aggregate_bytes="$phase_bytes"
      max_phase="cgo"
      selected_limits="$phase_details"
    fi
  fi

  guard_bytes="$((available_bytes * 80 / 100))"
  if [[ "$aggregate_bytes" -gt "$guard_bytes" ]]; then
    {
      echo "refusing --jobs=$JOBS: aggregate effective container memory exceeds 80% of host MemAvailable"
      echo "effective_jobs=$effective_jobs"
      echo "phase=$max_phase aggregate_bytes=$aggregate_bytes memavailable_bytes=$available_bytes guard_bytes=$guard_bytes"
      echo "selected_limits=$selected_limits"
      echo "lower --jobs/--memory or pass --allow-host-oversubscribe on a dedicated host"
    } >&2
    exit 2
  fi
}

require_positive_int "--jobs" "$JOBS"
guard_parallel_memory_budget "${#TARGET_LANGS[@]}"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
REAL_REPORT_DIR="$REPORT_ROOT/$STAMP/real_corpus"

real_build_enabled="$BUILD_IMAGE"
cgo_build_enabled="$BUILD_IMAGE"

real_ok=0
real_fail=0
real_oom=0
real_skip=0

cgo_ok=0
cgo_fail=0
cgo_oom=0
cgo_skip=0

real_corpus_status_from_log() {
  local log_path="$1"
  local summary
  summary="$(grep -E 'real-corpus\[' "$log_path" 2>/dev/null | tail -1 || true)"
  if [[ -z "$summary" ]]; then
    echo fail
    return
  fi
  if [[ "$summary" =~ no-error[[:space:]]+([0-9]+)/([0-9]+),[[:space:]]+sexpr[[:space:]]+parity[[:space:]]+([0-9]+)/([0-9]+),[[:space:]]+deep[[:space:]]+parity[[:space:]]+([0-9]+)/([0-9]+) ]]; then
    local no_error="${BASH_REMATCH[1]}"
    local eligible_a="${BASH_REMATCH[2]}"
    local sexpr="${BASH_REMATCH[3]}"
    local eligible_b="${BASH_REMATCH[4]}"
    local deep="${BASH_REMATCH[5]}"
    local eligible_c="${BASH_REMATCH[6]}"
    if [[ "$no_error" == "$eligible_a" && "$sexpr" == "$eligible_b" && "$deep" == "$eligible_c" ]]; then
      echo ok
    else
      echo fail
    fi
    return
  fi
  echo fail
}

run_real_corpus_lang() {
  local lang="$1"
  local grammar log_path
  local effective_memory_limit
  local effective_cpus_limit
  local effective_pids_limit
  local effective_gomaxprocs_value
  local effective_goflags_value
  local effective_lr0_core_budget
  local effective_generate_timeout

  grammar="$(real_corpus_lang "$lang")"
  log_path="$REAL_REPORT_DIR/diag_${grammar}.log"
  effective_memory_limit="$(resolve_fortran_real_corpus_setting "$lang" "$MEMORY_LIMIT" "$MEMORY_SET" "$FORTRAN_SAFE_MEMORY_LIMIT")"
  effective_cpus_limit="$(resolve_fortran_real_corpus_setting "$lang" "$CPUS_LIMIT" "$CPUS_SET" "$FORTRAN_SAFE_CPUS_LIMIT")"
  effective_pids_limit="$(resolve_fortran_real_corpus_setting "$lang" "$PIDS_LIMIT" "$PIDS_SET" "$FORTRAN_SAFE_PIDS_LIMIT")"
  effective_gomaxprocs_value="$(resolve_fortran_real_corpus_setting "$lang" "$GOMAXPROCS_VALUE" "$GOMAXPROCS_SET" "$FORTRAN_SAFE_GOMAXPROCS_VALUE")"
  effective_goflags_value="$(resolve_fortran_real_corpus_setting "$lang" "$GOFLAGS_VALUE" "$GOFLAGS_SET" "$FORTRAN_SAFE_GOFLAGS_VALUE")"
  effective_lr0_core_budget="$(resolve_fortran_real_corpus_setting "$lang" "$REAL_LR0_CORE_BUDGET" "$REAL_LR0_CORE_BUDGET_SET" "$FORTRAN_SAFE_LR0_CORE_BUDGET")"
  effective_generate_timeout="$(resolve_fortran_real_corpus_setting "$lang" "$REAL_GENERATE_TIMEOUT" "$REAL_GENERATE_TIMEOUT_SET" "$FORTRAN_SAFE_GENERATE_TIMEOUT")"

  mkdir -p "$REAL_REPORT_DIR"

  local -a args=(
    --memory "$effective_memory_limit"
    --cpus "$effective_cpus_limit"
    --pids "$effective_pids_limit"
    --timeout "$REAL_TIMEOUT"
    --max-cases "$REAL_MAX_CASES"
    --profile "$REAL_PROFILE"
    --report-dir "$REAL_REPORT_DIR"
  )
  if [[ -n "$effective_gomaxprocs_value" ]]; then
    args+=(--gomaxprocs "$effective_gomaxprocs_value")
  fi
  if [[ -n "$effective_goflags_value" ]]; then
    args+=(--goflags "$effective_goflags_value")
  fi
  if [[ -n "$effective_lr0_core_budget" ]]; then
    args+=(--lr0-core-budget "$effective_lr0_core_budget")
  fi
  if [[ -n "$effective_generate_timeout" ]]; then
    args+=(--generate-timeout "$effective_generate_timeout")
  fi
  if [[ -n "$SEED_DIR" ]]; then
    args+=(--seed-dir "$SEED_DIR")
  fi
  if [[ "$OFFLINE" == "1" ]]; then
    args+=(--offline)
  fi
  if [[ "$LR_SPLIT" == "1" ]]; then
    args+=(--lr-split)
  fi
  if [[ "$FORTRAN_SAFE_DEFAULTS" == "0" ]]; then
    args+=(--unsafe-fortran-defaults)
  fi
  if [[ "$real_build_enabled" == "0" ]]; then
    args+=(--no-build)
  fi
  args+=("$grammar")

  set +e
  "$REAL_RUNNER" "${args[@]}"
  local call_exit=$?
  set -e
  real_build_enabled=0

  local status="fail"
  if [[ -f "$log_path" ]]; then
    if grep -q '^oom_killed: true$' "$log_path"; then
      status="oom"
    elif grep -q '^exit_code: 0$' "$log_path"; then
      status="$(real_corpus_status_from_log "$log_path")"
    fi
  elif [[ "$call_exit" == "0" ]]; then
    status="fail"
  fi

  case "$status" in
    ok)
      echo "[real-corpus] $lang -> PARITY"
      return 0
      ;;
    oom)
      echo "[real-corpus] $lang -> OOM"
      return 137
      ;;
    *)
      echo "[real-corpus] $lang -> MISMATCH"
      return 1
      ;;
  esac
}

run_cgo_lang() {
  local lang="$1"
  if ! supports_cgo_parity "$lang"; then
    echo "[cgo] $lang -> SKIP (not wired in direct grammargen-vs-C harness)"
    return 125
  fi

  local -a args=(
    --memory "$MEMORY_LIMIT"
    --cpus "$CPUS_LIMIT"
    --pids "$PIDS_LIMIT"
    --max-cases "$CGO_MAX_CASES"
    --max-bytes "$CGO_MAX_BYTES"
    --langs "$lang"
    --timeout "$CGO_TIMEOUT_MINS"
    --label "focus-${lang}"
    --src-dir "$REPO_ROOT"
  )
  if [[ -n "$GOMAXPROCS_VALUE" ]]; then
    args+=(--gomaxprocs "$GOMAXPROCS_VALUE")
  fi
  if [[ -n "$GOFLAGS_VALUE" ]]; then
    args+=(--goflags "$GOFLAGS_VALUE")
  fi
  if [[ -n "$SEED_DIR" ]]; then
    args+=(--seed-dir "$SEED_DIR")
  fi
  if [[ "$OFFLINE" == "1" ]]; then
    args+=(--offline)
  fi
  if [[ "$cgo_build_enabled" == "0" ]]; then
    args+=(--no-build)
  fi

  set +e
  "$CGO_RUNNER" "${args[@]}"
  local exit_code=$?
  set -e
  cgo_build_enabled=0

  if [[ "$exit_code" == "0" ]]; then
    echo "[cgo] $lang -> OK"
    return 0
  elif [[ "$exit_code" == "137" ]]; then
    echo "[cgo] $lang -> OOM"
    return 137
  else
    echo "[cgo] $lang -> FAIL (exit=$exit_code)"
    return 1
  fi
}

record_real_rc() {
  local lang="$1"
  local rc="$2"
  case "$rc" in
    0) ((real_ok+=1)) || true ;;
    137) ((real_oom+=1)) || true ;;
    *) ((real_fail+=1)) || true ;;
  esac
}

record_cgo_rc() {
  local lang="$1"
  local rc="$2"
  case "$rc" in
    0) ((cgo_ok+=1)) || true ;;
    125) ((cgo_skip+=1)) || true ;;
    137) ((cgo_oom+=1)) || true ;;
    *) ((cgo_fail+=1)) || true ;;
  esac
}

run_real_corpus_phase() {
  if [[ "$JOBS" -eq 1 || "${#TARGET_LANGS[@]}" -eq 1 ]]; then
    for lang in "${TARGET_LANGS[@]}"; do
      local rc
      if run_real_corpus_lang "$lang"; then
        rc=0
      else
        rc=$?
      fi
      record_real_rc "$lang" "$rc"
    done
    return
  fi

  declare -a pids=()
  declare -a pid_langs=()

  wait_for_one_real() {
    local finished_pid lang idx rc
    if [[ "${#pids[@]}" -eq 0 ]]; then
      return 0
    fi
    if wait -n -p finished_pid "${pids[@]}"; then
      rc=0
    else
      rc=$?
    fi
    lang="$finished_pid"
    for idx in "${!pids[@]}"; do
      if [[ "${pids[$idx]}" == "$finished_pid" ]]; then
        lang="${pid_langs[$idx]}"
        unset 'pids[idx]'
        unset 'pid_langs[idx]'
        pids=("${pids[@]}")
        pid_langs=("${pid_langs[@]}")
        break
      fi
    done
    echo "[done] real-corpus: $lang exit=$rc"
    record_real_rc "$lang" "$rc"
  }

  for lang in "${TARGET_LANGS[@]}"; do
    while [[ "${#pids[@]}" -ge "$JOBS" ]]; do
      wait_for_one_real
    done
    echo "[start] real-corpus: $lang"
    run_real_corpus_lang "$lang" &
    pids+=("$!")
    pid_langs+=("$lang")
  done

  while [[ "${#pids[@]}" -gt 0 ]]; do
    wait_for_one_real
  done
}

run_cgo_phase() {
  if [[ "$JOBS" -eq 1 || "${#TARGET_LANGS[@]}" -eq 1 ]]; then
    for lang in "${TARGET_LANGS[@]}"; do
      local rc
      if run_cgo_lang "$lang"; then
        rc=0
      else
        rc=$?
      fi
      record_cgo_rc "$lang" "$rc"
    done
    return
  fi

  declare -a pids=()
  declare -a pid_langs=()

  wait_for_one_cgo() {
    local finished_pid lang idx rc
    if [[ "${#pids[@]}" -eq 0 ]]; then
      return 0
    fi
    if wait -n -p finished_pid "${pids[@]}"; then
      rc=0
    else
      rc=$?
    fi
    lang="$finished_pid"
    for idx in "${!pids[@]}"; do
      if [[ "${pids[$idx]}" == "$finished_pid" ]]; then
        lang="${pid_langs[$idx]}"
        unset 'pids[idx]'
        unset 'pid_langs[idx]'
        pids=("${pids[@]}")
        pid_langs=("${pid_langs[@]}")
        break
      fi
    done
    echo "[done] cgo: $lang exit=$rc"
    record_cgo_rc "$lang" "$rc"
  }

  for lang in "${TARGET_LANGS[@]}"; do
    while [[ "${#pids[@]}" -ge "$JOBS" ]]; do
      wait_for_one_cgo
    done
    echo "[start] cgo: $lang"
    run_cgo_lang "$lang" &
    pids+=("$!")
    pid_langs+=("$lang")
  done

  while [[ "${#pids[@]}" -gt 0 ]]; do
    wait_for_one_cgo
  done
}

if [[ "$JOBS" -gt 1 && "$BUILD_IMAGE" == "1" ]]; then
  if [[ "$MODE" == "all" || "$MODE" == "real-corpus" ]]; then
    docker build -t gotreesitter/cgo-harness:go1.25-local "$SCRIPT_DIR"
    real_build_enabled=0
  fi
  if [[ "$MODE" == "all" || "$MODE" == "cgo" ]]; then
    docker build -t gotreesitter-grammargen-cparity:latest -f "$SCRIPT_DIR/Dockerfile" "$SCRIPT_DIR"
    cgo_build_enabled=0
  fi
fi

echo "Focused grammargen targets: ${TARGET_LANGS[*]}"
echo "mode=$MODE memory=$MEMORY_LIMIT cpus=$CPUS_LIMIT pids=$PIDS_LIMIT gomaxprocs=${GOMAXPROCS_VALUE:-inherit} goflags=${GOFLAGS_VALUE:-inherit} lr0_core_budget=${REAL_LR0_CORE_BUDGET:-inherit} generate_timeout=${REAL_GENERATE_TIMEOUT:-inherit} fortran_safe_defaults=$FORTRAN_SAFE_DEFAULTS offline=$OFFLINE lr_split=$LR_SPLIT jobs=$JOBS allow_host_oversubscribe=$ALLOW_HOST_OVERSUBSCRIBE"
echo ""

if [[ "$MODE" == "all" || "$MODE" == "real-corpus" ]]; then
  echo "=== Real-Corpus Parity (per-grammar isolation) ==="
  run_real_corpus_phase
  echo ""
fi

if [[ "$MODE" == "all" || "$MODE" == "cgo" ]]; then
  echo "=== Direct Grammargen-vs-C Parity (per-language isolation) ==="
  run_cgo_phase
  echo ""
fi

echo "=== Summary ==="
if [[ "$MODE" == "all" || "$MODE" == "real-corpus" ]]; then
  echo "real-corpus: ok=$real_ok fail=$real_fail oom=$real_oom skip=$real_skip report_dir=$REAL_REPORT_DIR"
fi
if [[ "$MODE" == "all" || "$MODE" == "cgo" ]]; then
  echo "cgo: ok=$cgo_ok fail=$cgo_fail oom=$cgo_oom skip=$cgo_skip"
fi

if [[ "$real_fail" -gt 0 || "$real_oom" -gt 0 || "$cgo_fail" -gt 0 || "$cgo_oom" -gt 0 ]]; then
  exit 1
fi
