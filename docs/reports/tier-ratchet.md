# Perf-Tier Ratchet — the floor-lifting program for all 206 grammars

The organizing principle for gotreesitter: **parity vs the C oracle is a HARD GATE.** A
grammar whose tree diverges from tree-sitter C is untrusted — untrustworthy regardless of
how fast it is — and sits in **tier IV**, full stop. Only parity-clean grammars are ranked
I/II/III by performance. So the program is: **gate every grammar on parity, then rank the
trustworthy ones by speed; lift grammars IV→I/II/III by making them match C, and never let a
grammar regress below its floor.** Performance is meaningless until the tree is correct — a
fast wrong parser is worthless.

## Ground rule: the C oracle is the ONLY correctness reference (2026-06-08)

Every correctness check — forest **and** production — is made against the cgo / C-native
tree-sitter oracle, never against the production parser as a baseline. Production is a
culled, imperfect parser; using it as a reference hides bugs. This was proven the moment we
switched: gating the forest against production passed grammars (squirrel, and the
pre-existing bash/css) that **diverge from C**. Re-checking against C showed the forest
introduces **zero** new C-divergences (bash 4/4, css 1/1, squirrel 2/2 are all *inherited*
from production) — so the forest is exactly as correct as production, and the divergences
are a **pre-existing production-parser C-bug backlog** that production-baseline gating
masked.

**Parity is the gate, performance is the sub-rank (one tier scale, parity-gated):**
1. **Parity vs C** — the HARD GATE. Not parity-clean → tier IV (not-parity-clean), full stop. The
   ~149 IV grammars ARE the production-vs-C divergence backlog; clearing it is the program.
2. **Perf** — only ranks the parity-clean set, into I (fast) / II (normal) / III (heavy).

The forest lifts both: faster (tier) AND, gated against C, it can *fix* production's
C-divergence where production is the one that's wrong (e.g. gitattributes: forest=C,
production≠C → a parity lift *and* a 34.7× speed lift). The canonical forest gate is
therefore **forest-vs-C** (`cgo_harness/zz_forest_vs_c_sources_test.go` /
`TestForestVsCOracleParity`), not forest-vs-production.

## The tiers (workload fit, not quality)

Tiers are **Roman numerals (I best … IV worst)** so a tier label never collides with
the C language (the perf reference). Old A/B/C/D → I/II/III/IV.

| Tier | Meaning | Rule |
|---|---|---|
| **I** | parity-clean **and** latency-friendly | parity-clean vs C **and** ≤1.5× C, cold ≤5ms, blob ≤150KB |
| **II** | parity-clean, normal | parity-clean vs C **and** ≤8× C, cold ≤20ms |
| **III** | parity-clean, heavier | parity-clean vs C **and** (slow >8× C or cold >20ms or blob >400KB) |
| **IV** | **not parity-clean — tree untrusted, or unmeasured** | diverges from C (ANY parity failure) **or** unmeasured |

The gate is parity FIRST: a grammar that is not parity-clean is IV no matter how fast. I/II/III
are the *trustworthy* set, sub-ranked by speed.

## The floor (ratchet baseline, 2026-06-08)

`tier_floors.json` is the committed floor — one entry per grammar. Under the parity gate
(2026-06-08): **I=40 · II=14 · III=3 · IV=149** (206/206 measured).

**57 of 206 are parity-clean** (trustworthy vs C — tiers I–III); the remaining 149 diverge
from the C oracle and sit in IV until their parity is fixed. The earlier perf-only floor
(I=72 II=87 III=47 IV=0) measured speed only; re-gating on parity is a one-time reset that
reclassifies fast-but-divergent grammars to IV. Clearing IV — i.e. making grammars match C —
is now the program.

### 2026-06-10 frame reset

The tier scan frame moved to the **full 206-row `cgo_harness/tier_scan/exts.tsv` + the
canonical `~/work/gotreesitter-corpora/corpus_sources` corpus** (scan binary built at commit
`bf55ae63`). The previous ratchet (clean_grammars.txt, 78 entries) was accreted under an
older, richer-extension frame against a pre-Jun-7 corpus state; under the canonical frame
**12 of those entries no longer parse byte-identical to the C oracle** and are demoted to IV.
Per the ratchet README rule ("clean_grammars.txt entries are never removed without an
accepted engineering decision"), **this reconciliation is that decision: a one-time
canonical-frame reset**, mirroring the 2026-06-08 perf→parity reset precedent above. The 12
demotions, with named sub-causes (from the canonical scan / post-merge overlay):

- **awk** IV-recovery 28/29 — T.gawk rule-splitting vs C in-rule absorption
- **bitbake** IV-recovery 35/40 — canonical-frame demotion
- **comment** IV-perf 35/40 — trunc=5, 59× median (blowup truncation)
- **corn** IV-recovery 22/23 — quoted_keys path_seg child-count
- **dot** IV-perf 39/40 — bsdarch.dot 2257× agg blowup truncation
- **erlang** IV-recovery 38/40 — bif_SUITE ambiguity-selection + binary_SUITE blowup trunc
- **fidl** IV-shape 39/40 — INVERSE: C fragments versioned-FIDL syntax, Go parses clean; must match C error shapes
- **go** IV-recovery 37/40 — reader/writer_test root ERROR vs source_file (recovery class)
- **java** IV-recovery? 39/40 — canonical-frame demotion
- **jsonnet** IV-recovery? 39/40 — canonical-frame demotion
- **ssh_config** IV-recovery? 1/2 — canonical-frame demotion (2 eligible files under the canonical frame)
- **typescript** IV-recovery? 38/40 — canonical-frame demotion

Same-day, the mainline merges that landed *after* the scan binary — redwood stage-2
C-error-recovery gates, hazel lexgen, and oak's mojo/move repins — were folded in via a
post-merge overlay measure at the current HEAD: **css, zig, hack, jsdoc lifted to 100%
(40/40)** and join the ratchet; **go 37/40, mojo 30/40, move 14/40** improved but remain IV.
Net ratchet **78 → 67** (−12 demoted, +1 newly added: `zig`; css/hack/jsdoc were already
in the ratchet), but **every remaining entry is canonical-frame-verified**. Published tiers
after the reset: **I=28 · II=32 · III=7 · IV=139** (206/206).

## The gate (ratchet enforcement)

`tier_ratchet_check.py` re-reads the latest measurement, recomputes each grammar's tier,
and **fails if any grammar drops below its floor** in `tier_floors.json`. A lift updates the
floor (ratchet up); a regression fails CI. Run after a perf-affecting change:

```sh
python3 docs/reports/tier_ratchet_check.py            # fail on any below-floor grammar
python3 docs/reports/tier_ratchet_check.py --bump     # ratchet the floor up to current (after a verified lift)
```

## The tiers group pathology FAMILIES (optimize families, not languages)

A grammar's tier is *caused* by a pathology, and grammars that share a pathology cluster
together — so the unit of optimization is the **family**, not the language. `tier ×
pathology` (full data in `pathology_families.json`) yields the natural clusters; **one
lever lifts a whole family**. This guarantees (1) no blind spots — every grammar is
classified — and (2) no blind stabbing — every optimization targets a family with a known
lever. The families, by lever:

| Family (pathology) | Grammars (tiers) | Lever |
|---|---|---|
| **GLR-merge / blowup** | **28** (II:4, III:14, IV:10, pre-Phase-1) | **GSS-forest (+ recovery / selection-model fix)** — the single biggest lever |
| **action-loop-bound** | ~145 (I:49, II:74, III:22) | lexer-codegen + parse-table compaction (language-agnostic; hardest) |
| **huge-blob (cold/distribution)** | 6 (cpp, fsharp, nim, sql, swift, verilog) | blob compaction / lex-state packing |
| **result-build / compat-normalization** | 4 (c, dart, php, hack) | reduce/short-circuit per-language normalization passes |
| **lexer / DFA-bound** | 3 (mojo, thrift, html) | DFA tuning / lex-state compaction |
| **slow / recover-timeout** | 7 (agda, apex, cobol, cue, hare, pug, rst) | longer-timeout measurement, then family-classify |
| **small-file fixed-overhead** | (mixed III/II) | per-parse setup/arena amortization |

The GLR-merge family spans IV→II: the forest lifts its IV members out of "unmeasurable
truncation," and its III members III→II. That is why the forest is the first ratchet program.

## Lift map — what moves each grammar up, highest-leverage first

### IV → III (17): get them measurable — ✅ historical (Phase 1 done)
- **GLR-blowup (forest/recovery lever):** beancount, comment, cooklang, csv, fish, norg,
  promql, racket, scala, tlaplus — they truncate; the GSS-forest (+ recovery) is the fix.
- **recover-fail / parse-short (measurement lever):** agda, apex, cobol, cue, hare, pug,
  rst — large/slow grammars; longer-timeout allow-mismatch timing, or a forest dispatch.

### III → II (43): the biggest single lever is the FOREST
- **Forest/selection-model cluster (14):** authzed, bibtex, commonlisp, dockerfile, dtd,
  facility, gitattributes, json5, ledger, make, nginx, org, vimdoc, yuck. All are
  glr-merge-blowup grammars the forest *would* fix — currently blocked by the **one
  selection-model divergence** (forest builds alternatives production prunes; `bestLink`
  score-dominance vs production `branchOrder`). **Reconciling that selection model lifts
  this whole cluster III→II (and unblocks several IV→III).**
- **action-loop / lexer-codegen (26):** cpp, desktop, diff, ebnf, eds, eex,
  embedded_template, fsharp, gomod, http, hurl, hyprlang, kdl, less, liquid, … — bound by
  the table-driven LR loop (per `parse-attribution.md`); the lexer-codegen / parse-table
  compaction lever (language-agnostic, helps the most grammars but is the hardest).

### II → I (82): incremental + forest tuning
Mostly grammars 1.5–8× C; the levers are forest promotion (where the blowup applies),
fixed-overhead reduction for small-file grammars, and the incremental-path allowlist.

## The bottom-up elimination program

The strategy is explicit: **eliminate IV entirely, then III, then ratchet II→I over time.**
Work the floor up from the bottom, family by family (one lever clears a family), and never
regress (the gate enforces it). Each phase has a finite, named worklist.

### Phase 1 — eliminate IV (target: IV → 0) — ✅ DONE 2026-06-08

IV = not cleanly measurable. Two earlier clears (this program's first half) used the forest
lever: csv, fish, racket, tlaplus, beancount (forest+recovery = C, `introduced=0`, 14–312×
wall). The **final 12** (agda, apex, cobol, cue, hare, pug, rst, comment, cooklang, norg,
promql, scala) turned out NOT to be a parser pathology at all. Root cause, established by
investigation (not the assumed "GLR-blowup / action-loop-timeout" story):

1. **Corpus-coverage gap.** All 12 were in `grammars/languages.lock` with repo+commit AND
   had abundant real corpus in the sibling `../gotreesitter-corpora/corpus_sources/<lang>`
   — they were simply never *selected* into the bench's measured set (the bench ran a
   subset, not all 206). Pointing the measurement at the lock-filtered corpus fixed it.
2. **False-positive truncation detector.** The first measurement flagged 6 as "truncating"
   using `root.EndByte < lastNonWSByte(src)`. That is WRONG: trailing comments / extras are
   legitimately not covered by the root node — in **both** Go and C (e.g. cobol's parses end
   `accepted` exactly 70 bytes short = a trailing `* $ Version …` COBOL comment line). The
   correct validity criterion is **`ParseStopReason == accepted`** (the parse completed),
   not byte coverage. Under the corrected criterion every one of the 12 parses to `accepted`.
3. **Thin-corpus grammars** (cooklang 3 files, norg 2) were bolstered by extracting source
   blocks from their grammar repo's `test/corpus` / `corpus/` examples (norg 2→156,
   cooklang 3→13) so the median is trustworthy.

Final IV-clears (production parser, median ratio vs C, parity tracked separately):
cue→I (1.03×), pug→I (0.91×), apex→II (2.14×), promql→II (6.28×), hare→II (4.23×),
agda→III (28×; forest upside 0.85×→II), comment→III (29.9×, **parity-clean 100%**),
norg→III (8.8×), rst→III (139×), cooklang→III (166×), cobol→III (628KB blob), scala→III (472KB blob).

**Honest caveat:** IV=0 is the *perf-measurability* ratchet. Only **comment** is parity-clean;
the other 11 are parity-blocked (cobol/pug/promql/scala/agda/norg 0%, cue 32%, cooklang 43%,
hare/rst 5–10%). The harness: `cgo_harness/zz_measure_dtier_test.go` (timing+parity vs C),
`zz_dtier_trunc_diag_test.go` (ParseStopReason diagnosis). Reusable for any future grammar.

### Phase 2 — eliminate III (target: III → II or better) — IN PROGRESS (47→41)

III = heavy. Three families:

- **GLR-merge III family (forest lever).** **First batch done 2026-06-08 (6 lifts, III 47→41):**
  agda III→II (1.06×), org III→II (1.67×), ledger III→II (2.62×), json5 III→II (1.66×,
  parity-clean), gitattributes III→II (1.82×, stale floor — was forest-promoted but floored at
  the old 35× production number), and **yuck III→I (1.25×, parity-clean)**. All via forest+recovery
  with **introduced=0 vs the C oracle** (the "org/vimdoc selection-model bug" is vs *production*;
  vs C it is clean — a key C-oracle-ground-rule payoff). Remaining cluster: dockerfile, nginx
  (forest×external-scanner gap — 0 dispatch), vimdoc (likely promotable like org — verify),
  commonlisp, just, dtd (thin corpus / no registered ext — needs corpus bolstering).
- **action-loop C family (codegen lever, ~22).** cpp, desktop, diff, ebnf, eds, eex,
  embedded_template, gomod, http, hurl, hyprlang, kdl, less, liquid, … — bound by the
  table-driven LR loop. Lever: lexer-codegen + parse-table compaction (language-agnostic,
  highest reach, hardest). This is the long-pole engineering investment.
- **huge-blob C family (compaction lever, 6).** cpp, fsharp, nim, sql, swift, verilog —
  tier-III on cold/distribution cost (blob >400KB). Lever: blob compaction / lex-state packing
  (swift's 4.9MB is the marquee target). Independent of parse speed.

### Phase 3 — ratchet II → I (over time)

The ~80 B grammars (1.5–8× C) move up via: forest promotion where the blowup applies,
small-file fixed-overhead reduction, and the incremental-path allowlist. Worked continuously,
gated, never regressing.

### The scoreboard

The single number that tracks the program is the **tier histogram**, ratcheted only upward.

| Milestone (2026-06-08) | I | II | III | IV | measured |
|---|---|---|---|---|---|
| session start | 64 | 82 | 43 | 17 | 72/206 |
| after forest IV-clears + III-lifts | 70 | 84 | 40 | 12 | 194/206 |
| after final IV-elimination | 72 | 87 | 47 | 0 | 206/206 |
| **after Phase 2 batch 1 (6 forest promotions)** | **73** | **92** | **41** | **0** | **206/206** |
| **2026-06-10 canonical-frame reset + stage-2 lifts** | **28** | **32** | **7** | **139** | **206/206** |

The perf ratchet is complete. Each future session's job shifts to the **parity ratchet** and
the III→II lifts; push grammars up a tier and `--bump` the floor.

### The parity reality (the next mountain)

IV=0 means every grammar is *perf-measured*, NOT that every grammar is *correct*. Against the
C oracle: **70 parity-clean, 136 timing-only** (not parity-verified / divergent). Readiness
breakdown: needs-full-parse-work 94, meets-current-targets 39, needs-full-and-incremental 34,
parity-blocked 16, needs-incremental 15, incomplete-measurement 8 (stale label; all have a
ratio). The parity ratchet is now the dominant program — and the C-oracle ground rule makes
it a *visible, finite* backlog rather than a blind spot.

## Why this works

Every lift is **gated and irreversible** (the floor only ratchets up), so progress
compounds and never silently regresses. The map turns "make gotreesitter faster" into a
finite, prioritized, measurable backlog: **D done (0 left)**, 47 C's (forest cluster +
action-loop codegen + huge-blob compaction), 87 B's to tune, and the parity ratchet
(136 timing-only grammars) as the correctness counterpart.
