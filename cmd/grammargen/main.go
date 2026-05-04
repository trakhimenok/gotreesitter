// Command grammargen generates tree-sitter parser artifacts from grammar definitions.
//
// Usage:
//
//	grammargen [flags] <grammar-name-or-file>
//
// Input sources:
//
//	<name>        Built-in grammar (json, calc, glr, go, kotlin, swift, etc.)
//	-js <path>    Import a tree-sitter grammar.js file
//	-json <path>  Import a resolved tree-sitter grammar.json file
//
// Output formats:
//
//	-bin <path>    Write gotreesitter .bin blob
//	-c <path>      Write tree-sitter parser.c
//	-go <path>     Write grammargen Go DSL source
//
// Other flags:
//
//	-validate      Check grammar for issues without generating
//	-report        Show generation report with conflict diagnostics
//	-list          List available built-in grammars
package main

import (
	"flag"
	"fmt"
	"os"
	"unicode"

	"github.com/odvcencio/gotreesitter/grammargen"
)

var builtinGrammars = map[string]func() *grammargen.Grammar{
	"json":               grammargen.JSONGrammar,
	"calc":               grammargen.CalcGrammar,
	"glr":                grammargen.GLRGrammar,
	"go":                 grammargen.GoGrammar,
	"keyword":            grammargen.KeywordGrammar,
	"ext":                grammargen.ExtScannerGrammar,
	"alias":              grammargen.AliasSuperGrammar,
	"kotlin":             grammargen.KotlinGrammar,
	"swift":              grammargen.SwiftGrammar,
	"swift-abi-mangling": grammargen.SwiftABIManglingGrammar,
}

func main() {
	binOut := flag.String("bin", "", "output path for gotreesitter .bin blob")
	cOut := flag.String("c", "", "output path for tree-sitter parser.c")
	goOut := flag.String("go", "", "output path for grammargen Go DSL source")
	jsInput := flag.String("js", "", "path to a tree-sitter grammar.js file to import")
	jsonInput := flag.String("json", "", "path to a resolved tree-sitter grammar.json file to import")
	grammarFile := flag.String("grammar", "", "path to a .grammar file to parse")
	pkgName := flag.String("pkg", "grammargen", "package name for -go output")
	funcName := flag.String("func", "", "function name for -go output (default: <GrammarName>Grammar)")
	highlight := flag.Bool("highlight", false, "generate a highlight query for the grammar")
	validate := flag.Bool("validate", false, "validate grammar without generating")
	report := flag.Bool("report", false, "show generation report with conflict diagnostics")
	list := flag.Bool("list", false, "list available built-in grammars")
	flag.Parse()

	if *list {
		fmt.Println("Available built-in grammars:")
		for name := range builtinGrammars {
			fmt.Printf("  %s\n", name)
		}
		os.Exit(0)
	}

	var g *grammargen.Grammar
	var name string

	switch {
	case *jsInput != "" && *jsonInput != "":
		fmt.Fprintln(os.Stderr, "use only one of -js or -json")
		os.Exit(1)

	case *jsonInput != "":
		// Import from resolved grammar.json file.
		source, err := os.ReadFile(*jsonInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", *jsonInput, err)
			os.Exit(1)
		}
		imported, err := grammargen.ImportGrammarJSON(source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import %s: %v\n", *jsonInput, err)
			os.Exit(1)
		}
		g = imported
		name = g.Name
		if name == "" {
			name = *jsonInput
		}

	case *jsInput != "":
		// Import from grammar.js file.
		source, err := os.ReadFile(*jsInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", *jsInput, err)
			os.Exit(1)
		}
		imported, err := grammargen.ImportGrammarJS(source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import %s: %v\n", *jsInput, err)
			os.Exit(1)
		}
		g = imported
		name = g.Name
		if name == "" {
			name = *jsInput
		}

	case *grammarFile != "":
		// Parse .grammar file.
		source, err := os.ReadFile(*grammarFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", *grammarFile, err)
			os.Exit(1)
		}
		parsed, err := grammargen.ParseGrammarFile(string(source))
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", *grammarFile, err)
			os.Exit(1)
		}
		g = parsed
		name = g.Name
		if name == "" {
			name = *grammarFile
		}

	default:
		// Use built-in grammar.
		args := flag.Args()
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "usage: grammargen [flags] <grammar-name>")
			fmt.Fprintln(os.Stderr, "       grammargen -js <grammar.js> [flags]")
			fmt.Fprintln(os.Stderr, "       grammargen -json <grammar.json> [flags]")
			fmt.Fprintln(os.Stderr, "       grammargen -grammar <file.grammar> [flags]")
			fmt.Fprintln(os.Stderr, "run with -list to see available built-in grammars")
			os.Exit(1)
		}

		name = args[0]
		fn, ok := builtinGrammars[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown grammar %q (use -list, -js, -json, or -grammar)\n", name)
			os.Exit(1)
		}
		g = fn()
	}

	// Highlight query mode.
	if *highlight {
		query := grammargen.GenerateHighlightQuery(g)
		fmt.Print(query)
		return
	}

	// Validate mode.
	if *validate {
		warnings := grammargen.Validate(g)
		if len(warnings) == 0 {
			fmt.Printf("grammar %q: OK (no warnings)\n", name)
		} else {
			fmt.Printf("grammar %q: %d warning(s):\n", name, len(warnings))
			for _, w := range warnings {
				fmt.Printf("  - %s\n", w)
			}
			os.Exit(1)
		}

		if len(g.Tests) > 0 {
			fmt.Printf("running %d embedded test(s)...\n", len(g.Tests))
			if err := grammargen.RunTests(g); err != nil {
				fmt.Fprintf(os.Stderr, "tests failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("all tests passed")
		}
		return
	}

	// Report mode.
	if *report {
		rpt, err := grammargen.GenerateWithReport(g)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generation failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Grammar: %s\n", name)
		fmt.Printf("Symbols: %d\n", rpt.SymbolCount)
		fmt.Printf("States:  %d\n", rpt.StateCount)
		fmt.Printf("Tokens:  %d\n", rpt.TokenCount)
		fmt.Printf("Blob:    %d bytes\n", len(rpt.Blob))

		if len(rpt.Warnings) > 0 {
			fmt.Printf("\nWarnings (%d):\n", len(rpt.Warnings))
			for _, w := range rpt.Warnings {
				fmt.Printf("  - %s\n", w)
			}
		}

		if len(rpt.Conflicts) > 0 {
			ng, _ := grammargen.Normalize(g)
			fmt.Printf("\nConflicts resolved (%d):\n", len(rpt.Conflicts))
			for i, c := range rpt.Conflicts {
				fmt.Printf("\n[%d] %s\n", i+1, c.String(ng))
			}
		} else {
			fmt.Println("\nNo conflicts")
		}
		return
	}

	// Default: generate output.
	if *binOut == "" && *cOut == "" && *goOut == "" {
		fmt.Fprintln(os.Stderr, "specify at least one output: -bin <path>, -c <path>, or -go <path>")
		os.Exit(1)
	}

	if *binOut != "" {
		blob, err := grammargen.Generate(g)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generation failed: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*binOut, blob, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *binOut, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", *binOut, len(blob))
	}

	if *goOut != "" {
		outFunc := *funcName
		if outFunc == "" {
			outFunc = defaultGrammarFuncName(g.Name)
		}
		source, err := grammargen.EmitGrammarGo(g, *pkgName, outFunc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Go source generation failed: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*goOut, source, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *goOut, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", *goOut, len(source))
	}

	if *cOut != "" {
		code, err := grammargen.GenerateC(g)
		if err != nil {
			fmt.Fprintf(os.Stderr, "C generation failed: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*cOut, []byte(code), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *cOut, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", *cOut, len(code))
	}
}

func defaultGrammarFuncName(name string) string {
	var out []rune
	upperNext := true
	for _, r := range name {
		if !isIdentRune(r) {
			upperNext = true
			continue
		}
		if len(out) == 0 && unicode.IsDigit(r) {
			out = append(out, '_')
		}
		if upperNext {
			r = unicode.ToUpper(r)
			upperNext = false
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return "Grammar"
	}
	return string(out) + "Grammar"
}

func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
