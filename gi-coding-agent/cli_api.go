package gicodingagent

import internalcli "github.com/nowa/gi/gi-coding-agent/internal/cli"

type Mode = internalcli.Mode

const (
	ModeText Mode = internalcli.ModeText
	ModeJSON Mode = internalcli.ModeJSON
	ModeRPC  Mode = internalcli.ModeRPC
)

type ThinkingLevel = internalcli.ThinkingLevel

const (
	ThinkingOff     ThinkingLevel = internalcli.ThinkingOff
	ThinkingMinimal ThinkingLevel = internalcli.ThinkingMinimal
	ThinkingLow     ThinkingLevel = internalcli.ThinkingLow
	ThinkingMedium  ThinkingLevel = internalcli.ThinkingMedium
	ThinkingHigh    ThinkingLevel = internalcli.ThinkingHigh
	ThinkingXHigh   ThinkingLevel = internalcli.ThinkingXHigh
)

type Diagnostic = internalcli.Diagnostic
type Args = internalcli.Args

func IsValidThinkingLevel(level string) bool {
	return internalcli.IsValidThinkingLevel(level)
}

func ParseArgs(argv []string) Args {
	return internalcli.ParseArgs(argv)
}

type InitialMessageInput = internalcli.InitialMessageInput
type InitialMessageResult = internalcli.InitialMessageResult

func BuildInitialMessage(input InitialMessageInput) InitialMessageResult {
	return internalcli.BuildInitialMessage(input)
}
