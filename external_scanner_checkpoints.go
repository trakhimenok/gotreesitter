package gotreesitter

import (
	"bytes"
	"unsafe"
)

type externalScannerCheckpoint struct {
	start []byte
	end   []byte
}

type externalScannerSnapshotRef struct {
	slab uint16
	off  uint32
	len  uint16
}

type externalScannerCheckpointRef struct {
	start externalScannerSnapshotRef
	end   externalScannerSnapshotRef
}

func languageUsesExternalScannerCheckpoints(lang *Language) bool {
	if lang == nil || lang.ExternalScanner == nil {
		return false
	}
	switch lang.Name {
	case "python", "mojo", "starlark":
		return true
	default:
		return false
	}
}

func underlyingDFATokenSource(ts TokenSource) *dfaTokenSource {
	switch src := ts.(type) {
	case *dfaTokenSource:
		return src
	case *includedRangeTokenSource:
		return underlyingDFATokenSource(src.base)
	default:
		return nil
	}
}

func (a *nodeArena) recordExternalScannerLeafCheckpoint(node *Node, start, end []byte) {
	if a == nil || node == nil {
		return
	}
	slot := a.externalScannerCheckpointSlot(node, true)
	if slot == nil {
		return
	}
	a.externalScannerCheckpointRecords++
	startRef := a.copyExternalScannerSnapshotRef(start)
	endRef := startRef
	if !bytes.Equal(start, end) {
		endRef = a.copyExternalScannerSnapshotRef(end)
	}
	*slot = externalScannerCheckpointRef{
		start: startRef,
		end:   endRef,
	}
}

func (a *nodeArena) copyExternalScannerSnapshotRef(src []byte) externalScannerSnapshotRef {
	if a == nil || len(src) == 0 {
		return externalScannerSnapshotRef{}
	}
	return a.allocExternalScannerSnapshotRef(src)
}

func (a *nodeArena) setExternalScannerCheckpoint(node *Node, cp externalScannerCheckpointRef) {
	if a == nil || node == nil {
		return
	}
	slot := a.externalScannerCheckpointSlot(node, true)
	if slot == nil {
		return
	}
	*slot = cp
}

func externalScannerCheckpointForNode(node *Node) (externalScannerCheckpoint, bool) {
	cp, ok := externalScannerCheckpointRefForNode(node)
	if !ok || node == nil || node.ownerArena == nil {
		return externalScannerCheckpoint{}, false
	}
	return externalScannerCheckpoint{
		start: node.ownerArena.externalScannerSnapshotBytes(cp.start),
		end:   node.ownerArena.externalScannerSnapshotBytes(cp.end),
	}, true
}

func externalScannerCheckpointRefForNode(node *Node) (externalScannerCheckpointRef, bool) {
	if node == nil || node.ownerArena == nil {
		return externalScannerCheckpointRef{}, false
	}
	slot := node.ownerArena.externalScannerCheckpointSlot(node, false)
	if slot == nil || (slot.start.len == 0 && slot.end.len == 0) {
		return externalScannerCheckpointRef{}, false
	}
	return *slot, true
}

func rebuildExternalScannerCheckpoints(root *Node, lang *Language) {
	if root == nil || !languageUsesExternalScannerCheckpoints(lang) {
		return
	}

	type frame struct {
		node    *Node
		visited bool
	}

	stack := []frame{{node: root}}
	for len(stack) > 0 {
		last := len(stack) - 1
		f := stack[last]
		stack = stack[:last]
		n := f.node
		if n == nil {
			continue
		}
		if !f.visited {
			stack = append(stack, frame{node: n, visited: true})
			for i := len(n.children) - 1; i >= 0; i-- {
				stack = append(stack, frame{node: n.children[i]})
			}
			continue
		}
		if len(n.children) == 0 {
			continue
		}

		var start []byte
		var end []byte
		var startRef externalScannerSnapshotRef
		var endRef externalScannerSnapshotRef
		for _, child := range n.children {
			cp, ok := externalScannerCheckpointRefForNode(child)
			if !ok {
				continue
			}
			startRef = cp.start
			start = n.ownerArena.externalScannerSnapshotBytes(cp.start)
			break
		}
		for i := len(n.children) - 1; i >= 0; i-- {
			cp, ok := externalScannerCheckpointRefForNode(n.children[i])
			if !ok {
				continue
			}
			endRef = cp.end
			end = n.ownerArena.externalScannerSnapshotBytes(cp.end)
			break
		}
		if start == nil && end == nil {
			continue
		}
		n.ownerArena.setExternalScannerCheckpoint(n, externalScannerCheckpointRef{start: startRef, end: endRef})
	}
}

func currentExternalScannerCheckpoint(ts TokenSource) (externalScannerCheckpoint, uint32, uint32, bool) {
	dts := underlyingDFATokenSource(ts)
	if dts == nil || !languageUsesExternalScannerCheckpoints(dts.language) {
		return externalScannerCheckpoint{}, 0, 0, false
	}
	return dts.lastExternalScannerCheckpoint()
}

func canReuseNodeWithExternalScannerCheckpoint(ts TokenSource, startState StateID, node *Node) (externalScannerCheckpointRef, bool) {
	dts := underlyingDFATokenSource(ts)
	if dts == nil || !languageUsesExternalScannerCheckpoints(dts.language) {
		return externalScannerCheckpointRef{}, true
	}
	if node == nil || startState != node.PreGotoState() {
		return externalScannerCheckpointRef{}, false
	}
	cp, ok := externalScannerCheckpointRefForNode(node)
	if !ok {
		return externalScannerCheckpointRef{}, false
	}
	if !dts.externalScannerStateMatches(node.ownerArena.externalScannerSnapshotBytes(cp.start)) {
		return externalScannerCheckpointRef{}, false
	}
	return cp, true
}

func fastForwardWithExternalScannerCheckpoint(ts TokenSource, node *Node, cp externalScannerCheckpointRef) (Token, bool) {
	dts := underlyingDFATokenSource(ts)
	if dts == nil || !languageUsesExternalScannerCheckpoints(dts.language) {
		return Token{}, false
	}
	if node == nil {
		return Token{}, false
	}
	dts.restoreExternalScannerState(node.ownerArena.externalScannerSnapshotBytes(cp.end))
	if skipper, ok := ts.(PointSkippableTokenSource); ok {
		return skipper.SkipToByteWithPoint(node.EndByte(), node.EndPoint()), true
	}
	if skipper, ok := ts.(ByteSkippableTokenSource); ok {
		return skipper.SkipToByte(node.EndByte()), true
	}
	return advanceTokenSourceTo(ts, Token{
		StartByte:  node.StartByte(),
		EndByte:    node.StartByte(),
		StartPoint: node.StartPoint(),
		EndPoint:   node.StartPoint(),
	}, node.EndByte()), true
}

func (a *nodeArena) externalScannerCheckpointSlot(node *Node, create bool) *externalScannerCheckpointRef {
	if a == nil || node == nil {
		return nil
	}
	if idx, ok := nodeIndexInStorage(node, a.nodes); ok {
		if create {
			a.ensureExternalScannerPrimaryCheckpoints()
		}
		if idx >= len(a.externalScannerNodeCheckpoints) {
			return nil
		}
		return &a.externalScannerNodeCheckpoints[idx]
	}
	for i := range a.nodeSlabs {
		idx, ok := nodeIndexInStorage(node, a.nodeSlabs[i].data)
		if !ok {
			continue
		}
		if create {
			a.ensureExternalScannerCheckpointSlab(i)
		}
		if i >= len(a.externalScannerNodeCheckpointSlabs) || idx >= len(a.externalScannerNodeCheckpointSlabs[i].data) {
			return nil
		}
		return &a.externalScannerNodeCheckpointSlabs[i].data[idx]
	}
	return nil
}

func (a *nodeArena) ensureExternalScannerPrimaryCheckpoints() {
	if a == nil || len(a.nodes) == 0 || len(a.externalScannerNodeCheckpoints) == len(a.nodes) {
		return
	}
	a.externalScannerNodeCheckpoints = make([]externalScannerCheckpointRef, len(a.nodes))
	a.allocatedBytes += externalScannerCheckpointBytesForCap(len(a.externalScannerNodeCheckpoints))
}

func (a *nodeArena) ensureExternalScannerCheckpointSlab(idx int) {
	if a == nil || idx < 0 || idx >= len(a.nodeSlabs) {
		return
	}
	for len(a.externalScannerNodeCheckpointSlabs) <= idx {
		a.externalScannerNodeCheckpointSlabs = append(a.externalScannerNodeCheckpointSlabs, externalScannerCheckpointSlab{})
	}
	if len(a.externalScannerNodeCheckpointSlabs[idx].data) == len(a.nodeSlabs[idx].data) {
		return
	}
	a.externalScannerNodeCheckpointSlabs[idx].data = make([]externalScannerCheckpointRef, len(a.nodeSlabs[idx].data))
	a.allocatedBytes += externalScannerCheckpointBytesForCap(len(a.externalScannerNodeCheckpointSlabs[idx].data))
}

func nodeIndexInStorage(node *Node, storage []Node) (int, bool) {
	if node == nil || len(storage) == 0 {
		return 0, false
	}
	start := uintptr(unsafe.Pointer(&storage[0]))
	ptr := uintptr(unsafe.Pointer(node))
	size := unsafe.Sizeof(Node{})
	end := start + uintptr(len(storage))*size
	if ptr < start || ptr >= end {
		return 0, false
	}
	offset := ptr - start
	if offset%size != 0 {
		return 0, false
	}
	return int(offset / size), true
}
