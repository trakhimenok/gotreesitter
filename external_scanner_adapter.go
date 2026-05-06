package gotreesitter

// externalScannerOrderAdapter adapts an external scanner from one language to
// another by remapping external token order and scanner result symbols.
//
// It is intended for parity scenarios where two languages share scanner logic
// but use different symbol IDs (or external symbol names/aliases).
type externalScannerOrderAdapter struct {
	inner                ExternalScanner
	targetToSource       []int // len == len(targetExt)
	sourceCount          int   // len(sourceExt), for sizing sourceValid in Scan
	sourceResultToTarget map[Symbol]Symbol
}

type externalScannerOrderAdapterPayload struct {
	inner       any
	sourceValid []bool
}

func externalSymbolName(lang *Language, sym Symbol) string {
	if lang == nil {
		return ""
	}
	if int(sym) >= 0 && int(sym) < len(lang.SymbolNames) {
		return lang.SymbolNames[sym]
	}
	return ""
}

func hasDuplicateExternalNames(lang *Language, externals []Symbol) bool {
	seen := make(map[string]bool, len(externals))
	for _, sym := range externals {
		name := externalSymbolName(lang, sym)
		if seen[name] {
			return true
		}
		seen[name] = true
	}
	return false
}

func (a *externalScannerOrderAdapter) Create() any {
	return &externalScannerOrderAdapterPayload{
		inner:       a.inner.Create(),
		sourceValid: make([]bool, a.sourceCount),
	}
}

func (a *externalScannerOrderAdapter) Destroy(payload any) {
	a.inner.Destroy(a.innerPayload(payload))
}

func (a *externalScannerOrderAdapter) Serialize(payload any, buf []byte) int {
	return a.inner.Serialize(a.innerPayload(payload), buf)
}

func (a *externalScannerOrderAdapter) Deserialize(payload any, buf []byte) {
	a.inner.Deserialize(a.innerPayload(payload), buf)
}

func (a *externalScannerOrderAdapter) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	if a == nil || a.inner == nil {
		return false
	}

	adapterPayload, _ := payload.(*externalScannerOrderAdapterPayload)
	var sourceValid []bool
	innerPayload := payload
	if adapterPayload != nil {
		innerPayload = adapterPayload.inner
		if cap(adapterPayload.sourceValid) < a.sourceCount {
			adapterPayload.sourceValid = make([]bool, a.sourceCount)
		}
		sourceValid = adapterPayload.sourceValid[:a.sourceCount]
		for i := range sourceValid {
			sourceValid[i] = false
		}
	} else {
		sourceValid = make([]bool, a.sourceCount)
	}
	for targetIdx, isValid := range validSymbols {
		if !isValid || targetIdx < 0 || targetIdx >= len(a.targetToSource) {
			continue
		}
		sourceIdx := a.targetToSource[targetIdx]
		if sourceIdx >= 0 && sourceIdx < len(sourceValid) {
			sourceValid[sourceIdx] = true
		}
	}

	ok := a.inner.Scan(innerPayload, lexer, sourceValid)
	if !ok {
		return false
	}

	// Map scanner result symbol from source language ID to target language ID.
	if lexer != nil && lexer.hasResult {
		if mapped, exists := a.sourceResultToTarget[lexer.resultSymbol]; exists {
			lexer.resultSymbol = mapped
		}
	}
	return true
}

func (a *externalScannerOrderAdapter) innerPayload(payload any) any {
	if adapterPayload, ok := payload.(*externalScannerOrderAdapterPayload); ok {
		return adapterPayload.inner
	}
	return payload
}

// AdaptExternalScannerByExternalOrder builds an ExternalScanner adapter that
// reuses sourceLang's scanner for targetLang by remapping external symbols.
//
// Mapping strategy:
//  1. If either side has duplicate external names, use index mapping
//     (capped to the shorter list length).
//  2. Otherwise, prefer exact external-symbol-name matches.
//  3. Fill remaining slots by index order (within the shorter dimension).
//
// When source and target have different external symbol counts, name-based
// matching pairs tokens that exist in both grammars. Target externals with no
// source match get -1 (the scanner will never produce them). Source externals
// with no target match are silently ignored.
//
// Returns (nil, false) when adaptation is not possible.
func AdaptExternalScannerByExternalOrder(sourceLang, targetLang *Language) (ExternalScanner, bool) {
	if sourceLang == nil || targetLang == nil || sourceLang.ExternalScanner == nil {
		return nil, false
	}

	sourceExt := sourceLang.ExternalSymbols
	targetExt := targetLang.ExternalSymbols
	if len(sourceExt) == 0 || len(targetExt) == 0 {
		return nil, false
	}

	nSource := len(sourceExt)
	nTarget := len(targetExt)
	targetToSource := make([]int, nTarget)
	usedSource := make([]bool, nSource)
	for i := range targetToSource {
		targetToSource[i] = -1
	}

	// Index-only mapping is only safe when both sides have the same count
	// AND there are duplicate names that make name matching ambiguous.
	// When counts differ, name-based matching (which consumes duplicates
	// in order) is always more correct than positional mapping.
	useIndexOnly := nSource == nTarget &&
		(hasDuplicateExternalNames(sourceLang, sourceExt) ||
			hasDuplicateExternalNames(targetLang, targetExt))

	if useIndexOnly {
		for i := 0; i < nSource; i++ {
			targetToSource[i] = i
			usedSource[i] = true
		}
	} else {
		// Build source symbol-name buckets (for exact-name alignment).
		sourceByName := make(map[string][]int, nSource)
		for i, sym := range sourceExt {
			name := externalSymbolName(sourceLang, sym)
			sourceByName[name] = append(sourceByName[name], i)
		}

		// Pass 1: name-based matching.
		for targetIdx, targetSym := range targetExt {
			candidates := sourceByName[externalSymbolName(targetLang, targetSym)]
			for _, sourceIdx := range candidates {
				if !usedSource[sourceIdx] {
					targetToSource[targetIdx] = sourceIdx
					usedSource[sourceIdx] = true
					break
				}
			}
		}

		// Pass 2: index fallback for unmatched target slots whose
		// corresponding source index is also unmatched.
		for i := 0; i < nTarget && i < nSource; i++ {
			if targetToSource[i] != -1 {
				continue
			}
			if !usedSource[i] {
				targetToSource[i] = i
				usedSource[i] = true
			}
		}
	}

	// Pass 3: assign remaining unmatched target slots to unused source
	// indices. Target slots that cannot be paired remain -1.
	nextUnused := 0
	for i := 0; i < nTarget; i++ {
		if targetToSource[i] != -1 {
			continue
		}
		for nextUnused < nSource && usedSource[nextUnused] {
			nextUnused++
		}
		if nextUnused >= nSource {
			break // remaining target externals stay -1 (no source match)
		}
		targetToSource[i] = nextUnused
		usedSource[nextUnused] = true
		nextUnused++
	}

	// Build mapping from source scanner result symbols to target symbols.
	sourceResultToTarget := make(map[Symbol]Symbol, nSource)
	sourceAssigned := make([]bool, nSource)
	for targetIdx, sourceIdx := range targetToSource {
		if sourceIdx < 0 || sourceIdx >= nSource {
			continue
		}
		sourceAssigned[sourceIdx] = true
		sourceResultToTarget[sourceExt[sourceIdx]] = targetExt[targetIdx]
	}
	for sourceIdx, assigned := range sourceAssigned {
		if assigned {
			continue
		}
		// Source external with no target match: map to itself (harmless;
		// the result symbol won't appear in the target parse table).
		sourceResultToTarget[sourceExt[sourceIdx]] = sourceExt[sourceIdx]
	}

	return &externalScannerOrderAdapter{
		inner:                sourceLang.ExternalScanner,
		targetToSource:       targetToSource,
		sourceCount:          nSource,
		sourceResultToTarget: sourceResultToTarget,
	}, true
}
