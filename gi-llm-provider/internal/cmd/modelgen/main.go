// Command modelgen converts Pi's published provider model JSON into Gi's
// compiled Go catalog. It intentionally consumes the published package data:
// Pi's source tag does not contain the generated providers/data directory.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type options struct {
	dataDir string
	index   string
	output  string
	source  string
	pkg     string
}

type sourceModel struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	API              string             `json:"api"`
	Provider         string             `json:"provider"`
	BaseURL          string             `json:"baseUrl"`
	Reasoning        bool               `json:"reasoning"`
	Input            []string           `json:"input"`
	Cost             sourceCost         `json:"cost"`
	ContextWindow    int                `json:"contextWindow"`
	MaxTokens        int                `json:"maxTokens"`
	Headers          map[string]string  `json:"headers"`
	Compat           *sourceModelCompat `json:"compat"`
	ThinkingLevelMap map[string]*string `json:"thinkingLevelMap"`
}

type sourceCost struct {
	Input      float64          `json:"input"`
	Output     float64          `json:"output"`
	CacheRead  float64          `json:"cacheRead"`
	CacheWrite float64          `json:"cacheWrite"`
	Tiers      []sourceCostTier `json:"tiers"`
}

type sourceCostTier struct {
	InputTokensAbove int     `json:"inputTokensAbove"`
	Input            float64 `json:"input"`
	Output           float64 `json:"output"`
	CacheRead        float64 `json:"cacheRead"`
	CacheWrite       float64 `json:"cacheWrite"`
}

type sourceModelCompat struct {
	SupportsStore                               *bool          `json:"supportsStore"`
	SupportsDeveloperRole                       *bool          `json:"supportsDeveloperRole"`
	SupportsReasoningEffort                     *bool          `json:"supportsReasoningEffort"`
	SupportsUsageInStreaming                    *bool          `json:"supportsUsageInStreaming"`
	SupportsStrictMode                          *bool          `json:"supportsStrictMode"`
	SupportsOpenAIGrammarTools                  *bool          `json:"supportsOpenAIGrammarTools"`
	SupportsLongCacheRetention                  *bool          `json:"supportsLongCacheRetention"`
	SupportsEagerToolInputStreaming             *bool          `json:"supportsEagerToolInputStreaming"`
	SupportsCacheControlOnTools                 *bool          `json:"supportsCacheControlOnTools"`
	SupportsExplicitPromptCacheMode             *bool          `json:"supportsExplicitPromptCacheMode"`
	SupportsTemperature                         *bool          `json:"supportsTemperature"`
	SupportsStrictTools                         *bool          `json:"supportsStrictTools"`
	SupportsToolReferences                      *bool          `json:"supportsToolReferences"`
	SupportsToolSearch                          *bool          `json:"supportsToolSearch"`
	ForceAdaptiveThinking                       *bool          `json:"forceAdaptiveThinking"`
	AllowEmptySignature                         *bool          `json:"allowEmptySignature"`
	SendSessionAffinityHeaders                  *bool          `json:"sendSessionAffinityHeaders"`
	SendSessionIDHeader                         *bool          `json:"sendSessionIdHeader"`
	RequiresToolResultName                      *bool          `json:"requiresToolResultName"`
	RequiresAssistantAfterToolResult            *bool          `json:"requiresAssistantAfterToolResult"`
	RequiresThinkingAsText                      *bool          `json:"requiresThinkingAsText"`
	RequiresReasoningContentOnAssistantMessages *bool          `json:"requiresReasoningContentOnAssistantMessages"`
	RequiresReasoningContentOnAssistantTurns    *bool          `json:"requiresReasoningContentOnAssistantTurns"`
	RequiresReasoningContentOnAssistantEvents   *bool          `json:"requiresReasoningContentOnAssistantEvents"`
	ZAIToolStream                               *bool          `json:"zaiToolStream"`
	OpenRouterRouting                           map[string]any `json:"openRouterRouting"`
	VercelGatewayRouting                        map[string]any `json:"vercelGatewayRouting"`
	ChatTemplateKwargs                          map[string]any `json:"chatTemplateKwargs"`
	MaxTokensField                              string         `json:"maxTokensField"`
	ThinkingFormat                              string         `json:"thinkingFormat"`
	CacheControlFormat                          string         `json:"cacheControlFormat"`
	DeferredToolsMode                           string         `json:"deferredToolsMode"`
	SessionAffinityFormat                       string         `json:"sessionAffinityFormat"`
}

type providerCatalog struct {
	id     string
	models []sourceModel
}

func main() {
	var opts options
	flag.StringVar(&opts.dataDir, "data-dir", "", "Pi package dist/providers/data directory")
	flag.StringVar(&opts.index, "index", "", "Pi package dist/models.generated.js path")
	flag.StringVar(&opts.output, "output", "pi_models_generated.go", "generated Go output")
	flag.StringVar(&opts.source, "source", "", "source provenance written into the generated header")
	flag.StringVar(&opts.pkg, "package", "gillmprovider", "generated Go package name")
	flag.Parse()

	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "modelgen:", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	if strings.TrimSpace(opts.dataDir) == "" {
		return errors.New("-data-dir is required")
	}
	dataDir, err := filepath.Abs(opts.dataDir)
	if err != nil {
		return err
	}
	if opts.index == "" {
		opts.index = filepath.Join(dataDir, "..", "..", "models.generated.js")
	}
	if opts.source == "" {
		opts.source = dataDir
	}
	if !isGoIdentifier(opts.pkg) {
		return fmt.Errorf("invalid package name %q", opts.pkg)
	}

	providerOrder, err := readProviderOrder(opts.index)
	if err != nil {
		return err
	}
	catalogs := make([]providerCatalog, 0, len(providerOrder))
	for _, providerID := range providerOrder {
		models, err := readProviderModels(
			filepath.Join(dataDir, providerID+".json"),
			providerID,
		)
		if err != nil {
			return err
		}
		catalogs = append(catalogs, providerCatalog{id: providerID, models: models})
	}
	if err := verifyNoUnindexedCatalogs(dataDir, providerOrder); err != nil {
		return err
	}

	generated, err := renderCatalog(opts.pkg, opts.source, catalogs)
	if err != nil {
		return err
	}
	return os.WriteFile(opts.output, generated, 0o644)
}

func readProviderOrder(indexPath string) ([]string, error) {
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read model index: %w", err)
	}
	re := regexp.MustCompile(`from\s+["']\./providers/([^"']+)\.models\.(?:js|ts)["']`)
	matches := re.FindAllSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no provider imports found in %s", indexPath)
	}
	order := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		providerID := string(match[1])
		if _, exists := seen[providerID]; exists {
			continue
		}
		seen[providerID] = struct{}{}
		order = append(order, providerID)
	}
	return order, nil
}

func verifyNoUnindexedCatalogs(dataDir string, providerOrder []string) error {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return fmt.Errorf("read data directory: %w", err)
	}
	indexed := make(map[string]struct{}, len(providerOrder))
	for _, providerID := range providerOrder {
		indexed[providerID+".json"] = struct{}{}
	}
	var unexpected []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") ||
			strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if _, exists := indexed[entry.Name()]; !exists {
			unexpected = append(unexpected, entry.Name())
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return fmt.Errorf("catalogs absent from model index: %s", strings.Join(unexpected, ", "))
	}
	return nil
}

func readProviderModels(path, providerID string) ([]sourceModel, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s catalog: %w", providerID, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := expectDelimiter(decoder, '{'); err != nil {
		return nil, fmt.Errorf("decode %s catalog: %w", providerID, err)
	}
	var models []sourceModel
	for decoder.More() {
		api, err := readObjectKey(decoder)
		if err != nil {
			return nil, fmt.Errorf("decode %s API name: %w", providerID, err)
		}
		if err := expectDelimiter(decoder, '{'); err != nil {
			return nil, fmt.Errorf("decode %s/%s group: %w", providerID, api, err)
		}
		for decoder.More() {
			modelID, err := readObjectKey(decoder)
			if err != nil {
				return nil, fmt.Errorf("decode %s/%s model ID: %w", providerID, api, err)
			}
			var model sourceModel
			if err := decoder.Decode(&model); err != nil {
				return nil, fmt.Errorf("decode %s/%s: %w", providerID, modelID, err)
			}
			if model.ID != modelID {
				return nil, fmt.Errorf(
					"%s catalog key %q does not match model ID %q",
					providerID,
					modelID,
					model.ID,
				)
			}
			if model.Provider != providerID {
				return nil, fmt.Errorf(
					"%s/%s declares provider %q",
					providerID,
					modelID,
					model.Provider,
				)
			}
			if model.API != api {
				return nil, fmt.Errorf(
					"%s/%s is in API group %q but declares %q",
					providerID,
					modelID,
					api,
					model.API,
				)
			}
			models = append(models, model)
		}
		if err := expectDelimiter(decoder, '}'); err != nil {
			return nil, fmt.Errorf("close %s/%s group: %w", providerID, api, err)
		}
	}
	if err := expectDelimiter(decoder, '}'); err != nil {
		return nil, fmt.Errorf("close %s catalog: %w", providerID, err)
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing token %v in %s catalog", token, providerID)
		}
		return nil, fmt.Errorf("decode trailing %s data: %w", providerID, err)
	}
	return models, nil
}

func readObjectKey(decoder *json.Decoder) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", err
	}
	key, ok := token.(string)
	if !ok {
		return "", fmt.Errorf("got token %v, want object key", token)
	}
	return key, nil
}

func expectDelimiter(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	got, ok := token.(json.Delim)
	if !ok || got != want {
		return fmt.Errorf("got token %v, want %q", token, want)
	}
	return nil
}

func renderCatalog(pkg, source string, catalogs []providerCatalog) ([]byte, error) {
	var output bytes.Buffer
	fmt.Fprintf(
		&output,
		"// Code generated by go run ./internal/cmd/modelgen; DO NOT EDIT.\n"+
			"// Source: %s\n\npackage %s\n\nfunc registerPiGeneratedModels() {\n"+
			"\tresetModelRegistry()\n",
		source,
		pkg,
	)
	for _, catalog := range catalogs {
		for _, model := range catalog.models {
			renderModel(&output, model)
		}
	}
	output.WriteString("}\n")
	command := exec.Command("gofmt")
	command.Stdin = bytes.NewReader(output.Bytes())
	formatted, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf(
				"format generated catalog: %w: %s",
				err,
				strings.TrimSpace(string(exitErr.Stderr)),
			)
		}
		return nil, fmt.Errorf("format generated catalog: %w", err)
	}
	return formatted, nil
}

func renderModel(output *bytes.Buffer, model sourceModel) {
	output.WriteString("\tRegisterModel(Model{\n")
	writeStringField(output, "ID", model.ID)
	writeStringField(output, "Name", model.Name)
	writeStringField(output, "API", model.API)
	writeStringField(output, "Provider", model.Provider)
	writeStringField(output, "BaseURL", model.BaseURL)
	if model.Reasoning {
		output.WriteString("\t\tReasoning: true,\n")
	}
	if len(model.Input) > 0 {
		fmt.Fprintf(output, "\t\tInput: %s,\n", stringSliceLiteral(model.Input))
	}
	fmt.Fprintf(output, "\t\tCost: %s,\n", costLiteral(model.Cost))
	fmt.Fprintf(output, "\t\tContextWindow: %d,\n", model.ContextWindow)
	fmt.Fprintf(output, "\t\tMaxTokens: %d,\n", model.MaxTokens)
	if len(model.Headers) > 0 {
		fmt.Fprintf(output, "\t\tHeaders: %s,\n", stringMapLiteral(model.Headers))
	}
	if model.Compat != nil {
		fmt.Fprintf(output, "\t\tCompat: %s,\n", compatLiteral(*model.Compat))
	}
	if len(model.ThinkingLevelMap) > 0 {
		fmt.Fprintf(
			output,
			"\t\tThinkingLevelMap: %s,\n",
			optionalStringMapLiteral(model.ThinkingLevelMap),
		)
	}
	output.WriteString("\t})\n")
}

func writeStringField(output *bytes.Buffer, name, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(output, "\t\t%s: %s,\n", name, strconv.Quote(value))
}

func costLiteral(cost sourceCost) string {
	fields := []string{
		"Input: " + floatLiteral(cost.Input),
		"Output: " + floatLiteral(cost.Output),
		"CacheRead: " + floatLiteral(cost.CacheRead),
		"CacheWrite: " + floatLiteral(cost.CacheWrite),
	}
	if len(cost.Tiers) > 0 {
		tiers := make([]string, 0, len(cost.Tiers))
		for _, tier := range cost.Tiers {
			tiers = append(tiers, fmt.Sprintf(
				"{InputTokensAbove: %d, Input: %s, Output: %s, CacheRead: %s, CacheWrite: %s}",
				tier.InputTokensAbove,
				floatLiteral(tier.Input),
				floatLiteral(tier.Output),
				floatLiteral(tier.CacheRead),
				floatLiteral(tier.CacheWrite),
			))
		}
		fields = append(fields, "Tiers: []ModelCostTier{"+strings.Join(tiers, ", ")+"}")
	}
	return "ModelCost{" + strings.Join(fields, ", ") + "}"
}

func compatLiteral(compat sourceModelCompat) string {
	var fields []string
	appendBool := func(name string, value *bool) {
		if value != nil {
			fields = append(fields, name+": ptrBool("+strconv.FormatBool(*value)+")")
		}
	}
	appendString := func(name, value string) {
		if value != "" {
			fields = append(fields, name+": "+strconv.Quote(value))
		}
	}
	appendMap := func(name string, value map[string]any) {
		if len(value) > 0 {
			fields = append(fields, name+": "+anyMapLiteral(value))
		}
	}

	appendBool("SupportsStore", compat.SupportsStore)
	appendBool("SupportsDeveloperRole", compat.SupportsDeveloperRole)
	appendBool("SupportsReasoningEffort", compat.SupportsReasoningEffort)
	appendBool("SupportsUsageInStreaming", compat.SupportsUsageInStreaming)
	appendBool("SupportsStrictMode", compat.SupportsStrictMode)
	appendBool("SupportsOpenAIGrammarTools", compat.SupportsOpenAIGrammarTools)
	appendBool("SupportsLongCacheRetention", compat.SupportsLongCacheRetention)
	appendBool("SupportsEagerToolInputStreaming", compat.SupportsEagerToolInputStreaming)
	appendBool("SupportsCacheControlOnTools", compat.SupportsCacheControlOnTools)
	appendBool("SupportsExplicitPromptCacheMode", compat.SupportsExplicitPromptCacheMode)
	appendBool("SupportsTemperature", compat.SupportsTemperature)
	appendBool("SupportsStrictTools", compat.SupportsStrictTools)
	appendBool("SupportsToolReferences", compat.SupportsToolReferences)
	appendBool("SupportsToolSearch", compat.SupportsToolSearch)
	appendBool("ForceAdaptiveThinking", compat.ForceAdaptiveThinking)
	appendBool("AllowEmptySignature", compat.AllowEmptySignature)
	appendBool("SendSessionAffinityHeaders", compat.SendSessionAffinityHeaders)
	appendBool("SendSessionIDHeader", compat.SendSessionIDHeader)
	appendBool("RequiresToolResultName", compat.RequiresToolResultName)
	appendBool("RequiresAssistantAfterToolResult", compat.RequiresAssistantAfterToolResult)
	appendBool("RequiresThinkingAsText", compat.RequiresThinkingAsText)
	appendBool(
		"RequiresReasoningContentOnAssistantMessages",
		compat.RequiresReasoningContentOnAssistantMessages,
	)
	appendBool(
		"RequiresReasoningContentOnAssistantTurns",
		compat.RequiresReasoningContentOnAssistantTurns,
	)
	appendBool(
		"RequiresReasoningContentOnAssistantEvents",
		compat.RequiresReasoningContentOnAssistantEvents,
	)
	appendBool("ZAIToolStream", compat.ZAIToolStream)
	appendMap("OpenRouterRouting", compat.OpenRouterRouting)
	appendMap("VercelGatewayRouting", compat.VercelGatewayRouting)
	appendMap("ChatTemplateKwargs", compat.ChatTemplateKwargs)
	appendString("MaxTokensField", compat.MaxTokensField)
	appendString("ThinkingFormat", compat.ThinkingFormat)
	appendString("CacheControlFormat", compat.CacheControlFormat)
	appendString("DeferredToolsMode", compat.DeferredToolsMode)
	appendString("SessionAffinityFormat", compat.SessionAffinityFormat)
	return "ModelCompat{" + strings.Join(fields, ", ") + "}"
}

func stringSliceLiteral(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = strconv.Quote(value)
	}
	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

func stringMapLiteral(values map[string]string) string {
	keys := sortedKeys(values)
	fields := make([]string, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, strconv.Quote(key)+": "+strconv.Quote(values[key]))
	}
	return "map[string]string{" + strings.Join(fields, ", ") + "}"
}

func optionalStringMapLiteral(values map[string]*string) string {
	keys := sortedKeys(values)
	fields := make([]string, 0, len(keys))
	for _, key := range keys {
		value := "nil"
		if values[key] != nil {
			value = "ptrString(" + strconv.Quote(*values[key]) + ")"
		}
		fields = append(fields, strconv.Quote(key)+": "+value)
	}
	return "map[string]*string{" + strings.Join(fields, ", ") + "}"
}

func anyMapLiteral(values map[string]any) string {
	keys := sortedKeys(values)
	fields := make([]string, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, strconv.Quote(key)+": "+anyLiteral(values[key]))
	}
	return "map[string]any{" + strings.Join(fields, ", ") + "}"
}

func anyLiteral(value any) string {
	switch value := value.(type) {
	case nil:
		return "nil"
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return floatLiteral(value)
	case string:
		return strconv.Quote(value)
	case []any:
		values := make([]string, len(value))
		for index, item := range value {
			values[index] = anyLiteral(item)
		}
		return "[]any{" + strings.Join(values, ", ") + "}"
	case map[string]any:
		return anyMapLiteral(value)
	default:
		panic(fmt.Sprintf("unsupported JSON value %T", value))
	}
}

func floatLiteral(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isGoIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
			index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}
