package gotreesitter

import "unsafe"

type pendingParent struct {
	noTreeNode
	startPoint Point
	endPoint   Point
	firstChild *pendingChildEntry
	childCount uint32
}

type pendingParentSlab struct {
	data []pendingParent
	used int
}

type pendingChildEntrySlab struct {
	data []pendingChildEntry
	used int
}

// pendingChildEntry stores arena-owned payload pointers with the stack-entry
// kind in the low bits. The payload parseState is the source of truth when the
// stack entry is reconstructed.
type pendingChildEntry uintptr

type pendingParentMaterializeReason = materializeReason

const (
	pendingParentMaterializeForParentReduce      pendingParentMaterializeReason = materializeForParentReduce
	pendingParentMaterializeForFinalTree         pendingParentMaterializeReason = materializeForFinalTree
	pendingParentMaterializeForNormalization     pendingParentMaterializeReason = materializeForNormalization
	pendingParentMaterializeForRecovery          pendingParentMaterializeReason = materializeForRecovery
	pendingParentMaterializeForQuery             pendingParentMaterializeReason = materializeForQuery
	pendingParentMaterializeForCursor            pendingParentMaterializeReason = materializeForCursor
	pendingParentMaterializeForParentAPI         pendingParentMaterializeReason = materializeForParentAPI
	pendingParentMaterializeForEdit              pendingParentMaterializeReason = materializeForEdit
	pendingParentMaterializeForCheckpointRebuild pendingParentMaterializeReason = materializeForCheckpointRebuild
)

func pendingParentBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(pendingParent{}))
}

func pendingChildEntryBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(pendingChildEntry(0)))
}

func defaultPendingParentSlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab
	}
	size := int(unsafe.Sizeof(pendingParent{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func defaultPendingChildEntrySlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab
	}
	size := int(unsafe.Sizeof(pendingChildEntry(0)))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func newPendingParentInArena(arena *nodeArena, sym Symbol, named bool, productionID uint16, children []stackEntry, startByte, endByte uint32, startPoint, endPoint Point, hasError bool) *pendingParent {
	p := newPendingParentShellInArena(arena, sym, named, productionID, len(children), startByte, endByte, startPoint, endPoint, hasError)
	for i, child := range children {
		p.setChildEntry(i, child)
	}
	return p
}

func newPendingParentShellInArena(arena *nodeArena, sym Symbol, named bool, productionID uint16, childCount int, startByte, endByte uint32, startPoint, endPoint Point, hasError bool) *pendingParent {
	var p *pendingParent
	var children []pendingChildEntry
	if arena == nil {
		if childCount > 0 {
			panic("pending parent child refs require arena-backed payloads")
		}
		p = &pendingParent{}
	} else {
		p = arena.allocPendingParent()
		children = arena.allocPendingChildEntries(childCount)
		arena.pendingParentCreated++
	}
	p.setChildEntries(children)
	p.symbol = sym
	p.startByte = startByte
	p.endByte = endByte
	p.parseState = 0
	p.preGotoState = 0
	p.productionID = productionID
	p.flags = noTreeNodeInitialFlags(named)
	p.setHasError(hasError)
	p.startPoint = startPoint
	p.endPoint = endPoint
	return p
}

func (p *pendingParent) setChildEntries(children []pendingChildEntry) {
	if p == nil || len(children) == 0 {
		return
	}
	p.firstChild = &children[0]
	p.childCount = uint32(len(children))
}

func (p *pendingParent) childRefs() []pendingChildEntry {
	if p == nil || p.firstChild == nil || p.childCount == 0 {
		return nil
	}
	return unsafe.Slice(p.firstChild, int(p.childCount))
}

func (p *pendingParent) childEntryCount() int {
	if p == nil {
		return 0
	}
	return int(p.childCount)
}

func (p *pendingParent) childEntry(i int) stackEntry {
	if p == nil || i < 0 || i >= int(p.childCount) {
		return stackEntry{}
	}
	return p.childRefs()[i].stackEntry()
}

func (p *pendingParent) setChildEntry(i int, entry stackEntry) {
	if p == nil || i < 0 || i >= int(p.childCount) {
		return
	}
	p.childRefs()[i] = newPendingChildEntry(entry)
}

func newPendingChildEntry(entry stackEntry) pendingChildEntry {
	if entry.node == nil {
		return 0
	}
	kind := uintptr(entry.kind)
	if kind&^pendingChildEntryKindMask != 0 {
		panic("pending child entry kind exceeds tag mask")
	}
	ptr := uintptr(unsafe.Pointer(entry.node))
	if ptr&pendingChildEntryKindMask != 0 {
		panic("pending child entry payload is under-aligned")
	}
	return pendingChildEntry(ptr | kind)
}

const pendingChildEntryKindMask = uintptr(3)

func (entry pendingChildEntry) stackEntry() stackEntry {
	if entry == 0 {
		return stackEntry{}
	}
	kind := uint32(uintptr(entry) & pendingChildEntryKindMask)
	node := (*Node)(unsafe.Pointer(uintptr(entry) &^ pendingChildEntryKindMask))
	stack := stackEntry{
		node: node,
		kind: kind,
	}
	stack.state = stackEntryNodeParseState(stack)
	return stack
}

func materializeStackEntryPendingParent(arena *nodeArena, entry *stackEntry, reason pendingParentMaterializeReason) *Node {
	return materializeStackEntryPendingParentWithParser(nil, arena, entry, reason)
}

func materializeStackEntryPendingParentWithParser(p *Parser, arena *nodeArena, entry *stackEntry, reason pendingParentMaterializeReason) *Node {
	if entry == nil {
		return nil
	}
	node, updated := materializeStackEntryPendingParentEntryWithParser(p, arena, *entry, reason)
	*entry = updated
	return node
}

func materializeStackEntryPendingParentEntryWithParser(p *Parser, arena *nodeArena, entry stackEntry, reason pendingParentMaterializeReason) (*Node, stackEntry) {
	parent := stackEntryPendingParent(entry)
	if parent == nil {
		return materializeStackEntryCompactFullLeafEntry(arena, entry, compactFullLeafMaterializeReason(reason))
	}
	childCount := parent.childEntryCount()
	children := arena.allocNodeSliceNoClear(childCount)
	for i := 0; i < childCount; i++ {
		child := parent.childEntry(i)
		var updatedChild stackEntry
		children[i], updatedChild = materializeStackEntryPayloadEntryWithParser(p, arena, child, compactFullLeafMaterializeReason(reason), reason)
		child = updatedChild
		parent.setChildEntry(i, child)
	}
	node := newParentNodeInArenaNoLinksWithFieldSources(arena, parent.symbol, parent.isNamed(), children, nil, nil, parent.productionID, parent.hasError())
	node.flags = parent.flags
	node.startByte = parent.startByte
	node.endByte = parent.endByte
	node.startPoint = parent.startPoint
	node.endPoint = parent.endPoint
	node.parseState = parent.parseState
	node.preGotoState = parent.preGotoState
	entry.node = node
	entry.kind = stackEntryKindNode
	entry.state = node.parseState
	arena.recordPendingParentMaterialized(reason)
	return node, entry
}

func materializeStackEntryPayload(arena *nodeArena, entry *stackEntry, leafReason compactFullLeafMaterializeReason, parentReason pendingParentMaterializeReason) *Node {
	return materializeStackEntryPayloadWithParser(nil, arena, entry, leafReason, parentReason)
}

func materializeStackEntryPayloadWithParser(p *Parser, arena *nodeArena, entry *stackEntry, leafReason compactFullLeafMaterializeReason, parentReason pendingParentMaterializeReason) *Node {
	if entry == nil {
		return nil
	}
	node, updated := materializeStackEntryPayloadEntryWithParser(p, arena, *entry, leafReason, parentReason)
	*entry = updated
	return node
}

func materializeStackEntryPayloadEntryWithParser(p *Parser, arena *nodeArena, entry stackEntry, leafReason compactFullLeafMaterializeReason, parentReason pendingParentMaterializeReason) (*Node, stackEntry) {
	restoreShape := false
	prevPayloadShape := pendingParentFieldRejectPayloadUnknown
	if p != nil && arena != nil && arena.breakdownEnabled && arena.pendingParentActiveRejectReason == pendingParentRejectFields {
		restoreShape = true
		prevPayloadShape = arena.pendingParentActiveFieldPayloadShape
		arena.pendingParentActiveFieldPayloadShape = p.pendingParentFieldRejectPayloadShape(entry)
	}
	if restoreShape {
		defer func() {
			arena.pendingParentActiveFieldPayloadShape = prevPayloadShape
		}()
	}
	if arena != nil && arena.pendingParentActiveRejectReason != pendingParentRejectUnknown {
		arena.recordParentRejectPayloadMaterialized(entry, arena.pendingParentActiveRejectReason)
	}
	if stackEntryPendingParent(entry) != nil {
		return materializeStackEntryPendingParentEntryWithParser(p, arena, entry, parentReason)
	}
	return materializeStackEntryCompactFullLeafEntry(arena, entry, leafReason)
}
