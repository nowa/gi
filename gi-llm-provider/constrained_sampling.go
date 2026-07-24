package gillmprovider

import (
	"encoding/json"
	"fmt"
	"strings"
)

type GrammarFormat string

const (
	GrammarFormatLark  GrammarFormat = "lark"
	GrammarFormatRegex GrammarFormat = "regex"
)

type GrammarConstrainedSampling struct {
	Format        GrammarFormat
	Definition    string
	InputProperty string
}

// GrammarToolInputJSONBuffer tracks the append-only JSON projection exposed to
// generic tool-call stream consumers while a provider streams raw grammar text.
type GrammarToolInputJSONBuffer struct {
	Input   string
	Started bool
	Closed  bool
}

// OpenAIResponsesSamplingState is the immutable sampling snapshot shared by
// request conversion and stream decoding.
type OpenAIResponsesSamplingState struct {
	ToolOptions                OpenAIResponsesToolOptions
	GrammarToolInputProperties map[string]string
}

type OpenAIResponsesSamplingDefaults struct {
	SupportsStrictMode bool
	Strict             OpenAIResponsesStrictDefault
}

func ResolveOpenAIResponsesSamplingState(
	model Model,
	tools []Tool,
	defaults OpenAIResponsesSamplingDefaults,
) (OpenAIResponsesSamplingState, error) {
	supportsStrictMode := defaults.SupportsStrictMode
	if model.Compat.SupportsStrictMode != nil {
		supportsStrictMode = *model.Compat.SupportsStrictMode
	}
	supportsGrammar := false
	if model.Compat.SupportsOpenAIGrammarTools != nil {
		supportsGrammar = *model.Compat.SupportsOpenAIGrammarTools
	}
	properties, err := CreateGrammarToolInputProperties(tools, supportsGrammar)
	if err != nil {
		return OpenAIResponsesSamplingState{}, err
	}
	return OpenAIResponsesSamplingState{
		ToolOptions: OpenAIResponsesToolOptions{
			DefaultStrict:              defaults.Strict,
			SupportsStrictMode:         ptrBool(supportsStrictMode),
			SupportsOpenAIGrammarTools: supportsGrammar,
		},
		GrammarToolInputProperties: properties,
	}, nil
}

func GetGrammarToolInput(toolName string, arguments map[string]any, inputProperty string) (string, error) {
	input, ok := arguments[inputProperty].(string)
	if !ok {
		return "", fmt.Errorf(
			"grammar tool call %q requires argument %q to be a string",
			toolName,
			inputProperty,
		)
	}
	return input, nil
}

// AppendGrammarToolInputJSONDelta turns a monotonically growing raw grammar
// input into an append-only JSON object stream. The bool result distinguishes a
// deliberate no-op from an emitted empty string.
func AppendGrammarToolInputJSONDelta(
	buffer *GrammarToolInputJSONBuffer,
	inputProperty string,
	nextInput string,
	closeInput bool,
) (string, bool, error) {
	if buffer == nil {
		return "", false, fmt.Errorf("grammar tool input buffer is nil")
	}
	if buffer.Closed {
		if closeInput && nextInput == buffer.Input {
			return "", false, nil
		}
		return "", false, fmt.Errorf(
			"grammar tool input for property %q changed after it was closed",
			inputProperty,
		)
	}
	if !strings.HasPrefix(nextInput, buffer.Input) {
		return "", false, fmt.Errorf(
			"grammar tool input for property %q changed non-monotonically",
			inputProperty,
		)
	}

	inputDelta := strings.TrimPrefix(nextInput, buffer.Input)
	if !closeInput && inputDelta == "" {
		return "", false, nil
	}

	var delta strings.Builder
	if !buffer.Started {
		propertyJSON, _ := json.Marshal(inputProperty)
		delta.WriteByte('{')
		delta.Write(propertyJSON)
		delta.WriteString(`:"`)
		buffer.Started = true
	}
	encodedDelta, _ := json.Marshal(inputDelta)
	delta.Write(encodedDelta[1 : len(encodedDelta)-1])
	buffer.Input = nextInput

	if closeInput {
		delta.WriteString(`"}`)
		buffer.Closed = true
	}
	return delta.String(), true, nil
}

func ResolveJSONSchemaStrictSampling(tool Tool, supportsStrictMode bool) (*bool, error) {
	config := tool.ConstrainedSampling
	if config == nil || config.Type != ConstrainedSamplingJSONSchema {
		return nil, nil
	}
	if supportsStrictMode {
		return ptrBool(true), nil
	}
	if config.Strict == ConstrainedSamplingRequire {
		return nil, fmt.Errorf(
			"tool %q requires JSON-schema constrained sampling, but strict tools are unsupported",
			tool.Name,
		)
	}
	return nil, nil
}

func ResolveGrammarConstrainedSampling(
	tool Tool,
	supportsOpenAIGrammarTools bool,
) (GrammarConstrainedSampling, bool, error) {
	config := tool.ConstrainedSampling
	if config == nil || config.Type != ConstrainedSamplingGrammar || !supportsOpenAIGrammarTools {
		return GrammarConstrainedSampling{}, false, nil
	}

	larkDefinition := config.Variants.OpenAILark
	regexDefinition := config.Variants.OpenAIRegex
	hasLarkDefinition := strings.TrimSpace(larkDefinition) != ""
	hasRegexDefinition := strings.TrimSpace(regexDefinition) != ""
	if !hasLarkDefinition && !hasRegexDefinition {
		return GrammarConstrainedSampling{}, false, fmt.Errorf(
			"tool %q cannot use grammar constrained sampling: no supported grammar variant was provided",
			tool.Name,
		)
	}

	inputProperty, err := inferGrammarInputProperty(tool)
	if err != nil {
		return GrammarConstrainedSampling{}, false, fmt.Errorf(
			"tool %q cannot use grammar constrained sampling: %w",
			tool.Name,
			err,
		)
	}
	if hasLarkDefinition {
		return GrammarConstrainedSampling{
			Format:        GrammarFormatLark,
			Definition:    larkDefinition,
			InputProperty: inputProperty,
		}, true, nil
	}
	return GrammarConstrainedSampling{
		Format:        GrammarFormatRegex,
		Definition:    regexDefinition,
		InputProperty: inputProperty,
	}, true, nil
}

func CreateGrammarToolInputProperties(
	tools []Tool,
	supportsOpenAIGrammarTools bool,
) (map[string]string, error) {
	properties := make(map[string]string)
	for _, tool := range tools {
		grammar, ok, err := ResolveGrammarConstrainedSampling(tool, supportsOpenAIGrammarTools)
		if err != nil {
			return nil, err
		}
		if ok {
			properties[tool.Name] = grammar.InputProperty
		}
	}
	return properties, nil
}

func inferGrammarInputProperty(tool Tool) (string, error) {
	schemaType, ok := tool.Parameters.Type.(string)
	if !ok || schemaType != "object" {
		return "", fmt.Errorf("grammar constrained sampling requires an object parameter schema")
	}
	if len(tool.Parameters.Required) != 1 {
		return "", fmt.Errorf("grammar constrained sampling requires exactly one required string property")
	}

	inputProperty := tool.Parameters.Required[0]
	property, ok := tool.Parameters.Properties[inputProperty]
	if !ok {
		return "", fmt.Errorf(
			"grammar constrained sampling requires a properties entry for %s",
			inputProperty,
		)
	}
	propertyType, ok := property.Type.(string)
	if !ok || propertyType != "string" {
		return "", fmt.Errorf(
			"grammar constrained sampling property %s must have type string",
			inputProperty,
		)
	}
	return inputProperty, nil
}
