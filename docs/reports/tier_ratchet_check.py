#!/usr/bin/env python3
"""Perf-tier ratchet gate: fail if any grammar regressed below its floor tier.

Reads the committed floor (tier_floors.json) and the latest measurement
(harness_out/perf_picture/merged_bench_report.json), recomputes each grammar's
tier, and exits non-zero if any grammar is below its floor. `--bump` ratchets the
floor up to the current tiers (use only after a verified lift).
"""
import json, os, sys

ROOT = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(ROOT, "..", ".."))
FLOOR = os.path.join(ROOT, "tier_floors.json")
MERGED = os.path.join(REPO, "harness_out/perf_picture/merged_bench_report.json")
COLD = os.path.join(REPO, "harness_out/perf_picture/cold_cost.json")

# Tiers are Roman numerals (I best .. IV worst) so "III" never collides with the
# C language. Mapping from the old A/B/C/D scheme: A=I, B=II, C=III, D=IV.
RANK = {"I": 3, "II": 2, "III": 1, "IV": 0}  # higher is better

# PARITY IS A HARD GATE (2026-06-08). A grammar whose tree diverges from the C
# oracle is POISONED — untrustworthy regardless of speed — and is tier IV, full
# stop. Only parity-clean grammars (they passed full-parse parity vs C) are
# ranked I/II/III by performance. "parity-clean" = readiness passed full parity.
CLEAN_READINESS = {"meets-current-targets", "parity-clean", "parity-clean-slow",
                   "needs-incremental-work"}


def tier(parity_clean, fr, blob, cold_ms):
    if not parity_clean or fr <= 0:
        return "IV"  # poisoned (diverges from C) or unmeasured — hard stop
    if fr <= 1.5 and cold_ms <= 5 and blob <= 150_000:
        return "I"
    if fr > 8 or cold_ms > 20 or blob > 400_000:
        return "III"
    return "II"


def current_tiers():
    m = json.load(open(MERGED))
    cold = {r["name"]: r for r in json.load(open(COLD))}
    out = {}
    for x in m["languages"]:
        n = x["language"]
        c = cold.get(n, {})
        parity_clean = x.get("readiness") in CLEAN_READINESS
        out[n] = tier(parity_clean,
                      x.get("full_ratio") or 0,
                      c.get("blob_bytes") or 0,
                      (c.get("cold_decode_ns") or 0) / 1e6)
    return out


def main():
    bump = "--bump" in sys.argv
    floor = {k: v["tier"] for k, v in json.load(open(FLOOR)).items()}
    cur = current_tiers()
    regressions, lifts = [], []
    for n, ft in floor.items():
        ct = cur.get(n, "IV")
        if RANK[ct] < RANK[ft]:
            regressions.append((n, ft, ct))
        elif RANK[ct] > RANK[ft]:
            lifts.append((n, ft, ct))
    if lifts:
        print(f"LIFTS ({len(lifts)}): " + ", ".join(f"{n} {a}->{b}" for n, a, b in sorted(lifts)))
    if regressions:
        print(f"\nRATCHET VIOLATION — {len(regressions)} grammar(s) below floor:")
        for n, ft, ct in sorted(regressions):
            print(f"  {n}: floor={ft} current={ct}")
        if not bump:
            sys.exit(1)
    if bump:
        full = json.load(open(FLOOR))
        for n, ct in cur.items():
            if n in full and RANK[ct] > RANK[full[n]["tier"]]:
                full[n]["tier"] = ct
        json.dump(full, open(FLOOR, "w"), indent=2)
        print(f"\nfloor ratcheted up ({len(lifts)} lifts applied)")
    else:
        print("\nratchet OK — no grammar below floor" if not regressions else "")


if __name__ == "__main__":
    main()
