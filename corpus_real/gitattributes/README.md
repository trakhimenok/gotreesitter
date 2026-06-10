# Curated real-world gitattributes corpus

Hand-curated real-world `.gitattributes` files for the tier-scan parity gate
(`cgo_harness/docker/run_tier_scan.sh` consults this directory only when the
external corpus checkout yields zero eligible files; the external checkout for
`gitattributes` is the git project itself and normally provides these same
files). Files are verbatim copies; one subdirectory per source location so
each keeps its real `.gitattributes` basename.

| dir | source |
| --- | --- |
| git-root | github.com/git/git `.gitattributes` |
| git-po | github.com/git/git `po/.gitattributes` |
| git-gui | github.com/git/git `git-gui/.gitattributes` |
| git-vscode | github.com/git/git `contrib/vscode/.gitattributes` |
| git-sha1dc | github.com/git/git `sha1dc/.gitattributes` |
| git-compat | github.com/git/git `compat/.gitattributes` |
| git-t5100 | github.com/git/git `t/t5100/.gitattributes` |

License: GPL-2.0 (git project).
