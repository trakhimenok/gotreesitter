package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectLanguagesFromLock(t *testing.T) {
	entries := map[string]lockEntry{
		"go":     {Name: "go", RepoURL: "https://example.invalid/go", Commit: "abc"},
		"python": {Name: "python", RepoURL: "https://example.invalid/python", Commit: "def"},
	}
	got, err := selectLanguages(entries, "python,go,go", "")
	if err != nil {
		t.Fatalf("selectLanguages: %v", err)
	}
	if strings.Join(got, ",") != "go,python" {
		t.Fatalf("languages=%#v", got)
	}
	if _, err := selectLanguages(entries, "ruby", ""); err == nil {
		t.Fatal("expected unknown language error")
	}
}

func TestSelectLanguagesFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "langs.txt")
	if err := os.WriteFile(path, []byte("python\n# comment\ngo,rust\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := map[string]lockEntry{
		"go":     {Name: "go", RepoURL: "https://example.invalid/go", Commit: "abc"},
		"python": {Name: "python", RepoURL: "https://example.invalid/python", Commit: "def"},
		"rust":   {Name: "rust", RepoURL: "https://example.invalid/rust", Commit: "123"},
	}
	got, err := selectLanguages(entries, "", path)
	if err != nil {
		t.Fatalf("selectLanguages: %v", err)
	}
	if strings.Join(got, ",") != "go,python,rust" {
		t.Fatalf("languages=%#v", got)
	}
}

func TestBuildPlanProducesSourceCheckoutCommands(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "corpus_sources")
	entries := map[string]lockEntry{
		"go": {Name: "go", RepoURL: "https://example.invalid/go", Commit: "abc123"},
	}
	plan := buildPlan("languages.lock", root, entries, []string{"go"}, false, false)
	if len(plan.Languages) != 1 {
		t.Fatalf("languages=%d", len(plan.Languages))
	}
	status := plan.Languages[0]
	if !status.NeedsClone || !strings.Contains(status.Command, "git clone --filter=blob:none --no-checkout") {
		t.Fatalf("status=%+v", status)
	}
	if !strings.Contains(status.Command, "abc123") || !strings.Contains(status.Command, "https://example.invalid/go") {
		t.Fatalf("command missing source details: %s", status.Command)
	}
	if strings.Contains(status.Command, "submodule update --init --recursive") {
		t.Fatalf("default command should not initialize submodules: %s", status.Command)
	}

	recursivePlan := buildPlan("languages.lock", root, entries, []string{"go"}, false, true)
	if !strings.Contains(recursivePlan.Languages[0].Command, "submodule update --init --recursive") {
		t.Fatalf("explicit recursive command missing submodule update: %s", recursivePlan.Languages[0].Command)
	}
}

func TestCommandForUpdatesWrongRemoteBeforeFetch(t *testing.T) {
	status := sourceStatus{
		RepoURL:           "https://example.invalid/new-corpus.git",
		Commit:            "abc123",
		Path:              filepath.Join("corpus_sources", "go"),
		Exists:            true,
		GitDir:            true,
		Head:              "old123",
		RemoteURL:         "https://example.invalid/old-grammar",
		NeedsRemoteUpdate: true,
		NeedsFetch:        true,
		NeedsCheckout:     true,
	}
	command := commandFor(status, false)
	if !strings.Contains(command, "remote set-url origin 'https://example.invalid/new-corpus.git'") {
		t.Fatalf("command missing remote update: %s", command)
	}
	if !strings.Contains(command, "fetch --depth=1 origin 'abc123'") || !strings.Contains(command, "checkout --detach FETCH_HEAD") {
		t.Fatalf("command missing fetch/checkout: %s", command)
	}
	if !sameGitRemote("https://example.invalid/new-corpus.git", "https://example.invalid/new-corpus") {
		t.Fatal("sameGitRemote should ignore .git suffix")
	}
}

func TestWriteScriptOmitsReadyLanguages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "add.sh")
	statuses := []sourceStatus{
		{Language: "go", Command: "git clone --filter=blob:none --no-checkout https://example.invalid/go corpus_sources/go"},
		{Language: "python"},
	}
	if err := writeScript(path, statuses); err != nil {
		t.Fatalf("writeScript: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "git clone --filter=blob:none --no-checkout https://example.invalid/go corpus_sources/go") {
		t.Fatalf("script missing command:\n%s", text)
	}
	if strings.Contains(text, "python") {
		t.Fatalf("script should omit ready languages:\n%s", text)
	}
}
