#!/usr/bin/env python3
"""Per-release tier publication: generate docs/reports/tiers.{md,json}.

One tier scale (canonical: tier-ratchet.md / cgo_harness/tier_scan/README.md):
parity vs the C oracle is the HARD GATE, performance is the sub-rank.

  I    parity-clean and fast    (<=1.5x C full-parse, cold <=5ms, blob <=150KB)
  II   parity-clean, ok         (<=8x C full-parse, cold <=20ms)
  III  parity-clean, poor       (>8x C, or cold >20ms, or blob >400KB)
  IV   NOT parity-clean         (any divergence/truncation/unmeasured parity)

Sources of truth:
  - parity:    cgo_harness/tier_scan/clean_grammars.txt  (the byte-parity ratchet)
  - IV causes: cgo_harness/tier_scan/tier_classification.tsv
  - perf:      harness_out/perf_picture/{merged_bench_report,cold_cost}.json when
               present, else tier_floors.json floor tiers I/II/III as evidence.
               NOTE: floor tier IV is NOT perf evidence — historical floors
               derived parity from bench-report readiness, which wrongly
               classed parity-clean-but-slow grammars as IV. clean_grammars.txt
               overrides.

A parity-clean grammar with no perf evidence is published as "unranked
(parity-clean, perf pending)" — it never silently inflates or deflates a tier.

--require-zero-iv: exit 1 if any grammar is tier IV (the tiering-release gate;
set GTS_TIERS_REQUIRE_ZERO_IV=1 in the release scan to enforce it).
"""
import argparse
import datetime
import json
import os
import subprocess
import sys

ROOT = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(ROOT, "..", ".."))
CLEAN = os.path.join(REPO, "cgo_harness/tier_scan/clean_grammars.txt")
CLASS_TSV = os.path.join(REPO, "cgo_harness/tier_scan/tier_classification.tsv")
FLOOR = os.path.join(ROOT, "tier_floors.json")
MERGED = os.path.join(REPO, "harness_out/perf_picture/merged_bench_report.json")
COLD = os.path.join(REPO, "harness_out/perf_picture/cold_cost.json")

RANK = {"I": 0, "II": 1, "III": 2, "unranked": 3, "IV": 4}


def perf_tier(fr, blob, cold_ms):
    if fr <= 0:
        return None
    if fr <= 1.5 and cold_ms <= 5 and blob <= 150_000:
        return "I"
    if fr > 8 or cold_ms > 20 or blob > 400_000:
        return "III"
    return "II"


def load_clean():
    with open(CLEAN) as f:
        return {ln.strip() for ln in f if ln.strip()}


def load_classification():
    rows = {}
    with open(CLASS_TSV) as f:
        for i, ln in enumerate(f):
            parts = ln.rstrip("\n").split("\t")
            if i == 0 and parts and parts[0] == "grammar":
                continue
            if len(parts) >= 2 and parts[0]:
                rows[parts[0]] = {
                    "tier": parts[1],
                    "parity": parts[2] if len(parts) > 2 else "",
                    "notes": parts[3] if len(parts) > 3 else "",
                }
    return rows


def load_perf_evidence():
    """grammar -> (perf tier, source) from fresh measurements, else floors."""
    ev = {}
    if os.path.exists(MERGED) and os.path.exists(COLD):
        try:
            m = json.load(open(MERGED))
            cold = {r["name"]: r for r in json.load(open(COLD))}
            for x in m.get("languages", []):
                n = x["language"]
                c = cold.get(n, {})
                t = perf_tier(x.get("full_ratio") or 0,
                              c.get("blob_bytes") or 0,
                              (c.get("cold_decode_ns") or 0) / 1e6)
                if t:
                    ev[n] = (t, "measured")
        except (ValueError, KeyError) as e:
            print(f"warning: unreadable perf measurements: {e}", file=sys.stderr)
    if os.path.exists(FLOOR):
        floors = json.load(open(FLOOR))
        for n, v in floors.items():
            t = v.get("tier")
            if t in ("I", "II", "III") and n not in ev:
                ev[n] = (t, "floor")
    return ev


def head_sha():
    try:
        return subprocess.run(["git", "-C", REPO, "rev-parse", "--short", "HEAD"],
                              capture_output=True, text=True, check=True).stdout.strip()
    except Exception:
        return "unknown"


def build(version):
    clean = load_clean()
    classification = load_classification()
    perf = load_perf_evidence()
    universe = sorted(set(classification) | clean | set(perf))

    rows = []
    for g in universe:
        cls = classification.get(g, {})
        if g in clean:
            pt = perf.get(g)
            tier = pt[0] if pt else "unranked"
            rows.append({"grammar": g, "tier": tier, "parity": "clean",
                         "perf_source": pt[1] if pt else None})
        else:
            cause = cls.get("tier", "IV-unassessed")
            if not cause.startswith("IV"):
                cause = "IV-unassessed"
            rows.append({"grammar": g, "tier": "IV", "iv_cause": cause,
                         "parity": cls.get("parity", "unmeasured"),
                         "notes": cls.get("notes", "")})

    hist = {}
    for r in rows:
        hist[r["tier"]] = hist.get(r["tier"], 0) + 1
    return {
        "generated": datetime.datetime.now(datetime.timezone.utc)
        .strftime("%Y-%m-%dT%H:%M:%SZ"),
        "commit": head_sha(),
        "version": version,
        "tier_rules": {
            "I": "parity-clean and <=1.5x C full-parse, cold <=5ms, blob <=150KB",
            "II": "parity-clean and <=8x C full-parse, cold <=20ms",
            "III": "parity-clean and (>8x C or cold >20ms or blob >400KB)",
            "IV": "not parity-clean (any divergence vs the C oracle, or unmeasured)",
            "unranked": "parity-clean; perf measurement pending",
        },
        "histogram": hist,
        "grammars": rows,
    }


def to_md(doc):
    h = doc["histogram"]
    lines = [
        f"# Grammar tiers — {doc['version']}",
        "",
        f"Generated {doc['generated']} at `{doc['commit']}`. Parity vs the",
        "tree-sitter C oracle is the hard gate; performance is the sub-rank",
        "(rules in `cgo_harness/tier_scan/README.md`).",
        "",
        "| tier | count |",
        "| --- | --- |",
    ]
    for t in ("I", "II", "III", "unranked", "IV"):
        if h.get(t):
            lines.append(f"| {t} | {h[t]} |")
    by_tier = {}
    for r in doc["grammars"]:
        by_tier.setdefault(r["tier"], []).append(r)
    for t, title in (("I", "Tier I — parity-clean, fast"),
                     ("II", "Tier II — parity-clean, ok"),
                     ("III", "Tier III — parity-clean, poor perf"),
                     ("unranked", "Unranked — parity-clean, perf measurement pending")):
        rs = by_tier.get(t)
        if not rs:
            continue
        lines += ["", f"## {title} ({len(rs)})", ""]
        lines.append(", ".join(f"`{r['grammar']}`" for r in rs))
    rs = by_tier.get("IV")
    if rs:
        lines += ["", f"## Tier IV — not parity-clean ({len(rs)})", "",
                  "| grammar | cause | parity |", "| --- | --- | --- |"]
        for r in rs:
            lines.append(f"| `{r['grammar']}` | {r['iv_cause']} | {r['parity']} |")
    else:
        lines += ["", "## Tier IV — not parity-clean (0)", "",
                  "**Empty.** Every grammar parses byte-identical to the C oracle."]
    lines.append("")
    return "\n".join(lines)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--version", default="unreleased",
                    help="release label stamped into the artifact")
    ap.add_argument("--out-md", default=os.path.join(ROOT, "tiers.md"))
    ap.add_argument("--out-json", default=os.path.join(ROOT, "tiers.json"))
    ap.add_argument("--require-zero-iv", action="store_true",
                    help="exit 1 if any grammar is tier IV (tiering-release gate)")
    args = ap.parse_args()

    doc = build(args.version)
    with open(args.out_json, "w") as f:
        json.dump(doc, f, indent=1)
        f.write("\n")
    with open(args.out_md, "w") as f:
        f.write(to_md(doc))
    h = doc["histogram"]
    print("tiers: " + "  ".join(f"{t}={h.get(t, 0)}"
                                for t in ("I", "II", "III", "unranked", "IV")))
    print(f"wrote {args.out_md} and {args.out_json}")

    iv = [r for r in doc["grammars"] if r["tier"] == "IV"]
    if args.require_zero_iv and iv:
        print(f"\nTIERING GATE FAILED: {len(iv)} grammar(s) are tier IV "
              "(not parity-clean):", file=sys.stderr)
        for r in iv:
            print(f"  {r['grammar']}: {r['iv_cause']} ({r['parity']})",
                  file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
