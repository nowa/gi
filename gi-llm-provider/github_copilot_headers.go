package gillmprovider

func InferCopilotInitiator(messages []Message) string {
	if len(messages) == 0 {
		return "user"
	}
	if messages[len(messages)-1].Role == RoleUser {
		return "user"
	}
	return "agent"
}

func HasCopilotVisionInput(messages []Message) bool {
	for _, message := range messages {
		if message.Role != RoleUser && message.Role != RoleToolResult {
			continue
		}
		for _, part := range message.Content {
			if part.Type == ContentImage {
				return true
			}
		}
	}
	return false
}

func BuildCopilotDynamicHeaders(messages []Message) map[string]string {
	headers := map[string]string{
		"X-Initiator":   InferCopilotInitiator(messages),
		"Openai-Intent": "conversation-edits",
	}
	if HasCopilotVisionInput(messages) {
		headers["Copilot-Vision-Request"] = "true"
	}
	return headers
}

func BuildAnthropicRequestHeaders(model Model, context Context, options AnthropicPayloadOptions) map[string]string {
	headers := map[string]string{}
	for key, value := range model.Headers {
		headers[key] = value
	}
	if model.Provider == "github-copilot" {
		for key, value := range BuildCopilotDynamicHeaders(context.Messages) {
			headers[key] = value
		}
	}
	for key, value := range BuildAnthropicHeaders(model, context, options) {
		headers[key] = value
	}
	return headers
}
