//go:build cgo && treesitter_c_bench

package cgoharness

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitterpython "github.com/smacker/go-tree-sitter/python"
)

const defaultPythonCorpusPath = "corpus_real/python/large__python3.8_grammar.py"

type pythonCorpusFile struct {
	path   string
	source []byte
}

type pythonCorpusParseMode string

const (
	pythonCorpusParseModeDFA                      pythonCorpusParseMode = "dfa"
	pythonCorpusParseModeDFANoCompat              pythonCorpusParseMode = "dfa_no_compat"
	pythonCorpusParseModeDFANoTree                pythonCorpusParseMode = "dfa_no_tree"
	pythonCorpusParseModeDFANoTreeWithCheckpoints pythonCorpusParseMode = "dfa_no_tree_with_checkpoints"
)

type pythonReduceChildPathStats struct {
	slicesAllocated   uint64
	slicesRetained    uint64
	slicesDropped     uint64
	pointersAllocated uint64
	pointersRetained  uint64
	pointersDropped   uint64
}

func (s *pythonReduceChildPathStats) add(rt gotreesitter.ReduceChildPathRuntime) {
	s.slicesAllocated += rt.SlicesAllocated
	s.slicesRetained += rt.SlicesRetained
	s.slicesDropped += rt.SlicesDropped
	s.pointersAllocated += rt.PointersAllocated
	s.pointersRetained += rt.PointersRetained
	s.pointersDropped += rt.PointersDropped
}

func (s pythonReduceChildPathStats) report(b *testing.B, tokens float64, name string) {
	b.ReportMetric(float64(s.slicesAllocated)/tokens, "reduce_child_slices_"+name+"_alloc/token")
	b.ReportMetric(float64(s.slicesRetained)/tokens, "reduce_child_slices_"+name+"_retained/token")
	b.ReportMetric(float64(s.slicesDropped)/tokens, "reduce_child_slices_"+name+"_dropped/token")
	b.ReportMetric(float64(s.pointersAllocated)/tokens, "reduce_child_ptrs_"+name+"_alloc/token")
	b.ReportMetric(float64(s.pointersRetained)/tokens, "reduce_child_ptrs_"+name+"_retained/token")
	b.ReportMetric(float64(s.pointersDropped)/tokens, "reduce_child_ptrs_"+name+"_dropped/token")
}

type pythonRuntimeBenchStats struct {
	ops                               int
	arenaBreakdownSamples             int
	tokensConsumed                    uint64
	iterations                        uint64
	nodesAllocated                    uint64
	parentNodesAllocated              uint64
	parentNodesRetained               uint64
	parentNodesDropped                uint64
	leafNodesAllocated                uint64
	leafNodesRetained                 uint64
	leafNodesDropped                  uint64
	childSlicesAllocated              uint64
	childSlicesRetained               uint64
	childSlicesDropped                uint64
	childPointersAllocated            uint64
	childPointersRetained             uint64
	childPointersDropped              uint64
	reduceChildFastGSS                pythonReduceChildPathStats
	reduceChildAllVisible             pythonReduceChildPathStats
	reduceChildNoAlias                pythonReduceChildPathStats
	reduceChildScratchGeneral         pythonReduceChildPathStats
	reduceChildScratchNoAlias         pythonReduceChildPathStats
	finalNodes                        uint64
	finalParentNodes                  uint64
	finalLeafNodes                    uint64
	finalFieldedParentNodes           uint64
	finalUnfieldedParentNodes         uint64
	finalChildSlices                  uint64
	finalChildPointers                uint64
	finalFieldIDElements              uint64
	finalFieldSourceElements          uint64
	gssNodesAllocated                 uint64
	gssNodesRetained                  uint64
	gssNodesDropped                   uint64
	singleStackGSSNodes               uint64
	multiStackGSSNodes                uint64
	arenaBytesAllocated               int64
	arenaNodeStructBytesAllocated     int64
	arenaNoTreeNodeBytesAllocated     int64
	arenaChildSliceBytesAllocated     int64
	arenaFieldIDBytesAllocated        int64
	arenaFieldSourceBytesAllocated    int64
	scratchBytesAllocated             int64
	entryScratchBytesAllocated        int64
	gssBytesAllocated                 int64
	mergeScratchBytesAllocated        int64
	externalCheckpointRecords         uint64
	externalCheckpointSlots           uint64
	externalCheckpointBytes           int64
	externalCheckpointSnapshotBytes   uint64
	arenaNodesConstructed             uint64
	nodeLiveCount                     uint64
	nodeCapacityCount                 uint64
	nodeCapacityWaste                 uint64
	primaryNodeCapacity               uint64
	primaryNodeUsed                   uint64
	overflowNodeCapacity              uint64
	overflowNodeUsed                  uint64
	overflowNodeSlabs                 uint64
	largestNodeSlabUsedFractionSum    float64
	leafNodesConstructed              uint64
	parentNodesConstructed            uint64
	fieldedParentNodesConstructed     uint64
	unfieldedParentNodesConstructed   uint64
	parentConstructedChildLen0        uint64
	parentConstructedChildLen1        uint64
	parentConstructedChildLen2        uint64
	parentConstructedChildLen3        uint64
	parentConstructedChildLen4Plus    uint64
	parentConstructedNoLinks          uint64
	parentConstructedWithLinks        uint64
	parentConstructedTrackErrors      uint64
	parentConstructedFieldSources     uint64
	parentReductionVisible            uint64
	parentReductionInvisible          uint64
	parentReductionVisibleFielded     uint64
	parentReductionVisibleUnfielded   uint64
	parentReductionInvisibleFielded   uint64
	parentReductionInvisibleUnfielded uint64
	parentReductionVisibleChildPtrs   uint64
	parentReductionInvisibleChildPtrs uint64
	parentReductionVisibleLen0        uint64
	parentReductionVisibleLen1        uint64
	parentReductionVisibleLen2        uint64
	parentReductionVisibleLen3        uint64
	parentReductionVisibleLen4Plus    uint64
	parentReductionInvisibleLen0      uint64
	parentReductionInvisibleLen1      uint64
	parentReductionInvisibleLen2      uint64
	parentReductionInvisibleLen3      uint64
	parentReductionInvisibleLen4Plus  uint64
	reduceChildSlicesFastGSS          uint64
	reduceChildPointersFastGSS        uint64
	reduceChildSlicesAllVisible       uint64
	reduceChildPointersAllVisible     uint64
	reduceChildSlicesNoAlias          uint64
	reduceChildPointersNoAlias        uint64
	reduceChildSlicesScratchGeneral   uint64
	reduceChildPointersScratchGeneral uint64
	reduceChildSlicesScratchNoAlias   uint64
	reduceChildPointersScratchNoAlias uint64
	noTreeReduceNodesConstructed      uint64
	noTreeLeafNodesConstructed        uint64
	noTreePlaceholderNodesConstructed uint64
	otherNodesConstructed             uint64
	extraNodesConstructed             uint64
	errorSymbolNodesConstructed       uint64
	hasErrorNodesConstructed          uint64
	childSlicesConstructed            uint64
	childPointersConstructed          uint64
	childSlicesLen1                   uint64
	childSlicesLen2                   uint64
	childSlicesLen3                   uint64
	childSlicesLen4Plus               uint64
	parentChildPointersConstructed    uint64
	parentChildrenLen0                uint64
	parentChildrenLen1                uint64
	parentChildrenLen2                uint64
	parentChildrenLen3                uint64
	parentChildrenLen4Plus            uint64
	fieldIDElementsConstructed        uint64
	fieldSourceElementsConstructed    uint64
	normalizationPassesChecked        uint64
	normalizationPassesRun            uint64
	normalizationNodesVisited         uint64
	normalizationNodesRewritten       uint64
	normalizationNanos                int64
	maxStacksSeen                     int
}

func (s *pythonRuntimeBenchStats) add(rt gotreesitter.ParseRuntime, breakdown gotreesitter.ArenaBreakdown, hasBreakdown bool) {
	s.ops++
	s.tokensConsumed += rt.TokensConsumed
	s.iterations += uint64(rt.Iterations)
	s.nodesAllocated += uint64(rt.NodesAllocated)
	s.parentNodesAllocated += rt.ParentNodesAllocated
	s.parentNodesRetained += rt.ParentNodesRetained
	s.parentNodesDropped += rt.ParentNodesDroppedSameToken
	s.leafNodesAllocated += rt.LeafNodesAllocated
	s.leafNodesRetained += rt.LeafNodesRetained
	s.leafNodesDropped += rt.LeafNodesDroppedSameToken
	s.childSlicesAllocated += rt.ChildSlicesAllocated
	s.childSlicesRetained += rt.ChildSlicesRetained
	s.childSlicesDropped += rt.ChildSlicesDroppedSameToken
	s.childPointersAllocated += rt.ChildPointersAllocated
	s.childPointersRetained += rt.ChildPointersRetained
	s.childPointersDropped += rt.ChildPointersDroppedSameToken
	s.reduceChildFastGSS.add(rt.ReduceChildFastGSS)
	s.reduceChildAllVisible.add(rt.ReduceChildAllVisible)
	s.reduceChildNoAlias.add(rt.ReduceChildNoAlias)
	s.reduceChildScratchGeneral.add(rt.ReduceChildScratchGeneral)
	s.reduceChildScratchNoAlias.add(rt.ReduceChildScratchNoAlias)
	s.finalNodes += rt.FinalNodes
	s.finalParentNodes += rt.FinalParentNodes
	s.finalLeafNodes += rt.FinalLeafNodes
	s.finalFieldedParentNodes += rt.FinalFieldedParentNodes
	s.finalUnfieldedParentNodes += rt.FinalUnfieldedParentNodes
	s.finalChildSlices += rt.FinalChildSlices
	s.finalChildPointers += rt.FinalChildPointers
	s.finalFieldIDElements += rt.FinalFieldIDElements
	s.finalFieldSourceElements += rt.FinalFieldSourceElements
	s.gssNodesAllocated += rt.GSSNodesAllocated
	s.gssNodesRetained += rt.GSSNodesRetained
	s.gssNodesDropped += rt.GSSNodesDroppedSameToken
	s.singleStackGSSNodes += rt.SingleStackGSSNodes
	s.multiStackGSSNodes += rt.MultiStackGSSNodes
	s.arenaBytesAllocated += rt.ArenaBytesAllocated
	s.scratchBytesAllocated += rt.ScratchBytesAllocated
	s.entryScratchBytesAllocated += rt.EntryScratchBytesAllocated
	s.gssBytesAllocated += rt.GSSBytesAllocated
	s.externalCheckpointRecords += rt.ExternalScannerCheckpointRecords
	s.externalCheckpointSlots += rt.ExternalScannerCheckpointSlotsAllocated
	s.externalCheckpointBytes += rt.ExternalScannerCheckpointBytesAllocated
	s.externalCheckpointSnapshotBytes += rt.ExternalScannerSnapshotBytesAllocated
	s.leafNodesConstructed += rt.LeafNodesConstructed
	s.parentNodesConstructed += rt.ParentNodesConstructed
	s.noTreeReduceNodesConstructed += rt.NoTreeReduceNodesConstructed
	s.noTreeLeafNodesConstructed += rt.NoTreeLeafNodesConstructed
	if hasBreakdown {
		s.arenaBreakdownSamples++
		s.arenaNodeStructBytesAllocated += breakdown.NodeStructBytesAllocated
		s.arenaNoTreeNodeBytesAllocated += breakdown.NoTreeNodeBytesAllocated
		s.arenaChildSliceBytesAllocated += breakdown.ChildSliceBytesAllocated
		s.arenaFieldIDBytesAllocated += breakdown.FieldIDBytesAllocated
		s.arenaFieldSourceBytesAllocated += breakdown.FieldSourceBytesAllocated
		s.mergeScratchBytesAllocated += breakdown.MergeScratchBytesAllocated
		s.arenaNodesConstructed += breakdown.ArenaNodesConstructed
		s.nodeLiveCount += breakdown.NodeLiveCount
		s.nodeCapacityCount += breakdown.NodeCapacityCount
		s.nodeCapacityWaste += breakdown.NodeCapacityWaste
		s.primaryNodeCapacity += breakdown.PrimaryNodeCapacity
		s.primaryNodeUsed += breakdown.PrimaryNodeUsed
		s.overflowNodeCapacity += breakdown.OverflowNodeCapacity
		s.overflowNodeUsed += breakdown.OverflowNodeUsed
		s.overflowNodeSlabs += breakdown.OverflowNodeSlabs
		s.largestNodeSlabUsedFractionSum += breakdown.LargestNodeSlabUsedFraction
		s.fieldedParentNodesConstructed += breakdown.FieldedParentNodesConstructed
		s.unfieldedParentNodesConstructed += breakdown.UnfieldedParentNodesConstructed
		s.parentConstructedChildLen0 += breakdown.ParentConstructedChildLen0
		s.parentConstructedChildLen1 += breakdown.ParentConstructedChildLen1
		s.parentConstructedChildLen2 += breakdown.ParentConstructedChildLen2
		s.parentConstructedChildLen3 += breakdown.ParentConstructedChildLen3
		s.parentConstructedChildLen4Plus += breakdown.ParentConstructedChildLen4Plus
		s.parentConstructedNoLinks += breakdown.ParentConstructedNoLinks
		s.parentConstructedWithLinks += breakdown.ParentConstructedWithLinks
		s.parentConstructedTrackErrors += breakdown.ParentConstructedTrackErrors
		s.parentConstructedFieldSources += breakdown.ParentConstructedFieldSources
		s.parentReductionVisible += breakdown.ParentReductionVisible
		s.parentReductionInvisible += breakdown.ParentReductionInvisible
		s.parentReductionVisibleFielded += breakdown.ParentReductionVisibleFielded
		s.parentReductionVisibleUnfielded += breakdown.ParentReductionVisibleUnfielded
		s.parentReductionInvisibleFielded += breakdown.ParentReductionInvisibleFielded
		s.parentReductionInvisibleUnfielded += breakdown.ParentReductionInvisibleUnfielded
		s.parentReductionVisibleChildPtrs += breakdown.ParentReductionVisibleChildPtrs
		s.parentReductionInvisibleChildPtrs += breakdown.ParentReductionInvisibleChildPtrs
		s.parentReductionVisibleLen0 += breakdown.ParentReductionVisibleLen0
		s.parentReductionVisibleLen1 += breakdown.ParentReductionVisibleLen1
		s.parentReductionVisibleLen2 += breakdown.ParentReductionVisibleLen2
		s.parentReductionVisibleLen3 += breakdown.ParentReductionVisibleLen3
		s.parentReductionVisibleLen4Plus += breakdown.ParentReductionVisibleLen4Plus
		s.parentReductionInvisibleLen0 += breakdown.ParentReductionInvisibleLen0
		s.parentReductionInvisibleLen1 += breakdown.ParentReductionInvisibleLen1
		s.parentReductionInvisibleLen2 += breakdown.ParentReductionInvisibleLen2
		s.parentReductionInvisibleLen3 += breakdown.ParentReductionInvisibleLen3
		s.parentReductionInvisibleLen4Plus += breakdown.ParentReductionInvisibleLen4Plus
		s.reduceChildSlicesFastGSS += breakdown.ReduceChildSlicesFastGSS
		s.reduceChildPointersFastGSS += breakdown.ReduceChildPointersFastGSS
		s.reduceChildSlicesAllVisible += breakdown.ReduceChildSlicesAllVisible
		s.reduceChildPointersAllVisible += breakdown.ReduceChildPointersAllVisible
		s.reduceChildSlicesNoAlias += breakdown.ReduceChildSlicesNoAlias
		s.reduceChildPointersNoAlias += breakdown.ReduceChildPointersNoAlias
		s.reduceChildSlicesScratchGeneral += breakdown.ReduceChildSlicesScratchGeneral
		s.reduceChildPointersScratchGeneral += breakdown.ReduceChildPointersScratchGeneral
		s.reduceChildSlicesScratchNoAlias += breakdown.ReduceChildSlicesScratchNoAlias
		s.reduceChildPointersScratchNoAlias += breakdown.ReduceChildPointersScratchNoAlias
		s.noTreePlaceholderNodesConstructed += breakdown.NoTreePlaceholderNodesConstructed
		s.otherNodesConstructed += breakdown.OtherNodesConstructed
		s.extraNodesConstructed += breakdown.ExtraNodesConstructed
		s.errorSymbolNodesConstructed += breakdown.ErrorSymbolNodesConstructed
		s.hasErrorNodesConstructed += breakdown.HasErrorNodesConstructed
		s.childSlicesConstructed += breakdown.ChildSlicesConstructed
		s.childPointersConstructed += breakdown.ChildPointersConstructed
		s.childSlicesLen1 += breakdown.ChildSlicesLen1
		s.childSlicesLen2 += breakdown.ChildSlicesLen2
		s.childSlicesLen3 += breakdown.ChildSlicesLen3
		s.childSlicesLen4Plus += breakdown.ChildSlicesLen4Plus
		s.parentChildPointersConstructed += breakdown.ParentChildPointersConstructed
		s.parentChildrenLen0 += breakdown.ParentChildrenLen0
		s.parentChildrenLen1 += breakdown.ParentChildrenLen1
		s.parentChildrenLen2 += breakdown.ParentChildrenLen2
		s.parentChildrenLen3 += breakdown.ParentChildrenLen3
		s.parentChildrenLen4Plus += breakdown.ParentChildrenLen4Plus
		s.fieldIDElementsConstructed += breakdown.FieldIDElementsConstructed
		s.fieldSourceElementsConstructed += breakdown.FieldSourceElementsConstructed
	}
	s.normalizationPassesChecked += rt.NormalizationPassesChecked
	s.normalizationPassesRun += rt.NormalizationPassesRun
	s.normalizationNodesVisited += rt.NormalizationNodesVisited
	s.normalizationNodesRewritten += rt.NormalizationNodesRewritten
	s.normalizationNanos += rt.NormalizationNanos
	if rt.MaxStacksSeen > s.maxStacksSeen {
		s.maxStacksSeen = rt.MaxStacksSeen
	}
}

func (s pythonRuntimeBenchStats) report(b *testing.B) {
	if s.ops == 0 {
		return
	}
	b.ReportMetric(float64(s.tokensConsumed)/float64(s.ops), "tokens/op")
	b.ReportMetric(float64(s.maxStacksSeen), "max_stacks")
	b.ReportMetric(float64(s.normalizationNanos)/float64(s.ops), "norm_ns/op")
	b.ReportMetric(float64(s.normalizationPassesChecked)/float64(s.ops), "norm_checked/op")
	b.ReportMetric(float64(s.normalizationPassesRun)/float64(s.ops), "norm_runs/op")
	if s.tokensConsumed == 0 {
		return
	}
	tokens := float64(s.tokensConsumed)
	gssNodes := s.gssNodesAllocated
	if gssNodes == 0 {
		gssNodes = s.singleStackGSSNodes + s.multiStackGSSNodes
	}
	b.ReportMetric(float64(s.iterations)/tokens, "iters/token")
	b.ReportMetric(float64(s.nodesAllocated)/tokens, "nodes/token")
	b.ReportMetric(float64(s.leafNodesConstructed)/tokens, "leaf_nodes/token")
	b.ReportMetric(float64(s.leafNodesConstructed)/tokens, "leaf_full_nodes/token")
	b.ReportMetric(float64(s.parentNodesConstructed)/tokens, "parent_nodes/token")
	b.ReportMetric(float64(s.parentNodesConstructed)/tokens, "parent_full_nodes/token")
	b.ReportMetric(float64(s.noTreeReduceNodesConstructed)/tokens, "notree_nodes/token")
	b.ReportMetric(float64(s.noTreeLeafNodesConstructed)/tokens, "notree_leaf_nodes/token")
	if s.finalNodes != 0 {
		b.ReportMetric(float64(s.finalNodes)/tokens, "final_nodes/token")
		b.ReportMetric(float64(s.finalParentNodes)/tokens, "final_parent_nodes/token")
		b.ReportMetric(float64(s.finalLeafNodes)/tokens, "final_leaf_nodes/token")
		b.ReportMetric(float64(s.finalFieldedParentNodes)/tokens, "final_fielded_parent_nodes/token")
		b.ReportMetric(float64(s.finalUnfieldedParentNodes)/tokens, "final_unfielded_parent_nodes/token")
		b.ReportMetric(float64(s.finalChildSlices)/tokens, "final_child_slices/token")
		b.ReportMetric(float64(s.finalChildPointers)/tokens, "final_child_ptrs/token")
		b.ReportMetric(float64(s.finalFieldIDElements)/tokens, "final_field_ids/token")
		b.ReportMetric(float64(s.finalFieldSourceElements)/tokens, "final_field_sources/token")
	}
	if s.parentNodesAllocated != 0 || s.leafNodesAllocated != 0 {
		b.ReportMetric(float64(s.parentNodesAllocated)/tokens, "surv_parent_nodes/token")
		b.ReportMetric(float64(s.leafNodesAllocated)/tokens, "surv_leaf_nodes/token")
		fullNodesAllocated := s.parentNodesAllocated + s.leafNodesAllocated
		fullNodesRetained := s.parentNodesRetained + s.leafNodesRetained
		fullNodesDropped := s.parentNodesDropped + s.leafNodesDropped
		b.ReportMetric(float64(fullNodesAllocated)/tokens, "full_nodes_alloc/token")
		b.ReportMetric(float64(fullNodesRetained)/tokens, "full_nodes_retained/token")
		b.ReportMetric(float64(fullNodesRetained)/tokens, "nodes_retained/token")
		b.ReportMetric(float64(fullNodesDropped)/tokens, "full_nodes_dropped/token")
		b.ReportMetric(float64(fullNodesDropped)/tokens, "nodes_discarded/token")
		b.ReportMetric(float64(s.parentNodesRetained)/tokens, "parent_retained/token")
		b.ReportMetric(float64(s.parentNodesDropped)/tokens, "parent_dropped/token")
		b.ReportMetric(float64(s.leafNodesRetained)/tokens, "leaf_retained/token")
		b.ReportMetric(float64(s.leafNodesDropped)/tokens, "leaf_dropped/token")
		b.ReportMetric(float64(s.childSlicesAllocated)/tokens, "child_slices_alloc/token")
		b.ReportMetric(float64(s.childSlicesRetained)/tokens, "child_slices_retained/token")
		b.ReportMetric(float64(s.childSlicesDropped)/tokens, "child_slices_dropped/token")
		b.ReportMetric(float64(s.childSlicesDropped)/tokens, "child_slices_discarded/token")
		b.ReportMetric(float64(s.childPointersAllocated)/tokens, "child_ptrs_alloc/token")
		b.ReportMetric(float64(s.childPointersRetained)/tokens, "child_ptrs_retained/token")
		b.ReportMetric(float64(s.childPointersDropped)/tokens, "child_ptrs_dropped/token")
		b.ReportMetric(float64(s.childPointersDropped)/tokens, "child_ptrs_discarded/token")
		s.reduceChildFastGSS.report(b, tokens, "fast_gss")
		s.reduceChildAllVisible.report(b, tokens, "all_visible")
		s.reduceChildNoAlias.report(b, tokens, "no_alias")
		s.reduceChildScratchGeneral.report(b, tokens, "scratch_general")
		s.reduceChildScratchNoAlias.report(b, tokens, "scratch_no_alias")
	}
	if s.gssNodesRetained != 0 || s.gssNodesDropped != 0 {
		b.ReportMetric(float64(s.gssNodesRetained)/tokens, "gss_retained/token")
		b.ReportMetric(float64(s.gssNodesDropped)/tokens, "gss_dropped/token")
	}
	b.ReportMetric(float64(gssNodes)/tokens, "gss_nodes/token")
	b.ReportMetric(float64(s.singleStackGSSNodes)/tokens, "single_gss/token")
	b.ReportMetric(float64(s.multiStackGSSNodes)/tokens, "multi_gss/token")
	b.ReportMetric(float64(s.arenaBytesAllocated)/tokens, "arena_B/token")
	b.ReportMetric(float64(s.scratchBytesAllocated)/tokens, "scratch_B/token")
	b.ReportMetric(float64(s.entryScratchBytesAllocated)/tokens, "entry_B/token")
	b.ReportMetric(float64(s.gssBytesAllocated)/tokens, "gss_B/token")
	if s.arenaBreakdownSamples != 0 {
		b.ReportMetric(float64(s.arenaNodesConstructed)/tokens, "arena_nodes/token")
		b.ReportMetric(float64(s.nodeLiveCount)/tokens, "node_live/token")
		b.ReportMetric(float64(s.nodeCapacityCount)/tokens, "node_capacity/token")
		b.ReportMetric(float64(s.nodeCapacityWaste)/tokens, "node_capacity_waste/token")
		b.ReportMetric(float64(s.primaryNodeCapacity)/float64(s.arenaBreakdownSamples), "primary_node_capacity")
		b.ReportMetric(float64(s.primaryNodeUsed)/float64(s.arenaBreakdownSamples), "primary_node_used")
		b.ReportMetric(float64(s.overflowNodeCapacity)/float64(s.arenaBreakdownSamples), "overflow_node_capacity")
		b.ReportMetric(float64(s.overflowNodeUsed)/float64(s.arenaBreakdownSamples), "overflow_node_used")
		b.ReportMetric(float64(s.overflowNodeSlabs)/float64(s.arenaBreakdownSamples), "overflow_slabs")
		b.ReportMetric(s.largestNodeSlabUsedFractionSum/float64(s.arenaBreakdownSamples), "largest_slab_used_fraction")
		b.ReportMetric(float64(s.fieldedParentNodesConstructed)/tokens, "fielded_parent_nodes/token")
		b.ReportMetric(float64(s.unfieldedParentNodesConstructed)/tokens, "unfielded_parent_nodes/token")
		b.ReportMetric(float64(s.parentConstructedChildLen0)/tokens, "parent_ctor_len0/token")
		b.ReportMetric(float64(s.parentConstructedChildLen1)/tokens, "parent_ctor_len1/token")
		b.ReportMetric(float64(s.parentConstructedChildLen2)/tokens, "parent_ctor_len2/token")
		b.ReportMetric(float64(s.parentConstructedChildLen3)/tokens, "parent_ctor_len3/token")
		b.ReportMetric(float64(s.parentConstructedChildLen4Plus)/tokens, "parent_ctor_len4plus/token")
		b.ReportMetric(float64(s.parentConstructedNoLinks)/tokens, "parent_ctor_nolinks/token")
		b.ReportMetric(float64(s.parentConstructedWithLinks)/tokens, "parent_ctor_withlinks/token")
		b.ReportMetric(float64(s.parentConstructedTrackErrors)/tokens, "parent_ctor_track_errors/token")
		b.ReportMetric(float64(s.parentConstructedFieldSources)/tokens, "parent_ctor_field_sources/token")
		b.ReportMetric(float64(s.parentReductionVisible)/tokens, "parent_reduce_visible/token")
		b.ReportMetric(float64(s.parentReductionInvisible)/tokens, "parent_reduce_invisible/token")
		b.ReportMetric(float64(s.parentReductionVisibleFielded)/tokens, "parent_reduce_visible_fielded/token")
		b.ReportMetric(float64(s.parentReductionVisibleUnfielded)/tokens, "parent_reduce_visible_unfielded/token")
		b.ReportMetric(float64(s.parentReductionInvisibleFielded)/tokens, "parent_reduce_invisible_fielded/token")
		b.ReportMetric(float64(s.parentReductionInvisibleUnfielded)/tokens, "parent_reduce_invisible_unfielded/token")
		b.ReportMetric(float64(s.parentReductionVisibleChildPtrs)/tokens, "parent_reduce_visible_child_ptrs/token")
		b.ReportMetric(float64(s.parentReductionInvisibleChildPtrs)/tokens, "parent_reduce_invisible_child_ptrs/token")
		b.ReportMetric(float64(s.parentReductionVisibleLen0)/tokens, "parent_reduce_visible_len0/token")
		b.ReportMetric(float64(s.parentReductionVisibleLen1)/tokens, "parent_reduce_visible_len1/token")
		b.ReportMetric(float64(s.parentReductionVisibleLen2)/tokens, "parent_reduce_visible_len2/token")
		b.ReportMetric(float64(s.parentReductionVisibleLen3)/tokens, "parent_reduce_visible_len3/token")
		b.ReportMetric(float64(s.parentReductionVisibleLen4Plus)/tokens, "parent_reduce_visible_len4plus/token")
		b.ReportMetric(float64(s.parentReductionInvisibleLen0)/tokens, "parent_reduce_invisible_len0/token")
		b.ReportMetric(float64(s.parentReductionInvisibleLen1)/tokens, "parent_reduce_invisible_len1/token")
		b.ReportMetric(float64(s.parentReductionInvisibleLen2)/tokens, "parent_reduce_invisible_len2/token")
		b.ReportMetric(float64(s.parentReductionInvisibleLen3)/tokens, "parent_reduce_invisible_len3/token")
		b.ReportMetric(float64(s.parentReductionInvisibleLen4Plus)/tokens, "parent_reduce_invisible_len4plus/token")
		b.ReportMetric(float64(s.reduceChildSlicesFastGSS)/tokens, "reduce_child_slices_fast_gss/token")
		b.ReportMetric(float64(s.reduceChildPointersFastGSS)/tokens, "reduce_child_ptrs_fast_gss/token")
		b.ReportMetric(float64(s.reduceChildSlicesAllVisible)/tokens, "reduce_child_slices_all_visible/token")
		b.ReportMetric(float64(s.reduceChildPointersAllVisible)/tokens, "reduce_child_ptrs_all_visible/token")
		b.ReportMetric(float64(s.reduceChildSlicesNoAlias)/tokens, "reduce_child_slices_no_alias/token")
		b.ReportMetric(float64(s.reduceChildPointersNoAlias)/tokens, "reduce_child_ptrs_no_alias/token")
		b.ReportMetric(float64(s.reduceChildSlicesScratchGeneral)/tokens, "reduce_child_slices_scratch_general/token")
		b.ReportMetric(float64(s.reduceChildPointersScratchGeneral)/tokens, "reduce_child_ptrs_scratch_general/token")
		b.ReportMetric(float64(s.reduceChildSlicesScratchNoAlias)/tokens, "reduce_child_slices_scratch_no_alias/token")
		b.ReportMetric(float64(s.reduceChildPointersScratchNoAlias)/tokens, "reduce_child_ptrs_scratch_no_alias/token")
		b.ReportMetric(float64(s.noTreePlaceholderNodesConstructed)/tokens, "notree_placeholder_nodes/token")
		b.ReportMetric(float64(s.otherNodesConstructed)/tokens, "other_nodes/token")
		b.ReportMetric(float64(s.extraNodesConstructed)/tokens, "extra_nodes/token")
		b.ReportMetric(float64(s.errorSymbolNodesConstructed)/tokens, "error_symbol_nodes/token")
		b.ReportMetric(float64(s.errorSymbolNodesConstructed)/tokens, "error_nodes/token")
		b.ReportMetric(float64(s.hasErrorNodesConstructed)/tokens, "has_error_nodes/token")
		b.ReportMetric(float64(s.arenaNodeStructBytesAllocated)/tokens, "arena_node_B/token")
		b.ReportMetric(float64(s.arenaNoTreeNodeBytesAllocated)/tokens, "arena_notree_node_B/token")
		b.ReportMetric(float64(s.arenaChildSliceBytesAllocated)/tokens, "arena_child_B/token")
		b.ReportMetric(float64(s.arenaChildSliceBytesAllocated)/tokens, "child_slice_B/token")
		b.ReportMetric(float64(s.arenaFieldIDBytesAllocated)/tokens, "arena_field_id_B/token")
		b.ReportMetric(float64(s.arenaFieldIDBytesAllocated)/tokens, "field_id_B/token")
		b.ReportMetric(float64(s.arenaFieldSourceBytesAllocated)/tokens, "arena_field_src_B/token")
		b.ReportMetric(float64(s.arenaFieldSourceBytesAllocated)/tokens, "field_source_B/token")
		b.ReportMetric(float64(s.mergeScratchBytesAllocated)/tokens, "merge_B/token")
		b.ReportMetric(float64(s.childSlicesConstructed)/tokens, "child_slices/token")
		b.ReportMetric(float64(s.childPointersConstructed)/tokens, "child_ptrs/token")
		b.ReportMetric(float64(s.childSlicesLen1)/tokens, "child_slices_len1/token")
		b.ReportMetric(float64(s.childSlicesLen2)/tokens, "child_slices_len2/token")
		b.ReportMetric(float64(s.childSlicesLen3)/tokens, "child_slices_len3/token")
		b.ReportMetric(float64(s.childSlicesLen4Plus)/tokens, "child_slices_len4plus/token")
		b.ReportMetric(float64(s.parentChildPointersConstructed)/tokens, "parent_child_ptrs/token")
		b.ReportMetric(float64(s.parentChildrenLen0)/tokens, "parent_len0/token")
		b.ReportMetric(float64(s.parentChildrenLen1)/tokens, "parent_len1/token")
		b.ReportMetric(float64(s.parentChildrenLen2)/tokens, "parent_len2/token")
		b.ReportMetric(float64(s.parentChildrenLen3)/tokens, "parent_len3/token")
		b.ReportMetric(float64(s.parentChildrenLen4Plus)/tokens, "parent_len4plus/token")
		b.ReportMetric(float64(s.fieldIDElementsConstructed)/tokens, "field_ids/token")
		b.ReportMetric(float64(s.fieldSourceElementsConstructed)/tokens, "field_sources/token")
	}
	b.ReportMetric(float64(s.externalCheckpointRecords)/tokens, "chk_records/token")
	b.ReportMetric(float64(s.externalCheckpointSlots)/tokens, "chk_slots/token")
	b.ReportMetric(float64(s.externalCheckpointBytes)/tokens, "chk_B/token")
	b.ReportMetric(float64(s.externalCheckpointSnapshotBytes)/tokens, "chk_snap_B/token")
	b.ReportMetric(float64(s.normalizationNodesVisited)/tokens, "norm_visited/token")
	b.ReportMetric(float64(s.normalizationNodesRewritten)/tokens, "norm_rewritten/token")
}

func loadPythonCorpusFile(tb testing.TB) pythonCorpusFile {
	tb.Helper()

	candidates := []string{strings.TrimSpace(os.Getenv("GOT_PYTHON_CORPUS_FILE"))}
	if candidates[0] == "" {
		candidates = []string{
			defaultPythonCorpusPath,
			filepath.Join("cgo_harness", defaultPythonCorpusPath),
		}
	}

	for _, path := range candidates {
		if path == "" {
			continue
		}
		st, err := os.Stat(path)
		if err != nil || st.IsDir() {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			tb.Fatalf("read python corpus %s: %v", path, err)
		}
		if len(source) == 0 {
			tb.Fatalf("python corpus %s is empty", path)
		}
		tb.Logf("python corpus: path=%s bytes=%d", path, len(source))
		return pythonCorpusFile{path: path, source: source}
	}

	tb.Fatalf("python corpus file not found; tried %s; set GOT_PYTHON_CORPUS_FILE or run from the repository/cgo_harness root", strings.Join(candidates, ", "))
	return pythonCorpusFile{}
}

func pythonCorpusBenchTimeoutMicros(tb testing.TB) uint64 {
	tb.Helper()

	raw := strings.TrimSpace(os.Getenv("GOT_PYTHON_CORPUS_BENCH_TIMEOUT_MICROS"))
	if raw == "" {
		return 0
	}
	timeoutMicros, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		tb.Fatalf("invalid GOT_PYTHON_CORPUS_BENCH_TIMEOUT_MICROS=%q", raw)
	}
	return timeoutMicros
}

func pythonCorpusArenaBreakdownEnabled(tb testing.TB) bool {
	tb.Helper()

	raw := strings.TrimSpace(os.Getenv("GOT_PYTHON_CORPUS_ARENA_BREAKDOWN"))
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		tb.Fatalf("invalid GOT_PYTHON_CORPUS_ARENA_BREAKDOWN=%q", raw)
	}
	return enabled
}

func pythonCorpusRuntimeAuditEnabled(tb testing.TB) bool {
	tb.Helper()

	raw := strings.TrimSpace(os.Getenv("GOT_PYTHON_CORPUS_RUNTIME_AUDIT"))
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		tb.Fatalf("invalid GOT_PYTHON_CORPUS_RUNTIME_AUDIT=%q", raw)
	}
	return enabled
}

func requireCompletePythonCorpusTree(tb testing.TB, lang *gotreesitter.Language, file pythonCorpusFile, tree *gotreesitter.Tree, phase string) {
	tb.Helper()

	if tree == nil {
		tb.Fatalf("%s: parse returned nil tree", phase)
	}
	root := tree.RootNode()
	if root == nil {
		tb.Fatalf("%s: parse returned nil root", phase)
	}
	rt := tree.ParseRuntime()
	if root.HasError() || tree.ParseStoppedEarly() || root.EndByte() != uint32(len(file.source)) || rt.Truncated {
		tb.Fatalf(
			"%s: incomplete parse path=%s type=%q has_error=%v stopped=%v root_end=%d want=%d runtime=%s",
			phase,
			file.path,
			root.Type(lang),
			root.HasError(),
			tree.ParseStoppedEarly(),
			root.EndByte(),
			len(file.source),
			rt.Summary(),
		)
	}
}

func benchmarkPythonCorpusGoDFA(b *testing.B, mode pythonCorpusParseMode) {
	file := loadPythonCorpusFile(b)
	lang := grammars.PythonLanguage()
	if pythonCorpusArenaBreakdownEnabled(b) {
		gotreesitter.EnableArenaBreakdown(true)
		defer gotreesitter.EnableArenaBreakdown(false)
	}
	if pythonCorpusRuntimeAuditEnabled(b) {
		gotreesitter.EnableRuntimeAudit(true)
		defer gotreesitter.EnableRuntimeAudit(false)
	}
	pool := gotreesitter.NewParserPool(
		lang,
		gotreesitter.WithParserPoolTimeoutMicros(pythonCorpusBenchTimeoutMicros(b)),
	)

	b.ReportAllocs()
	b.SetBytes(int64(len(file.source)))
	b.ResetTimer()

	var stats pythonRuntimeBenchStats
	for i := 0; i < b.N; i++ {
		var (
			tree *gotreesitter.Tree
			err  error
		)
		switch mode {
		case pythonCorpusParseModeDFA:
			tree, err = pool.Parse(file.source)
		case pythonCorpusParseModeDFANoCompat:
			tree, err = pool.ParseNoResultCompatibilityBenchmarkOnly(file.source)
		case pythonCorpusParseModeDFANoTree:
			tree, err = pool.ParseNoTreeBenchmarkOnly(file.source)
		case pythonCorpusParseModeDFANoTreeWithCheckpoints:
			tree, err = pool.ParseNoTreeWithExternalCheckpointsBenchmarkOnly(file.source)
		default:
			b.Fatalf("unknown python corpus parse mode %q", mode)
		}
		if err != nil {
			if tree != nil {
				tree.Release()
			}
			b.Fatalf("%s: %v", file.path, err)
		}
		requireCompletePythonCorpusTree(b, lang, file, tree, string(mode))
		rt := tree.ParseRuntime()
		breakdown, hasBreakdown := tree.ArenaBreakdown()
		stats.add(rt, breakdown, hasBreakdown)
		tree.Release()
	}
	b.StopTimer()
	stats.report(b)
}

func BenchmarkPythonCorpusGoTreeSitterParseDFA(b *testing.B) {
	benchmarkPythonCorpusGoDFA(b, pythonCorpusParseModeDFA)
}

func BenchmarkPythonCorpusGoTreeSitterParseDFANoCompat(b *testing.B) {
	benchmarkPythonCorpusGoDFA(b, pythonCorpusParseModeDFANoCompat)
}

func BenchmarkPythonCorpusGoTreeSitterParseDFANoTree(b *testing.B) {
	benchmarkPythonCorpusGoDFA(b, pythonCorpusParseModeDFANoTree)
}

func BenchmarkPythonCorpusGoTreeSitterParseDFANoTreeWithCheckpoints(b *testing.B) {
	benchmarkPythonCorpusGoDFA(b, pythonCorpusParseModeDFANoTreeWithCheckpoints)
}

func BenchmarkPythonCorpusCTreeSitterParseFull(b *testing.B) {
	file := loadPythonCorpusFile(b)
	parser := newCTreeSitterParserWithLanguage(b, sitterpython.GetLanguage)
	defer parser.Close()

	b.ReportAllocs()
	b.SetBytes(int64(len(file.source)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree := parser.Parse(nil, file.source)
		root := requireCompleteCTree(b, tree, file.source, file.path)
		if root.HasError() {
			tree.Close()
			b.Fatalf("%s: cgo parse has errors", file.path)
		}
		tree.Close()
	}
}
