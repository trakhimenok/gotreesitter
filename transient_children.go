package gotreesitter

import "unsafe"

type transientChildScratch struct {
	slabs          []childSliceSlab
	slabCursor     int
	allocatedBytes int64
	seen           map[*Node]struct{}
}

func (s *transientChildScratch) alloc(n int) []*Node {
	if n <= 0 {
		return nil
	}
	if s == nil {
		return make([]*Node, n)
	}
	if len(s.slabs) == 0 {
		capacity := max(defaultChildSliceCap(arenaClassFull), n)
		s.slabs = append(s.slabs, childSliceSlab{data: make([]*Node, capacity)})
		s.allocatedBytes += childSliceBytesForCap(capacity)
		s.slabCursor = 0
	}
	if s.slabCursor < 0 || s.slabCursor >= len(s.slabs) {
		s.slabCursor = 0
	}
	for i := s.slabCursor; ; i++ {
		if i >= len(s.slabs) {
			lastCap := len(s.slabs[len(s.slabs)-1].data)
			capacity := max(lastCap*2, n)
			s.slabs = append(s.slabs, childSliceSlab{data: make([]*Node, capacity)})
			s.allocatedBytes += childSliceBytesForCap(capacity)
		}
		slab := &s.slabs[i]
		if len(slab.data)-slab.used < n {
			continue
		}
		start := slab.used
		slab.used += n
		s.slabCursor = i
		return slab.data[start:slab.used]
	}
}

func (s *transientChildScratch) owns(children []*Node) bool {
	if s == nil || len(children) == 0 {
		return false
	}
	ptr := uintptr(unsafe.Pointer(&children[0]))
	for i := range s.slabs {
		data := s.slabs[i].data
		if len(data) == 0 {
			continue
		}
		start := uintptr(unsafe.Pointer(&data[0]))
		end := start + uintptr(len(data))*unsafe.Sizeof((*Node)(nil))
		if ptr >= start && ptr < end {
			return true
		}
	}
	return false
}

func (s *transientChildScratch) materializeNode(root *Node, arena *nodeArena, scratch *[]*Node) {
	if s == nil || root == nil || arena == nil {
		return
	}
	defer s.clearSeen()
	var stack []*Node
	if scratch != nil {
		stack = (*scratch)[:0]
	} else {
		var local [64]*Node
		stack = local[:0]
	}
	stack = append(stack, root)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		if s.seen == nil {
			s.seen = make(map[*Node]struct{})
		}
		if _, ok := s.seen[n]; ok {
			continue
		}
		s.seen[n] = struct{}{}
		children := n.children
		if len(children) == 0 {
			continue
		}
		if s.owns(children) {
			out := arena.allocNodeSliceNoClear(len(children))
			copy(out, children)
			n.children = out
			children = out
		}
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	if scratch != nil {
		*scratch = stack[:0]
	}
}

func (s *transientChildScratch) reset() {
	if s == nil {
		return
	}
	for i := range s.slabs {
		slab := &s.slabs[i]
		if slab.used > 0 {
			clear(slab.data[:slab.used])
			slab.used = 0
		}
	}
	s.slabCursor = 0
	s.clearSeen()
}

func (s *transientChildScratch) resetForRelease() {
	if s == nil {
		return
	}
	s.reset()
	totalCap := 0
	for i := range s.slabs {
		totalCap += len(s.slabs[i].data)
	}
	if totalCap > maxRetainedFullSliceCap {
		for i := range s.slabs {
			s.slabs[i] = childSliceSlab{}
		}
		s.slabs = nil
		s.allocatedBytes = 0
	}
}

func (s *transientChildScratch) clearSeen() {
	if s == nil || s.seen == nil {
		return
	}
	for n := range s.seen {
		delete(s.seen, n)
	}
}
