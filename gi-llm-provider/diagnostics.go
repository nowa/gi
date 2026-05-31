package gillmprovider

import (
	"fmt"
	"strings"
)

func FormatThrownValue(value any) string {
	if value == nil {
		return "<nil>"
	}
	switch typed := value.(type) {
	case error:
		message := strings.TrimSpace(typed.Error())
		if message != "" {
			return message
		}
		return "error"
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}

func ExtractDiagnosticError(value any) DiagnosticErrorInfo {
	message := strings.TrimSpace(FormatThrownValue(value))
	if message == "" {
		message = "error"
	}
	info := DiagnosticErrorInfo{Message: message}
	if value == nil {
		info.Name = "ThrownValue"
		return info
	}
	if _, ok := value.(error); !ok {
		info.Name = "ThrownValue"
	}
	return info
}

func NewAssistantMessageDiagnostic(diagnosticType string, err error, details map[string]any) AssistantMessageDiagnostic {
	var errorInfo *DiagnosticErrorInfo
	if err != nil {
		extracted := ExtractDiagnosticError(err)
		errorInfo = &extracted
	}
	return AssistantMessageDiagnostic{
		Type:      diagnosticType,
		Timestamp: NowMillis(),
		Error:     errorInfo,
		Details:   details,
	}
}

func AppendAssistantMessageDiagnostic(message *Message, diagnostic AssistantMessageDiagnostic) {
	if message == nil {
		return
	}
	message.Diagnostics = append(message.Diagnostics, diagnostic)
}
