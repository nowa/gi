package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestEvaluateDurationAndRegexpArray(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "contract.go", `package contract
import (
	"regexp"
	"time"
)
const retryDelay = 60 * time.Second
var patterns = []*regexp.Regexp{
	regexp.MustCompile("(?i)first"),
	regexp.MustCompile("(?i)second"),
}
`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	extract := extractor{
		fileSet:     fileSet,
		expressions: make(map[string]ast.Expr),
	}
	extract.registerTopLevelDeclarations(file)
	declarations := extract.topLevelDeclarations("contract.go", file)
	byName := make(map[string]value, len(declarations))
	for _, declaration := range declarations {
		byName[declaration.Name] = declaration.Value
	}

	if got := byName["retryDelay"].Number; got != "60000000000" {
		t.Fatalf("retryDelay = %q, want 60000000000", got)
	}
	patterns := byName["patterns"]
	if patterns.Kind != "array" || len(patterns.Items) != 2 {
		t.Fatalf("patterns = %#v, want two-item array", patterns)
	}
	if got := patterns.Items[1].Pattern; got != "(?i)second" {
		t.Fatalf("second pattern = %q, want (?i)second", got)
	}
}
