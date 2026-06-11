package gotreesitter

import "testing"

func newQLTestLanguage() *Language {
	return &Language{
		Name: "ql",
		SymbolNames: []string{
			"EOF", "ql", "signatureExpr", "moduleExpr", "typeExpr",
			"simpleId", "className", "::", "moduleInstantiation",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "ql", Visible: true, Named: true},
			{Name: "signatureExpr", Visible: true, Named: true},
			{Name: "moduleExpr", Visible: true, Named: true},
			{Name: "typeExpr", Visible: true, Named: true},
			{Name: "simpleId", Visible: true, Named: true},
			{Name: "className", Visible: true, Named: true},
			{Name: "::", Visible: true},
			{Name: "moduleInstantiation", Visible: true, Named: true},
		},
	}
}

const (
	qlTestSymSignatureExpr = 2
	qlTestSymModuleExpr    = 3
	qlTestSymTypeExpr      = 4
	qlTestSymSimpleId      = 5
	qlTestSymClassName     = 6
	qlTestSymColonColon    = 7
	qlTestSymModuleInst    = 8
)

// Qualified upper-id signature: `implements DataFlow::ConfigSig` parses in Go
// as moduleExpr(moduleExpr,::,simpleId) where C resolves the GLR ambiguity to
// typeExpr(moduleExpr,::,className).
func TestNormalizeQLSignatureExprQualifiedUpperId(t *testing.T) {
	lang := newQLTestLanguage()
	source := []byte("DataFlow::ConfigSig")
	arena := newNodeArena(arenaClassFull)
	qual := newParentNodeInArena(arena, qlTestSymModuleExpr, true, []*Node{
		newLeafNodeInArena(arena, qlTestSymSimpleId, true, 0, 8, Point{}, Point{Column: 8}),
	}, nil, 0)
	sep := newLeafNodeInArena(arena, qlTestSymColonColon, false, 8, 10, Point{Column: 8}, Point{Column: 10})
	tail := newLeafNodeInArena(arena, qlTestSymSimpleId, true, 10, 19, Point{Column: 10}, Point{Column: 19})
	expr := newParentNodeInArena(arena, qlTestSymModuleExpr, true, []*Node{qual, sep, tail}, nil, 0)
	sig := newParentNodeInArena(arena, qlTestSymSignatureExpr, true, []*Node{expr}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{sig}, nil, 0)

	normalizeQLCompatibility(root, source, lang)

	if got, want := expr.Type(lang), "typeExpr"; got != want {
		t.Fatalf("outer expr type = %q, want %q", got, want)
	}
	if got, want := tail.Type(lang), "className"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
	if got, want := qual.Type(lang), "moduleExpr"; got != want {
		t.Fatalf("qualifier type = %q, want %q (must stay)", got, want)
	}
	if got, want := qual.Child(0).Type(lang), "simpleId"; got != want {
		t.Fatalf("qualifier name type = %q, want %q (must stay)", got, want)
	}
}

// Unqualified upper-id signature: `implements ConfigSig`.
func TestNormalizeQLSignatureExprUnqualifiedUpperId(t *testing.T) {
	lang := newQLTestLanguage()
	source := []byte("ConfigSig")
	arena := newNodeArena(arenaClassFull)
	tail := newLeafNodeInArena(arena, qlTestSymSimpleId, true, 0, 9, Point{}, Point{Column: 9})
	expr := newParentNodeInArena(arena, qlTestSymModuleExpr, true, []*Node{tail}, nil, 0)
	sig := newParentNodeInArena(arena, qlTestSymSignatureExpr, true, []*Node{expr}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{sig}, nil, 0)

	normalizeQLCompatibility(root, source, lang)

	if got, want := expr.Type(lang), "typeExpr"; got != want {
		t.Fatalf("expr type = %q, want %q", got, want)
	}
	if got, want := tail.Type(lang), "className"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
}

// Lower-id tails cannot be className (className is an upper-id token); the
// moduleExpr parse is the only one available to C too — leave it.
func TestNormalizeQLSignatureExprLowerIdUntouched(t *testing.T) {
	lang := newQLTestLanguage()
	source := []byte("dataflow")
	arena := newNodeArena(arenaClassFull)
	tail := newLeafNodeInArena(arena, qlTestSymSimpleId, true, 0, 8, Point{}, Point{Column: 8})
	expr := newParentNodeInArena(arena, qlTestSymModuleExpr, true, []*Node{tail}, nil, 0)
	sig := newParentNodeInArena(arena, qlTestSymSignatureExpr, true, []*Node{expr}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{sig}, nil, 0)

	normalizeQLCompatibility(root, source, lang)

	if got, want := expr.Type(lang), "moduleExpr"; got != want {
		t.Fatalf("expr type = %q, want %q", got, want)
	}
	if got, want := tail.Type(lang), "simpleId"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
}

// moduleInstantiation tails (`M::Inst<T>`) have no typeExpr counterpart;
// C keeps moduleExpr — leave it.
func TestNormalizeQLSignatureExprInstantiationUntouched(t *testing.T) {
	lang := newQLTestLanguage()
	source := []byte("M::Inst<T>")
	arena := newNodeArena(arenaClassFull)
	qual := newParentNodeInArena(arena, qlTestSymModuleExpr, true, []*Node{
		newLeafNodeInArena(arena, qlTestSymSimpleId, true, 0, 1, Point{}, Point{Column: 1}),
	}, nil, 0)
	sep := newLeafNodeInArena(arena, qlTestSymColonColon, false, 1, 3, Point{Column: 1}, Point{Column: 3})
	tail := newParentNodeInArena(arena, qlTestSymModuleInst, true, []*Node{
		newLeafNodeInArena(arena, qlTestSymSimpleId, true, 3, 7, Point{Column: 3}, Point{Column: 7}),
	}, nil, 0)
	expr := newParentNodeInArena(arena, qlTestSymModuleExpr, true, []*Node{qual, sep, tail}, nil, 0)
	sig := newParentNodeInArena(arena, qlTestSymSignatureExpr, true, []*Node{expr}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{sig}, nil, 0)

	normalizeQLCompatibility(root, source, lang)

	if got, want := expr.Type(lang), "moduleExpr"; got != want {
		t.Fatalf("expr type = %q, want %q", got, want)
	}
}

// moduleExpr outside a signatureExpr (import statements, module aliases) is
// not ambiguous in C either — leave it.
func TestNormalizeQLModuleExprOutsideSignatureUntouched(t *testing.T) {
	lang := newQLTestLanguage()
	source := []byte("DataFlow")
	arena := newNodeArena(arenaClassFull)
	tail := newLeafNodeInArena(arena, qlTestSymSimpleId, true, 0, 8, Point{}, Point{Column: 8})
	expr := newParentNodeInArena(arena, qlTestSymModuleExpr, true, []*Node{tail}, nil, 0)
	root := newParentNodeInArena(arena, 1, true, []*Node{expr}, nil, 0)

	normalizeQLCompatibility(root, source, lang)

	if got, want := expr.Type(lang), "moduleExpr"; got != want {
		t.Fatalf("expr type = %q, want %q", got, want)
	}
	if got, want := tail.Type(lang), "simpleId"; got != want {
		t.Fatalf("tail type = %q, want %q", got, want)
	}
}
