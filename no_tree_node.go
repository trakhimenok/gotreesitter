package gotreesitter

import "unsafe"

type noTreeNode struct {
	startPoint   Point
	endPoint     Point
	startByte    uint32
	endByte      uint32
	parseState   StateID
	preGotoState StateID
	symbol       Symbol
	productionID uint16
	flags        nodeFlags
}

type noTreeNodeSlab struct {
	data []noTreeNode
	used int
}

const (
	stackEntryKindNode uint32 = iota
	stackEntryKindNoTreeNode
)

func newStackEntryNode(state StateID, node *Node) stackEntry {
	return stackEntry{
		node:  node,
		state: state,
		kind:  stackEntryKindNode,
	}
}

func newStackEntryNoTreeNode(state StateID, node *noTreeNode) stackEntry {
	return stackEntry{
		node:  (*Node)(unsafe.Pointer(node)),
		state: state,
		kind:  stackEntryKindNoTreeNode,
	}
}

func stackEntryNode(e stackEntry) *Node {
	if e.kind != stackEntryKindNode || e.node == nil {
		return nil
	}
	return e.node
}

func stackEntryNoTreeNode(e stackEntry) *noTreeNode {
	if e.kind != stackEntryKindNoTreeNode || e.node == nil {
		return nil
	}
	return (*noTreeNode)(unsafe.Pointer(e.node))
}

func (n *noTreeNode) hasFlag(flag nodeFlags) bool {
	return n != nil && n.flags&flag != 0
}

func (n *noTreeNode) setFlag(flag nodeFlags, enabled bool) {
	if n == nil {
		return
	}
	if enabled {
		n.flags |= flag
		return
	}
	n.flags &^= flag
}

func (n *noTreeNode) isNamed() bool      { return n.hasFlag(nodeFlagNamed) }
func (n *noTreeNode) setNamed(v bool)    { n.setFlag(nodeFlagNamed, v) }
func (n *noTreeNode) isExtra() bool      { return n.hasFlag(nodeFlagExtra) }
func (n *noTreeNode) setExtra(v bool)    { n.setFlag(nodeFlagExtra, v) }
func (n *noTreeNode) isMissing() bool    { return n.hasFlag(nodeFlagMissing) }
func (n *noTreeNode) setMissing(v bool)  { n.setFlag(nodeFlagMissing, v) }
func (n *noTreeNode) hasError() bool     { return n.hasFlag(nodeFlagHasError) }
func (n *noTreeNode) setHasError(v bool) { n.setFlag(nodeFlagHasError, v) }

func noTreeNodeBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(noTreeNode{}))
}

func defaultNoTreeNodeSlabCap(class arenaClass) int {
	slabBytes := incrementalArenaSlab
	if class == arenaClassFull {
		slabBytes = fullParseArenaSlab
	}
	size := int(unsafe.Sizeof(noTreeNode{}))
	if size <= 0 {
		return minArenaNodeCap
	}
	capacity := slabBytes / size
	if capacity < minArenaNodeCap {
		return minArenaNodeCap
	}
	return capacity
}

func stackEntryNodeSymbol(e stackEntry) Symbol {
	if n := stackEntryNode(e); n != nil {
		return n.symbol
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.symbol
	}
	return 0
}

func stackEntryNodeStartByte(e stackEntry) uint32 {
	if n := stackEntryNode(e); n != nil {
		return n.startByte
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.startByte
	}
	return 0
}

func stackEntryNodeEndByte(e stackEntry) uint32 {
	if n := stackEntryNode(e); n != nil {
		return n.endByte
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.endByte
	}
	return 0
}

func stackEntryNodeStartPoint(e stackEntry) Point {
	if n := stackEntryNode(e); n != nil {
		return n.startPoint
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.startPoint
	}
	return Point{}
}

func stackEntryNodeEndPoint(e stackEntry) Point {
	if n := stackEntryNode(e); n != nil {
		return n.endPoint
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.endPoint
	}
	return Point{}
}

func stackEntryNodeParseState(e stackEntry) StateID {
	if n := stackEntryNode(e); n != nil {
		return n.parseState
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.parseState
	}
	return 0
}

func stackEntryNodePreGotoState(e stackEntry) StateID {
	if n := stackEntryNode(e); n != nil {
		return n.preGotoState
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.preGotoState
	}
	return 0
}

func stackEntryNodeProductionID(e stackEntry) uint16 {
	if n := stackEntryNode(e); n != nil {
		return n.productionID
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		return n.productionID
	}
	return 0
}

func stackEntryHasNode(e stackEntry) bool {
	return e.node != nil
}

func retargetStackEntryPayload(e stackEntry, state StateID) (stackEntry, bool) {
	if n := stackEntryNode(e); n != nil {
		n.parseState = state
		nodeBumpEquivVersion(n)
		e.state = state
		return e, true
	}
	if n := stackEntryNoTreeNode(e); n != nil {
		n.parseState = state
		e.state = state
		return e, true
	}
	return e, false
}

func stackEntryNodeIsExtra(e stackEntry) bool {
	if n := stackEntryNode(e); n != nil {
		return n.isExtra()
	}
	n := stackEntryNoTreeNode(e)
	return n != nil && n.isExtra()
}

func stackEntryNodeIsNamed(e stackEntry) bool {
	if n := stackEntryNode(e); n != nil {
		return n.isNamed()
	}
	n := stackEntryNoTreeNode(e)
	return n != nil && n.isNamed()
}

func stackEntryNodeIsMissing(e stackEntry) bool {
	if n := stackEntryNode(e); n != nil {
		return n.isMissing()
	}
	n := stackEntryNoTreeNode(e)
	return n != nil && n.isMissing()
}

func stackEntryNodeHasError(e stackEntry) bool {
	if n := stackEntryNode(e); n != nil {
		return n.hasError()
	}
	n := stackEntryNoTreeNode(e)
	return n != nil && n.hasError()
}

func stackEntryNodeChildCount(e stackEntry) int {
	if n := stackEntryNode(e); n != nil {
		return len(n.children)
	}
	return 0
}

func stackEntryNodeFieldIDCount(e stackEntry) int {
	if n := stackEntryNode(e); n != nil {
		return len(n.fieldIDs)
	}
	return 0
}

func newNoTreeLeafNodeInArena(arena *nodeArena, sym Symbol, named bool, startByte, endByte uint32, startPoint, endPoint Point) *noTreeNode {
	var n *noTreeNode
	if arena == nil {
		n = &noTreeNode{}
	} else {
		n = arena.allocNoTreeNode()
		arena.noTreeLeafNodesConstructed++
	}
	n.symbol = sym
	n.setNamed(named)
	n.startByte = startByte
	n.endByte = endByte
	n.startPoint = startPoint
	n.endPoint = endPoint
	return n
}
