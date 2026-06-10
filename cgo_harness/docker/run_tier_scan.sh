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

# Curated fallback corpus: repo-root corpus_real/<grammar> holds hand-curated
# real-world files for grammars whose external corpus checkout contains no
# matching sources (e.g. toml's checkout is the spec repo, gitcommit's is the
# grammar repo). The external corpus stays authoritative — the curated dir is
# only consulted when the external checkout yields zero eligible files.
CURATED="$REPO_ROOT/corpus_real"

measure_grammar() { # $1=grammar $2=exts $3=corpus_root
  timeout "$PER_GRAMMAR_TIMEOUT" env CGO_ENABLED=1 \
    REPRO_LANG="$1" REPRO_DIR="$3" REPRO_EXTS="$2" REPRO_N="$N" \
    "$BIN" -test.run '^TestMeasureDtierVsC$' -test.count=1 2>&1 | grep -E '^MEASURE-DTIER' || true
}

while IFS=$'\t' read -r grammar exts; do
  [ -z "$grammar" ] && continue
  line=""
  if [ -d "$CORPUS/$grammar" ]; then
    line=$(measure_grammar "$grammar" "$exts" "$CORPUS")
    files=""
    if [ -n "$line" ]; then
      files=$(awk -F= '{print $2}' <<<"$(awk '{print $4}' <<<"$line")")
    fi
  fi
  if { [ -z "$line" ] || [ "${files:-0}" = "0" ]; } && [ -d "$CURATED/$grammar" ]; then
    curated_line=$(measure_grammar "$grammar" "$exts" "$CURATED")
    if [ -n "$curated_line" ]; then
      line="$curated_line corpus=curated"
    fi
  fi
  if [ -z "$line" ]; then
    if [ -d "$CORPUS/$grammar" ]; then
      echo "$grammar timeout-or-fail" >> "$UNMEASURED_OUT"
    else
      echo "$grammar no-corpus" >> "$UNMEASURED_OUT"
    fi
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

# Tier-IV characterization gate. Every non-clean grammar must carry a named
# assessed tier in tier_classification.tsv (III-recovery / III-scanner /
# III-version / III-stackcap / III-extmap / III-perf / III-unknown). An
# UNCHARACTERIZED tier-IV grammar (no row, or tier 'IV-unassessed') is the
# tier we drive to zero: a measured incorrect parse nobody has triaged.
CLASS_TSV="$HARNESS/tier_scan/tier_classification.tsv"
if [ -f "$CLASS_TSV" ]; then
  echo
  echo "=== tier characterization (vs tier_classification.tsv)"
  uncharacterized=""
  while read -r grammar rest; do
    [ -z "$grammar" ] && continue
    tier=$(awk -F'\t' -v g="$grammar" '$1==g{print $2}' "$CLASS_TSV")
    if [ -z "$tier" ] || [ "$tier" = "IV-unassessed" ]; then
      uncharacterized="$uncharacterized $grammar"
    fi
  done < "$TIER_IV_OUT"
  if [ -n "$uncharacterized" ]; then
    echo "UNCHARACTERIZED TIER IV (must be triaged into a III-* tier):"
    for g in $uncharacterized; do echo "  $g"; done
    status=1
  else
    echo "all tier-IV grammars characterized (0 uncharacterized) ✓"
  fi
fi
exit "$status"
