package gotreesitter

import "unsafe"

type pendingParent struct {
	noTreeNode
	startPoint Point
	endPoint   Point
	firstChild *stackEntry
	childCount uint32
}

type pendingParentSlab struct {
	data []pendingParent
	used int
}

type pendingChildEntrySlab struct {
	data []stackEntry
	used int
}

type pendingParentMaterializeReason uint8

const (
	pendingParentMaterializeForParentReduce pendingParentMaterializeReason = iota
	pendingParentMaterializeForFinalTree
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
	return int64(n) * int64(unsafe.Sizeof(stackEntry{}))
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
	size := int(unsafe.Sizeof(stackEntry{}))
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
	copy(p.childEntries(), children)
	return p
}

func newPendingParentShellInArena(arena *nodeArena, sym Symbol, named bool, productionID uint16, childCount int, startByte, endByte uint32, startPoint, endPoint Point, hasError bool) *pendingParent {
	var p *pendingParent
	var children []stackEntry
	if arena == nil {
		p = &pendingParent{}
		if childCount > 0 {
			children = make([]stackEntry, childCount)
		}
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

func (p *pendingParent) setChildEntries(children []stackEntry) {
	if p == nil || len(children) == 0 {
		return
	}
	p.firstChild = &children[0]
	p.childCount = uint32(len(children))
}

func (p *pendingParent) childEntries() []stackEntry {
	if p == nil || p.firstChild == nil || p.childCount == 0 {
		return nil
	}
	return unsafe.Slice(p.firstChild, int(p.childCount))
}

func materializeStackEntryPendingParent(arena *nodeArena, entry *stackEntry, reason pendingParentMaterializeReason) *Node {
	if entry == nil {
		return nil
	}
	parent := stackEntryPendingParent(*entry)
	if parent == nil {
		return materializeStackEntryCompactFullLeaf(arena, entry, compactFullLeafMaterializeForParentReduce)
	}
	parentChildren := parent.childEntries()
	children := arena.allocNodeSliceNoClear(len(parentChildren))
	for i := range parentChildren {
		children[i] = materializeStackEntryPayload(arena, &parentChildren[i], compactFullLeafMaterializeForParentReduce, pendingParentMaterializeForParentReduce)
	}
	node := newParentNodeInArenaNoLinksWithFieldSources(arena, parent.symbol, parent.isNamed(), children, nil, nil, parent.productionID, parent.hasError())
	node.flags = parent.flags
	node.startByte = parent.startByte
	node.endByte = parent.endByte
	node.startPoint = parent.startPoint
	node.endPoint = parent.endPoint
	node.parseState = parent.parseState
	node.preGotoState = parent.preGotoState
	setStackEntryNode(entry, node)
	if arena != nil {
		arena.pendingParentMaterialized++
		switch reason {
		case pendingParentMaterializeForParentReduce:
			arena.pendingParentMaterializedForParentReduce++
		case pendingParentMaterializeForFinalTree:
			arena.pendingParentMaterializedForFinalTree++
		}
	}
	return node
}

func materializeStackEntryPayload(arena *nodeArena, entry *stackEntry, leafReason compactFullLeafMaterializeReason, parentReason pendingParentMaterializeReason) *Node {
	if entry == nil {
		return nil
	}
	if stackEntryPendingParent(*entry) != nil {
		return materializeStackEntryPendingParent(arena, entry, parentReason)
	}
	return materializeStackEntryCompactFullLeaf(arena, entry, leafReason)
}
