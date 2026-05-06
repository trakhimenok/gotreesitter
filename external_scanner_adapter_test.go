package gotreesitter

import "testing"

type recordingExternalScanner struct {
	seen [][]bool
}

func (s *recordingExternalScanner) Create() any { return nil }
func (s *recordingExternalScanner) Destroy(any) {}
func (s *recordingExternalScanner) Serialize(any, []byte) int {
	return 0
}
func (s *recordingExternalScanner) Deserialize(any, []byte) {}
func (s *recordingExternalScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	snapshot := append([]bool(nil), valid...)
	s.seen = append(s.seen, snapshot)
	for i, ok := range valid {
		if ok {
			switch i {
			case 0:
				lexer.SetResultSymbol(10)
			case 1:
				lexer.SetResultSymbol(20)
			}
			return true
		}
	}
	return false
}

func TestExternalScannerOrderAdapterReusesAndClearsSourceValid(t *testing.T) {
	scanner := &recordingExternalScanner{}
	sourceLang := &Language{
		SymbolNames:     []string{"", "", "", "", "", "", "", "", "", "", "a", "", "", "", "", "", "", "", "", "", "b"},
		ExternalSymbols: []Symbol{10, 20},
		ExternalScanner: scanner,
	}
	targetNames := make([]string, 201)
	targetNames[100] = "a"
	targetNames[200] = "b"
	targetLang := &Language{
		SymbolNames:     targetNames,
		ExternalSymbols: []Symbol{100, 200},
	}
	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	payload := adapted.Create()
	defer adapted.Destroy(payload)

	lexer := &ExternalLexer{}
	if !adapted.Scan(payload, lexer, []bool{false, true}) {
		t.Fatal("first scan failed")
	}
	if lexer.resultSymbol != 200 {
		t.Fatalf("first result symbol = %d, want 200", lexer.resultSymbol)
	}
	lexer = &ExternalLexer{}
	if !adapted.Scan(payload, lexer, []bool{true, false}) {
		t.Fatal("second scan failed")
	}
	if lexer.resultSymbol != 100 {
		t.Fatalf("second result symbol = %d, want 100", lexer.resultSymbol)
	}
	if len(scanner.seen) != 2 {
		t.Fatalf("scanner saw %d calls, want 2", len(scanner.seen))
	}
	if got := scanner.seen[0]; len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("first source valid = %v, want [false true]", got)
	}
	if got := scanner.seen[1]; len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("second source valid = %v, want [true false]", got)
	}
}
