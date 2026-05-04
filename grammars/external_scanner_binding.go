package grammars

import gotreesitter "github.com/odvcencio/gotreesitter"

func bindExternalScannerSpec(lang *gotreesitter.Language, spec ExternalScannerSpec, setSymbol func(int, gotreesitter.Symbol)) []int {
	return bindExternalScannerSymbolNames(lang, spec.Externals, setSymbol)
}

func bindExternalScannerSymbolNames(lang *gotreesitter.Language, names []string, setSymbol func(int, gotreesitter.Symbol)) []int {
	if lang == nil {
		return nil
	}

	externalToToken := make([]int, len(lang.ExternalSymbols))
	for i := range externalToToken {
		externalToToken[i] = -1
	}
	matched := make([]bool, len(names))

	for externalIdx, sym := range lang.ExternalSymbols {
		name := externalScannerSymbolName(lang, sym)
		for tokenIdx, want := range names {
			if matched[tokenIdx] || name != want {
				continue
			}
			setSymbol(tokenIdx, sym)
			externalToToken[externalIdx] = tokenIdx
			matched[tokenIdx] = true
			break
		}
	}

	// Fall back to positional binding for generated/test languages that do not
	// carry symbol names, while preserving any name-based matches above.
	for tokenIdx := 0; tokenIdx < len(names) && tokenIdx < len(lang.ExternalSymbols); tokenIdx++ {
		if matched[tokenIdx] || externalToToken[tokenIdx] != -1 {
			continue
		}
		setSymbol(tokenIdx, lang.ExternalSymbols[tokenIdx])
		externalToToken[tokenIdx] = tokenIdx
	}

	return externalToToken
}

func externalScannerSymbolName(lang *gotreesitter.Language, sym gotreesitter.Symbol) string {
	if lang == nil || int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
		return ""
	}
	return lang.SymbolNames[sym]
}
