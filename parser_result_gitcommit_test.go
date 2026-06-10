package gotreesitter

import "testing"

func gitcommitTestLanguage() *Language {
	return &Language{
		Name:        "gitcommit",
		SymbolNames: []string{"EOF", "source", "subject", "message", "message_line"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "source", Visible: true, Named: true},
			{Name: "subject", Visible: true, Named: true},
			{Name: "message", Visible: true, Named: true},
			{Name: "message_line", Visible: true, Named: true},
		},
	}
}

func TestNormalizeGitcommitCollapsedMessageLine(t *testing.T) {
	lang := gitcommitTestLanguage()
	// "subj\n\nbody line\n" — message collapsed to a childless leaf [6:15]
	// that stopped before the trailing newline.
	source := []byte("subj\n\nbody line\n")
	arena := newNodeArena(arenaClassFull)
	subject := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	message := newLeafNodeInArena(arena, 3, true, 6, 15, Point{Row: 2}, Point{Row: 2, Column: 9})
	root := newParentNodeInArena(arena, 1, true, []*Node{subject, message}, nil, 0)
	root.startByte = 0
	root.endByte = uint32(len(source))
	root.endPoint = Point{Row: 3}

	normalizeGitcommitCompatibility(root, source, lang)

	if got, want := message.EndByte(), uint32(16); got != want {
		t.Fatalf("message end byte = %d, want %d (trailing newline folded in)", got, want)
	}
	if got, want := message.EndPoint(), (Point{Row: 3}); got != want {
		t.Fatalf("message end point = %+v, want %+v", got, want)
	}
	if got, want := resultChildCount(message), 1; got != want {
		t.Fatalf("message child count = %d, want %d", got, want)
	}
	line := message.Child(0)
	if got, want := line.Type(lang), "message_line"; got != want {
		t.Fatalf("message child type = %q, want %q", got, want)
	}
	if line.StartByte() != 6 || line.EndByte() != 15 {
		t.Fatalf("message_line span = [%d:%d], want [6:15]", line.StartByte(), line.EndByte())
	}
}

func TestNormalizeGitcommitCollapsedMessageLineCRLFAndComments(t *testing.T) {
	lang := gitcommitTestLanguage()
	// Collapsed single-line message followed by CRLF + blank line + generated
	// comments: C folds both line breaks into message and stops at '#'.
	source := []byte("subj\r\n\r\nbody\r\n\r\n# comment\r\n")
	arena := newNodeArena(arenaClassFull)
	subject := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	message := newLeafNodeInArena(arena, 3, true, 8, 12, Point{Row: 2}, Point{Row: 2, Column: 4})
	root := newParentNodeInArena(arena, 1, true, []*Node{subject, message}, nil, 0)
	root.startByte = 0
	root.endByte = uint32(len(source))
	root.endPoint = Point{Row: 5}

	normalizeGitcommitCompatibility(root, source, lang)

	if got, want := message.EndByte(), uint32(16); got != want {
		t.Fatalf("message end byte = %d, want %d (CRLF run folded in)", got, want)
	}
	if got, want := resultChildCount(message), 1; got != want {
		t.Fatalf("message child count = %d, want %d", got, want)
	}
}

func TestNormalizeGitcommitLeavesNewlineOnlyMessageAlone(t *testing.T) {
	lang := gitcommitTestLanguage()
	// A childless message spanning only NEWLINE body lines is legitimate
	// (repeat of hidden NEWLINEs) — it must not gain a message_line child.
	source := []byte("subj\n\n\n\n")
	arena := newNodeArena(arenaClassFull)
	subject := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	message := newLeafNodeInArena(arena, 3, true, 6, 8, Point{Row: 2}, Point{Row: 4})
	root := newParentNodeInArena(arena, 1, true, []*Node{subject, message}, nil, 0)
	root.startByte = 0
	root.endByte = uint32(len(source))
	root.endPoint = Point{Row: 4}

	normalizeGitcommitCompatibility(root, source, lang)

	if got, want := resultChildCount(message), 0; got != want {
		t.Fatalf("message child count = %d, want %d", got, want)
	}
	if got, want := message.EndByte(), uint32(8); got != want {
		t.Fatalf("message end byte = %d, want %d", got, want)
	}
}

func TestNormalizeGitcommitLeavesPopulatedMessageAlone(t *testing.T) {
	lang := gitcommitTestLanguage()
	source := []byte("subj\n\nbody line\n")
	arena := newNodeArena(arenaClassFull)
	subject := newLeafNodeInArena(arena, 2, true, 0, 4, Point{}, Point{Column: 4})
	line := newLeafNodeInArena(arena, 4, true, 6, 15, Point{Row: 2}, Point{Row: 2, Column: 9})
	message := newParentNodeInArena(arena, 3, true, []*Node{line}, nil, 0)
	message.startByte = 6
	message.endByte = 16
	message.endPoint = Point{Row: 3}
	root := newParentNodeInArena(arena, 1, true, []*Node{subject, message}, nil, 0)
	root.startByte = 0
	root.endByte = uint32(len(source))
	root.endPoint = Point{Row: 3}

	normalizeGitcommitCompatibility(root, source, lang)

	if got, want := resultChildCount(message), 1; got != want {
		t.Fatalf("message child count = %d, want %d", got, want)
	}
	if got, want := message.EndByte(), uint32(16); got != want {
		t.Fatalf("message end byte = %d, want %d", got, want)
	}
}
