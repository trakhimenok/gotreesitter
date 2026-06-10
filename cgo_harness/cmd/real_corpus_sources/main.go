package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type lockEntry struct {
	Name    string   `json:"name"`
	RepoURL string   `json:"repo_url"`
	Commit  string   `json:"commit"`
	Subdir  string   `json:"subdir,omitempty"`
	Exts    []string `json:"extensions,omitempty"`
}

type sourcePlan struct {
	GeneratedFrom string         `json:"generated_from"`
	Root          string         `json:"root"`
	Apply         bool           `json:"apply"`
	Recursive     bool           `json:"recursive"`
	Languages     []sourceStatus `json:"languages"`
}

type sourceStatus struct {
	Language          string `json:"language"`
	RepoURL           string `json:"repo_url"`
	Commit            string `json:"commit"`
	Path              string `json:"path"`
	Exists            bool   `json:"exists"`
	GitDir            bool   `json:"git_dir"`
	Head              string `json:"head,omitempty"`
	RemoteURL         string `json:"remote_url,omitempty"`
	NeedsClone        bool   `json:"needs_clone"`
	NeedsRemoteUpdate bool   `json:"needs_remote_update"`
	NeedsFetch        bool   `json:"needs_fetch"`
	NeedsCheckout     bool   `json:"needs_checkout"`
	Command           string `json:"command,omitempty"`
	Error             string `json:"error,omitempty"`
}

func main() {
	var (
		lockPath    string
		root        string
		langsRaw    string
		langsFile   string
		outJSON     string
		outScript   string
		printScript bool
		apply       bool
		recursive   bool
	)
	flag.StringVar(&lockPath, "lock", "", "path to grammars/languages.lock")
	flag.StringVar(&root, "root", "", "external corpus source checkout root; default is ../gotreesitter-corpora/corpus_sources")
	flag.StringVar(&langsRaw, "langs", "all", "comma-separated languages, or all/all206/lock")
	flag.StringVar(&langsFile, "langs-file", "", "optional newline- or comma-separated language list")
	flag.StringVar(&outJSON, "out-json", "", "optional JSON plan/status path; stdout when no output option is set")
	flag.StringVar(&outScript, "out-script", "", "optional shell script path containing git checkout commands")
	flag.BoolVar(&printScript, "print-script", false, "print shell commands to stdout instead of JSON")
	flag.BoolVar(&apply, "apply", false, "run git clone/fetch/checkout operations")
	flag.BoolVar(&recursive, "recursive", false, "initialize nested submodules inside corpus repositories after checkout")
	flag.Parse()

	repoRoot, err := resolveRepoRoot()
	if err != nil {
		fatalf("resolve repo root: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		fatalf("cd repo root: %v", err)
	}

	resolvedLock, err := resolveLockPath(lockPath)
	if err != nil {
		fatalf("resolve lock: %v", err)
	}
	resolvedRoot := resolveRoot(root, repoRoot)
	entries, err := parseLockFile(resolvedLock)
	if err != nil {
		fatalf("parse lock: %v", err)
	}
	names, err := selectLanguages(entries, langsRaw, langsFile)
	if err != nil {
		fatalf("select languages: %v", err)
	}
	plan := buildPlan(resolvedLock, resolvedRoot, entries, names, apply, recursive)
	if apply {
		applyPlan(&plan)
	}
	if outScript != "" {
		if err := writeScript(outScript, plan.Languages); err != nil {
			fatalf("write script: %v", err)
		}
	}
	if printScript {
		if err := writeScript("-", plan.Languages); err != nil {
			fatalf("write script: %v", err)
		}
	}
	if outJSON != "" {
		if err := writeJSON(outJSON, plan); err != nil {
			fatalf("write json: %v", err)
		}
	}
	if outJSON == "" && outScript == "" && !printScript {
		if err := writeJSON("-", plan); err != nil {
			fatalf("write json: %v", err)
		}
	}
}

func resolveLockPath(raw string) (string, error) {
	if strings.TrimSpace(raw) != "" {
		if _, err := os.Stat(raw); err != nil {
			return "", err
		}
		return raw, nil
	}
	for _, candidate := range []string{
		filepath.Join("grammars", "languages.lock"),
		filepath.Join("..", "grammars", "languages.lock"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("languages.lock not found")
}

func resolveRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("empty git root")
	}
	return root, nil
}

func resolveRoot(raw string, repoRoot string) string {
	if strings.TrimSpace(raw) != "" {
		if filepath.IsAbs(raw) {
			return filepath.Clean(raw)
		}
		return filepath.Clean(filepath.Join(repoRoot, raw))
	}
	return filepath.Clean(filepath.Join(repoRoot, "..", "gotreesitter-corpora", "corpus_sources"))
}

func parseLockFile(path string) (map[string]lockEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]lockEntry{}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("%s:%d: expected name repo commit", path, lineNo)
		}
		entry := lockEntry{Name: fields[0], RepoURL: fields[1], Commit: fields[2]}
		if len(fields) >= 4 {
			entry.Subdir = fields[3]
		}
		if len(fields) >= 5 {
			for _, ext := range strings.Split(fields[4], ",") {
				ext = strings.TrimSpace(ext)
				if ext != "" {
					entry.Exts = append(entry.Exts, ext)
				}
			}
		}
		out[entry.Name] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func selectLanguages(entries map[string]lockEntry, raw string, langsFile string) ([]string, error) {
	var names []string
	if strings.TrimSpace(langsFile) != "" {
		data, err := os.ReadFile(langsFile)
		if err != nil {
			return nil, err
		}
		names = parseLanguageList(string(data))
	} else {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.EqualFold(raw, "all") || strings.EqualFold(raw, "all206") || strings.EqualFold(raw, "lock") || strings.EqualFold(raw, "locked") {
			for name := range entries {
				names = append(names, name)
			}
		} else {
			names = parseLanguageList(raw)
		}
	}
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			continue
		}
		if _, ok := entries[name]; !ok {
			return nil, fmt.Errorf("unknown language %q", name)
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func parseLanguageList(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", "\n")
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func buildPlan(lockPath, root string, entries map[string]lockEntry, names []string, apply bool, recursive bool) sourcePlan {
	plan := sourcePlan{
		GeneratedFrom: lockPath,
		Root:          root,
		Apply:         apply,
		Recursive:     recursive,
		Languages:     make([]sourceStatus, 0, len(names)),
	}
	for _, name := range names {
		entry := entries[name]
		path := filepath.Clean(filepath.Join(root, name))
		status := inspectSource(name, entry, path)
		status.Command = commandFor(status, recursive)
		plan.Languages = append(plan.Languages, status)
	}
	return plan
}

func inspectSource(name string, entry lockEntry, path string) sourceStatus {
	status := sourceStatus{
		Language: name,
		RepoURL:  entry.RepoURL,
		Commit:   entry.Commit,
		Path:     path,
	}
	if st, err := os.Stat(path); err == nil {
		status.Exists = true
		status.GitDir = st.IsDir() && isGitWorktree(path)
		status.Head = gitOutput(path, "rev-parse", "HEAD")
		status.RemoteURL = gitOutput(path, "remote", "get-url", "origin")
	}
	status.NeedsClone = !status.Exists || !status.GitDir
	status.NeedsRemoteUpdate = status.GitDir && strings.TrimSpace(entry.RepoURL) != "" && !sameGitRemote(status.RemoteURL, entry.RepoURL)
	status.NeedsFetch = status.GitDir && (status.NeedsRemoteUpdate || status.Head != entry.Commit)
	status.NeedsCheckout = status.Head != "" && status.Head != entry.Commit
	return status
}

func sameGitRemote(actual, expected string) bool {
	actual = strings.TrimSuffix(strings.TrimSpace(actual), ".git")
	expected = strings.TrimSuffix(strings.TrimSpace(expected), ".git")
	return actual != "" && actual == expected
}

func isGitWorktree(path string) bool {
	return gitOutput(path, "rev-parse", "--is-inside-work-tree") == "true"
}

func gitOutput(path string, args ...string) string {
	cmdArgs := append([]string{"-C", path}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func commandFor(status sourceStatus, recursive bool) string {
	quotedPath := shellQuote(status.Path)
	quotedRepo := shellQuote(status.RepoURL)
	quotedCommit := shellQuote(status.Commit)
	recursiveStep := ""
	if recursive {
		recursiveStep = fmt.Sprintf(" && git -C %s submodule update --init --recursive", quotedPath)
	}
	remoteStep := ""
	if status.NeedsRemoteUpdate {
		remoteStep = fmt.Sprintf("git -C %s remote set-url origin %s && ", quotedPath, quotedRepo)
	}
	if status.NeedsClone {
		return fmt.Sprintf("mkdir -p %s && git clone --filter=blob:none --no-checkout %s %s && git -C %s fetch --depth=1 origin %s && git -C %s checkout --detach FETCH_HEAD%s",
			shellQuote(filepath.Dir(status.Path)), quotedRepo, quotedPath, quotedPath, quotedCommit, quotedPath, recursiveStep)
	}
	if status.NeedsFetch {
		return fmt.Sprintf("%sgit -C %s fetch --depth=1 origin %s && git -C %s checkout --detach FETCH_HEAD%s",
			remoteStep, quotedPath, quotedCommit, quotedPath, recursiveStep)
	}
	return ""
}

func applyPlan(plan *sourcePlan) {
	if err := os.MkdirAll(plan.Root, 0o755); err != nil {
		fatalf("mkdir %s: %v", plan.Root, err)
	}
	for i := range plan.Languages {
		status := &plan.Languages[i]
		if status.Command == "" {
			continue
		}
		if status.NeedsClone {
			if status.Exists && !status.GitDir {
				status.Error = fmt.Sprintf("path exists but is not a git checkout: %s", status.Path)
				continue
			}
			if err := runGit("clone", "--filter=blob:none", "--no-checkout", status.RepoURL, status.Path); err != nil {
				status.Error = err.Error()
				continue
			}
		}
		if status.NeedsRemoteUpdate {
			if err := runGit("-C", status.Path, "remote", "set-url", "origin", status.RepoURL); err != nil {
				status.Error = err.Error()
				continue
			}
		}
		if err := runGit("-C", status.Path, "fetch", "--depth=1", "origin", status.Commit); err != nil {
			status.Error = err.Error()
			continue
		}
		if err := runGit("-C", status.Path, "checkout", "--detach", "FETCH_HEAD"); err != nil {
			status.Error = err.Error()
			continue
		}
		if plan.Recursive {
			if err := runGit("-C", status.Path, "submodule", "update", "--init", "--recursive"); err != nil {
				status.Error = err.Error()
				continue
			}
		}
		updated := inspectSource(status.Language, lockEntry{RepoURL: status.RepoURL, Commit: status.Commit}, status.Path)
		updated.Command = commandFor(updated, plan.Recursive)
		*status = updated
	}
}

func runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
}

func writeJSON(path string, plan sourcePlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeScript(path string, statuses []sourceStatus) error {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")
	for _, status := range statuses {
		if status.Command == "" {
			continue
		}
		fmt.Fprintf(&b, "%s\n", status.Command)
	}
	data := []byte(b.String())
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o755)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "real_corpus_sources: "+format+"\n", args...)
	os.Exit(1)
}
