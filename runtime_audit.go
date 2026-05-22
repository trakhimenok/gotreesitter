package gotreesitter

type runtimeAuditNodeKind uint8

const (
	runtimeAuditNodeKindLeaf runtimeAuditNodeKind = iota + 1
	runtimeAuditNodeKindParent
)

type runtimeAuditNodeInfo struct {
	gen                 uint32
	kind                runtimeAuditNodeKind
	reduceChildPath     reduceChildPath
	reduceChildPointers uint32
}

type runtimeAudit struct {
	enabled         bool
	equivEnabled    bool
	currentTokenGen uint32
	tokenActive     bool

	gssGen   map[*gssNode]uint32
	nodeInfo map[*Node]runtimeAuditNodeInfo
	seenGSS  map[*gssNode]struct{}
	seenNode map[*Node]struct{}

	currentGSSAllocated                 uint64
	currentGSSRetained                  uint64
	currentParentAllocated              uint64
	currentParentRetained               uint64
	currentLeafAllocated                uint64
	currentLeafRetained                 uint64
	currentChildSlicesAllocated         uint64
	currentChildSlicesRetained          uint64
	currentChildPointersAllocated       uint64
	currentChildPointersRetained        uint64
	currentReduceChildSlicesAllocated   [reduceChildPathCount]uint64
	currentReduceChildSlicesRetained    [reduceChildPathCount]uint64
	currentReduceChildPointersAllocated [reduceChildPathCount]uint64
	currentReduceChildPointersRetained  [reduceChildPathCount]uint64

	totalGSSAllocated                 uint64
	totalGSSRetained                  uint64
	totalGSSDropped                   uint64
	totalParentAllocated              uint64
	totalParentRetained               uint64
	totalParentDropped                uint64
	totalLeafAllocated                uint64
	totalLeafRetained                 uint64
	totalLeafDropped                  uint64
	totalChildSlicesAllocated         uint64
	totalChildSlicesRetained          uint64
	totalChildSlicesDropped           uint64
	totalChildPointersAllocated       uint64
	totalChildPointersRetained        uint64
	totalChildPointersDropped         uint64
	totalReduceChildSlicesAllocated   [reduceChildPathCount]uint64
	totalReduceChildSlicesRetained    [reduceChildPathCount]uint64
	totalReduceChildSlicesDropped     [reduceChildPathCount]uint64
	totalReduceChildPointersAllocated [reduceChildPathCount]uint64
	totalReduceChildPointersRetained  [reduceChildPathCount]uint64
	totalReduceChildPointersDropped   [reduceChildPathCount]uint64

	mergeStacksIn       uint64
	mergeStacksOut      uint64
	mergeSlotsUsed      uint64
	globalCullStacksIn  uint64
	globalCullStacksOut uint64

	equivCacheLookups      uint64
	equivCacheHits         uint64
	equivCacheStores       uint64
	equivSkipError         uint64
	equivSkipLeaf          uint64
	equivSkipFieldMismatch uint64
}

var runtimeAuditEnabled bool
var runtimeEquivAuditEnabled bool

// EnableRuntimeAudit toggles per-parse survivor instrumentation.
// This debug hook is intended for single-threaded benchmark/profiling runs.
func EnableRuntimeAudit(enabled bool) {
	runtimeAuditEnabled = enabled
}

// EnableGLREquivAudit toggles lightweight GLR equivalence attribution.
// This is intended for parser gap diagnostics and avoids the heavier survivor
// maps used by EnableRuntimeAudit.
func EnableGLREquivAudit(enabled bool) {
	runtimeEquivAuditEnabled = enabled
}

func (a *runtimeAudit) beginParse() {
	equivEnabled := runtimeEquivAuditEnabled
	if !runtimeAuditEnabled && !equivEnabled {
		a.reset()
		return
	}
	a.enabled = runtimeAuditEnabled
	a.equivEnabled = equivEnabled
	a.currentTokenGen = 0
	a.tokenActive = false
	a.currentGSSAllocated = 0
	a.currentGSSRetained = 0
	a.currentParentAllocated = 0
	a.currentParentRetained = 0
	a.currentLeafAllocated = 0
	a.currentLeafRetained = 0
	a.currentChildSlicesAllocated = 0
	a.currentChildSlicesRetained = 0
	a.currentChildPointersAllocated = 0
	a.currentChildPointersRetained = 0
	a.currentReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersRetained = [reduceChildPathCount]uint64{}
	a.totalGSSAllocated = 0
	a.totalGSSRetained = 0
	a.totalGSSDropped = 0
	a.totalParentAllocated = 0
	a.totalParentRetained = 0
	a.totalParentDropped = 0
	a.totalLeafAllocated = 0
	a.totalLeafRetained = 0
	a.totalLeafDropped = 0
	a.totalChildSlicesAllocated = 0
	a.totalChildSlicesRetained = 0
	a.totalChildSlicesDropped = 0
	a.totalChildPointersAllocated = 0
	a.totalChildPointersRetained = 0
	a.totalChildPointersDropped = 0
	a.totalReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.totalReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.totalReduceChildSlicesDropped = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersRetained = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersDropped = [reduceChildPathCount]uint64{}
	a.mergeStacksIn = 0
	a.mergeStacksOut = 0
	a.mergeSlotsUsed = 0
	a.globalCullStacksIn = 0
	a.globalCullStacksOut = 0
	a.equivCacheLookups = 0
	a.equivCacheHits = 0
	a.equivCacheStores = 0
	a.equivSkipError = 0
	a.equivSkipLeaf = 0
	a.equivSkipFieldMismatch = 0
	if !a.enabled {
		return
	}
	if a.gssGen == nil {
		a.gssGen = make(map[*gssNode]uint32)
	} else {
		clearRuntimeAuditGSSMap(a.gssGen)
	}
	if a.nodeInfo == nil {
		a.nodeInfo = make(map[*Node]runtimeAuditNodeInfo)
	} else {
		clearRuntimeAuditNodeMap(a.nodeInfo)
	}
	if a.seenGSS == nil {
		a.seenGSS = make(map[*gssNode]struct{})
	} else {
		clearRuntimeAuditSeenGSSMap(a.seenGSS)
	}
	if a.seenNode == nil {
		a.seenNode = make(map[*Node]struct{})
	} else {
		clearRuntimeAuditSeenNodeMap(a.seenNode)
	}
}

func (a *runtimeAudit) reset() {
	a.enabled = false
	a.equivEnabled = false
	a.currentTokenGen = 0
	a.tokenActive = false
	a.currentGSSAllocated = 0
	a.currentGSSRetained = 0
	a.currentParentAllocated = 0
	a.currentParentRetained = 0
	a.currentLeafAllocated = 0
	a.currentLeafRetained = 0
	a.currentChildSlicesAllocated = 0
	a.currentChildSlicesRetained = 0
	a.currentChildPointersAllocated = 0
	a.currentChildPointersRetained = 0
	a.currentReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersRetained = [reduceChildPathCount]uint64{}
	a.totalGSSAllocated = 0
	a.totalGSSRetained = 0
	a.totalGSSDropped = 0
	a.totalParentAllocated = 0
	a.totalParentRetained = 0
	a.totalParentDropped = 0
	a.totalLeafAllocated = 0
	a.totalLeafRetained = 0
	a.totalLeafDropped = 0
	a.totalChildSlicesAllocated = 0
	a.totalChildSlicesRetained = 0
	a.totalChildSlicesDropped = 0
	a.totalChildPointersAllocated = 0
	a.totalChildPointersRetained = 0
	a.totalChildPointersDropped = 0
	a.totalReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.totalReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.totalReduceChildSlicesDropped = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersRetained = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersDropped = [reduceChildPathCount]uint64{}
	a.mergeStacksIn = 0
	a.mergeStacksOut = 0
	a.mergeSlotsUsed = 0
	a.globalCullStacksIn = 0
	a.globalCullStacksOut = 0
	a.equivCacheLookups = 0
	a.equivCacheHits = 0
	a.equivCacheStores = 0
	a.equivSkipError = 0
	a.equivSkipLeaf = 0
	a.equivSkipFieldMismatch = 0
	if a.gssGen != nil {
		clearRuntimeAuditGSSMap(a.gssGen)
	}
	if a.nodeInfo != nil {
		clearRuntimeAuditNodeMap(a.nodeInfo)
	}
	if a.seenGSS != nil {
		clearRuntimeAuditSeenGSSMap(a.seenGSS)
	}
	if a.seenNode != nil {
		clearRuntimeAuditSeenNodeMap(a.seenNode)
	}
}

func (a *runtimeAudit) startToken(stacks []glrStack) {
	if a == nil || !a.enabled {
		return
	}
	if a.tokenActive {
		a.observeFrontier(stacks)
		a.finishToken()
	}
	a.currentTokenGen++
	a.tokenActive = true
	a.currentGSSAllocated = 0
	a.currentGSSRetained = 0
	a.currentParentAllocated = 0
	a.currentParentRetained = 0
	a.currentLeafAllocated = 0
	a.currentLeafRetained = 0
	a.currentChildSlicesAllocated = 0
	a.currentChildSlicesRetained = 0
	a.currentChildPointersAllocated = 0
	a.currentChildPointersRetained = 0
	a.currentReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersRetained = [reduceChildPathCount]uint64{}
}

func (a *runtimeAudit) finishParse(stacks []glrStack) {
	if a == nil || !a.enabled || !a.tokenActive {
		return
	}
	a.observeFrontier(stacks)
	a.finishToken()
}

func (a *runtimeAudit) finishToken() {
	if a == nil || !a.enabled || !a.tokenActive {
		return
	}
	a.totalGSSAllocated += a.currentGSSAllocated
	a.totalGSSRetained += a.currentGSSRetained
	if a.currentGSSAllocated > a.currentGSSRetained {
		a.totalGSSDropped += a.currentGSSAllocated - a.currentGSSRetained
	}
	a.totalParentAllocated += a.currentParentAllocated
	a.totalParentRetained += a.currentParentRetained
	if a.currentParentAllocated > a.currentParentRetained {
		a.totalParentDropped += a.currentParentAllocated - a.currentParentRetained
	}
	a.totalLeafAllocated += a.currentLeafAllocated
	a.totalLeafRetained += a.currentLeafRetained
	if a.currentLeafAllocated > a.currentLeafRetained {
		a.totalLeafDropped += a.currentLeafAllocated - a.currentLeafRetained
	}
	a.totalChildSlicesAllocated += a.currentChildSlicesAllocated
	a.totalChildSlicesRetained += a.currentChildSlicesRetained
	if a.currentChildSlicesAllocated > a.currentChildSlicesRetained {
		a.totalChildSlicesDropped += a.currentChildSlicesAllocated - a.currentChildSlicesRetained
	}
	a.totalChildPointersAllocated += a.currentChildPointersAllocated
	a.totalChildPointersRetained += a.currentChildPointersRetained
	if a.currentChildPointersAllocated > a.currentChildPointersRetained {
		a.totalChildPointersDropped += a.currentChildPointersAllocated - a.currentChildPointersRetained
	}
	for path := reduceChildPath(1); path < reduceChildPathCount; path++ {
		a.totalReduceChildSlicesAllocated[path] += a.currentReduceChildSlicesAllocated[path]
		a.totalReduceChildSlicesRetained[path] += a.currentReduceChildSlicesRetained[path]
		if a.currentReduceChildSlicesAllocated[path] > a.currentReduceChildSlicesRetained[path] {
			a.totalReduceChildSlicesDropped[path] += a.currentReduceChildSlicesAllocated[path] - a.currentReduceChildSlicesRetained[path]
		}
		a.totalReduceChildPointersAllocated[path] += a.currentReduceChildPointersAllocated[path]
		a.totalReduceChildPointersRetained[path] += a.currentReduceChildPointersRetained[path]
		if a.currentReduceChildPointersAllocated[path] > a.currentReduceChildPointersRetained[path] {
			a.totalReduceChildPointersDropped[path] += a.currentReduceChildPointersAllocated[path] - a.currentReduceChildPointersRetained[path]
		}
	}
	a.tokenActive = false
}

func (a *runtimeAudit) recordGSSAlloc(n *gssNode) {
	if a == nil || !a.enabled || !a.tokenActive || n == nil {
		return
	}
	a.currentGSSAllocated++
	a.gssGen[n] = a.currentTokenGen
}

func (a *runtimeAudit) recordNodeAlloc(n *Node, kind runtimeAuditNodeKind) {
	if a == nil || !a.enabled || !a.tokenActive || n == nil {
		return
	}
	switch kind {
	case runtimeAuditNodeKindParent:
		a.currentParentAllocated++
		if childCount := len(n.children); childCount > 0 {
			a.currentChildSlicesAllocated++
			a.currentChildPointersAllocated += uint64(childCount)
		}
	case runtimeAuditNodeKindLeaf:
		a.currentLeafAllocated++
	default:
		return
	}
	a.nodeInfo[n] = runtimeAuditNodeInfo{gen: a.currentTokenGen, kind: kind}
}

func (a *runtimeAudit) recordReduceParentChildPath(n *Node, path reduceChildPath, childCount int) {
	if a == nil || !a.enabled || !a.tokenActive || n == nil || !path.valid() || childCount <= 0 {
		return
	}
	info, ok := a.nodeInfo[n]
	if !ok || info.gen != a.currentTokenGen || info.kind != runtimeAuditNodeKindParent {
		return
	}
	info.reduceChildPath = path
	info.reduceChildPointers = uint32(childCount)
	a.nodeInfo[n] = info
	a.currentReduceChildSlicesAllocated[path]++
	a.currentReduceChildPointersAllocated[path] += uint64(childCount)
}

func (a *runtimeAudit) recordMerge(in, out, slots int) {
	if a == nil || !a.enabled {
		return
	}
	a.mergeStacksIn += uint64(in)
	a.mergeStacksOut += uint64(out)
	a.mergeSlotsUsed += uint64(slots)
}

func (a *runtimeAudit) recordGlobalCull(in, out int) {
	if a == nil || !a.enabled {
		return
	}
	a.globalCullStacksIn += uint64(in)
	a.globalCullStacksOut += uint64(out)
}

func (a *runtimeAudit) recordEquivCacheLookup() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivCacheLookups++
}

func (a *runtimeAudit) recordEquivCacheHit() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivCacheHits++
}

func (a *runtimeAudit) recordEquivCacheStore() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivCacheStores++
}

func (a *runtimeAudit) recordEquivSkipError() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivSkipError++
}

func (a *runtimeAudit) recordEquivSkipLeaf() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivSkipLeaf++
}

func (a *runtimeAudit) recordEquivSkipFieldMismatch() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivSkipFieldMismatch++
}

func (a *runtimeAudit) observeFrontier(stacks []glrStack) {
	if a == nil || !a.enabled || !a.tokenActive {
		return
	}
	clearRuntimeAuditSeenGSSMap(a.seenGSS)
	clearRuntimeAuditSeenNodeMap(a.seenNode)
	var gssRetained uint64
	var parentRetained uint64
	var leafRetained uint64
	var childSlicesRetained uint64
	var childPointersRetained uint64
	for i := range stacks {
		if stacks[i].dead {
			continue
		}
		if stacks[i].gss.head != nil {
			a.observeGSSChain(stacks[i].gss.head, &gssRetained, &parentRetained, &leafRetained, &childSlicesRetained, &childPointersRetained)
			continue
		}
		a.observeEntries(stacks[i].entries, &parentRetained, &leafRetained, &childSlicesRetained, &childPointersRetained)
	}
	a.currentGSSRetained = gssRetained
	a.currentParentRetained = parentRetained
	a.currentLeafRetained = leafRetained
	a.currentChildSlicesRetained = childSlicesRetained
	a.currentChildPointersRetained = childPointersRetained
}

func (a *runtimeAudit) observeGSSChain(head *gssNode, gssRetained, parentRetained, leafRetained, childSlicesRetained, childPointersRetained *uint64) {
	if a == nil || head == nil {
		return
	}
	for n := head; n != nil; n = n.prev {
		gen, ok := a.gssGen[n]
		if !ok || gen != a.currentTokenGen {
			break
		}
		if _, seen := a.seenGSS[n]; !seen {
			a.seenGSS[n] = struct{}{}
			*gssRetained = *gssRetained + 1
		}
		a.observeNode(stackEntryNode(n.entry), parentRetained, leafRetained, childSlicesRetained, childPointersRetained)
	}
}

func (a *runtimeAudit) observeEntries(entries []stackEntry, parentRetained, leafRetained, childSlicesRetained, childPointersRetained *uint64) {
	if a == nil || len(entries) == 0 {
		return
	}
	for i := len(entries) - 1; i >= 0; i-- {
		node := stackEntryNode(entries[i])
		if node == nil && stackEntryNoTreeNode(entries[i]) != nil {
			continue
		}
		info, ok := a.nodeInfo[node]
		if !ok || info.gen != a.currentTokenGen {
			break
		}
		a.observeNode(node, parentRetained, leafRetained, childSlicesRetained, childPointersRetained)
	}
}

func (a *runtimeAudit) observeNode(node *Node, parentRetained, leafRetained, childSlicesRetained, childPointersRetained *uint64) {
	if a == nil || node == nil {
		return
	}
	info, ok := a.nodeInfo[node]
	if !ok || info.gen != a.currentTokenGen {
		return
	}
	if _, seen := a.seenNode[node]; seen {
		return
	}
	a.seenNode[node] = struct{}{}
	switch info.kind {
	case runtimeAuditNodeKindParent:
		*parentRetained = *parentRetained + 1
		if childCount := len(node.children); childCount > 0 {
			*childSlicesRetained = *childSlicesRetained + 1
			*childPointersRetained = *childPointersRetained + uint64(childCount)
		}
		if info.reduceChildPath.valid() && info.reduceChildPointers > 0 {
			a.currentReduceChildSlicesRetained[info.reduceChildPath]++
			a.currentReduceChildPointersRetained[info.reduceChildPath] += uint64(info.reduceChildPointers)
		}
	case runtimeAuditNodeKindLeaf:
		*leafRetained = *leafRetained + 1
	}
}

func (a *runtimeAudit) reduceChildPathRuntime(path reduceChildPath) ReduceChildPathRuntime {
	if a == nil || !path.valid() {
		return ReduceChildPathRuntime{}
	}
	return ReduceChildPathRuntime{
		SlicesAllocated:   a.totalReduceChildSlicesAllocated[path],
		SlicesRetained:    a.totalReduceChildSlicesRetained[path],
		SlicesDropped:     a.totalReduceChildSlicesDropped[path],
		PointersAllocated: a.totalReduceChildPointersAllocated[path],
		PointersRetained:  a.totalReduceChildPointersRetained[path],
		PointersDropped:   a.totalReduceChildPointersDropped[path],
	}
}

func clearRuntimeAuditGSSMap(m map[*gssNode]uint32) {
	for k := range m {
		delete(m, k)
	}
}

func clearRuntimeAuditNodeMap(m map[*Node]runtimeAuditNodeInfo) {
	for k := range m {
		delete(m, k)
	}
}

func clearRuntimeAuditSeenGSSMap(m map[*gssNode]struct{}) {
	for k := range m {
		delete(m, k)
	}
}

func clearRuntimeAuditSeenNodeMap(m map[*Node]struct{}) {
	for k := range m {
		delete(m, k)
	}
}
