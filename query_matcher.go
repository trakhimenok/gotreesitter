package gotreesitter

import "slices"

type queryChildStepInfo struct {
	stepIdx int
	field   FieldID
}

func (q *Query) matchStepsWithPredicates(steps []QueryStep, stepIdx int, node *Node, lang *Language, source []byte, predicates []QueryPredicate, captures *[]QueryCapture) bool {
	return q.matchStepsWithParentPredicates(steps, stepIdx, node, nil, -1, lang, source, predicates, captures)
}

func (q *Query) matchStepsWithParentPredicates(steps []QueryStep, stepIdx int, node *Node, parent *Node, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures *[]QueryCapture) bool {
	if stepIdx >= len(steps) {
		return false
	}

	step := &steps[stepIdx]

	if len(step.alternatives) > 0 {
		if !q.matchAlternationStep(step, node, parent, childIdx, lang, source, predicates, captures) {
			return false
		}
	} else {
		// Check if this node matches the current step.
		if !q.nodeMatchesStep(step, node, lang) {
			return false
		}
		q.appendCaptureIDs(step.captureIDs, step.captureID, node, captures)
		if !q.predicatesStillViable(predicates, *captures, source) {
			return false
		}
	}

	// Find child steps (steps at depth = step.depth + 1) that are direct
	// descendants of this step.
	childDepth := step.depth + 1
	childStart := stepIdx + 1

	// If there are no more steps, we matched successfully.
	if childStart >= len(steps) {
		return true
	}

	// If the next step is at the same depth or shallower, there are no
	// child constraints -- we matched.
	if steps[childStart].depth <= step.depth {
		return true
	}

	// Collect child step indices at childDepth (stop when we see a step
	// at a depth <= step.depth, meaning it belongs to a sibling/ancestor).
	var childStepsBuf [16]queryChildStepInfo
	childSteps := childStepsBuf[:0]
	for i := childStart; i < len(steps); i++ {
		if steps[i].depth <= step.depth {
			break
		}
		if steps[i].depth == childDepth {
			childSteps = append(childSteps, queryChildStepInfo{
				stepIdx: i,
				field:   steps[i].field,
			})
		}
	}
	return q.matchChildSteps(node, steps, childSteps, lang, source, predicates, captures)
}

func (q *Query) appendCaptureIDs(ids []int, legacyID int, node *Node, captures *[]QueryCapture) {
	if len(ids) > 0 {
		if len(q.disabledCaptureName) == 0 {
			start := len(*captures)
			*captures = slices.Grow(*captures, len(ids))
			expanded := (*captures)[:start+len(ids)]
			for i, captureID := range ids {
				expanded[start+i] = QueryCapture{
					Name: q.captures[captureID],
					Node: node,
				}
			}
			*captures = expanded
			return
		}
		for _, captureID := range ids {
			if q.isCaptureDisabled(q.captures[captureID]) {
				continue
			}
			*captures = append(*captures, QueryCapture{
				Name: q.captures[captureID],
				Node: node,
			})
		}
		return
	}
	if legacyID >= 0 {
		if len(q.disabledCaptureName) == 0 {
			*captures = append(*captures, QueryCapture{
				Name: q.captures[legacyID],
				Node: node,
			})
			return
		}
		if q.isCaptureDisabled(q.captures[legacyID]) {
			return
		}
		*captures = append(*captures, QueryCapture{
			Name: q.captures[legacyID],
			Node: node,
		})
	}
}

func cloneQueryCaptures(captures []QueryCapture) []QueryCapture {
	if len(captures) == 0 {
		return nil
	}
	out := make([]QueryCapture, len(captures))
	copy(out, captures)
	return out
}

func (q *Query) matchPatternAll(pat *Pattern, node *Node, lang *Language, source []byte) [][]QueryCapture {
	if len(pat.steps) == 0 {
		return nil
	}
	if pat.steps[0].quantifier == queryQuantifierZeroOrMore {
		return q.matchRootRepetitionPatternAll(pat, node, lang, source)
	}
	if pat.steps[0].quantifier == queryQuantifierOneOrMore {
		return nil
	}

	var matches [][]QueryCapture
	q.matchStepsAllWithParentPredicates(pat.steps, 0, node, nil, -1, lang, source, pat.predicates, nil, func(captures []QueryCapture) {
		if !q.matchesPredicates(pat.predicates, captures, lang, source) {
			return
		}
		captures = q.applyDirectives(pat.predicates, captures, source)
		matches = append(matches, cloneQueryCaptures(captures))
	})
	return matches
}

func (q *Query) matchRootRepetitionPatternAll(pat *Pattern, node *Node, lang *Language, source []byte) [][]QueryCapture {
	if node == nil {
		return nil
	}
	if pat.steps[0].quantifier == queryQuantifierZeroOrMore {
		return q.matchRootZeroOrMorePatternAll(pat, node, lang, source)
	}

	parent := node.Parent()
	childIdx := nodeChildIndexInParent(node, parent)
	if prev := node.PrevSibling(); prev != nil {
		prevParent := prev.Parent()
		prevChildIdx := nodeChildIndexInParent(prev, prevParent)
		if len(q.matchPatternOnceAll(pat, prev, prevParent, prevChildIdx, lang, source, nil)) > 0 {
			return nil
		}
	}

	partials := [][]QueryCapture{nil}
	count := 0
	for current := node; current != nil; current = current.NextSibling() {
		currentParent := parent
		currentChildIdx := childIdx
		if current != node {
			currentParent = current.Parent()
			currentChildIdx = nodeChildIndexInParent(current, currentParent)
		}

		var nextPartials [][]QueryCapture
		for _, captures := range partials {
			for _, next := range q.matchPatternOnceAll(pat, current, currentParent, currentChildIdx, lang, source, captures) {
				nextPartials = append(nextPartials, next)
			}
		}
		if len(nextPartials) == 0 {
			break
		}
		count++
		partials = nextPartials
		childIdx++
	}
	if count == 0 {
		return nil
	}

	var matches [][]QueryCapture
	for _, captures := range partials {
		if !q.matchesPredicates(pat.predicates, captures, lang, source) {
			continue
		}
		captures = q.applyDirectives(pat.predicates, captures, source)
		matches = append(matches, cloneQueryCaptures(captures))
	}
	return matches
}

func (q *Query) matchRootZeroOrMorePatternAll(pat *Pattern, node *Node, lang *Language, source []byte) [][]QueryCapture {
	if q.matchesPredicates(pat.predicates, nil, lang, source) {
		captures := q.applyDirectives(pat.predicates, nil, source)
		return [][]QueryCapture{cloneQueryCaptures(captures)}
	}
	return nil
}

func (q *Query) matchPatternPostorderAll(pat *Pattern, node *Node, lang *Language, source []byte) [][]QueryCapture {
	if len(pat.steps) == 0 {
		return nil
	}
	switch pat.steps[0].quantifier {
	case queryQuantifierZeroOrMore, queryQuantifierOneOrMore:
	default:
		return nil
	}
	parent := node.Parent()
	childIdx := nodeChildIndexInParent(node, parent)
	if len(q.matchPatternOnceAll(pat, node, parent, childIdx, lang, source, nil)) == 0 {
		return nil
	}
	if next := node.NextSibling(); next != nil {
		nextParent := next.Parent()
		nextChildIdx := nodeChildIndexInParent(next, nextParent)
		if len(q.matchPatternOnceAll(pat, next, nextParent, nextChildIdx, lang, source, nil)) > 0 {
			return nil
		}
	}

	runStart := node
	for prev := node.PrevSibling(); prev != nil; prev = prev.PrevSibling() {
		prevParent := prev.Parent()
		prevChildIdx := nodeChildIndexInParent(prev, prevParent)
		if len(q.matchPatternOnceAll(pat, prev, prevParent, prevChildIdx, lang, source, nil)) == 0 {
			break
		}
		runStart = prev
	}

	partials := [][]QueryCapture{nil}
	for current := runStart; current != nil; current = current.NextSibling() {
		currentParent := current.Parent()
		currentChildIdx := nodeChildIndexInParent(current, currentParent)

		var nextPartials [][]QueryCapture
		for _, captures := range partials {
			for _, next := range q.matchPatternOnceAll(pat, current, currentParent, currentChildIdx, lang, source, captures) {
				nextPartials = append(nextPartials, next)
			}
		}
		if len(nextPartials) == 0 {
			break
		}
		partials = nextPartials
		if current == node {
			break
		}
	}

	var matches [][]QueryCapture
	for _, captures := range partials {
		if !q.matchesPredicates(pat.predicates, captures, lang, source) {
			continue
		}
		captures = q.applyDirectives(pat.predicates, captures, source)
		matches = append(matches, cloneQueryCaptures(captures))
	}
	return matches
}

func (q *Query) matchPatternOnceAll(
	pat *Pattern,
	node *Node,
	parent *Node,
	childIdx int,
	lang *Language,
	source []byte,
	captures []QueryCapture,
) [][]QueryCapture {
	var matches [][]QueryCapture
	q.matchStepsAllWithParentPredicates(pat.steps, 0, node, parent, childIdx, lang, source, pat.predicates, captures, func(next []QueryCapture) {
		matches = append(matches, cloneQueryCaptures(next))
	})
	return matches
}

func nodeChildIndexInParent(node *Node, parent *Node) int {
	if node == nil || parent == nil {
		return -1
	}
	if idx := int(node.childIndex); idx >= 0 && idx < len(parent.children) && parent.children[idx] == node {
		return idx
	}
	for i, child := range parent.children {
		if child == node {
			return i
		}
	}
	return -1
}

func (q *Query) matchStepsAllWithParentPredicates(
	steps []QueryStep,
	stepIdx int,
	node *Node,
	parent *Node,
	childIdx int,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures []QueryCapture,
	emit func([]QueryCapture),
) {
	if stepIdx >= len(steps) || node == nil {
		return
	}

	step := &steps[stepIdx]
	if len(step.alternatives) > 0 {
		q.matchAlternationStepAll(step, node, parent, childIdx, lang, source, predicates, captures, func(next []QueryCapture) {
			q.matchStepChildrenAll(steps, stepIdx, node, lang, source, predicates, next, emit)
		})
		return
	}

	if !q.nodeMatchesStep(step, node, lang) {
		return
	}
	next := cloneQueryCaptures(captures)
	q.appendCaptureIDs(step.captureIDs, step.captureID, node, &next)
	if !q.predicatesStillViable(predicates, next, source) {
		return
	}
	q.matchStepChildrenAll(steps, stepIdx, node, lang, source, predicates, next, emit)
}

func (q *Query) matchStepChildrenAll(
	steps []QueryStep,
	stepIdx int,
	node *Node,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures []QueryCapture,
	emit func([]QueryCapture),
) {
	step := &steps[stepIdx]
	childDepth := step.depth + 1
	childStart := stepIdx + 1

	if childStart >= len(steps) || steps[childStart].depth <= step.depth {
		emit(captures)
		return
	}

	var childStepsBuf [16]queryChildStepInfo
	childSteps := childStepsBuf[:0]
	for i := childStart; i < len(steps); i++ {
		if steps[i].depth <= step.depth {
			break
		}
		if steps[i].depth == childDepth {
			childSteps = append(childSteps, queryChildStepInfo{
				stepIdx: i,
				field:   steps[i].field,
			})
		}
	}
	q.matchChildStepsAll(node, steps, childSteps, lang, source, predicates, captures, emit)
}

func (q *Query) matchAlternationStepAll(
	step *QueryStep,
	node *Node,
	parent *Node,
	childIdx int,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures []QueryCapture,
	emit func([]QueryCapture),
) {
	hasStepCaptures := len(step.captureIDs) > 0 || step.captureID >= 0
	nodeNamed := node.IsNamed()
	nodeSymbol := lang.PublicSymbolForNamedness(node.Symbol(), nodeNamed)
	var nodeType string
	nodeTypeLoaded := false

	for i := range step.alternatives {
		alt := &step.alternatives[i]
		if !alternativeMatchesNodeCached(*alt, node, lang, nodeSymbol, nodeNamed, &nodeType, &nodeTypeLoaded) {
			continue
		}
		if !q.alternativeFieldMatches(alt, node, parent, childIdx, lang) {
			continue
		}

		next := cloneQueryCaptures(captures)
		if q.matchAlternationBranch(step, alt, node, lang, source, predicates, &next, hasStepCaptures) {
			emit(next)
		}
	}
}

func (q *Query) matchChildStepsAll(
	parent *Node,
	steps []QueryStep,
	childSteps []queryChildStepInfo,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures []QueryCapture,
	emit func([]QueryCapture),
) {
	children := parent.Children()
	var namedPosByIndexBuf [64]int
	var namedPosByIndex []int
	if len(children) <= len(namedPosByIndexBuf) {
		namedPosByIndex = namedPosByIndexBuf[:len(children)]
	} else {
		namedPosByIndex = make([]int, len(children))
	}
	namedPos := 0
	for i, child := range children {
		if child != nil && child.IsNamed() {
			namedPosByIndex[i] = namedPos
			namedPos++
		} else {
			namedPosByIndex[i] = -1
		}
	}

	q.matchChildStepsRecursiveAll(
		parent, children, namedPosByIndex, namedPos-1,
		steps, childSteps, 0, 0, false, -1,
		lang, source, predicates, captures, emit,
	)
}

func (q *Query) matchChildStepsRecursiveAll(
	parent *Node,
	children []*Node,
	namedPosByIndex []int,
	parentLastNamedPos int,
	steps []QueryStep,
	childSteps []queryChildStepInfo,
	childPos int,
	nextChildIdx int,
	prevHasNamed bool,
	prevLastNamedPos int,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures []QueryCapture,
	emit func([]QueryCapture),
) {
	if childPos >= len(childSteps) {
		emit(captures)
		return
	}

	cs := childSteps[childPos]
	step := &steps[cs.stepIdx]
	minCount, maxCount, ok := quantifierBounds(step.quantifier)
	if !ok {
		return
	}

	var candidateIndicesBuf [32]int
	candidateIndices := candidateIndicesBuf[:0]
	var candidatesOK bool
	candidateIndices, candidatesOK = q.collectChildCandidateIndices(parent, children, step, cs.field, nextChildIdx, lang, candidateIndices)
	if !candidatesOK {
		return
	}

	if maxCount < 0 || maxCount > len(candidateIndices) {
		maxCount = len(candidateIndices)
	}
	if minCount > len(candidateIndices) {
		return
	}

	for count := maxCount; count >= minCount; count-- {
		emittedForCount := false
		emitForCount := emit
		if step.quantifier != queryQuantifierOne {
			emitForCount = func(captures []QueryCapture) {
				emittedForCount = true
				emit(captures)
			}
		}

		var tryCombinations func(
			candidatePos int,
			chosen int,
			nextIdx int,
			hasNamed bool,
			firstNamedPos int,
			lastNamedPos int,
			current []QueryCapture,
		)

		tryCombinations = func(
			candidatePos int,
			chosen int,
			nextIdx int,
			hasNamed bool,
			firstNamedPos int,
			lastNamedPos int,
			current []QueryCapture,
		) {
			if chosen == count {
				if count > 0 && !q.stepAnchorsSatisfied(
					step, childPos, hasNamed, firstNamedPos, lastNamedPos,
					prevHasNamed, prevLastNamedPos, parentLastNamedPos,
				) {
					return
				}
				nextPrevHasNamed := prevHasNamed || hasNamed
				nextPrevLastNamedPos := prevLastNamedPos
				if hasNamed {
					nextPrevLastNamedPos = lastNamedPos
				}
				q.matchChildStepsRecursiveAll(
					parent, children, namedPosByIndex, parentLastNamedPos,
					steps, childSteps, childPos+1, nextIdx, nextPrevHasNamed, nextPrevLastNamedPos,
					lang, source, predicates, current, emitForCount,
				)
				return
			}

			remaining := count - chosen
			limit := len(candidateIndices) - remaining
			for i := candidatePos; i <= limit; i++ {
				childIdx := candidateIndices[i]
				child := children[childIdx]

				nextIdxForChoice := nextIdx
				if childIdx+1 > nextIdxForChoice {
					nextIdxForChoice = childIdx + 1
				}

				hasNamedForChoice := hasNamed
				firstNamedForChoice := firstNamedPos
				lastNamedForChoice := lastNamedPos
				if namedPos := namedPosByIndex[childIdx]; namedPos >= 0 {
					if !hasNamedForChoice {
						hasNamedForChoice = true
						firstNamedForChoice = namedPos
					}
					lastNamedForChoice = namedPos
				}

				q.matchStepsAllWithParentPredicates(
					steps, cs.stepIdx, child, parent, childIdx, lang, source, predicates, current,
					func(next []QueryCapture) {
						tryCombinations(
							i+1, chosen+1, nextIdxForChoice,
							hasNamedForChoice, firstNamedForChoice, lastNamedForChoice,
							next,
						)
					},
				)
			}
		}

		tryCombinations(0, 0, nextChildIdx, false, -1, -1, captures)
		if step.quantifier != queryQuantifierOne && emittedForCount {
			return
		}
	}
}

func (q *Query) collectChildCandidateIndices(
	parent *Node,
	children []*Node,
	step *QueryStep,
	field FieldID,
	nextChildIdx int,
	lang *Language,
	dst []int,
) ([]int, bool) {
	fieldName := ""
	if field != 0 {
		if int(field) <= 0 || int(field) >= len(lang.FieldNames) {
			return dst, false
		}
		fieldName = lang.FieldNames[field]
		if fieldName == "" {
			return dst, false
		}
	}

	contiguousRun := step.quantifier == queryQuantifierZeroOrMore || step.quantifier == queryQuantifierOneOrMore
	startedRun := false
	for i := nextChildIdx; i < len(children); i++ {
		child := children[i]
		if child == nil {
			continue
		}

		fieldMatches := true
		if fieldName != "" {
			fieldMatches = parent.FieldNameForChild(i, lang) == fieldName
		}
		if fieldMatches && q.nodeMatchesStep(step, child, lang) {
			dst = append(dst, i)
			startedRun = true
			continue
		}

		if contiguousRun && startedRun {
			break
		}
	}
	return dst, true
}

func quantifierBounds(quantifier queryQuantifier) (int, int, bool) {
	switch quantifier {
	case queryQuantifierOne:
		return 1, 1, true
	case queryQuantifierZeroOrOne:
		return 0, 1, true
	case queryQuantifierZeroOrMore:
		return 0, -1, true
	case queryQuantifierOneOrMore:
		return 1, -1, true
	default:
		return 0, 0, false
	}
}

func (q *Query) stepAnchorsSatisfied(
	step *QueryStep,
	childPos int,
	hasNamed bool,
	firstNamedPos int,
	lastNamedPos int,
	priorHasNamed bool,
	priorLastNamedPos int,
	parentLastNamedPos int,
) bool {
	if step.anchorBefore {
		if !hasNamed {
			return false
		}
		if childPos == 0 {
			if firstNamedPos != 0 {
				return false
			}
		} else {
			if !priorHasNamed {
				if firstNamedPos != 0 {
					return false
				}
			} else if firstNamedPos != priorLastNamedPos+1 {
				return false
			}
		}
	}

	if step.anchorAfter {
		if !hasNamed {
			return false
		}
		if lastNamedPos != parentLastNamedPos {
			return false
		}
	}

	return true
}

func (q *Query) matchChildSteps(
	parent *Node,
	steps []QueryStep,
	childSteps []queryChildStepInfo,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures *[]QueryCapture,
) bool {
	children := parent.Children()
	var namedPosByIndexBuf [64]int
	var namedPosByIndex []int
	if len(children) <= len(namedPosByIndexBuf) {
		namedPosByIndex = namedPosByIndexBuf[:len(children)]
	} else {
		namedPosByIndex = make([]int, len(children))
	}
	namedPos := 0
	for i, child := range children {
		if child != nil && child.IsNamed() {
			namedPosByIndex[i] = namedPos
			namedPos++
		} else {
			namedPosByIndex[i] = -1
		}
	}
	parentLastNamedPos := namedPos - 1

	return q.matchChildStepsRecursive(
		parent, children, namedPosByIndex, parentLastNamedPos,
		steps, childSteps, 0, 0, false, -1,
		lang, source, predicates, captures,
	)
}

func (q *Query) matchChildStepsRecursive(
	parent *Node,
	children []*Node,
	namedPosByIndex []int,
	parentLastNamedPos int,
	steps []QueryStep,
	childSteps []queryChildStepInfo,
	childPos int,
	nextChildIdx int,
	prevHasNamed bool,
	prevLastNamedPos int,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures *[]QueryCapture,
) bool {
	if childPos >= len(childSteps) {
		return true
	}

	cs := childSteps[childPos]
	step := &steps[cs.stepIdx]
	minCount, maxCount, ok := quantifierBounds(step.quantifier)
	if !ok {
		return false
	}

	var candidateIndicesBuf [32]int
	candidateIndices := candidateIndicesBuf[:0]
	var candidatesOK bool
	candidateIndices, candidatesOK = q.collectChildCandidateIndices(parent, children, step, cs.field, nextChildIdx, lang, candidateIndices)
	if !candidatesOK {
		return false
	}

	if maxCount < 0 || maxCount > len(candidateIndices) {
		maxCount = len(candidateIndices)
	}
	if minCount > len(candidateIndices) {
		return false
	}

	// Final child steps without match predicates intentionally aggregate
	// structurally matching siblings into one match; predicate-bearing patterns
	// need normal backtracking so invalid candidates do not leak captures or
	// block later ones.
	if !predicatesCanRejectMatch(predicates) && childPos == len(childSteps)-1 && minCount == 1 && maxCount == 1 && !step.anchorBefore && !step.anchorAfter {
		any := false
		checkpoint := len(*captures)
		for _, childIdx := range candidateIndices {
			child := children[childIdx]
			childCheckpoint := len(*captures)
			if !q.matchStepWithRollbackAtParentPredicates(steps, cs.stepIdx, child, parent, childIdx, lang, source, nil, captures) {
				*captures = (*captures)[:childCheckpoint]
				continue
			}
			hasNamed := false
			firstNamedPos := -1
			lastNamedPos := -1
			if namedPos := namedPosByIndex[childIdx]; namedPos >= 0 {
				hasNamed = true
				firstNamedPos = namedPos
				lastNamedPos = namedPos
			}
			if !q.stepAnchorsSatisfied(
				step, childPos, hasNamed, firstNamedPos, lastNamedPos,
				prevHasNamed, prevLastNamedPos, parentLastNamedPos,
			) {
				*captures = (*captures)[:childCheckpoint]
				continue
			}
			any = true
		}
		if any {
			return true
		}
		*captures = (*captures)[:checkpoint]
		return false
	}

	// Greedy-first for consistency with prior quantifier behavior; backtrack as needed.
	for count := maxCount; count >= minCount; count-- {
		checkpoint := len(*captures)
		var tryCombinations func(
			candidatePos int,
			chosen int,
			nextIdx int,
			hasNamed bool,
			firstNamedPos int,
			lastNamedPos int,
		) bool

		tryCombinations = func(
			candidatePos int,
			chosen int,
			nextIdx int,
			hasNamed bool,
			firstNamedPos int,
			lastNamedPos int,
		) bool {
			if chosen == count {
				if !q.stepAnchorsSatisfied(
					step, childPos, hasNamed, firstNamedPos, lastNamedPos,
					prevHasNamed, prevLastNamedPos, parentLastNamedPos,
				) {
					return false
				}
				nextPrevHasNamed := prevHasNamed || hasNamed
				nextPrevLastNamedPos := prevLastNamedPos
				if hasNamed {
					nextPrevLastNamedPos = lastNamedPos
				}
				return q.matchChildStepsRecursive(
					parent, children, namedPosByIndex, parentLastNamedPos,
					steps, childSteps, childPos+1, nextIdx, nextPrevHasNamed, nextPrevLastNamedPos,
					lang, source, predicates, captures,
				)
			}

			remaining := count - chosen
			limit := len(candidateIndices) - remaining
			for i := candidatePos; i <= limit; i++ {
				childIdx := candidateIndices[i]
				child := children[childIdx]

				childCheckpoint := len(*captures)
				if !q.matchStepWithRollbackAtParentPredicates(steps, cs.stepIdx, child, parent, childIdx, lang, source, predicates, captures) {
					continue
				}

				nextIdxForChoice := nextIdx
				if childIdx+1 > nextIdxForChoice {
					nextIdxForChoice = childIdx + 1
				}

				hasNamedForChoice := hasNamed
				firstNamedForChoice := firstNamedPos
				lastNamedForChoice := lastNamedPos
				if namedPos := namedPosByIndex[childIdx]; namedPos >= 0 {
					if !hasNamedForChoice {
						hasNamedForChoice = true
						firstNamedForChoice = namedPos
					}
					lastNamedForChoice = namedPos
				}

				if tryCombinations(
					i+1, chosen+1, nextIdxForChoice,
					hasNamedForChoice, firstNamedForChoice, lastNamedForChoice,
				) {
					return true
				}

				*captures = (*captures)[:childCheckpoint]
			}

			return false
		}

		if tryCombinations(0, 0, nextChildIdx, false, -1, -1) {
			return true
		}

		*captures = (*captures)[:checkpoint]
	}

	return false
}

func (q *Query) matchAlternationStep(step *QueryStep, node *Node, parent *Node, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures *[]QueryCapture) bool {
	hasStepCaptures := len(step.captureIDs) > 0 || step.captureID >= 0
	nodeNamed := node.IsNamed()
	nodeSymbol := lang.PublicSymbolForNamedness(node.Symbol(), nodeNamed)
	var nodeType string
	nodeTypeLoaded := false

	if idx := step.altIndex; idx != nil {
		key := alternationSymbolNamedKey(nodeSymbol, nodeNamed)
		symbolMatches := idx.bySymbolNamed[key]
		var textMatches []int
		if !nodeNamed && len(idx.byText) > 0 {
			nodeType = node.Type(lang)
			nodeTypeLoaded = true
			textMatches = idx.byText[nodeType]
		}
		wildcardMatches := idx.wildcard
		if len(symbolMatches) == 0 && len(textMatches) == 0 && len(wildcardMatches) == 0 {
			return false
		}

		iSym, iText, iWild := 0, 0, 0
		for {
			nextSrc := 0
			nextAlt := -1
			if iSym < len(symbolMatches) {
				nextSrc = 1
				nextAlt = symbolMatches[iSym]
			}
			if iText < len(textMatches) && (nextAlt < 0 || textMatches[iText] < nextAlt) {
				nextSrc = 2
				nextAlt = textMatches[iText]
			}
			if iWild < len(wildcardMatches) && (nextAlt < 0 || wildcardMatches[iWild] < nextAlt) {
				nextSrc = 3
				nextAlt = wildcardMatches[iWild]
			}
			if nextAlt < 0 {
				break
			}

			alt := &step.alternatives[nextAlt]
			if !q.alternativeFieldMatches(alt, node, parent, childIdx, lang) {
				switch nextSrc {
				case 1:
					iSym++
				case 2:
					iText++
				case 3:
					iWild++
				}
				continue
			}

			if q.matchAlternationBranch(step, alt, node, lang, source, predicates, captures, hasStepCaptures) {
				return true
			}

			switch nextSrc {
			case 1:
				iSym++
			case 2:
				iText++
			case 3:
				iWild++
			}
		}
		return false
	}

	for _, alt := range step.alternatives {
		if !alternativeMatchesNodeCached(alt, node, lang, nodeSymbol, nodeNamed, &nodeType, &nodeTypeLoaded) {
			continue
		}
		if !q.alternativeFieldMatches(&alt, node, parent, childIdx, lang) {
			continue
		}
		if q.matchAlternationBranch(step, &alt, node, lang, source, predicates, captures, hasStepCaptures) {
			return true
		}
	}
	return false
}

func (q *Query) alternativeFieldMatches(alt *alternativeSymbol, node *Node, parent *Node, childIdx int, lang *Language) bool {
	if alt == nil || alt.field == 0 {
		return true
	}
	if parent == nil || childIdx < 0 {
		// Root-level field-constrained patterns (for example `field: (node)`) are
		// matched against each candidate node and must resolve the real parent
		// relationship at match time.
		parent = node.Parent()
		if parent == nil {
			return false
		}
		childIdx = -1
		for i, child := range parent.children {
			if child == node {
				childIdx = i
				break
			}
		}
		if childIdx < 0 {
			return false
		}
	}
	if int(alt.field) <= 0 || int(alt.field) >= len(lang.FieldNames) {
		return false
	}
	fieldName := lang.FieldNames[alt.field]
	if fieldName == "" {
		return false
	}
	return parent.FieldNameForChild(childIdx, lang) == fieldName
}

func (q *Query) matchAlternationBranch(
	step *QueryStep,
	alt *alternativeSymbol,
	node *Node,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures *[]QueryCapture,
	hasStepCaptures bool,
) bool {
	if len(alt.steps) > 0 {
		// Fast path: no alternation-level captures and no branch predicates.
		// The predicate-aware rollback path protects captures from failed branches.
		if !hasStepCaptures && len(alt.predicates) == 0 {
			return q.matchStepWithRollbackPredicates(alt.steps, 0, node, lang, source, predicates, captures)
		}

		checkpoint := len(*captures)
		if hasStepCaptures {
			// Captures on the alternation itself apply regardless of chosen branch.
			q.appendCaptureIDs(step.captureIDs, step.captureID, node, captures)
			if !q.predicatesStillViable(predicates, *captures, source) {
				*captures = (*captures)[:checkpoint]
				return false
			}
		}
		if !q.matchStepWithRollbackPredicates(alt.steps, 0, node, lang, source, predicates, captures) {
			*captures = (*captures)[:checkpoint]
			return false
		}
		if len(alt.predicates) > 0 && !q.matchesPredicates(alt.predicates, *captures, lang, source) {
			*captures = (*captures)[:checkpoint]
			return false
		}
		return true
	}

	// Simple alternation branch captures (no nested structure).
	if !hasStepCaptures && len(alt.captureIDs) == 0 && alt.captureID < 0 {
		return true
	}
	checkpoint := len(*captures)
	if hasStepCaptures {
		q.appendCaptureIDs(step.captureIDs, step.captureID, node, captures)
		if !q.predicatesStillViable(predicates, *captures, source) {
			*captures = (*captures)[:checkpoint]
			return false
		}
	}
	q.appendCaptureIDs(alt.captureIDs, alt.captureID, node, captures)
	if !q.predicatesStillViable(predicates, *captures, source) {
		*captures = (*captures)[:checkpoint]
		return false
	}
	return true
}

// nodeMatchesStep checks if a single node matches a single step's type/symbol constraint.
func (q *Query) nodeMatchesStep(step *QueryStep, node *Node, lang *Language) bool {
	// Alternation matching.
	if len(step.alternatives) > 0 {
		if idx := step.altIndex; idx != nil {
			if len(idx.wildcard) > 0 {
				return true
			}
			nodeNamed := node.IsNamed()
			nodeSymbol := lang.PublicSymbolForNamedness(node.Symbol(), nodeNamed)
			if len(idx.bySymbolNamed[alternationSymbolNamedKey(nodeSymbol, nodeNamed)]) > 0 {
				return true
			}
			if !node.IsNamed() && len(idx.byText) > 0 {
				if len(idx.byText[node.Type(lang)]) > 0 {
					return true
				}
			}
			return false
		}
		for _, alt := range step.alternatives {
			if alternativeMatchesNode(alt, node, lang) {
				return true
			}
		}
		return false
	}

	// Text matching for string literals like "func".
	if step.textMatch != "" {
		return !node.IsNamed() && node.Type(lang) == step.textMatch
	}

	// Wildcard (symbol == 0 and no textMatch and no alternatives).
	if step.symbol == 0 {
		return !step.isNamed || node.IsNamed()
	}

	nodeNamed := node.IsNamed()
	if nodeNamed != step.isNamed {
		return false
	}

	// Symbol matching uses the public symbol with namedness to handle aliases
	// without conflating anonymous tokens and named nodes that share text.
	if lang.PublicSymbolForNamedness(node.Symbol(), nodeNamed) != lang.PublicSymbolForNamedness(step.symbol, step.isNamed) {
		return false
	}

	// Field-negation constraints: each listed field must be absent.
	for _, fid := range step.absentFields {
		if int(fid) <= 0 || int(fid) >= len(lang.FieldNames) {
			return false
		}
		fieldName := lang.FieldNames[fid]
		if fieldName == "" {
			return false
		}
		if node.ChildByFieldName(fieldName, lang) != nil {
			return false
		}
	}

	return true
}

func alternativeMatchesNode(alt alternativeSymbol, node *Node, lang *Language) bool {
	// Wildcard in alternation `( _ )` should match any node.
	if alt.symbol == 0 && alt.textMatch == "" {
		return !alt.isNamed || node.IsNamed()
	}

	if alt.textMatch != "" {
		// String match for anonymous nodes.
		return !node.IsNamed() && node.Type(lang) == alt.textMatch
	}

	nodeNamed := node.IsNamed()
	return nodeNamed == alt.isNamed &&
		lang.PublicSymbolForNamedness(node.Symbol(), nodeNamed) == lang.PublicSymbolForNamedness(alt.symbol, alt.isNamed)
}

func alternativeMatchesNodeCached(
	alt alternativeSymbol,
	node *Node,
	lang *Language,
	nodeSymbol Symbol,
	nodeNamed bool,
	nodeType *string,
	nodeTypeLoaded *bool,
) bool {
	// Wildcard in alternation `( _ )` should match any node.
	if alt.symbol == 0 && alt.textMatch == "" {
		return !alt.isNamed || nodeNamed
	}

	if alt.textMatch != "" {
		// String match for anonymous nodes.
		if nodeNamed {
			return false
		}
		if !*nodeTypeLoaded {
			*nodeType = node.Type(lang)
			*nodeTypeLoaded = true
		}
		return *nodeType == alt.textMatch
	}

	return nodeNamed == alt.isNamed && nodeSymbol == lang.PublicSymbolForNamedness(alt.symbol, alt.isNamed)
}
