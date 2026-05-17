package gillmprovider

import "strings"

type GoogleToolGroup struct {
	FunctionDeclarations []GoogleFunctionDeclaration `json:"functionDeclarations"`
}

type GoogleFunctionDeclaration struct {
	Name                 string `json:"name"`
	Description          string `json:"description,omitempty"`
	Parameters           any    `json:"parameters,omitempty"`
	ParametersJSONSchema any    `json:"parametersJsonSchema,omitempty"`
}

type GoogleContent struct {
	Role  string       `json:"role"`
	Parts []GooglePart `json:"parts"`
}

type GooglePart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	InlineData       *GoogleInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *GoogleFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GoogleFunctionResponse `json:"functionResponse,omitempty"`
}

type GoogleInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type GoogleFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
	ID   string         `json:"id,omitempty"`
}

type GoogleFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
	Parts    []GooglePart   `json:"parts,omitempty"`
	ID       string         `json:"id,omitempty"`
}

type GoogleThinkingOptions struct {
	Enabled       *bool
	Level         string
	BudgetTokens  *int
	Reasoning     string
	CustomBudgets map[string]int
}

type GoogleThinkingConfig struct {
	IncludeThoughts *bool  `json:"includeThoughts,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
	ThinkingBudget  *int   `json:"thinkingBudget,omitempty"`
}

func ConvertGoogleTools(tools []Tool, useParameters bool) []GoogleToolGroup {
	if len(tools) == 0 {
		return nil
	}
	declarations := make([]GoogleFunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		declaration := GoogleFunctionDeclaration{Name: tool.Name, Description: tool.Description}
		if useParameters {
			declaration.Parameters = SanitizeSchemaForOpenAPI(SchemaToMap(tool.Parameters))
		} else {
			declaration.ParametersJSONSchema = SchemaToMap(tool.Parameters)
		}
		declarations = append(declarations, declaration)
	}
	return []GoogleToolGroup{{FunctionDeclarations: declarations}}
}

func SanitizeSchemaForOpenAPI(schema any) any {
	switch value := schema.(type) {
	case map[string]any:
		result := map[string]any{}
		for key, child := range value {
			if isJSONSchemaMetaDeclaration(key) {
				continue
			}
			result[key] = SanitizeSchemaForOpenAPI(child)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, child := range value {
			result[i] = SanitizeSchemaForOpenAPI(child)
		}
		return result
	default:
		return schema
	}
}

func SchemaToMap(schema Schema) map[string]any {
	result := map[string]any{}
	if schema.Type != nil {
		result["type"] = schema.Type
	}
	if len(schema.Properties) > 0 {
		properties := map[string]any{}
		for key, value := range schema.Properties {
			properties[key] = SchemaToMap(value)
		}
		result["properties"] = properties
	}
	if len(schema.Required) > 0 {
		required := make([]any, len(schema.Required))
		for i, value := range schema.Required {
			required[i] = value
		}
		result["required"] = required
	}
	if schema.Items != nil {
		result["items"] = SchemaToMap(*schema.Items)
	}
	if len(schema.Enum) > 0 {
		result["enum"] = append([]any{}, schema.Enum...)
	}
	return result
}

func ConvertGoogleMessages(model Model, context Context) []GoogleContent {
	normalize := func(id string, target Model, _ Message) string {
		if !RequiresGoogleToolCallID(target.ID) {
			return id
		}
		return NormalizeToolCallIDForAnthropic(id)
	}
	transformed := TransformMessages(context.Messages, model, normalize)
	var contents []GoogleContent

	for _, message := range transformed {
		switch message.Role {
		case RoleUser:
			parts := convertGoogleUserParts(message.Content)
			if len(parts) > 0 {
				contents = append(contents, GoogleContent{Role: "user", Parts: parts})
			}
		case RoleAssistant:
			parts := convertGoogleAssistantParts(model, message)
			if len(parts) > 0 {
				contents = append(contents, GoogleContent{Role: "model", Parts: parts})
			}
		case RoleToolResult:
			functionResponsePart, imageParts := convertGoogleToolResultPart(model, message)
			last := len(contents) - 1
			if last >= 0 && contents[last].Role == "user" && googlePartsContainFunctionResponse(contents[last].Parts) {
				contents[last].Parts = append(contents[last].Parts, functionResponsePart)
			} else {
				contents = append(contents, GoogleContent{Role: "user", Parts: []GooglePart{functionResponsePart}})
			}
			if len(imageParts) > 0 && !SupportsGoogleMultimodalFunctionResponse(model.ID) {
				contents = append(contents, GoogleContent{Role: "user", Parts: append([]GooglePart{{Text: "Tool result image:"}}, imageParts...)})
			}
		}
	}
	return contents
}

func RequiresGoogleToolCallID(modelID string) bool {
	return strings.HasPrefix(modelID, "claude-") || strings.HasPrefix(modelID, "gpt-oss-")
}

func SupportsGoogleMultimodalFunctionResponse(modelID string) bool {
	lower := strings.ToLower(modelID)
	if strings.HasPrefix(lower, "gemini-3") || strings.HasPrefix(lower, "gemini-live-3") {
		return true
	}
	if strings.HasPrefix(lower, "gemini-1") || strings.HasPrefix(lower, "gemini-2") || strings.HasPrefix(lower, "gemini-live-1") || strings.HasPrefix(lower, "gemini-live-2") {
		return false
	}
	return true
}

func RetainGoogleThoughtSignature(existing, incoming string) string {
	if incoming != "" {
		return incoming
	}
	return existing
}

func IsGoogleThinkingPart(part GooglePart) bool {
	return part.Thought
}

func BuildGoogleThinkingConfig(model Model, options GoogleThinkingOptions) *GoogleThinkingConfig {
	if !model.Reasoning {
		return nil
	}
	if options.Enabled != nil {
		if !*options.Enabled {
			return DisabledGoogleThinkingConfig(model)
		}
		config := &GoogleThinkingConfig{IncludeThoughts: ptrBool(true)}
		if options.Level != "" {
			config.ThinkingLevel = options.Level
		} else if options.BudgetTokens != nil {
			config.ThinkingBudget = options.BudgetTokens
		}
		return config
	}
	if options.Reasoning == "" {
		return DisabledGoogleThinkingConfig(model)
	}
	level := ClampThinkingLevel(model, options.Reasoning)
	if level == "off" {
		level = "high"
	}
	if isGemini3ProModel(model) || isGemini3FlashModel(model) || isGemma4Model(model) {
		return &GoogleThinkingConfig{IncludeThoughts: ptrBool(true), ThinkingLevel: GoogleThinkingLevel(level, model)}
	}
	budget := GoogleThinkingBudget(model, level, options.CustomBudgets)
	return &GoogleThinkingConfig{IncludeThoughts: ptrBool(true), ThinkingBudget: &budget}
}

func DisabledGoogleThinkingConfig(model Model) *GoogleThinkingConfig {
	if isGemini3ProModel(model) {
		return &GoogleThinkingConfig{ThinkingLevel: "LOW"}
	}
	if isGemini3FlashModel(model) || isGemma4Model(model) {
		return &GoogleThinkingConfig{ThinkingLevel: "MINIMAL"}
	}
	zero := 0
	return &GoogleThinkingConfig{ThinkingBudget: &zero}
}

func GoogleThinkingLevel(effort string, model Model) string {
	if isGemini3ProModel(model) {
		switch effort {
		case "minimal", "low":
			return "LOW"
		case "medium", "high", "xhigh":
			return "HIGH"
		}
	}
	if isGemma4Model(model) {
		switch effort {
		case "minimal", "low":
			return "MINIMAL"
		case "medium", "high", "xhigh":
			return "HIGH"
		}
	}
	switch effort {
	case "minimal":
		return "MINIMAL"
	case "low":
		return "LOW"
	case "medium":
		return "MEDIUM"
	case "high", "xhigh":
		return "HIGH"
	default:
		return "HIGH"
	}
}

func GoogleThinkingBudget(model Model, effort string, custom map[string]int) int {
	if custom != nil {
		if value, ok := custom[effort]; ok {
			return value
		}
	}
	if strings.Contains(model.ID, "2.5-pro") {
		switch effort {
		case "minimal":
			return 128
		case "low":
			return 2048
		case "medium":
			return 8192
		default:
			return 32768
		}
	}
	if strings.Contains(model.ID, "2.5-flash-lite") {
		switch effort {
		case "minimal":
			return 512
		case "low":
			return 2048
		case "medium":
			return 8192
		default:
			return 24576
		}
	}
	if strings.Contains(model.ID, "2.5-flash") {
		switch effort {
		case "minimal":
			return 128
		case "low":
			return 2048
		case "medium":
			return 8192
		default:
			return 24576
		}
	}
	return -1
}

func convertGoogleUserParts(content []ContentPart) []GooglePart {
	parts := make([]GooglePart, 0, len(content))
	for _, part := range content {
		switch part.Type {
		case ContentText:
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			parts = append(parts, GooglePart{Text: SanitizeSurrogates(part.Text)})
		case ContentImage:
			parts = append(parts, GooglePart{InlineData: &GoogleInlineData{MIMEType: part.MIMEType, Data: part.Data}})
		}
	}
	return parts
}

func convertGoogleAssistantParts(model Model, message Message) []GooglePart {
	isSameProviderAndModel := message.Provider == model.Provider && message.Model == model.ID
	parts := []GooglePart{}
	for _, part := range message.Content {
		switch part.Type {
		case ContentText:
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			parts = append(parts, GooglePart{Text: SanitizeSurrogates(part.Text), ThoughtSignature: resolveGoogleThoughtSignature(isSameProviderAndModel, part.TextSignature)})
		case ContentThinking:
			if strings.TrimSpace(part.Thinking) == "" {
				continue
			}
			if isSameProviderAndModel {
				parts = append(parts, GooglePart{Thought: true, Text: SanitizeSurrogates(part.Thinking), ThoughtSignature: resolveGoogleThoughtSignature(isSameProviderAndModel, part.ThinkingSignature)})
			} else {
				parts = append(parts, GooglePart{Text: SanitizeSurrogates(part.Thinking)})
			}
		case ContentToolCall:
			call := &GoogleFunctionCall{Name: part.Name, Args: part.Arguments}
			if call.Args == nil {
				call.Args = map[string]any{}
			}
			if RequiresGoogleToolCallID(model.ID) {
				call.ID = part.ID
			}
			parts = append(parts, GooglePart{FunctionCall: call, ThoughtSignature: resolveGoogleThoughtSignature(isSameProviderAndModel, part.ThoughtSignature)})
		}
	}
	return parts
}

func convertGoogleToolResultPart(model Model, message Message) (GooglePart, []GooglePart) {
	textResult := SanitizeSurrogates(joinTextContent(message.Content))
	imageParts := []GooglePart{}
	if containsString(model.Input, "image") {
		for _, part := range message.Content {
			if part.Type == ContentImage {
				imageParts = append(imageParts, GooglePart{InlineData: &GoogleInlineData{MIMEType: part.MIMEType, Data: part.Data}})
			}
		}
	}
	responseValue := textResult
	if responseValue == "" && len(imageParts) > 0 {
		responseValue = "(see attached image)"
	}
	key := "output"
	if message.IsError {
		key = "error"
	}
	functionResponse := &GoogleFunctionResponse{
		Name:     message.ToolName,
		Response: map[string]any{key: responseValue},
	}
	if len(imageParts) > 0 && SupportsGoogleMultimodalFunctionResponse(model.ID) {
		functionResponse.Parts = imageParts
	}
	if RequiresGoogleToolCallID(model.ID) {
		functionResponse.ID = message.ToolCallID
	}
	return GooglePart{FunctionResponse: functionResponse}, imageParts
}

func googlePartsContainFunctionResponse(parts []GooglePart) bool {
	for _, part := range parts {
		if part.FunctionResponse != nil {
			return true
		}
	}
	return false
}

func resolveGoogleThoughtSignature(isSameProviderAndModel bool, signature string) string {
	if !isSameProviderAndModel || !isValidGoogleThoughtSignature(signature) {
		return ""
	}
	return signature
}

func isValidGoogleThoughtSignature(signature string) bool {
	if signature == "" || len(signature)%4 != 0 {
		return false
	}
	for _, r := range signature {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '=' {
			continue
		}
		return false
	}
	return true
}

func isGemini3ProModel(model Model) bool {
	id := strings.ToLower(model.ID)
	return strings.HasPrefix(id, "gemini-3-pro") || strings.HasPrefix(id, "gemini-3.1-pro")
}

func isGemini3FlashModel(model Model) bool {
	id := strings.ToLower(model.ID)
	return strings.HasPrefix(id, "gemini-3-flash") || strings.HasPrefix(id, "gemini-3.1-flash")
}

func isGemma4Model(model Model) bool {
	return strings.Contains(strings.ToLower(model.ID), "gemma-4") || strings.Contains(strings.ToLower(model.ID), "gemma4")
}

func isJSONSchemaMetaDeclaration(key string) bool {
	switch key {
	case "$schema", "$id", "$anchor", "$dynamicAnchor", "$vocabulary", "$comment", "$defs", "definitions":
		return true
	default:
		return false
	}
}
