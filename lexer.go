package gotreesitter

import (
	"unicode/utf8"
	"unsafe"
)

// Point is a row/column position in source text.
type Point struct {
	Row    uint32
	Column uint32
}

// Token is a lexed token with position info.
type Token struct {
	Symbol     Symbol
	Text       string
	StartByte  uint32
	EndByte    uint32
	StartPoint Point
	EndPoint   Point
	Missing    bool
	// NoLookahead marks a synthetic EOF used to force EOF-table reductions
	// without consuming input, matching tree-sitter's lex_state = -1.
	NoLookahead bool
}

func bytesToStringNoCopy(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// Lexer tokenizes source text using a table-driven DFA.
type Lexer struct {
	states          []LexState
	asciiTable      [][128]int32 // ASCII fast-path transition table (nil = not available)
	source          []byte
	pos             int
	row             uint32
	col             uint32
	immediateTokens []bool // symbol IDs that are token.immediate(); rejected after whitespace skip
	zeroWidthTokens []bool // symbol IDs whose terminal pattern can intentionally match empty input

	// Set by scan on failure: where the token attempt began after the DFA
	// consumed skip (whitespace) transitions. errorRunToken uses this so an
	// unlexable run starts after legitimately skipped whitespace, like C.
	failTokenStartPos int
	failTokenStartRow uint32
	failTokenStartCol uint32

	// The grammar's most permissive lex state (LexModes[0], C's ERROR_STATE
	// mode). NextWithErrorRuns only emits an error-run token when even this
	// state cannot lex at a position — mirroring C, which retries failed
	// lexes in error mode before skipping characters into an error subtree.
	errorRunLexState    uint32
	hasErrorRunLexState bool
}

// NewLexer creates a new Lexer that will tokenize source using the given
// DFA state table.
func NewLexer(states []LexState, source []byte) *Lexer {
	return &Lexer{
		states: states,
		source: source,
	}
}

// Next lexes the next token starting from the given lex state index.
// It automatically skips tokens from states where Skip=true (whitespace).
// Returns a zero-Symbol token with StartByte==EndByte at EOF.
func (l *Lexer) Next(startState uint32) Token {
	return l.next(startState, false)
}

// NextWithErrorRuns behaves like Next, except that bytes for which no
// accepting DFA state exists are not silently dropped: the whole unlexable
// run is consumed and returned as an errorSymbol token. This mirrors C
// ts_parser__lex, which surfaces skipped characters as an error subtree —
// the run starts after any whitespace the DFA legitimately skipped and ends
// at the first position where a token can be lexed (or EOF).
func (l *Lexer) NextWithErrorRuns(startState uint32) Token {
	return l.next(startState, true)
}

func (l *Lexer) next(startState uint32, emitErrorRuns bool) Token {
	for {
		// EOF check.
		if l.pos >= len(l.source) {
			return Token{
				StartByte:  uint32(l.pos),
				EndByte:    uint32(l.pos),
				StartPoint: Point{Row: l.row, Column: l.col},
				EndPoint:   Point{Row: l.row, Column: l.col},
			}
		}

		tokenStartPos := l.pos
		tokenStartRow := l.row
		tokenStartCol := l.col

		tok, ok := l.scan(startState, tokenStartPos, tokenStartRow, tokenStartCol)
		if ok {
			if tok.Symbol == 0 {
				// Skip token (whitespace). Verify the lexer actually
				// advanced past the skipped content to prevent an
				// infinite loop on zero-width skip matches.
				if l.pos <= tokenStartPos {
					l.skipOneRune()
				}
				continue
			}
			return tok
		}

		if emitErrorRuns && l.hasErrorRunLexState && !l.canLexAt(l.errorRunLexState, tokenStartPos, tokenStartRow, tokenStartCol) {
			return l.errorRunToken()
		}
		// No accepting state was found. Skip one rune as error recovery.
		l.skipOneRune()
	}
}

// canLexAt reports whether the DFA can lex a token (or whitespace skip)
// starting at the given position, without moving the lexer.
func (l *Lexer) canLexAt(lexState uint32, pos int, row, col uint32) bool {
	savedPos, savedRow, savedCol := l.pos, l.row, l.col
	_, ok := l.scan(lexState, pos, row, col)
	l.pos, l.row, l.col = savedPos, savedRow, savedCol
	return ok
}

// errorRunToken consumes the unlexable run beginning at the last failed
// scan's token start (i.e. after any whitespace the DFA skipped) and returns
// it as an errorSymbol token. The run ends at the first position where the
// error-mode lex state can lex again, matching C's character-by-character
// error skipping.
func (l *Lexer) errorRunToken() Token {
	// Position the lexer at the real error start: scan records where the
	// token attempt began after consuming skip (whitespace) transitions.
	if l.failTokenStartPos > l.pos && l.failTokenStartPos <= len(l.source) {
		l.pos = l.failTokenStartPos
		l.row = l.failTokenStartRow
		l.col = l.failTokenStartCol
	}
	if l.pos >= len(l.source) {
		// Only whitespace remained: this is end-of-input, not an error run.
		return Token{
			StartByte:  uint32(l.pos),
			EndByte:    uint32(l.pos),
			StartPoint: Point{Row: l.row, Column: l.col},
			EndPoint:   Point{Row: l.row, Column: l.col},
		}
	}
	startPos, startRow, startCol := l.pos, l.row, l.col

	l.skipOneRune()
	for l.pos < len(l.source) {
		if l.canLexAt(l.errorRunLexState, l.pos, l.row, l.col) {
			break
		}
		l.skipOneRune()
	}
	return Token{
		Symbol:     errorSymbol,
		Text:       bytesToStringNoCopy(l.source[startPos:l.pos]),
		StartByte:  uint32(startPos),
		EndByte:    uint32(l.pos),
		StartPoint: Point{Row: startRow, Column: startCol},
		EndPoint:   Point{Row: l.row, Column: l.col},
	}
}

// scan runs the DFA from the given start state and position. It returns
// a token and true if an accepting state was reached, or false if not.
// On a skip (whitespace) match, it returns a zero-Symbol token and true.
func (l *Lexer) scan(startState uint32, startPos int, startRow, startCol uint32) (Token, bool) {
	curState := int32(startState)
	if curState < 0 || int(curState) >= len(l.states) {
		return Token{}, false
	}

	scanPos := startPos
	scanRow := startRow
	scanCol := startCol
	tokenStartPos := startPos
	tokenStartRow := startRow
	tokenStartCol := startCol

	// Track the last accepting state.
	acceptPos := -1
	acceptRow := uint32(0)
	acceptCol := uint32(0)
	acceptStartPos := 0
	acceptStartRow := uint32(0)
	acceptStartCol := uint32(0)
	acceptSymbol := Symbol(0)
	acceptSkip := false
	acceptPriorityBest := int16(32767) // max int16; any real priority beats this

	eofHops := 0
	// Walk the DFA in the same style as tree-sitter START_LEXER/ADVANCE/SKIP.
	for {
		if curState < 0 || int(curState) >= len(l.states) {
			break
		}
		st := &l.states[int(curState)]

		if st.AcceptToken > 0 || st.Skip {
			// Reject immediate tokens that matched after whitespace was
			// consumed. Immediate tokens must match at the original position.
			isImmediate := st.AcceptToken > 0 && int(st.AcceptToken) < len(l.immediateTokens) && l.immediateTokens[st.AcceptToken]
			skippedWhitespace := tokenStartPos > startPos
			zeroWidthVisible := st.AcceptToken > 0 && scanPos == tokenStartPos && !l.allowsZeroWidthToken(st.AcceptToken)
			if !(isImmediate && skippedWhitespace) && !zeroWidthVisible {
				newPrio := st.AcceptPriority
				if acceptPos < 0 || newPrio < acceptPriorityBest || (newPrio == acceptPriorityBest && scanPos > acceptPos) {
					acceptPos = scanPos
					acceptRow = scanRow
					acceptCol = scanCol
					acceptStartPos = tokenStartPos
					acceptStartRow = tokenStartRow
					acceptStartCol = tokenStartCol
					acceptSymbol = st.AcceptToken
					acceptSkip = st.Skip
					acceptPriorityBest = newPrio
				}
			}
		}

		if scanPos >= len(l.source) {
			if st.EOF >= 0 && eofHops <= len(l.states) {
				curState = int32(st.EOF)
				eofHops++
				continue
			}
			break
		}
		eofHops = 0

		b := l.source[scanPos]
		var r rune
		var size int
		if b < 0x80 {
			r = rune(b)
			size = 1
		} else {
			r, size = utf8.DecodeRune(l.source[scanPos:])
		}
		nextState := int32(-1)
		skipTransition := false
		if b < 0x80 && l.asciiTable != nil && int(curState) < len(l.asciiTable) {
			// ASCII fast-path: O(1) lookup instead of linear scan.
			v := l.asciiTable[curState][b]
			if v != lexAsciiNoMatch {
				nextState = v & ^lexAsciiSkipBit
				skipTransition = v&lexAsciiSkipBit != 0
			}
		} else {
			for i := range st.Transitions {
				tr := &st.Transitions[i]
				if r >= tr.Lo && r <= tr.Hi {
					nextState = int32(tr.NextState)
					skipTransition = tr.Skip
					break
				}
			}
		}
		// Default transitions are treated as non-skipping.
		skipTransition = skipTransition && nextState >= 0
		if nextState < 0 && st.Default >= 0 {
			nextState = int32(st.Default)
			skipTransition = false
		}
		if nextState < 0 {
			break
		}

		scanPos += size
		if r == '\n' {
			scanRow++
			scanCol = 0
		} else {
			scanCol += uint32(size)
		}

		if skipTransition {
			// tree-sitter SKIP(state) consumes and resets token start.
			tokenStartPos = scanPos
			tokenStartRow = scanRow
			tokenStartCol = scanCol
			acceptPos = -1
			acceptSymbol = 0
			acceptSkip = false
		}

		curState = nextState
	}

	if acceptPos < 0 {
		l.failTokenStartPos = tokenStartPos
		l.failTokenStartRow = tokenStartRow
		l.failTokenStartCol = tokenStartCol
		return Token{}, false
	}

	// Rewind (or advance) to the accept position.
	l.pos = acceptPos
	l.row = acceptRow
	l.col = acceptCol

	if acceptSkip {
		// Return a zero-Symbol token to signal "skip".
		return Token{
			StartByte:  uint32(acceptStartPos),
			EndByte:    uint32(acceptPos),
			StartPoint: Point{Row: acceptStartRow, Column: acceptStartCol},
			EndPoint:   Point{Row: acceptRow, Column: acceptCol},
		}, true
	}

	return Token{
		Symbol:     acceptSymbol,
		Text:       bytesToStringNoCopy(l.source[acceptStartPos:acceptPos]),
		StartByte:  uint32(acceptStartPos),
		EndByte:    uint32(acceptPos),
		StartPoint: Point{Row: acceptStartRow, Column: acceptStartCol},
		EndPoint:   Point{Row: acceptRow, Column: acceptCol},
	}, true
}

// skipOneRune advances the lexer position by one rune, updating row/column.
func (l *Lexer) skipOneRune() {
	if l.pos >= len(l.source) {
		return
	}
	r, size := utf8.DecodeRune(l.source[l.pos:])
	l.pos += size
	if r == '\n' {
		l.row++
		l.col = 0
	} else {
		l.col += uint32(size)
	}
}

func (l *Lexer) allowsZeroWidthToken(sym Symbol) bool {
	if l == nil || len(l.zeroWidthTokens) == 0 {
		return true
	}
	return int(sym) < len(l.zeroWidthTokens) && l.zeroWidthTokens[sym]
}
