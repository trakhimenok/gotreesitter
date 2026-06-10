#!/usr/bin/env python3
"""Perf-tier ratchet gate: fail if any grammar regressed below its floor tier.

Current state is read from the canonical published artifacts, NOT a perf rerun:

  * parity  — ``cgo_harness/tier_scan/clean_grammars.txt`` (the hard gate; a
    grammar IS eligible for tiers I/II/III iff it is listed there, i.e. it
    parses byte-identical to the C oracle on its full measured corpus).
  * tier    — ``docs/reports/tiers.json`` (the published per-release tier
    table). The current tier of grammar ``g`` is tiers.json's tier for ``g``
    (``IV`` if absent). The parity gate is re-asserted defensively here: any
    grammar not in clean_grammars.txt is forced to ``IV`` regardless of what
    tiers.json says — parity-vs-C is the hard gate, full stop.

The committed floor lives in ``tier_floors.json`` (one ``{"tier": ...}`` entry
per grammar). The gate exits non-zero if any grammar's current tier is BELOW
its floor. ``--bump`` ratchets the floor up to the current tiers (use only
after a verified, published lift).

Canonical: ``docs/reports/tier-ratchet.md`` and
``cgo_harness/tier_scan/README.md``.
"""
import json, os, sys

ROOT = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(ROOT, "..", ".."))
FLOOR = os.path.join(ROOT, "tier_floors.json")
TIERS = os.path.join(ROOT, "tiers.json")
CLEAN = os.path.join(REPO, "cgo_harness", "tier_scan", "clean_grammars.txt")

# Tiers are Roman numerals (I best .. IV worst) so "III" never collides with the
# C language. Mapping from the old A/B/C/D scheme: A=I, B=II, C=III, D=IV.
RANK = {"I": 3, "II": 2, "III": 1, "IV": 0}  # higher is better


def clean_set():
    """Grammars that passed full-corpus parity vs the C oracle (the hard gate)."""
    with open(CLEAN) as f:
        return {ln.strip() for ln in f if ln.strip()}


def current_tiers():
    """Current published tier per grammar: tiers.json tier, IV if absent.

    PARITY IS A HARD GATE (2026-06-08): a grammar whose tree diverges from the C
    oracle is POISONED — untrustworthy regardless of speed — and is tier IV,
    full stop. Only parity-clean grammars (those in clean_grammars.txt) are
    ranked I/II/III. We therefore clamp any non-clean grammar to IV even if
    tiers.json happens to list it higher.
    """
    clean = clean_set()
    tiers = json.load(open(TIERS))
    out = {}
    for x in tiers["grammars"]:
        n = x["grammar"]
        t = x.get("tier", "IV")
        if n not in clean:
            t = "IV"
        out[n] = t if t in RANK else "IV"
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
        with open(FLOOR, "w") as f:
            json.dump(full, f, indent=2)
            f.write("\n")
        print(f"\nfloor ratcheted up ({len(lifts)} lifts applied)")
    else:
        print("\nratchet OK — no grammar below floor" if not regressions else "")


if __name__ == "__main__":
    main()
