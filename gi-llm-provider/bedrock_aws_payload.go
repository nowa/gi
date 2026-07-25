package gillmprovider

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

func buildAWSBedrockConverseStreamInput(
	request BedrockConverseStreamRequest,
) (*bedrockruntime.ConverseStreamInput, error) {
	if request.Model.ID == "" {
		return nil, errors.New("Bedrock model ID is required")
	}
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(request.Model.ID),
		RequestMetadata: cloneStringMap(request.RequestMetadata),
	}

	system, err := convertAWSBedrockSystem(request.Payload.System)
	if err != nil {
		return nil, err
	}
	input.System = system
	messages, err := convertAWSBedrockMessages(request.Payload.Messages)
	if err != nil {
		return nil, err
	}
	input.Messages = messages
	toolConfig, err := convertAWSBedrockToolConfig(request.Payload.ToolConfig)
	if err != nil {
		return nil, err
	}
	input.ToolConfig = toolConfig
	if len(request.Payload.AdditionalModelRequestFields) > 0 {
		input.AdditionalModelRequestFields = document.NewLazyDocument(
			request.Payload.AdditionalModelRequestFields,
		)
	}

	inference, err := convertAWSBedrockInferenceConfig(
		request.MaxTokens,
		request.Temperature,
	)
	if err != nil {
		return nil, err
	}
	input.InferenceConfig = inference
	return input, nil
}

func convertAWSBedrockInferenceConfig(
	maxTokens int,
	temperature *float64,
) (*types.InferenceConfiguration, error) {
	if maxTokens < 0 || int64(maxTokens) > math.MaxInt32 {
		return nil, fmt.Errorf("Bedrock max tokens %d is outside the int32 range", maxTokens)
	}
	if temperature != nil &&
		(math.IsNaN(*temperature) || math.IsInf(*temperature, 0)) {
		return nil, errors.New("Bedrock temperature must be finite")
	}
	if maxTokens == 0 && temperature == nil {
		return nil, nil
	}
	config := &types.InferenceConfiguration{}
	if maxTokens > 0 {
		value := int32(maxTokens)
		config.MaxTokens = &value
	}
	if temperature != nil {
		value := float32(*temperature)
		if math.IsInf(float64(value), 0) {
			return nil, errors.New("Bedrock temperature exceeds the float32 range")
		}
		config.Temperature = &value
	}
	return config, nil
}

func convertAWSBedrockSystem(
	blocks []BedrockContentBlock,
) ([]types.SystemContentBlock, error) {
	result := make([]types.SystemContentBlock, 0, len(blocks))
	for index, block := range blocks {
		memberCount := 0
		if block.Text != "" {
			memberCount++
		}
		if block.CachePoint != nil {
			memberCount++
		}
		if memberCount != 1 {
			return nil, fmt.Errorf(
				"Bedrock system content block %d must contain exactly one supported member, got %d",
				index,
				memberCount,
			)
		}
		switch {
		case block.CachePoint != nil:
			result = append(result, &types.SystemContentBlockMemberCachePoint{
				Value: convertAWSBedrockCachePoint(*block.CachePoint),
			})
		case block.Text != "":
			result = append(result, &types.SystemContentBlockMemberText{Value: block.Text})
		default:
			return nil, fmt.Errorf("unsupported Bedrock system content block at index %d", index)
		}
	}
	return result, nil
}

func convertAWSBedrockMessages(
	messages []BedrockMessage,
) ([]types.Message, error) {
	result := make([]types.Message, 0, len(messages))
	for index, message := range messages {
		var role types.ConversationRole
		switch message.Role {
		case "user":
			role = types.ConversationRoleUser
		case "assistant":
			role = types.ConversationRoleAssistant
		default:
			return nil, fmt.Errorf(
				"unsupported Bedrock message role %q at index %d",
				message.Role,
				index,
			)
		}
		content, err := convertAWSBedrockContent(message.Content)
		if err != nil {
			return nil, fmt.Errorf("convert Bedrock message %d: %w", index, err)
		}
		if len(content) == 0 {
			return nil, fmt.Errorf("Bedrock message %d has no content", index)
		}
		result = append(result, types.Message{Role: role, Content: content})
	}
	return result, nil
}

func convertAWSBedrockContent(
	blocks []BedrockContentBlock,
) ([]types.ContentBlock, error) {
	result := make([]types.ContentBlock, 0, len(blocks))
	for index, block := range blocks {
		member, err := convertAWSBedrockContentBlock(block)
		if err != nil {
			return nil, fmt.Errorf("content block %d: %w", index, err)
		}
		result = append(result, member)
	}
	return result, nil
}

func convertAWSBedrockContentBlock(
	block BedrockContentBlock,
) (types.ContentBlock, error) {
	memberCount := 0
	for _, present := range []bool{
		block.Text != "",
		block.Image != nil,
		block.ToolUse != nil,
		block.ToolResult != nil,
		block.ReasoningContent != nil,
		block.CachePoint != nil,
	} {
		if present {
			memberCount++
		}
	}
	if memberCount != 1 {
		return nil, fmt.Errorf(
			"Bedrock content block must contain exactly one member, got %d",
			memberCount,
		)
	}
	switch {
	case block.Image != nil:
		image, err := convertAWSBedrockImage(*block.Image)
		if err != nil {
			return nil, err
		}
		return &types.ContentBlockMemberImage{Value: image}, nil
	case block.ToolUse != nil:
		input := block.ToolUse.Input
		if input == nil {
			input = map[string]any{}
		}
		return &types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
			ToolUseId: aws.String(block.ToolUse.ToolUseID),
			Name:      aws.String(block.ToolUse.Name),
			Input:     document.NewLazyDocument(input),
		}}, nil
	case block.ToolResult != nil:
		toolResult, err := convertAWSBedrockToolResult(*block.ToolResult)
		if err != nil {
			return nil, err
		}
		return &types.ContentBlockMemberToolResult{Value: toolResult}, nil
	case block.ReasoningContent != nil:
		reasoning := types.ReasoningTextBlock{
			Text: aws.String(block.ReasoningContent.Text),
		}
		if block.ReasoningContent.Signature != "" {
			reasoning.Signature = aws.String(block.ReasoningContent.Signature)
		}
		return &types.ContentBlockMemberReasoningContent{
			Value: &types.ReasoningContentBlockMemberReasoningText{
				Value: reasoning,
			},
		}, nil
	case block.CachePoint != nil:
		return &types.ContentBlockMemberCachePoint{
			Value: convertAWSBedrockCachePoint(*block.CachePoint),
		}, nil
	case block.Text != "":
		return &types.ContentBlockMemberText{Value: block.Text}, nil
	default:
		return nil, errors.New("empty or unsupported content block")
	}
}

func convertAWSBedrockImage(block BedrockImageBlock) (types.ImageBlock, error) {
	data, err := decodeBedrockBase64(block.Data)
	if err != nil {
		return types.ImageBlock{}, fmt.Errorf("decode Bedrock %s image: %w", block.Format, err)
	}
	format := types.ImageFormat(block.Format)
	switch format {
	case types.ImageFormatJpeg, types.ImageFormatPng,
		types.ImageFormatGif, types.ImageFormatWebp:
	default:
		return types.ImageBlock{}, fmt.Errorf("unsupported Bedrock image format %q", block.Format)
	}
	return types.ImageBlock{
		Format: format,
		Source: &types.ImageSourceMemberBytes{Value: data},
	}, nil
}

func decodeBedrockBase64(value string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func convertAWSBedrockToolResult(
	block BedrockToolResultBlock,
) (types.ToolResultBlock, error) {
	content := make([]types.ToolResultContentBlock, 0, len(block.Content))
	for index, item := range block.Content {
		memberCount := 0
		if item.Text != "" {
			memberCount++
		}
		if item.Image != nil {
			memberCount++
		}
		if memberCount != 1 {
			return types.ToolResultBlock{}, fmt.Errorf(
				"Bedrock tool result content %d must contain exactly one supported member, got %d",
				index,
				memberCount,
			)
		}
		switch {
		case item.Image != nil:
			image, err := convertAWSBedrockImage(*item.Image)
			if err != nil {
				return types.ToolResultBlock{}, fmt.Errorf(
					"convert tool result image %d: %w",
					index,
					err,
				)
			}
			content = append(content, &types.ToolResultContentBlockMemberImage{
				Value: image,
			})
		case item.Text != "":
			content = append(content, &types.ToolResultContentBlockMemberText{
				Value: item.Text,
			})
		default:
			return types.ToolResultBlock{}, fmt.Errorf(
				"unsupported Bedrock tool result content at index %d",
				index,
			)
		}
	}
	if len(content) == 0 {
		return types.ToolResultBlock{}, errors.New("Bedrock tool result has no content")
	}
	var status types.ToolResultStatus
	switch block.Status {
	case "", "success":
		status = types.ToolResultStatusSuccess
	case "error":
		status = types.ToolResultStatusError
	default:
		return types.ToolResultBlock{}, fmt.Errorf(
			"unsupported Bedrock tool result status %q",
			block.Status,
		)
	}
	return types.ToolResultBlock{
		ToolUseId: aws.String(block.ToolUseID),
		Content:   content,
		Status:    status,
	}, nil
}

func convertAWSBedrockCachePoint(block BedrockCachePoint) types.CachePointBlock {
	result := types.CachePointBlock{Type: types.CachePointTypeDefault}
	if block.TTL == "1h" {
		result.Ttl = types.CacheTTLOneHour
	}
	return result
}

func convertAWSBedrockToolConfig(
	config *BedrockToolConfig,
) (*types.ToolConfiguration, error) {
	if config == nil {
		return nil, nil
	}
	tools := make([]types.Tool, 0, len(config.Tools))
	for index, tool := range config.Tools {
		spec := tool.ToolSpec
		if spec.Name == "" {
			return nil, fmt.Errorf("Bedrock tool %d has an empty name", index)
		}
		tools = append(tools, &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(spec.Name),
				Description: aws.String(spec.Description),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(spec.InputSchema.JSON),
				},
				Strict: spec.Strict,
			},
		})
	}
	if len(tools) == 0 {
		return nil, errors.New("Bedrock tool configuration has no tools")
	}
	choice, err := convertAWSBedrockToolChoice(config.ToolChoice)
	if err != nil {
		return nil, err
	}
	return &types.ToolConfiguration{Tools: tools, ToolChoice: choice}, nil
}

func convertAWSBedrockToolChoice(choice any) (types.ToolChoice, error) {
	if choice == nil {
		return nil, nil
	}
	value, ok := choice.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unsupported Bedrock tool choice %T", choice)
	}
	if _, ok := value["auto"]; ok {
		return &types.ToolChoiceMemberAuto{Value: types.AutoToolChoice{}}, nil
	}
	if _, ok := value["any"]; ok {
		return &types.ToolChoiceMemberAny{Value: types.AnyToolChoice{}}, nil
	}
	if rawTool, ok := value["tool"].(map[string]any); ok {
		name, _ := rawTool["name"].(string)
		if name == "" {
			return nil, errors.New("Bedrock named tool choice requires a name")
		}
		return &types.ToolChoiceMemberTool{Value: types.SpecificToolChoice{
			Name: aws.String(name),
		}}, nil
	}
	return nil, errors.New("unsupported Bedrock tool choice")
}
