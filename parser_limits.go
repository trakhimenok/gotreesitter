package gotreesitter

import "sync/atomic"

func parseIterations(sourceLen int) int {
	return max(10_000, sourceLen*30)
}

// parseStackDepth returns the stack depth limit scaled to input size.
func parseStackDepth(sourceLen int) int {
	return max(1_000, sourceLen*2)
}

// parseNodeLimit returns the maximum number of Node allocations allowed.
// This is the hard ceiling that prevents OOM regardless of iteration count.
func parseNodeLimit(sourceLen int) int {
	return parseNodeLimitForLanguage(sourceLen, nil)
}

// parseNodeLimitForLanguage is parseNodeLimit with a per-language budget tuned
// for grammars that allocate unusually many nodes per input byte. The default
// sourceLen*52 budget is calibrated against the synthetic Go full-parse
// workload; grammars with heavy GLR fanout (notably tree-sitter-markdown's
// inline code-span / emphasis / latex-span / strikethrough external scanner
// and its ambiguous block/inline split) can legitimately consume 150+ nodes
// per byte on dense real-world inputs. Raising only the per-byte factor for
// those grammars avoids forcing two full-parse retries on every small doc
// while preserving the OOM ceiling for other languages.
func parseNodeLimitForLanguage(sourceLen int, lang *Language) int {
	perByte := 52
	if lang != nil {
		switch lang.Name {
		case "markdown", "markdown_inline":
			// Measured on the mdpp zero-cgo-parsing.mdpp corpus: 11 KB input
			// drove ~1.9M node allocations (~170/byte). 200/byte keeps the
			// first parse inside the ceiling without retry churn and still
			// bounds pathological inputs.
			perByte = 200
		}
	}
	limit := max(300_000, sourceLen*perByte)
	scale := parseNodeLimitScaleFactor()
	if scale <= 1 {
		return limit
	}
	maxInt := int(^uint(0) >> 1)
	if limit > maxInt/scale {
		return maxInt
	}
	return limit * scale
}

func parseMemoryBudget(sourceLen int) int64 {
	mb := parseMemoryBudgetMB()
	if mb <= 0 {
		return 0
	}
	// Keep the budget source-length aware so callers can lower it to zero for
	// tests without introducing an unused-parameter path here.
	if sourceLen < 0 {
		sourceLen = 0
	}
	return int64(mb) * 1024 * 1024
}

func parseMemoryBudgetForParser(p *Parser, sourceLen int) int64 {
	budget := parseMemoryBudget(sourceLen)
	if p == nil || !p.skipRecoveryReparse || p.language == nil {
		return budget
	}
	if p.language.Name != "c_sharp" {
		return budget
	}
	const csharpRecoveryBudget = int64(64 * 1024 * 1024)
	if budget == 0 || budget > csharpRecoveryBudget {
		return csharpRecoveryBudget
	}
	return budget
}

func parseFullArenaNodeCapacity(sourceLen, hint int) int {
	target := parseFullArenaInitialNodeCapacity(sourceLen)
	if hint <= 0 || hint < target {
		return target
	}
	limit := parseFullArenaHintLimit(sourceLen)
	if hint > limit {
		return max(target, limit)
	}
	return max(target, hint)
}

func parseFullArenaHintLimit(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if sourceLen <= 0 {
		return base
	}
	// Hints are learned from previous parses on the same Parser. In a ParserPool,
	// a parser that just handled a large file can later be checked out for a much
	// smaller file. Cap the reusable hint by the current source size so normal
	// concurrent full parses do not inherit a stale large-file preallocation.
	// sourceLen (bytes) is used directly as a loose upper bound on node count —
	// grammars produce well under 1 node per byte, so this is a conservative
	// ceiling, intentionally roomier than parseFullArenaInitialNodeCapacity
	// (sourceLen/4) so useful same-size hints fall between initial and limit.
	limit := sourceLen
	retainedFullNodes := nodeCapacityForBytes(maxRetainedFullNodeBytes)
	if limit > retainedFullNodes {
		limit = retainedFullNodes
	}
	return max(base, limit)
}

func parseFullArenaInitialNodeCapacity(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if sourceLen <= 0 {
		return base
	}
	// First-pass sizing when no adaptive hint exists yet. Empirically our Go
	// grammar consumes ~1 node per 5-10 input bytes, so sourceLen/4 gives
	// comfortable headroom without front-loading arena memory that never
	// becomes live. Any shortfall is absorbed by overflow slabs and the
	// adaptive hint on the next parse trims to observed peak + 25%.
	estimate := sourceLen / 4
	const maxPreallocNodes = 1_500_000
	if estimate > maxPreallocNodes {
		estimate = maxPreallocNodes
	}
	return max(base, estimate)
}

func (p *Parser) fullArenaHintCapacity() int {
	if p == nil {
		return 0
	}
	return int(atomic.LoadUint32(&p.fullArenaHint))
}

func (p *Parser) incrementalArenaHintCapacity() int {
	if p == nil {
		return 0
	}
	return int(atomic.LoadUint32(&p.incrementalArenaHint))
}

func (p *Parser) incrementalGSSHintCapacity() int {
	if p == nil {
		return defaultGSSNodeSlabCap
	}
	hint := int(atomic.LoadUint32(&p.incrementalGSSHint))
	if hint < defaultGSSNodeSlabCap {
		return defaultGSSNodeSlabCap
	}
	return hint
}

func (p *Parser) fullGSSHintCapacity() int {
	if p == nil {
		return fullParseGSSNodeSlabCap
	}
	hint := int(atomic.LoadUint32(&p.fullGSSHint))
	if hint < fullParseGSSNodeSlabCap {
		return fullParseGSSNodeSlabCap
	}
	return hint
}

func (p *Parser) recordFullArenaUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	target := used + used/4 // keep 25% headroom above observed peak.
	base := nodeCapacityForClass(arenaClassFull)
	if target < base {
		target = base
	}
	const maxHintNodes = 2_000_000
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(&p.fullArenaHint)
		var next uint32
		if old == 0 {
			next = uint32(target)
		} else {
			blended := (int(old)*3 + target) / 4
			if blended < base {
				blended = base
			}
			next = uint32(blended)
		}
		if old == next || atomic.CompareAndSwapUint32(&p.fullArenaHint, old, next) {
			return
		}
	}
}

func (p *Parser) recordIncrementalArenaUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	target := used + used/8 // keep 12.5% headroom above observed peak.
	base := nodeCapacityForClass(arenaClassIncremental)
	if target < base {
		target = base
	}
	const maxHintNodes = 1_000_000
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(&p.incrementalArenaHint)
		var next uint32
		if old == 0 {
			next = uint32(target)
		} else {
			blended := (int(old)*3 + target) / 4
			if blended < base {
				blended = base
			}
			next = uint32(blended)
		}
		if old == next || atomic.CompareAndSwapUint32(&p.incrementalArenaHint, old, next) {
			return
		}
	}
}

func (p *Parser) recordIncrementalGSSUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	target := used + used/8 // keep 12.5% headroom above observed peak.
	if target < defaultGSSNodeSlabCap {
		target = defaultGSSNodeSlabCap
	}
	const maxHintNodes = 512 * 1024
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(&p.incrementalGSSHint)
		var next uint32
		if old == 0 {
			next = uint32(target)
		} else {
			blended := (int(old)*3 + target) / 4
			if blended < defaultGSSNodeSlabCap {
				blended = defaultGSSNodeSlabCap
			}
			next = uint32(blended)
		}
		if old == next || atomic.CompareAndSwapUint32(&p.incrementalGSSHint, old, next) {
			return
		}
	}
}

func (p *Parser) recordFullGSSUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	target := used + used/4 // keep 25% headroom above observed peak.
	if target < fullParseGSSNodeSlabCap {
		target = fullParseGSSNodeSlabCap
	}
	const maxHintNodes = 1_024 * 1_024
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(&p.fullGSSHint)
		var next uint32
		if old == 0 {
			next = uint32(target)
		} else {
			blended := (int(old)*3 + target) / 4
			if blended < fullParseGSSNodeSlabCap {
				blended = fullParseGSSNodeSlabCap
			}
			next = uint32(blended)
		}
		if old == next || atomic.CompareAndSwapUint32(&p.fullGSSHint, old, next) {
			return
		}
	}
}

func parseFullEntryScratchCapacity(sourceLen int) int {
	if sourceLen <= 0 {
		return defaultStackEntrySlabCap
	}
	estimate := sourceLen * 12
	if estimate < defaultStackEntrySlabCap {
		estimate = defaultStackEntrySlabCap
	}
	// Keep initial scratch growth bounded; larger capacities are still
	// reached on demand and retained up to maxRetainedStackEntryCap.
	const maxPreallocEntries = 768 * 1024
	if estimate > maxPreallocEntries {
		estimate = maxPreallocEntries
	}
	return estimate
}

func tuneIncrementalGLRCaps(maxStacks, mergePerKeyCap int) (int, int) {
	if maxStacks > 2 {
		maxStacks = 2
	}
	if mergePerKeyCap > 2 {
		mergePerKeyCap = 2
	}
	return maxStacks, mergePerKeyCap
}

func parseIncrementalArenaNodeCapacity(sourceLen, hint int) int {
	base := nodeCapacityForClass(arenaClassIncremental)
	target := base
	if sourceLen > 0 {
		estimate := sourceLen * 4
		const maxPreallocNodes = 512 * 1024
		if estimate > maxPreallocNodes {
			estimate = maxPreallocNodes
		}
		target = max(base, estimate)
	}
	if hint <= 0 || hint < target {
		return target
	}
	limit := parseNodeLimit(sourceLen)
	if hint > limit {
		return max(base, limit)
	}
	return max(base, hint)
}

func parseIncrementalEntryScratchCapacity(sourceLen int) int {
	if sourceLen <= 0 {
		return defaultStackEntrySlabCap
	}
	estimate := sourceLen * 8
	if estimate < defaultStackEntrySlabCap {
		estimate = defaultStackEntrySlabCap
	}
	const maxPreallocEntries = 256 * 1024
	if estimate > maxPreallocEntries {
		estimate = maxPreallocEntries
	}
	return estimate
}
