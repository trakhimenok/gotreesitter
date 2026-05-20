package gotreesitter

import "testing"

func TestTransientReduceLanguageDefaultsToDisabled(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS_LANGS", "")

	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "python"}) {
		t.Fatal("python transient reduce children enabled by default")
	}
	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "java"}) {
		t.Fatal("java transient reduce children enabled by default")
	}
}

func TestTransientReduceLanguageAllowlist(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "java, typescript")
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS_LANGS", "")

	if !parseTransientReduceChildrenLanguageEnabled(&Language{Name: "java"}) {
		t.Fatal("java transient reduce children disabled by allowlist")
	}
	if !parseTransientReduceParentsLanguageEnabled(&Language{Name: "typescript"}) {
		t.Fatal("typescript transient reduce parents disabled by allowlist")
	}
	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "python"}) {
		t.Fatal("python transient reduce children enabled outside allowlist")
	}
}

func TestTransientReduceLanguageSpecificOverride(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_LANGS", "all")
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN_LANGS", "kotlin")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS_LANGS", "none")

	if !parseTransientReduceChildrenLanguageEnabled(&Language{Name: "kotlin"}) {
		t.Fatal("kotlin transient reduce children disabled by specific allowlist")
	}
	if parseTransientReduceChildrenLanguageEnabled(&Language{Name: "java"}) {
		t.Fatal("java transient reduce children enabled despite specific override")
	}
	if parseTransientReduceParentsLanguageEnabled(&Language{Name: "kotlin"}) {
		t.Fatal("kotlin transient reduce parents enabled despite specific none override")
	}
}

func TestTransientReduceLegacyDisable(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "")
	t.Setenv("GOT_PYTHON_TRANSIENT_REDUCE_CHILDREN", "false")

	if parseTransientReduceChildrenEnabled() {
		t.Fatal("legacy transient reduce disable ignored")
	}
	if parseTransientReduceParentsEnabled() {
		t.Fatal("legacy transient reduce parent disable ignored")
	}
}

func TestTransientReducePathDisable(t *testing.T) {
	t.Setenv("GOT_TRANSIENT_REDUCE_CHILDREN", "0")
	t.Setenv("GOT_TRANSIENT_REDUCE_PARENTS", "false")
	t.Setenv("GOT_PYTHON_TRANSIENT_REDUCE_CHILDREN", "")

	if parseTransientReduceChildrenEnabled() {
		t.Fatal("transient reduce children disable ignored")
	}
	if parseTransientReduceParentsEnabled() {
		t.Fatal("transient reduce parent disable ignored")
	}
}

func TestTransientReduceScratchNoAliasLargeOnly(t *testing.T) {
	if parseShouldUseTransientReduceScratchNoAlias(50 * 1024) {
		t.Fatal("scratch no-alias transient reduce enabled for 50KB input")
	}
	if !parseShouldUseTransientReduceScratchNoAlias(256 * 1024) {
		t.Fatal("scratch no-alias transient reduce disabled at large-file threshold")
	}
}
