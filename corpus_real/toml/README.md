# Curated real-world TOML corpus

Hand-curated real-world TOML files for the tier-scan parity gate
(`cgo_harness/docker/run_tier_scan.sh` falls back to this directory because
the external corpus checkout for `toml` is the spec repo, which contains no
eligible `.toml` sources). Files are verbatim copies from open-source
projects; names are prefixed with the source project.

| file | source project | license |
| --- | --- | --- |
| cylc-pyproject.toml | github.com/cylc/cylc-flow | GPL-3.0 |
| gleam-website-gleam.toml | github.com/gleam-lang/website | Apache-2.0 |
| haskell-typos-srcs.toml | github.com/ghc/ghc | BSD-3-Clause |
| jinja2-pyproject.toml | github.com/pallets/jinja | BSD-3-Clause |
| llvm-pyproject.toml | github.com/llvm/llvm-project | Apache-2.0 WITH LLVM-exception |
| lua-stylua.toml | github.com/lua/lua mirror tooling | MIT |
| markdown-book.toml | github.com/rust-lang/mdBook corpus checkout | MPL-2.0 |
| markdown-cargo.toml | github.com/rust-lang/mdBook corpus checkout | MPL-2.0 |
| nickel-cargo.toml | github.com/tweag/nickel | MIT |
| pip-pyproject.toml | github.com/pypa/pip | MIT |
| ron-cargo.toml | github.com/ron-rs/ron | MIT/Apache-2.0 |
