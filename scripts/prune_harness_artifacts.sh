#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/prune_harness_artifacts.sh [--delete] [--include-golden]

Lists repo-local generated harness artifacts by size. With --delete, removes
the listed paths. The default is a dry run.

Options:
  --delete          Remove matching artifact paths.
  --include-golden  Also include .golden/ generated golden corpus artifacts.
  -h, --help        Show this help.
USAGE
}

delete=0
include_golden=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --delete)
      delete=1
      ;;
    --include-golden)
      include_golden=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || (cd "$script_dir/.." && pwd))"
cd "$repo_root"

targets=(
  "harness_out"
  "parity_out"
  "cgo_harness/parity_out"
  "cgo_harness/reports"
  "cgo_harness/corpus_real"
  "cgo_harness/grammar_seed"
  ".parity_seed"
)

if [[ "$include_golden" -eq 1 ]]; then
  targets+=(".golden")
fi

found=0
for target in "${targets[@]}"; do
  if [[ ! -e "$target" ]]; then
    continue
  fi
  found=1
  size="$(du -sh -- "$target" | awk '{print $1}')"
  if [[ "$delete" -eq 1 ]]; then
    printf 'removing %8s %s\n' "$size" "$target"
    rm -rf -- "$target"
  else
    printf '%8s %s\n' "$size" "$target"
  fi
done

if [[ "$found" -eq 0 ]]; then
  echo "no generated harness artifacts found"
elif [[ "$delete" -eq 0 ]]; then
  echo
  echo "dry run only; rerun with --delete to remove these paths"
fi
