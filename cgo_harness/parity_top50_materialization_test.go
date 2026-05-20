//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestParityTop50ParseMaterializationTrends(t *testing.T) {
	parityRequireTop50(t, "TestParityTop50ParseMaterializationTrends")

	gotreesitter.EnableRuntimeAudit(true)
	t.Cleanup(func() {
		gotreesitter.EnableRuntimeAudit(false)
	})

	for _, name := range top50ParityLanguages {
		if parityLanguageExcluded(name) {
			continue
		}
		report, ok := paritySupportByName[name]
		if !ok || report.Backend == grammars.ParseBackendUnsupported {
			continue
		}
		if !hasDedicatedSample[name] {
			continue
		}

		tc := parityCase{name: name, source: grammars.ParseSmokeSample(name)}
		t.Run(name, func(t *testing.T) {
			scheduleParityMemoryScavenge(t)

			src := normalizedSource(tc.name, tc.source)
			tree, lang, err := parseWithGo(tc, src, nil)
			if err != nil {
				t.Fatalf("gotreesitter parse error: %v", err)
			}
			defer releaseGoTree(tree)

			root := tree.RootNode()
			rt := tree.ParseRuntime()
			divergences, firstDivergence := top50StructuralDivergenceSummary(t, tc, src, tree, lang)

			t.Logf("TOP50_PARSE_MATERIALIZATION language=%s backend=%s bytes=%d root=%q has_error=%v structural_divergences=%d first_divergence=%q stop=%s tokens=%d nodes_alloc=%d final_nodes=%d final_parents=%d final_leaves=%d max_stacks=%d arena_bytes=%d scratch_bytes=%d gss_bytes=%d parent_alloc=%d parent_retained=%d leaf_alloc=%d leaf_retained=%d compact_leaf_created=%d compact_leaf_materialized=%d compact_leaf_final=%d pending_parent_created=%d pending_parent_materialized=%d pending_parent_final=%d transient_parent_alloc=%d transient_parent_materialized=%d transient_child_slices_alloc=%d transient_child_slices_materialized=%d transient_child_ptrs_alloc=%d transient_child_ptrs_materialized=%d normalization_checked=%d normalization_run=%d normalization_rewritten=%d summary=%q",
				tc.name,
				report.Backend,
				len(src),
				root.Type(lang),
				root.HasError(),
				divergences,
				firstDivergence,
				rt.StopReason,
				rt.TokensConsumed,
				rt.NodesAllocated,
				rt.FinalNodes,
				rt.FinalParentNodes,
				rt.FinalLeafNodes,
				rt.MaxStacksSeen,
				rt.ArenaBytesAllocated,
				rt.ScratchBytesAllocated,
				rt.GSSBytesAllocated,
				rt.ParentNodesAllocated,
				rt.ParentNodesRetained,
				rt.LeafNodesAllocated,
				rt.LeafNodesRetained,
				rt.CompactFullLeafCreated,
				rt.CompactFullLeafMaterialized,
				rt.CompactFullLeafMaterializedForFinalTree,
				rt.PendingParentCreated,
				rt.PendingParentMaterialized,
				rt.PendingParentMaterializedForFinalTree,
				rt.TransientParentNodesAllocated,
				rt.TransientParentNodesMaterialized,
				rt.TransientChildSlicesAllocated,
				rt.TransientChildSlicesMaterialized,
				rt.TransientChildPointersAllocated,
				rt.TransientChildPointersMaterialized,
				rt.NormalizationPassesChecked,
				rt.NormalizationPassesRun,
				rt.NormalizationNodesRewritten,
				rt.Summary(),
			)
		})
	}
}

func top50StructuralDivergenceSummary(t *testing.T, tc parityCase, src []byte, goTree *gotreesitter.Tree, goLang *gotreesitter.Language) (int, string) {
	t.Helper()

	cLang, err := ParityCLanguage(tc.name)
	if err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			return 0, "skip C reference: " + skipReason
		}
		return 1, "load C parser: " + err.Error()
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			return 0, "skip C SetLanguage: " + skipReason
		}
		return 1, "C SetLanguage: " + err.Error()
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		return 1, "C parser returned nil tree"
	}
	defer cTree.Close()

	var errs []string
	compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
	if len(errs) == 0 {
		return 0, ""
	}
	return len(errs), strings.TrimSpace(errs[0])
}
