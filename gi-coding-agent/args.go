package gicodingagent

import "strings"

type Mode string

const (
	ModeText Mode = "text"
	ModeJSON Mode = "json"
	ModeRPC  Mode = "rpc"
)

type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
)

type Diagnostic struct {
	Type    string
	Message string
}

type Args struct {
	Provider           string
	Model              string
	APIKey             string
	SystemPrompt       string
	AppendSystemPrompt []string
	Thinking           ThinkingLevel
	Continue           bool
	Resume             bool
	Help               bool
	Version            bool
	Mode               Mode
	NoSession          bool
	Session            string
	Fork               string
	SessionDir         string
	Models             []string
	Tools              []string
	NoTools            bool
	NoBuiltinTools     bool
	Extensions         []string
	NoExtensions       bool
	Print              bool
	Export             string
	NoSkills           bool
	Skills             []string
	PromptTemplates    []string
	NoPromptTemplates  bool
	Themes             []string
	NoThemes           bool
	NoContextFiles     bool
	ListModels         any
	Offline            bool
	Verbose            bool
	Messages           []string
	FileArgs           []string
	UnknownFlags       map[string]any
	Diagnostics        []Diagnostic
}

var validThinkingLevels = map[string]ThinkingLevel{
	"off":     ThinkingOff,
	"minimal": ThinkingMinimal,
	"low":     ThinkingLow,
	"medium":  ThinkingMedium,
	"high":    ThinkingHigh,
	"xhigh":   ThinkingXHigh,
}

func IsValidThinkingLevel(level string) bool {
	_, ok := validThinkingLevels[level]
	return ok
}

func ParseArgs(argv []string) Args {
	result := Args{
		Messages:     []string{},
		FileArgs:     []string{},
		UnknownFlags: map[string]any{},
		Diagnostics:  []Diagnostic{},
	}

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "--help" || arg == "-h":
			result.Help = true
		case arg == "--version" || arg == "-v":
			result.Version = true
		case arg == "--mode" && i+1 < len(argv):
			i++
			switch Mode(argv[i]) {
			case ModeText, ModeJSON, ModeRPC:
				result.Mode = Mode(argv[i])
			}
		case arg == "--continue" || arg == "-c":
			result.Continue = true
		case arg == "--resume" || arg == "-r":
			result.Resume = true
		case arg == "--provider" && i+1 < len(argv):
			i++
			result.Provider = argv[i]
		case arg == "--model" && i+1 < len(argv):
			i++
			result.Model = argv[i]
		case arg == "--api-key" && i+1 < len(argv):
			i++
			result.APIKey = argv[i]
		case arg == "--system-prompt" && i+1 < len(argv):
			i++
			result.SystemPrompt = argv[i]
		case arg == "--append-system-prompt" && i+1 < len(argv):
			i++
			result.AppendSystemPrompt = append(result.AppendSystemPrompt, argv[i])
		case arg == "--no-session":
			result.NoSession = true
		case arg == "--session" && i+1 < len(argv):
			i++
			result.Session = argv[i]
		case arg == "--fork" && i+1 < len(argv):
			i++
			result.Fork = argv[i]
		case arg == "--session-dir" && i+1 < len(argv):
			i++
			result.SessionDir = argv[i]
		case arg == "--models" && i+1 < len(argv):
			i++
			result.Models = splitCommaList(argv[i], false)
		case arg == "--no-tools" || arg == "-nt":
			result.NoTools = true
		case arg == "--no-builtin-tools" || arg == "-nbt":
			result.NoBuiltinTools = true
		case (arg == "--tools" || arg == "-t") && i+1 < len(argv):
			i++
			result.Tools = splitCommaList(argv[i], true)
		case arg == "--thinking" && i+1 < len(argv):
			i++
			if level, ok := validThinkingLevels[argv[i]]; ok {
				result.Thinking = level
			} else {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{
					Type:    "warning",
					Message: `Invalid thinking level "` + argv[i] + `". Valid values: off, minimal, low, medium, high, xhigh`,
				})
			}
		case arg == "--print" || arg == "-p":
			result.Print = true
			next := nextArg(argv, i)
			if next != "" && !strings.HasPrefix(next, "@") && (!strings.HasPrefix(next, "-") || strings.HasPrefix(next, "---")) {
				result.Messages = append(result.Messages, next)
				i++
			}
		case arg == "--export" && i+1 < len(argv):
			i++
			result.Export = argv[i]
		case (arg == "--extension" || arg == "-e") && i+1 < len(argv):
			i++
			result.Extensions = append(result.Extensions, argv[i])
		case arg == "--no-extensions" || arg == "-ne":
			result.NoExtensions = true
		case arg == "--skill" && i+1 < len(argv):
			i++
			result.Skills = append(result.Skills, argv[i])
		case arg == "--prompt-template" && i+1 < len(argv):
			i++
			result.PromptTemplates = append(result.PromptTemplates, argv[i])
		case arg == "--theme" && i+1 < len(argv):
			i++
			result.Themes = append(result.Themes, argv[i])
		case arg == "--no-skills" || arg == "-ns":
			result.NoSkills = true
		case arg == "--no-prompt-templates" || arg == "-np":
			result.NoPromptTemplates = true
		case arg == "--no-themes":
			result.NoThemes = true
		case arg == "--no-context-files" || arg == "-nc":
			result.NoContextFiles = true
		case arg == "--list-models":
			next := nextArg(argv, i)
			if next != "" && !strings.HasPrefix(next, "-") && !strings.HasPrefix(next, "@") {
				result.ListModels = next
				i++
			} else {
				result.ListModels = true
			}
		case arg == "--verbose":
			result.Verbose = true
		case arg == "--offline":
			result.Offline = true
		case strings.HasPrefix(arg, "@"):
			result.FileArgs = append(result.FileArgs, arg[1:])
		case strings.HasPrefix(arg, "--"):
			flag := strings.TrimPrefix(arg, "--")
			if before, after, ok := strings.Cut(flag, "="); ok {
				result.UnknownFlags[before] = after
				continue
			}
			next := nextArg(argv, i)
			if next != "" && !strings.HasPrefix(next, "-") && !strings.HasPrefix(next, "@") {
				result.UnknownFlags[flag] = next
				i++
			} else {
				result.UnknownFlags[flag] = true
			}
		case strings.HasPrefix(arg, "-"):
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Type: "error", Message: "Unknown option: " + arg})
		default:
			result.Messages = append(result.Messages, arg)
		}
	}

	return result
}

func nextArg(args []string, index int) string {
	if index+1 >= len(args) {
		return ""
	}
	return args[index+1]
}

func splitCommaList(value string, dropEmpty bool) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if dropEmpty && part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
