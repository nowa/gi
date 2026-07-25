package gicodingagent

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/nowa/gi/gi-coding-agent/internal/outputguard"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type PrintModeRuntimeHost interface {
	PrintModeSession() PrintModeSession
	Dispose() error
}

type PrintModeSession interface {
	Prompt(message string, options PrintModePromptOptions) error
	WaitForIdle() error
	Messages() []llm.Message
}

type PrintModePromptOptions struct {
	Images []llm.ContentPart
}

type PrintModeOptions struct {
	Mode           string
	InitialMessage string
	InitialImages  []llm.ContentPart
	Messages       []string
	Stdout         io.Writer
	Stderr         io.Writer
}

func RunPrintMode(host PrintModeRuntimeHost, options PrintModeOptions) (exitCode int) {
	defer func() {
		if err := host.Dispose(); err != nil && exitCode == 0 {
			exitCode = 1
		}
	}()

	session := host.PrintModeSession()
	if options.InitialMessage != "" {
		if err := session.Prompt(options.InitialMessage, PrintModePromptOptions{Images: options.InitialImages}); err != nil {
			writePrintModeError(options.Stderr, err.Error())
			return 1
		}
	} else {
		for _, message := range options.Messages {
			if err := session.Prompt(message, PrintModePromptOptions{}); err != nil {
				writePrintModeError(options.Stderr, err.Error())
				return 1
			}
		}
	}
	if err := session.WaitForIdle(); err != nil {
		writePrintModeError(options.Stderr, err.Error())
		return 1
	}

	last := lastPrintModeAssistantMessage(session.Messages())
	if last.StopReason == llm.StopReasonError {
		writePrintModeError(options.Stderr, last.ErrorMessage)
		return 1
	}
	if options.Stdout != nil {
		if err := writePrintModeOutput(
			options.Stdout,
			options.Mode,
			last,
		); err != nil {
			writePrintModeError(options.Stderr, err.Error())
			return 1
		}
	}
	return exitCode
}

func lastPrintModeAssistantMessage(messages []llm.Message) llm.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleAssistant {
			return messages[i]
		}
	}
	return llm.Message{}
}

func writePrintModeError(writer io.Writer, message string) {
	if writer == nil || message == "" {
		return
	}
	_, _ = fmt.Fprintln(writer, message)
}

func writePrintModeOutput(
	writer io.Writer,
	mode string,
	message llm.Message,
) error {
	output := outputguard.New(writer, outputguard.Options{})
	if mode == "json" {
		encoded, err := json.Marshal(message)
		if err != nil {
			return err
		}
		encoded = append(encoded, '\n')
		if _, err := output.Write(encoded); err != nil {
			return err
		}
		return output.Flush()
	}
	for _, part := range message.Content {
		if part.Type == llm.ContentText {
			if _, err := output.WriteString(part.Text); err != nil {
				return err
			}
		}
	}
	return output.Flush()
}
