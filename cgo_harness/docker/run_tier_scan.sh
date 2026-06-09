#!/usr/bin/env bash
# Tier classification scan — run on every release.
#
# Measures every grammar listed in cgo_harness/tier_scan/exts.tsv against the
# tree-sitter C oracle over a real source corpus (REPRO_N first-sorted files,
# 32B..200KB) and classifies it:
#
#   CLEAN    parityMatch == measured files (100%, files > 0)
#   TIER-IV  anything below 100% (incorrect parse vs the C oracle)
#   UNMEASURED  no corpus dir / zero eligible files / timeout
#
# The committed ratchet (cgo_harness/tier_scan/clean_grammars.txt) makes tier
# IV strictly transitory: any previously-clean grammar that drops below 100%
# FAILS the scan (exit 1). Newly-clean grammars are reported so the ratchet
# can be advanced in the same release PR.
#
# Usage:
#   GTS_CORPUS_DIR=/path/to/corpus_sources cgo_harness/docker/run_tier_scan.sh [out_dir]
#
# Env:
#   GTS_CORPUS_DIR        corpus root with per-grammar subdirs (required)
#   GTS_TIER_SCAN_N       files per grammar (default 40)
#   GTS_TIER_SCAN_TIMEOUT per-grammar timeout seconds (default 600)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HARNESS="$REPO_ROOT/cgo_harness"
EXTS_TSV="$HARNESS/tier_scan/exts.tsv"
RATCHET="$HARNESS/tier_scan/clean_grammars.txt"
CORPUS="${GTS_CORPUS_DIR:?set GTS_CORPUS_DIR to the corpus_sources root}"
N="${GTS_TIER_SCAN_N:-40}"
PER_GRAMMAR_TIMEOUT="${GTS_TIER_SCAN_TIMEOUT:-600}"
OUT_DIR="${1:-$HARNESS/harness_out/tier_scan/$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "$OUT_DIR"
REPORT="$OUT_DIR/tier_scan.txt"
CLEAN_OUT="$OUT_DIR/clean.txt"
TIER_IV_OUT="$OUT_DIR/tier_iv.txt"
UNMEASURED_OUT="$OUT_DIR/unmeasured.txt"
: > "$REPORT"; : > "$CLEAN_OUT"; : > "$TIER_IV_OUT"; : > "$UNMEASURED_OUT"

BIN="$OUT_DIR/measure.test"
echo "building measure binary..."
(cd "$HARNESS" && CGO_ENABLED=1 go test -c -tags treesitter_c_parity -o "$BIN" .)

while IFS=$'\t' read -r grammar exts; do
  [ -z "$grammar" ] && continue
  if [ ! -d "$CORPUS/$grammar" ]; then
    echo "$grammar no-corpus" >> "$UNMEASURED_OUT"
    continue
  fi
  line=$(timeout "$PER_GRAMMAR_TIMEOUT" env CGO_ENABLED=1 \
    REPRO_LANG="$grammar" REPRO_DIR="$CORPUS" REPRO_EXTS="$exts" REPRO_N="$N" \
    "$BIN" -test.run '^TestMeasureDtierVsC$' -test.count=1 2>&1 | grep -E '^MEASURE-DTIER' || true)
  if [ -z "$line" ]; then
    echo "$grammar timeout-or-fail" >> "$UNMEASURED_OUT"
    continue
  fi
  echo "$line" >> "$REPORT"
  # parityMatch=A/B(P%) is field 7; files=N is field 4.
  parity=$(awk '{print $7}' <<<"$line")
  files=$(awk -F= '{print $2}' <<<"$(awk '{print $4}' <<<"$line")")
  matched="${parity#parityMatch=}"; matched="${matched%%/*}"
  total="${parity#*/}"; total="${total%%(*}"
  if [ "$files" -gt 0 ] && [ "$matched" = "$total" ]; then
    echo "$grammar" >> "$CLEAN_OUT"
  else
    echo "$grammar $parity" >> "$TIER_IV_OUT"
  fi
done < "$EXTS_TSV"

sort -o "$CLEAN_OUT" "$CLEAN_OUT"
echo
echo "=== tier scan summary ($OUT_DIR)"
echo "clean:      $(wc -l < "$CLEAN_OUT")"
echo "tier IV:    $(wc -l < "$TIER_IV_OUT")"
echo "unmeasured: $(wc -l < "$UNMEASURED_OUT")"

status=0
regressions=$(comm -23 <(sort "$RATCHET") "$CLEAN_OUT")
if [ -n "$regressions" ]; then
  echo
  echo "RATCHET REGRESSIONS (previously clean, now tier IV/unmeasured):"
  echo "$regressions" | sed 's/^/  /'
  status=1
fi
newly_clean=$(comm -13 <(sort "$RATCHET") "$CLEAN_OUT")
if [ -n "$newly_clean" ]; then
  echo
  echo "NEWLY CLEAN (advance the ratchet in tier_scan/clean_grammars.txt):"
  echo "$newly_clean" | sed 's/^/  /'
fi
exit "$status"
