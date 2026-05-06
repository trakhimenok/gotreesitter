package gotreesitter

import "testing"

func TestCSharpFindQueryAssignmentSpecs(t *testing.T) {
	src := []byte("var x = from a in source\n  where a.B == \"A\"\n  select new { Name = a.B };\n")

	specs, ok := csharpFindQueryAssignmentSpecs(src)
	if !ok {
		t.Fatal("expected query assignment spec")
	}
	if got := len(specs); got != 1 {
		t.Fatalf("spec count = %d, want 1", got)
	}
	if got := len(specs[0].clauses); got != 3 {
		t.Fatalf("clause count = %d, want 3", got)
	}
	if got := specs[0].clauses[0].kind; got != csharpQueryFromClause {
		t.Fatalf("first clause kind = %v, want from", got)
	}
	if got := specs[0].clauses[1].kind; got != csharpQueryWhereClause {
		t.Fatalf("second clause kind = %v, want where", got)
	}
	if got := specs[0].clauses[2].kind; got != csharpQuerySelectClause {
		t.Fatalf("third clause kind = %v, want select", got)
	}
}

func TestCSharpParseQueryExpressionSpecWithGroupIntoOrder(t *testing.T) {
	src := []byte("from a in sourceA\n" +
		"        join b in sourceB on a.FK equals b.PK\n" +
		"        group a by a.X into g\n" +
		"        orderby g ascending\n" +
		"        select new { A.A, B.B }")
	spec, ok := csharpParseQueryExpressionSpec(src, csharpQueryAssignmentSpec{
		queryStart: 0,
		queryEnd:   uint32(len(src)),
	})
	if !ok {
		t.Fatal("expected query expression spec")
	}
	if got, want := len(spec.clauses), 5; got != want {
		t.Fatalf("clause count = %d, want %d", got, want)
	}
}

func TestCSharpFirstStatementEndHandlesScopedLambda(t *testing.T) {
	src := []byte("    var l = scoped => null;\n    var l = (scoped i) => null;\n")
	got, ok := csharpFirstStatementEndInRange(src, 4, uint32(len(src)))
	if !ok {
		t.Fatal("expected statement span")
	}
	if want := uint32(len("    var l = scoped => null;")); got != want {
		t.Fatalf("statement end = %d, want %d", got, want)
	}
}

func TestCSharpFindTopLevelOperatorHandlesLambdaArrow(t *testing.T) {
	src := []byte("scoped => null")
	pos, ok := csharpFindTopLevelOperator(src, 0, uint32(len(src)), "=>")
	if !ok {
		t.Fatal("expected lambda arrow")
	}
	if want := uint32(len("scoped ")); pos != want {
		t.Fatalf("arrow pos = %d, want %d", pos, want)
	}
}

func TestCSharpTopLevelChunkSpansHandleAttributeCorpus(t *testing.T) {
	src := []byte("[A(B.C)]\n" +
		"class D {}\n\n" +
		"[NS.A(B.C)]\n" +
		"class D {}\n\n" +
		"[One][Two]\n" +
		"[Three]\n" +
		"class A { }\n\n" +
		"[A,B()][C]\n" +
		"struct A { }\n\n" +
		"class Zzz {\n" +
		"  [A,B()][C]\n" +
		"  public int Z;\n" +
		"}\n\n" +
		"class Methods {\n" +
		"  [ValidatedContract]\n" +
		"  int Method1() { return 0; }\n\n" +
		"  [method: ValidatedContract]\n" +
		"  int Method2() { return 0; }\n\n" +
		"  [return: ValidatedContract]\n" +
		"  int Method3() { return 0; }\n" +
		"}\n\n" +
		"[Single]\n" +
		"enum A { B, C }\n\n" +
		"class Zzz {\n" +
		"  [A,B()][C]\n" +
		"  public event EventHandler SomeEvent { add { } remove { } }\n" +
		"}\n\n" +
		"class Class<[A, B][C()]T1> {\n" +
		"  void Method<[E] [F, G(1)] T2>() {\n" +
		"  }\n" +
		"}\n\n" +
		"class Zzz {\n" +
		"  public event EventHandler SomeEvent {\n" +
		"    [A,B()][C] add { }\n" +
		"    [A,B()][C] remove { }\n" +
		"  }\n" +
		"}\n")
	spans := csharpTopLevelChunkSpans(src)
	if got, want := len(spans), 10; got != want {
		t.Fatalf("chunk span count = %d, want %d: %#v", got, want, spans)
	}
}
