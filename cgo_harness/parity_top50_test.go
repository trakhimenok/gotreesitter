//go:build cgo && treesitter_c_parity

package cgoharness

// top50ParityLanguages is the lock-step top-50 correctness surface used by
// grammars/update_tier1_top50.txt and cgo_harness/testdata/top50_manifest.json.
var top50ParityLanguages = []string{
	"bash",
	"c",
	"cpp",
	"c_sharp",
	"cmake",
	"css",
	"dart",
	"elixir",
	"elm",
	"erlang",
	"go",
	"gomod",
	"graphql",
	"haskell",
	"hcl",
	"html",
	"ini",
	"java",
	"javascript",
	"json",
	"json5",
	"julia",
	"kotlin",
	"lua",
	"make",
	"markdown",
	"nix",
	"objc",
	"ocaml",
	"perl",
	"php",
	"powershell",
	"python",
	"r",
	"ruby",
	"rust",
	"scala",
	"scss",
	"sql",
	"svelte",
	"swift",
	"toml",
	"tsx",
	"typescript",
	"xml",
	"yaml",
	"zig",
	"awk",
	"clojure",
	"d",
}

var top50ParityLanguageSet = func() map[string]bool {
	out := make(map[string]bool, len(top50ParityLanguages))
	for _, name := range top50ParityLanguages {
		out[name] = true
	}
	return out
}()
