package gicodingagent

type InitialMessageInput struct {
	Parsed       *Args
	FileText     string
	FileImages   []any
	StdinContent *string
}

type InitialMessageResult struct {
	InitialMessage string
	InitialImages  []any
}

func BuildInitialMessage(input InitialMessageInput) InitialMessageResult {
	parts := []string{}
	if input.StdinContent != nil {
		parts = append(parts, *input.StdinContent)
	}
	if input.FileText != "" {
		parts = append(parts, input.FileText)
	}
	if input.Parsed != nil && len(input.Parsed.Messages) > 0 {
		parts = append(parts, input.Parsed.Messages[0])
		input.Parsed.Messages = input.Parsed.Messages[1:]
	}
	result := InitialMessageResult{}
	for _, part := range parts {
		result.InitialMessage += part
	}
	if len(input.FileImages) > 0 {
		result.InitialImages = append([]any(nil), input.FileImages...)
	}
	return result
}
