# Scripts

For heavy correctness, parity, and race work, use CI or the Docker runners under
`cgo_harness/docker` and keep runs to one language at a time. The scripts in
this directory are focused host-side helpers, not the default path for OOM
diagnosis.

`with_grammar_subset.sh` is the host-side low-memory wrapper for focused grammar
work. It forces serial subset builds, wires in external blob loading, and can
point built-in grammar loaders at local grammargen `.bin` overrides.

`prune_harness_artifacts.sh` reports generated harness artifact directories by
size and removes them only when run with `--delete`. It intentionally excludes
private notes and defaults to a dry run.
