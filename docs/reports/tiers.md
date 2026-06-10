# Grammar tiers — unreleased

Generated 2026-06-10T08:51:04Z at `bfecdbaf`. Parity vs the
tree-sitter C oracle is the hard gate; performance is the sub-rank
(rules in `cgo_harness/tier_scan/README.md`).

| tier | count |
| --- | --- |
| I | 32 |
| II | 35 |
| III | 9 |
| IV | 130 |

## Tier I — parity-clean, fast (32)

`astro`, `awk`, `clojure`, `css`, `csv`, `elisp`, `erlang`, `faust`, `fidl`, `fish`, `gitcommit`, `gleam`, `hcl`, `java`, `javascript`, `jsonnet`, `llvm`, `nickel`, `nix`, `php`, `prisma`, `puppet`, `racket`, `smithy`, `squirrel`, `starlark`, `thrift`, `tsx`, `turtle`, `xml`, `yaml`, `yuck`

## Tier II — parity-clean, ok (35)

`arduino`, `bass`, `beancount`, `bitbake`, `capnp`, `cmake`, `corn`, `devicetree`, `dot`, `editorconfig`, `foam`, `fortran`, `git_config`, `git_rebase`, `gitattributes`, `gitignore`, `go`, `hack`, `heex`, `janet`, `jq`, `json`, `json5`, `ocaml`, `pem`, `python`, `ron`, `sparql`, `tablegen`, `textproto`, `todotxt`, `toml`, `twig`, `typescript`, `vue`

## Tier III — parity-clean, poor perf (9)

`comment`, `desktop`, `diff`, `eex`, `embedded_template`, `gomod`, `nginx`, `properties`, `ssh_config`

## Tier IV — not parity-clean (130)

| grammar | cause | parity |
| --- | --- | --- |
| `ada` | IV-shape? | 24/30 |
| `agda` | IV-scanner | 0/40 |
| `angular` | IV-recovery? | 35/40 |
| `apex` | IV-shape? | 17/30 |
| `asm` | IV-recovery | 0/40 |
| `authzed` | IV-recovery? | 23/30 |
| `bash` | IV-recovery? | 30/40 |
| `bibtex` | IV-recovery? | 37/40 |
| `bicep` | IV-recovery? | 24/30 |
| `blade` | IV-recovery? | 17/30 |
| `brightscript` | IV-recovery? | 0/30 |
| `c` | IV-recovery | 21/40 |
| `c_sharp` | IV-recovery | 26/40 |
| `caddy` | IV-recovery? | 9/30 |
| `cairo` | IV-recovery? | 0/30 |
| `chatito` | IV-recovery | 1/5 |
| `circom` | IV-shape? | 11/30 |
| `cobol` | IV-scanner | 0/40 |
| `commonlisp` | IV-recovery? | 22/30 |
| `cooklang` | IV-recovery | 0/3 |
| `cpon` | IV-recovery? | 9/10 |
| `cpp` | IV-recovery | 10/40 |
| `crystal` | IV-perf | 0/0 |
| `cuda` | IV-recovery? | 17/30 |
| `cue` | IV-shape? | 21/30 |
| `cylc` | IV-recovery? | 4/30 |
| `d` | IV-recovery? | 14/30 |
| `dart` | IV-recovery? | 11/30 |
| `dhall` | IV-unknown | 23/40 |
| `disassembly` | IV-scanner | 0/40 |
| `djot` | IV-scanner? | 0/40 |
| `dockerfile` | IV-recovery? | 0/30 |
| `doxygen` | IV-recovery? | 19/30 |
| `dtd` | IV-recovery? | 0/5 |
| `earthfile` | IV-recovery? | 0/30 |
| `ebnf` | IV-recovery? | 0/30 |
| `eds` | IV-recovery? | 0/1 |
| `elixir` | IV-unknown | 0/40 |
| `elm` | IV-recovery? | 7/8 |
| `elsa` | IV-recovery? | 12/27 |
| `enforce` | IV-shape? | 21/30 |
| `facility` | IV-recovery? | 1/4 |
| `fennel` | IV-recovery? | 8/30 |
| `firrtl` | IV-recovery? | 5/27 |
| `forth` | IV-unknown | 34/40 |
| `fsharp` | IV-recovery? | 0/8 |
| `gdscript` | IV-scanner | 1/40 |
| `glsl` | IV-recovery | 11/40 |
| `gn` | IV-scanner | 27/40 |
| `godot_resource` | IV-scanner | 21/40 |
| `graphql` | IV-recovery | 0/1 |
| `groovy` | IV-recovery | 4/40 |
| `hare` | IV-recovery | 20/40 |
| `haskell` | IV-scanner | 7/40 |
| `haxe` | IV-version | 7/40 |
| `hlsl` | IV-recovery | 33/40 |
| `html` | IV-recovery | 0/40 |
| `http` | IV-unknown | 6/11 |
| `hurl` | IV-recovery | 13/40 |
| `hyprlang` | IV-recovery | 1/2 |
| `ini` | IV-unknown | 4/11 |
| `jinja2` | IV-recovery | 3/40 |
| `jsdoc` | IV-recovery? | 39/40 |
| `julia` | IV-recovery | 28/40 |
| `just` | IV-recovery? | 2/8 |
| `kconfig` | IV-recovery? | 13/30 |
| `kdl` | IV-version | 12/40 |
| `kotlin` | IV-unknown | 17/40 |
| `ledger` | IV-recovery | 2/4 |
| `less` | IV-recovery? | 10/40 |
| `linkerscript` | IV-recovery | 1/40 |
| `liquid` | IV-recovery? | 11/36 |
| `lua` | IV-scanner | 7/40 |
| `luau` | IV-recovery | 35/40 |
| `make` | IV-recovery? | 19/20 |
| `markdown` | IV-shape? | 31/40 |
| `markdown_inline` | IV-shape? | 13/30 |
| `matlab` | IV-recovery? | 4/40 |
| `mermaid` | IV-recovery? | 0/40 |
| `meson` | IV-recovery? | 1/30 |
| `mojo` | IV-version | 4/40 |
| `move` | IV-version | 0/40 |
| `nim` | IV-recovery? | 3/40 |
| `ninja` | IV-recovery | 3/5 |
| `norg` | IV-scanner | 0/2 |
| `nushell` | IV-recovery? | 5/40 |
| `objc` | IV-recovery? | 1/40 |
| `odin` | IV-recovery? | 13/40 |
| `org` | IV-recovery? | 5/39 |
| `pascal` | IV-recovery? | 0/40 |
| `perl` | IV-recovery? | 0/40 |
| `pkl` | IV-shape? | 34/40 |
| `powershell` | IV-recovery? | 22/40 |
| `prolog` | IV-recovery? | 4/40 |
| `promql` | IV-recovery? | 0/4 |
| `proto` | IV-recovery? | 25/40 |
| `pug` | IV-recovery? | 0/40 |
| `purescript` | IV-recovery? | 1/40 |
| `ql` | IV-shape? | 33/40 |
| `r` | IV-shape? | 33/40 |
| `regex` | IV-unknown? | 0/1 |
| `rego` | IV-recovery? | 7/40 |
| `requirements` | IV-recovery? | 8/9 |
| `rescript` | IV-recovery? | 23/40 |
| `robot` | IV-recovery? | 28/40 |
| `rst` | IV-shape? | 1/8 |
| `ruby` | IV-shape? | 25/40 |
| `rust` | IV-recovery? | 21/40 |
| `scala` | IV-recovery? | 25/40 |
| `scheme` | IV-recovery | 36/40 |
| `scss` | IV-recovery? | 6/40 |
| `solidity` | IV-shape? | 10/40 |
| `sql` | IV-recovery? | 8/40 |
| `svelte` | IV-recovery? | 37/40 |
| `swift` | IV-recovery? | 0/40 |
| `tcl` | IV-recovery? | 10/40 |
| `teal` | IV-recovery? | 4/40 |
| `templ` | IV-recovery? | 24/40 |
| `tlaplus` | IV-unknown? | 14/40 |
| `tmux` | IV-recovery? | 0/1 |
| `typst` | IV-recovery? | 28/40 |
| `uxntal` | IV-recovery? | 0/40 |
| `v` | IV-recovery? | 25/40 |
| `verilog` | IV-recovery? | 4/40 |
| `vhdl` | IV-recovery? | 14/40 |
| `vimdoc` | IV-recovery? | 0/30 |
| `wat` | IV-recovery? | 4/34 |
| `wgsl` | IV-recovery? | 20/40 |
| `wolfram` | IV-recovery? | 0/11 |
| `zig` | IV-recovery? | 39/40 |
