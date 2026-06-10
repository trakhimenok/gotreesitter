package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectFilesByBucketFillsToTarget(t *testing.T) {
	candidates := []corpusFile{
		{RelPath: "a.txt", Size: 400},
		{RelPath: "b.txt", Size: 800},
		{RelPath: "c.txt", Size: 2500},
		{RelPath: "d.txt", Size: 4200},
	}

	selected := selectFilesByBucket(candidates, 1, 256, 2000, 16000)
	if len(selected) != 3 {
		t.Fatalf("expected 3 selected files, got %d", len(selected))
	}

	seen := map[string]struct{}{}
	for _, sf := range selected {
		if _, ok := seen[sf.RelPath]; ok {
			t.Fatalf("duplicate selected path: %s", sf.RelPath)
		}
		seen[sf.RelPath] = struct{}{}
		if sf.Bucket == "" {
			t.Fatalf("empty bucket for %s", sf.RelPath)
		}
	}
}

func TestSelectFilesByBucketKeepsSmallMediumLargeWhenAvailable(t *testing.T) {
	candidates := []corpusFile{
		{RelPath: "small.go", Size: 512},
		{RelPath: "medium.go", Size: 4096},
		{RelPath: "large.go", Size: 65536},
	}

	selected := selectFilesByBucket(candidates, 1, 256, 2000, 16000)
	if len(selected) != 3 {
		t.Fatalf("expected 3 selected files, got %d", len(selected))
	}

	buckets := map[string]bool{}
	for _, sf := range selected {
		buckets[sf.Bucket] = true
	}
	for _, bucket := range []string{"small", "medium", "large"} {
		if !buckets[bucket] {
			t.Fatalf("missing bucket %q in selection: %#v", bucket, selected)
		}
	}
}

func TestSelectFilesByBucketDoesNotRelabelCrossBucketFallbacks(t *testing.T) {
	candidates := []corpusFile{
		{RelPath: "medium.kt", Size: 4520},
		{RelPath: "large.ts", Size: 376385},
	}

	selected := selectFilesByBucket(candidates, 1, 256, 2000, 16000)
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected files, got %d", len(selected))
	}

	got := map[string]string{}
	for _, sf := range selected {
		got[sf.RelPath] = sf.Bucket
	}
	if got["medium.kt"] != "medium" {
		t.Fatalf("medium.kt bucket = %q, want medium", got["medium.kt"])
	}
	if got["large.ts"] != "large" {
		t.Fatalf("large.ts bucket = %q, want large", got["large.ts"])
	}
}

func TestCollectCandidatesWithoutExtsSkipsLockfiles(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "Cargo.lock"), 512)
	mustWriteSizedText(t, filepath.Join(tmp, "go.sum"), 512)
	mustWriteSizedText(t, filepath.Join(tmp, "package-lock.json"), 512)
	mustWriteSizedText(t, filepath.Join(tmp, "test", "corpus", "valid.chatito"), 512)

	candidates, err := collectCandidates(tmp, nil, defaultMaxBytes, true)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatalf("expected candidates, got none")
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if seen["Cargo.lock"] || seen["go.sum"] || seen["package-lock.json"] {
		t.Fatalf("lockfiles must be excluded from candidates: %#v", candidates)
	}
	if !seen["test/corpus/valid.chatito"] {
		t.Fatalf("expected corpus candidate missing: %#v", candidates)
	}
}

func TestCollectCandidatesWithoutExtsRequiresCorpusLikePaths(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "src", "program.scala"), 600)
	mustWriteSizedText(t, filepath.Join(tmp, "examples", "hello.chatito"), 600)
	mustWriteSizedText(t, filepath.Join(tmp, ".github", "workflow.yml"), 600)

	candidates, err := collectCandidates(tmp, nil, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}
	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if seen[".github/workflow.yml"] {
		t.Fatalf("metadata/config files should be excluded: %#v", candidates)
	}
	if seen["src/program.scala"] {
		t.Fatalf("non-corpus source files should be excluded without explicit ext hints: %#v", candidates)
	}
	if !seen["examples/hello.chatito"] {
		t.Fatalf("expected corpus-like path missing: %#v", candidates)
	}
}

func TestCollectCandidatesWithExtsKeepsCorpusTextFixtures(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "corpus", "declarations.txt"), 1200)
	mustWriteSizedText(t, filepath.Join(tmp, "examples", "demo.swift"), 1200)
	mustWriteSizedText(t, filepath.Join(tmp, "examples", "README.txt"), 1200)

	candidates, err := collectCandidates(tmp, []string{".swift"}, defaultMaxBytes, true)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if !seen["corpus/declarations.txt"] {
		t.Fatalf("expected corpus text fixture to be retained: %#v", candidates)
	}
	if !seen["examples/demo.swift"] {
		t.Fatalf("expected example source file to be retained: %#v", candidates)
	}
	if seen["examples/README.txt"] {
		t.Fatalf("example docs with mismatched ext should be excluded: %#v", candidates)
	}
}

func TestCollectCandidatesWithoutFixturesExcludesCorpusTests(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "test", "corpus", "valid.chatito"), 512)
	mustWriteSizedText(t, filepath.Join(tmp, "examples", "hello.chatito"), 512)

	candidates, err := collectCandidates(tmp, nil, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if seen["test/corpus/valid.chatito"] {
		t.Fatalf("fixture corpus file should be excluded in real-world mode: %#v", candidates)
	}
	if !seen["examples/hello.chatito"] {
		t.Fatalf("expected example candidate missing: %#v", candidates)
	}
}

func TestCollectCandidatesWithoutFixturesAllowsSourceTypedHighlightAndTagTests(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "test", "highlight", "operators.ex"), 800)
	mustWriteSizedText(t, filepath.Join(tmp, "tests", "tags", "module.ex"), 900)
	mustWriteSizedText(t, filepath.Join(tmp, "test", "unit", "helper.ex"), 900)
	mustWriteSizedText(t, filepath.Join(tmp, "test", "corpus", "fixture.ex"), 900)

	candidates, err := collectCandidates(tmp, []string{".ex"}, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if !seen["test/highlight/operators.ex"] {
		t.Fatalf("expected highlight source candidate missing: %#v", candidates)
	}
	if !seen["tests/tags/module.ex"] {
		t.Fatalf("expected tags source candidate missing: %#v", candidates)
	}
	if seen["test/unit/helper.ex"] {
		t.Fatalf("general test file should remain excluded in source-only mode: %#v", candidates)
	}
	if seen["test/corpus/fixture.ex"] {
		t.Fatalf("fixture corpus file should remain excluded in source-only mode: %#v", candidates)
	}
}

func TestCollectCandidatesWithNamedFilesAllowsSpecialLanguageFiles(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "go.mod"), 600)
	mustWriteSizedText(t, filepath.Join(tmp, "CMakeLists.txt"), 900)
	mustWriteSizedText(t, filepath.Join(tmp, "README.txt"), 900)

	candidates, err := collectCandidatesWithNames(tmp, nil, []string{"go.mod", "cmakelists.txt"}, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidatesWithNames: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if !seen["go.mod"] {
		t.Fatalf("expected go.mod candidate missing: %#v", candidates)
	}
	if !seen["CMakeLists.txt"] {
		t.Fatalf("expected CMakeLists.txt candidate missing: %#v", candidates)
	}
	if seen["README.txt"] {
		t.Fatalf("readme should remain excluded: %#v", candidates)
	}
}

func TestCollectCandidatesWithNamedFilesAllowsMarkdownReadmeWhenRequested(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "README.md"), 900)
	mustWriteSizedText(t, filepath.Join(tmp, "README.txt"), 900)

	candidates, err := collectCandidatesWithNames(tmp, []string{".md"}, []string{"readme.md"}, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidatesWithNames: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if !seen["README.md"] {
		t.Fatalf("expected README.md candidate missing: %#v", candidates)
	}
	if seen["README.txt"] {
		t.Fatalf("README.txt should remain excluded: %#v", candidates)
	}
}

func TestCollectCandidatesWithNamedFilesAllowsUndersizedCMakeHighlightSource(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "CMakeLists.txt"), 900)
	mustWriteSizedText(t, filepath.Join(tmp, "test", "highlight", "block.cmake"), 147)

	candidates, err := collectCandidatesWithNames(tmp, []string{".cmake"}, []string{"cmakelists.txt"}, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidatesWithNames: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if !seen["CMakeLists.txt"] {
		t.Fatalf("expected CMakeLists.txt candidate missing: %#v", candidates)
	}
	if !seen["test/highlight/block.cmake"] {
		t.Fatalf("expected undersized highlight source candidate missing: %#v", candidates)
	}
}

func TestCollectCandidatesAllowsNarrowRBindingsSourcePaths(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "bindings", "r", "R", "import-standalone-language.R"), 2250)
	mustWriteSizedText(t, filepath.Join(tmp, "bindings", "r", "bootstrap.R"), 7221)
	mustWriteSizedText(t, filepath.Join(tmp, "bindings", "node", "index.js"), 4096)

	candidates, err := collectCandidates(tmp, []string{".r", ".R"}, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if !seen["bindings/r/R/import-standalone-language.R"] {
		t.Fatalf("expected R binding source candidate missing: %#v", candidates)
	}
	if !seen["bindings/r/bootstrap.R"] {
		t.Fatalf("expected R bootstrap source candidate missing: %#v", candidates)
	}
	if seen["bindings/node/index.js"] {
		t.Fatalf("non-R bindings file should remain excluded: %#v", candidates)
	}
}

func TestCollectCandidatesAllowsYamlGithubWorkflowSources(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, ".github", "workflows", "ci.yml"), 2400)
	mustWriteSizedText(t, filepath.Join(tmp, ".github", "ISSUE_TEMPLATE", "bug_report.yaml"), 2300)
	mustWriteSizedText(t, filepath.Join(tmp, ".github", "workflows", "ci.json"), 2400)

	candidates, err := collectCandidates(tmp, []string{".yaml", ".yml"}, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if !seen[".github/workflows/ci.yml"] {
		t.Fatalf("expected workflow yaml candidate missing: %#v", candidates)
	}
	if !seen[".github/ISSUE_TEMPLATE/bug_report.yaml"] {
		t.Fatalf("expected issue template yaml candidate missing: %#v", candidates)
	}
	if seen[".github/workflows/ci.json"] {
		t.Fatalf("non-yaml workflow file should remain excluded: %#v", candidates)
	}
}

func TestCandidateMatchersForLanguageInfersKnownExtensionsAndNames(t *testing.T) {
	tests := []struct {
		lang      string
		wantExts  []string
		wantNames []string
	}{
		{lang: "bibtex", wantExts: []string{".bib"}},
		{lang: "bicep", wantExts: []string{".bicep"}},
		{lang: "blade", wantExts: []string{".blade.php"}},
		{lang: "capnp", wantExts: []string{".capnp"}},
		{lang: "dart", wantExts: []string{".dart"}},
		{lang: "dockerfile", wantExts: []string{".dockerfile"}, wantNames: []string{"dockerfile", "containerfile", "1", "9"}},
		{lang: "earthfile", wantExts: []string{".earth"}, wantNames: []string{"earthfile"}},
		{lang: "erlang", wantExts: []string{".erl", ".hrl"}},
		{lang: "git_rebase", wantExts: []string{".git-rebase-todo"}, wantNames: []string{"git-rebase-todo", "rebase-todo"}},
		{lang: "gomod", wantNames: []string{"go.mod"}},
		{lang: "cmake", wantExts: []string{".cmake"}, wantNames: []string{"cmakelists.txt"}},
		{lang: "make", wantExts: []string{".mk"}, wantNames: []string{"makefile"}},
		{lang: "markdown", wantExts: []string{".md"}, wantNames: []string{"readme.md"}},
		{lang: "meson", wantNames: []string{"meson.build", "meson_options.txt"}},
		{lang: "nginx", wantExts: []string{".nginx"}, wantNames: []string{"nginx.conf", "conf.nginx"}},
		{lang: "requirements", wantNames: []string{"requirements.txt"}},
		{lang: "ssh_config", wantNames: []string{"ssh_config", "sshd_config", "known_hosts", "authorized_keys"}},
		{lang: "tmux", wantNames: []string{"tmux.conf", ".tmux.conf"}},
		{lang: "todotxt", wantNames: []string{"todo.txt"}},
	}

	for _, tc := range tests {
		gotExts, gotNames, _ := candidateMatchersForLanguage(tc.lang, nil)
		for _, want := range tc.wantExts {
			if !containsString(gotExts, want) {
				t.Fatalf("%s ext matchers missing %q: %#v", tc.lang, want, gotExts)
			}
		}
		for _, want := range tc.wantNames {
			if !containsString(gotNames, want) {
				t.Fatalf("%s name matchers missing %q: %#v", tc.lang, want, gotNames)
			}
		}
	}
}

func TestCandidateMatchersForLanguageHonorsExplicitLockMatchers(t *testing.T) {
	gotExts, gotNames, gotPaths := candidateMatchersForLanguage("ssh_config", []string{"ssh_config", ".conf"})
	if !containsString(gotExts, ".conf") {
		t.Fatalf("extension matcher missing .conf: %#v", gotExts)
	}
	if !containsString(gotNames, "ssh_config") {
		t.Fatalf("basename matcher missing ssh_config: %#v", gotNames)
	}
	for _, unexpected := range []string{"sshd_config", "known_hosts", "authorized_keys"} {
		if containsString(gotNames, unexpected) {
			t.Fatalf("explicit lock matchers should not add %q: %#v", unexpected, gotNames)
		}
	}
	if len(gotPaths) != 0 {
		t.Fatalf("unexpected path matchers: %#v", gotPaths)
	}
}

func TestCandidateMatchersForLanguageHonorsExplicitPathMatchers(t *testing.T) {
	gotExts, gotNames, gotPaths := candidateMatchersForLanguage("ini", []string{"Lib/tomllib/mypy.ini", "Tools\\jit\\mypy.ini", "./root.ini"})
	if len(gotExts) != 0 {
		t.Fatalf("expected no extension matchers, got %#v", gotExts)
	}
	if len(gotNames) != 0 {
		t.Fatalf("expected no basename matchers, got %#v", gotNames)
	}
	for _, want := range []string{"lib/tomllib/mypy.ini", "tools/jit/mypy.ini", "root.ini"} {
		if !containsString(gotPaths, want) {
			t.Fatalf("path matcher missing %q: %#v", want, gotPaths)
		}
	}
}

func TestCollectCandidatesWithPathMatchersSelectsExactRelativePaths(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "Lib", "tomllib", "mypy.ini"), 80)
	mustWriteSizedText(t, filepath.Join(tmp, "Lib", "_pyrepl", "mypy.ini"), 900)

	candidates, err := collectCandidatesWithMatchers(tmp, nil, nil, []string{"Lib/tomllib/mypy.ini"}, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidatesWithMatchers: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one exact path candidate, got %#v", candidates)
	}
	if got := filepath.ToSlash(candidates[0].RelPath); got != "Lib/tomllib/mypy.ini" {
		t.Fatalf("unexpected candidate %q: %#v", got, candidates)
	}
}

func TestCollectCandidatesMatchesCompoundExtension(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSizedText(t, filepath.Join(tmp, "resources", "views", "welcome.blade.php"), 2048)
	mustWriteSizedText(t, filepath.Join(tmp, "resources", "views", "plain.php"), 2048)

	candidates, err := collectCandidates(tmp, []string{".blade.php"}, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		seen[filepath.ToSlash(c.RelPath)] = true
	}
	if !seen["resources/views/welcome.blade.php"] {
		t.Fatalf("expected blade candidate missing: %#v", candidates)
	}
	if seen["resources/views/plain.php"] {
		t.Fatalf("plain php should not match .blade.php: %#v", candidates)
	}
}

func TestCollectCandidatesFromRepoCacheAddsMissingMediumBucket(t *testing.T) {
	primary := t.TempDir()
	cacheRoot := t.TempDir()
	repo := filepath.Join(cacheRoot, "sample-repo")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	initGitRepo(t, repo, "https://example.com/sample-repo.git")
	mustWriteSizedText(t, filepath.Join(repo, "src", "example.ts"), 4096)
	gitRun(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=test", "add", ".")
	gitRun(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-m", "init")

	candidates, err := collectCandidatesFromRepoCache(cacheRoot, primary, []string{".ts"}, nil, nil, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidatesFromRepoCache: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatalf("expected repo-cache candidates, got none")
	}
	if got := candidates[0].SourceRoot; got != repo {
		t.Fatalf("SourceRoot = %q, want %q", got, repo)
	}
	if got := filepath.ToSlash(candidates[0].RelPath); got != "src/example.ts" {
		t.Fatalf("RelPath = %q, want %q", got, "src/example.ts")
	}
}

func TestCollectCandidatesFromPlainRepoCacheDirectoryAddsCandidates(t *testing.T) {
	primary := t.TempDir()
	cacheRoot := t.TempDir()
	repo := filepath.Join(cacheRoot, "sample-repo-deadbeef")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	mustWriteSizedText(t, filepath.Join(repo, "src", "example.ts"), 4096)

	candidates, err := collectCandidatesFromRepoCache(cacheRoot, primary, []string{".ts"}, nil, nil, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidatesFromRepoCache: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatalf("expected repo-cache candidates, got none")
	}
	meta := repoMetadataForRoot(repo, map[string]repoMetadata{})
	if got, want := meta.URL, "local:sample-repo-deadbeef"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
	if got, want := meta.Commit, "deadbeef"; got != want {
		t.Fatalf("Commit = %q, want %q", got, want)
	}
}

func TestCollectCandidatesFromRepoCacheSkipsPrimaryBasenameDuplicate(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "r-deadbeef")
	cacheRoot := t.TempDir()
	dup := filepath.Join(cacheRoot, "r-deadbeef")
	other := filepath.Join(cacheRoot, "other-feed")
	for _, dir := range []string{dup, other} {
		if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
			t.Fatalf("mkdir repo: %v", err)
		}
	}
	mustWriteSizedText(t, filepath.Join(dup, "src", "skip.R"), 4096)
	mustWriteSizedText(t, filepath.Join(other, "src", "keep.R"), 4096)

	candidates, err := collectCandidatesFromRepoCache(cacheRoot, primary, []string{".R", ".r"}, nil, nil, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidatesFromRepoCache: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %#v", len(candidates), candidates)
	}
	if got := filepath.ToSlash(candidates[0].RelPath); got != "src/keep.R" {
		t.Fatalf("RelPath = %q, want %q", got, "src/keep.R")
	}
}

func TestCollectLanguageCorpusCandidatesExternalOnlySkipsPrimaryRepo(t *testing.T) {
	primary := t.TempDir()
	cacheRoot := t.TempDir()
	external := filepath.Join(cacheRoot, "json-corpus")
	if err := os.MkdirAll(filepath.Join(primary, "src"), 0o755); err != nil {
		t.Fatalf("mkdir primary: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(external, "data"), 0o755); err != nil {
		t.Fatalf("mkdir external: %v", err)
	}
	mustWriteSizedText(t, filepath.Join(primary, "src", "grammar.json"), 4096)
	mustWriteSizedText(t, filepath.Join(external, "data", "sample.json"), 4096)

	entry := lockEntry{Name: "json", Exts: []string{".json"}}
	candidates, warnings, err := collectLanguageCorpusCandidates(
		"json",
		entry,
		primary,
		nil,
		filepath.Join(t.TempDir(), "work"),
		cacheRoot,
		defaultMaxBytes,
		defaultMediumMin,
		defaultLargeMin,
		false,
		true,
	)
	if err != nil {
		t.Fatalf("collectLanguageCorpusCandidates: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one external candidate", candidates)
	}
	if got, want := candidates[0].SourceRoot, external; got != want {
		t.Fatalf("SourceRoot = %q, want %q", got, want)
	}
	if got, want := filepath.ToSlash(candidates[0].RelPath), "data/sample.json"; got != want {
		t.Fatalf("RelPath = %q, want %q", got, want)
	}
}

func TestLoadProfileSupportsPinnedSecondarySources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	raw := profileFile{
		Name:      "top50",
		Languages: []string{"clojure", "tsx"},
		Sources: []profileSource{{
			Language: "clojure",
			RepoURL:  "https://github.com/metabase/metabase",
			Commit:   "5cd8e165fa88b34e82d2e5cafee478715b4e53a5",
			Subdir:   "src",
		}},
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	profile, err := loadProfile(path)
	if err != nil {
		t.Fatalf("loadProfile: %v", err)
	}
	if got, want := len(profile.Sources), 1; got != want {
		t.Fatalf("len(profile.Sources) = %d, want %d", got, want)
	}
	if got, want := profile.Sources[0].Language, "clojure"; got != want {
		t.Fatalf("profile.Sources[0].Language = %q, want %q", got, want)
	}
}

func TestResolveLanguageListAllUsesSortedLockEntries(t *testing.T) {
	lockEntries := map[string]lockEntry{
		"python": {Name: "python"},
		"go":     {Name: "go"},
		"rust":   {Name: "rust"},
	}
	profilePath, profile, langs, err := resolveLanguageList("", "all", "", lockEntries)
	if err != nil {
		t.Fatalf("resolveLanguageList: %v", err)
	}
	if profilePath != "" {
		t.Fatalf("profilePath = %q, want empty", profilePath)
	}
	if profile.Name != "all" {
		t.Fatalf("profile.Name = %q, want all", profile.Name)
	}
	want := []string{"go", "python", "rust"}
	if strings.Join(langs, ",") != strings.Join(want, ",") {
		t.Fatalf("langs = %#v, want %#v", langs, want)
	}
}

func TestResolveLanguageListAllRequiresLockEntries(t *testing.T) {
	if _, _, _, err := resolveLanguageList("", "all", "", nil); err == nil {
		t.Fatal("expected error for empty lock entries")
	}
}

func TestResolveLanguageListInlineStillDedupes(t *testing.T) {
	_, profile, langs, err := resolveLanguageList("", "go, python,go", "", nil)
	if err != nil {
		t.Fatalf("resolveLanguageList: %v", err)
	}
	if profile.Name != "inline" {
		t.Fatalf("profile.Name = %q, want inline", profile.Name)
	}
	want := []string{"go", "python"}
	if strings.Join(langs, ",") != strings.Join(want, ",") {
		t.Fatalf("langs = %#v, want %#v", langs, want)
	}
}

func TestResolveLanguageListFileParsesGeneratedBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "langs.txt")
	mustWriteText(t, path, strings.Join([]string{
		"# from real_corpus_inventory -select missing-corpus",
		"go",
		"python,rust",
		"  go  # duplicate",
		"",
	}, "\n"))

	profilePath, profile, langs, err := resolveLanguageList("", "top50", path, nil)
	if err != nil {
		t.Fatalf("resolveLanguageList: %v", err)
	}
	if profilePath != "" {
		t.Fatalf("profilePath = %q, want empty", profilePath)
	}
	if profile.Name != "file" {
		t.Fatalf("profile.Name = %q, want file", profile.Name)
	}
	want := []string{"go", "python", "rust"}
	if strings.Join(langs, ",") != strings.Join(want, ",") {
		t.Fatalf("langs = %#v, want %#v", langs, want)
	}
}

func TestResolveLanguageListFileRejectsAmbiguousSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "langs.txt")
	mustWriteText(t, path, "go\n")
	if _, _, _, err := resolveLanguageList("profile.json", "top50", path, nil); err == nil {
		t.Fatal("expected error for profile plus langs-file")
	}
	if _, _, _, err := resolveLanguageList("", "go", path, nil); err == nil {
		t.Fatal("expected error for explicit langs plus langs-file")
	}
}

func TestLoadLanguageListFileRequiresLanguages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "langs.txt")
	mustWriteText(t, path, "# empty\n\n")
	if _, err := loadLanguageListFile(path); err == nil {
		t.Fatal("expected error for empty language list")
	}
}

func TestMergeExistingCorpusManifestReplacesOnlySelectedLanguages(t *testing.T) {
	existing := corpusManifest{
		Languages: []string{"go", "python", "zig", "ini"},
		Entries: []corpusManifestEntry{
			{Language: "go", Bucket: "small", SourcePath: "old.go", OutputPath: "corpus/go/old.go"},
			{Language: "python", Bucket: "small", SourcePath: "keep.py", OutputPath: "corpus/python/keep.py"},
			{Language: "zig", Bucket: "small", SourcePath: "stale.zig", OutputPath: "corpus/zig/stale.zig"},
		},
		Missing: []string{"zig", "ini"},
	}
	current := corpusManifest{
		Languages: []string{"go", "zig"},
		Entries: []corpusManifestEntry{
			{Language: "go", Bucket: "large", SourcePath: "new.go", OutputPath: "corpus/go/new.go"},
		},
		Missing: []string{"zig"},
	}

	merged := mergeExistingCorpusManifest(existing, current, []string{"go", "zig"})
	if got, want := strings.Join(merged.Languages, ","), "go,ini,python,zig"; got != want {
		t.Fatalf("Languages = %q, want %q", got, want)
	}
	if got, want := strings.Join(merged.Missing, ","), "ini,zig"; got != want {
		t.Fatalf("Missing = %q, want %q", got, want)
	}
	if len(merged.Entries) != 2 {
		t.Fatalf("Entries = %#v, want 2 entries", merged.Entries)
	}
	gotByLang := map[string]string{}
	for _, entry := range merged.Entries {
		gotByLang[entry.Language] = entry.SourcePath
	}
	if gotByLang["python"] != "keep.py" {
		t.Fatalf("python entry not preserved: %#v", merged.Entries)
	}
	if gotByLang["go"] != "new.go" {
		t.Fatalf("go entry not replaced: %#v", merged.Entries)
	}
	if _, ok := gotByLang["zig"]; ok {
		t.Fatalf("zig stale entry should have been removed: %#v", merged.Entries)
	}
}

func TestLoadExistingCorpusManifestMissingFileIsOptional(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	_, ok, err := loadExistingCorpusManifest(path)
	if err != nil {
		t.Fatalf("loadExistingCorpusManifest: %v", err)
	}
	if ok {
		t.Fatal("missing manifest should report ok=false")
	}
}

func TestCollectCandidatesFromProfileSourcesUsesPinnedRepoAndSubdir(t *testing.T) {
	cacheRoot := t.TempDir()
	sourceRepo := t.TempDir()
	initGitRepo(t, sourceRepo, "https://example.com/curated/clojure.git")
	mustWriteSizedText(t, filepath.Join(sourceRepo, "pkg", "examples", "backfill.clj"), 4096)
	gitRun(t, sourceRepo, "-c", "user.email=test@example.com", "-c", "user.name=test", "add", ".")
	gitRun(t, sourceRepo, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-m", "init")
	commit := strings.TrimSpace(gitOutputTest(t, sourceRepo, "rev-parse", "HEAD"))

	candidates, err := collectCandidatesFromProfileSources([]profileSource{{
		Language: "clojure",
		RepoURL:  sourceRepo,
		Commit:   commit,
		Subdir:   "pkg",
	}}, cacheRoot, []string{".clj"}, nil, nil, defaultMaxBytes, false)
	if err != nil {
		t.Fatalf("collectCandidatesFromProfileSources: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatalf("expected curated source candidates, got none")
	}
	if got, want := filepath.ToSlash(candidates[0].RelPath), "pkg/examples/backfill.clj"; got != want {
		t.Fatalf("RelPath = %q, want %q", got, want)
	}
}

func TestRepoMetadataForRootReadsGitRemoteAndCommit(t *testing.T) {
	repo := t.TempDir()
	initGitRepo(t, repo, "https://example.com/real/repo.git")
	mustWriteSizedText(t, filepath.Join(repo, "sample.js"), 512)
	gitRun(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=test", "add", ".")
	gitRun(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-m", "init")

	meta := repoMetadataForRoot(repo, map[string]repoMetadata{})
	if got, want := meta.URL, "https://example.com/real/repo.git"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
	if len(strings.TrimSpace(meta.Commit)) < 7 {
		t.Fatalf("Commit = %q, want non-empty git hash", meta.Commit)
	}
}

func TestNeedsRepoCacheFallbackRequiresMissingMediumOrLarge(t *testing.T) {
	if !needsRepoCacheFallback([]corpusFile{{Size: 300}}, 2000, 16000) {
		t.Fatalf("small-only candidates should need fallback")
	}
	if !needsRepoCacheFallback([]corpusFile{{Size: 3000}}, 2000, 16000) {
		t.Fatalf("medium-only candidates should need fallback")
	}
	if needsRepoCacheFallback([]corpusFile{{Size: 3000}, {Size: 20000}}, 2000, 16000) {
		t.Fatalf("medium+large candidates should not need fallback")
	}
}

func TestSplitTreeSitterCorpusSources(t *testing.T) {
	content := []byte(`================================================================================
First case
================================================================================

class A {}

--------------------------------------------------------------------------------

(compilation_unit)

================================================================================
Second case
================================================================================

class B {}

--------------------------------------------------------------------------------

(compilation_unit)
`)

	cases, ok := splitTreeSitterCorpusSources(content)
	if !ok {
		t.Fatal("expected tree-sitter corpus fixture to split")
	}
	if len(cases) != 2 {
		t.Fatalf("len(cases) = %d, want 2", len(cases))
	}
	if got, want := cases[0].Title, "First case"; got != want {
		t.Fatalf("cases[0].Title = %q, want %q", got, want)
	}
	if got, want := string(cases[0].Source), "class A {}\n\n"; got != want {
		t.Fatalf("cases[0].Source = %q, want %q", got, want)
	}
	if got, want := cases[1].Title, "Second case"; got != want {
		t.Fatalf("cases[1].Title = %q, want %q", got, want)
	}
	if got, want := string(cases[1].Source), "class B {}\n\n"; got != want {
		t.Fatalf("cases[1].Source = %q, want %q", got, want)
	}
}

func initGitRepo(t *testing.T, repo, remote string) {
	t.Helper()
	gitRun(t, repo, "init")
	gitRun(t, repo, "remote", "add", "origin", remote)
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func gitOutputTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return string(out)
}

func TestSplitTreeSitterCorpusSourcesRejectsPlainFixtureText(t *testing.T) {
	if cases, ok := splitTreeSitterCorpusSources([]byte("aaaa\nbbbb\n")); ok || len(cases) != 0 {
		t.Fatalf("plain fixture text must not be treated as tree-sitter corpus: ok=%v cases=%d", ok, len(cases))
	}
}

func TestRetryableGitCheckoutError(t *testing.T) {
	tests := map[string]bool{
		"fatal: unable to access 'https://github.com/x/y/': Could not resolve host: github.com": true,
		"fatal: unable to access 'https://github.com/x/y/': TLS handshake timeout":              true,
		"fatal: unable to access 'https://github.com/x/y/': Connection reset by peer":           true,
		"fatal: repository not found": false,
	}

	for msg, want := range tests {
		if got := retryableGitCheckoutError(testError(msg)); got != want {
			t.Fatalf("retryableGitCheckoutError(%q) = %v, want %v", msg, got, want)
		}
	}
}

func mustWriteSizedText(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = 'a'
	}
	buf[size-1] = '\n'
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustWriteText(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type testError string

func (e testError) Error() string { return string(e) }

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
