//go:build !grammar_subset || grammar_subset_kotlin

package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestKotlinDottedPackageAndImportsParseWithoutErrors(t *testing.T) {
	src := []byte("package a.b.c\n\nimport d.y.*\nimport x.y.Z\nfun main() {}\n")
	lang := KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if root.HasError() {
		t.Fatalf("Kotlin dotted package/import parse has errors:\n%s", root.SExpr(lang))
	}
}

func TestKotlinFunInterfaceWithPackageImportParsesWithoutErrors(t *testing.T) {
	src := []byte("package com.example\n\nimport com.example.dep.Foo\n\nfun interface MyHandler {\n    fun handle(value: String): Boolean\n}\n")
	lang := KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if root.HasError() {
		t.Fatalf("Kotlin fun interface parse has errors:\n%s", root.SExpr(lang))
	}
}
