package gitui

import (
	"strings"

	"github.com/yuin/goldmark"
	goldmarkast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var markdownGoldmarkParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

type markdownDocument struct {
	Source          []byte
	Root            goldmarkast.Node
	Context         parser.Context
	LinkDefinitions map[string]string
}

func parseMarkdownDocument(lines []string) markdownDocument {
	source := []byte(strings.Join(lines, "\n"))
	root, context := parseMarkdownGoldmarkAST(source)
	return markdownDocument{
		Source:          source,
		Root:            root,
		Context:         context,
		LinkDefinitions: parseMarkdownLinkDefinitionsFromGoldmark(lines, context),
	}
}

func parseMarkdownGoldmarkAST(source []byte) (goldmarkast.Node, parser.Context) {
	context := parser.NewContext()
	root := markdownGoldmarkParser.Parser().Parse(text.NewReader(source), parser.WithContext(context))
	return root, context
}

func parseMarkdownLinkDefinitions(lines []string) map[string]string {
	return parseMarkdownDocument(lines).LinkDefinitions
}

func parseMarkdownLinkDefinitionsFromGoldmark(lines []string, context parser.Context) map[string]string {
	definitions := map[string]string{}

	// Keep the existing Pi compatibility shim as the authoritative source
	// while Markdown rendering migrates toward a full goldmark AST renderer.
	for label, url := range parseMarkdownLinkDefinitionsLegacy(lines) {
		addNormalizedMarkdownLinkDefinition(definitions, label, url)
	}

	for _, reference := range context.References() {
		addMarkdownLinkDefinition(definitions, string(reference.Label()), string(reference.Destination()))
	}
	return definitions
}
