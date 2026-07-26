package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type value struct {
	Kind       string           `json:"kind"`
	Number     string           `json:"number,omitempty"`
	Text       string           `json:"text,omitempty"`
	Pattern    string           `json:"pattern,omitempty"`
	Items      []value          `json:"items,omitempty"`
	Properties map[string]value `json:"properties,omitempty"`
	Expression string           `json:"expression,omitempty"`
}

type declaration struct {
	File  string `json:"file"`
	Name  string `json:"name"`
	Value value  `json:"value"`
}

type literal struct {
	Kind  string   `json:"kind"`
	Value string   `json:"value"`
	Count int      `json:"count"`
	Files []string `json:"files"`
}

type inventory struct {
	SchemaVersion int           `json:"schemaVersion"`
	Parser        string        `json:"parser"`
	Files         []string      `json:"files"`
	Declarations  []declaration `json:"declarations"`
	Literals      []literal     `json:"literals"`
}

type literalAccumulator struct {
	count int
	files map[string]struct{}
}

type extractor struct {
	fileSet     *token.FileSet
	expressions map[string]ast.Expr
}

func main() {
	var root string
	var paths string
	flag.StringVar(&root, "root", ".", "Gi repository root")
	flag.StringVar(&paths, "paths", "gi-llm-provider", "comma-separated Go paths relative to root")
	flag.Parse()

	result, err := collect(root, strings.Split(paths, ","))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func collect(root string, paths []string) (inventory, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return inventory{}, err
	}
	fileSet := token.NewFileSet()
	type parsedFile struct {
		relative string
		file     *ast.File
	}
	var files []parsedFile
	for _, requestedPath := range paths {
		requestedPath = strings.TrimSpace(requestedPath)
		if requestedPath == "" {
			continue
		}
		start := requestedPath
		if !filepath.IsAbs(start) {
			start = filepath.Join(absoluteRoot, start)
		}
		walkErr := filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "vendor" || entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") ||
				strings.HasSuffix(name, "_test.go") ||
				strings.HasSuffix(name, "_generated.go") {
				return nil
			}
			parsed, parseErr := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
			if parseErr != nil {
				return parseErr
			}
			relative, relativeErr := filepath.Rel(absoluteRoot, path)
			if relativeErr != nil {
				return relativeErr
			}
			files = append(files, parsedFile{
				relative: filepath.ToSlash(relative),
				file:     parsed,
			})
			return nil
		})
		if walkErr != nil {
			return inventory{}, walkErr
		}
	}
	slices.SortFunc(files, func(left, right parsedFile) int {
		return strings.Compare(left.relative, right.relative)
	})

	extract := extractor{
		fileSet:     fileSet,
		expressions: make(map[string]ast.Expr),
	}
	for _, parsed := range files {
		extract.registerTopLevelDeclarations(parsed.file)
	}

	result := inventory{
		SchemaVersion: 1,
		Parser:        "go/parser",
		Files:         make([]string, 0, len(files)),
	}
	literals := make(map[string]*literalAccumulator)
	for _, parsed := range files {
		result.Files = append(result.Files, parsed.relative)
		result.Declarations = append(
			result.Declarations,
			extract.topLevelDeclarations(parsed.relative, parsed.file)...,
		)
		regexpArguments := make(map[token.Pos]struct{})
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !isRegexpMustCompile(call.Fun) || len(call.Args) != 1 {
				return true
			}
			if basic, ok := call.Args[0].(*ast.BasicLit); ok && basic.Kind == token.STRING {
				regexpArguments[basic.Pos()] = struct{}{}
			}
			return true
		})
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			basic, ok := node.(*ast.BasicLit)
			if !ok {
				return true
			}
			kind, normalized, normalizeErr := normalizeBasicLiteral(basic)
			if normalizeErr != nil {
				return true
			}
			if _, ok := regexpArguments[basic.Pos()]; ok {
				kind = "regexp"
				normalized = strings.TrimPrefix(normalized, "(?i)")
			}
			key := kind + "\x00" + normalized
			entry := literals[key]
			if entry == nil {
				entry = &literalAccumulator{files: make(map[string]struct{})}
				literals[key] = entry
			}
			entry.count++
			entry.files[parsed.relative] = struct{}{}
			return true
		})
	}
	slices.SortFunc(result.Declarations, func(left, right declaration) int {
		if compared := strings.Compare(left.File, right.File); compared != 0 {
			return compared
		}
		return strings.Compare(left.Name, right.Name)
	})
	for key, accumulated := range literals {
		kind, normalized, _ := strings.Cut(key, "\x00")
		files := make([]string, 0, len(accumulated.files))
		for file := range accumulated.files {
			files = append(files, file)
		}
		slices.Sort(files)
		result.Literals = append(result.Literals, literal{
			Kind:  kind,
			Value: normalized,
			Count: accumulated.count,
			Files: files,
		})
	}
	slices.SortFunc(result.Literals, func(left, right literal) int {
		if compared := strings.Compare(left.Kind, right.Kind); compared != 0 {
			return compared
		}
		return strings.Compare(left.Value, right.Value)
	})
	return result, nil
}

func (e *extractor) registerTopLevelDeclarations(file *ast.File) {
	for _, declarationNode := range file.Decls {
		generic, ok := declarationNode.(*ast.GenDecl)
		if !ok || (generic.Tok != token.CONST && generic.Tok != token.VAR) {
			continue
		}
		var inherited []ast.Expr
		for _, specification := range generic.Specs {
			spec, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			values := spec.Values
			if len(values) == 0 {
				values = inherited
			} else {
				inherited = values
			}
			for index, name := range spec.Names {
				expression := expressionForIndex(values, index)
				if expression == nil {
					continue
				}
				e.expressions[name.Name] = expression
			}
		}
	}
}

func (e *extractor) topLevelDeclarations(relative string, file *ast.File) []declaration {
	var result []declaration
	for _, declarationNode := range file.Decls {
		generic, ok := declarationNode.(*ast.GenDecl)
		if !ok || (generic.Tok != token.CONST && generic.Tok != token.VAR) {
			continue
		}
		var inherited []ast.Expr
		for _, specification := range generic.Specs {
			spec, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			values := spec.Values
			if len(values) == 0 {
				values = inherited
			} else {
				inherited = values
			}
			for index, name := range spec.Names {
				expression := expressionForIndex(values, index)
				if expression == nil {
					continue
				}
				value := e.evaluate(expression, make(map[string]bool))
				value.Expression = renderExpression(e.fileSet, expression)
				result = append(result, declaration{
					File:  relative,
					Name:  name.Name,
					Value: value,
				})
			}
		}
	}
	return result
}

func expressionForIndex(expressions []ast.Expr, index int) ast.Expr {
	if len(expressions) == 1 {
		return expressions[0]
	}
	if index >= 0 && index < len(expressions) {
		return expressions[index]
	}
	return nil
}

func (e *extractor) evaluate(expression ast.Expr, resolving map[string]bool) value {
	switch typed := expression.(type) {
	case *ast.ParenExpr:
		return e.evaluate(typed.X, resolving)
	case *ast.BasicLit:
		switch typed.Kind {
		case token.INT:
			integer, ok := new(big.Int).SetString(strings.ReplaceAll(typed.Value, "_", ""), 0)
			if ok {
				return value{Kind: "number", Number: integer.String()}
			}
		case token.STRING, token.CHAR:
			text, err := strconv.Unquote(typed.Value)
			if err == nil {
				return value{Kind: "string", Text: text}
			}
		}
	case *ast.Ident:
		switch typed.Name {
		case "true", "false":
			return value{Kind: "boolean", Text: typed.Name}
		}
		if resolving[typed.Name] {
			return value{Kind: "unknown"}
		}
		target := e.expressions[typed.Name]
		if target == nil {
			return value{Kind: "unknown"}
		}
		resolving[typed.Name] = true
		result := e.evaluate(target, resolving)
		delete(resolving, typed.Name)
		return result
	case *ast.SelectorExpr:
		if packageName, ok := typed.X.(*ast.Ident); ok && packageName.Name == "time" {
			durations := map[string]int64{
				"Nanosecond":  1,
				"Microsecond": 1_000,
				"Millisecond": 1_000_000,
				"Second":      1_000_000_000,
				"Minute":      60_000_000_000,
				"Hour":        3_600_000_000_000,
			}
			if duration, ok := durations[typed.Sel.Name]; ok {
				return value{Kind: "number", Number: strconv.FormatInt(duration, 10)}
			}
		}
	case *ast.UnaryExpr:
		operand := e.evaluate(typed.X, resolving)
		if operand.Kind == "number" && (typed.Op == token.ADD || typed.Op == token.SUB) {
			number, ok := new(big.Int).SetString(operand.Number, 10)
			if ok {
				if typed.Op == token.SUB {
					number.Neg(number)
				}
				return value{Kind: "number", Number: number.String()}
			}
		}
	case *ast.BinaryExpr:
		left := e.evaluate(typed.X, resolving)
		right := e.evaluate(typed.Y, resolving)
		if left.Kind == "number" && right.Kind == "number" {
			leftNumber, leftOK := new(big.Int).SetString(left.Number, 10)
			rightNumber, rightOK := new(big.Int).SetString(right.Number, 10)
			if leftOK && rightOK {
				result := new(big.Int)
				switch typed.Op {
				case token.ADD:
					result.Add(leftNumber, rightNumber)
				case token.SUB:
					result.Sub(leftNumber, rightNumber)
				case token.MUL:
					result.Mul(leftNumber, rightNumber)
				case token.QUO:
					if rightNumber.Sign() == 0 {
						return value{Kind: "unknown"}
					}
					result.Quo(leftNumber, rightNumber)
				case token.SHL:
					if !rightNumber.IsUint64() {
						return value{Kind: "unknown"}
					}
					result.Lsh(leftNumber, uint(rightNumber.Uint64()))
				default:
					return value{Kind: "unknown"}
				}
				return value{Kind: "number", Number: result.String()}
			}
		}
	case *ast.CompositeLit:
		items := make([]value, 0, len(typed.Elts))
		properties := make(map[string]value)
		keyed := false
		for _, element := range typed.Elts {
			if keyValue, ok := element.(*ast.KeyValueExpr); ok {
				keyed = true
				key := renderExpression(e.fileSet, keyValue.Key)
				if identifier, ok := keyValue.Key.(*ast.Ident); ok {
					key = identifier.Name
				} else if basic, ok := keyValue.Key.(*ast.BasicLit); ok && basic.Kind == token.STRING {
					if decoded, err := strconv.Unquote(basic.Value); err == nil {
						key = decoded
					}
				}
				properties[key] = e.evaluate(keyValue.Value, resolving)
				continue
			}
			items = append(items, e.evaluate(element, resolving))
		}
		if keyed {
			return value{Kind: "object", Properties: properties}
		}
		return value{Kind: "array", Items: items}
	case *ast.CallExpr:
		if isRegexpMustCompile(typed.Fun) && len(typed.Args) == 1 {
			pattern := e.evaluate(typed.Args[0], resolving)
			if pattern.Kind == "string" {
				return value{Kind: "regexp", Pattern: pattern.Text}
			}
		}
	}
	return value{Kind: "unknown"}
}

func isRegexpMustCompile(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "MustCompile" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && packageName.Name == "regexp"
}

func renderExpression(fileSet *token.FileSet, expression ast.Expr) string {
	var builder strings.Builder
	if err := format.Node(&builder, fileSet, expression); err != nil {
		return ""
	}
	return builder.String()
}

func normalizeBasicLiteral(literal *ast.BasicLit) (string, string, error) {
	switch literal.Kind {
	case token.INT:
		number, ok := new(big.Int).SetString(strings.ReplaceAll(literal.Value, "_", ""), 0)
		if !ok {
			return "", "", errors.New("invalid integer literal")
		}
		return "number", number.String(), nil
	case token.FLOAT:
		rational, ok := new(big.Rat).SetString(strings.ReplaceAll(literal.Value, "_", ""))
		if !ok {
			return "", "", errors.New("invalid float literal")
		}
		return "number", rational.RatString(), nil
	case token.STRING, token.CHAR:
		text, err := strconv.Unquote(literal.Value)
		if err != nil {
			return "", "", err
		}
		return "string", text, nil
	default:
		return "", "", errors.New("unsupported literal")
	}
}
