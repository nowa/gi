package gicodingagent

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseArgsPiFlagMatrix(t *testing.T) {
	t.Run("basic mode flags", func(t *testing.T) {
		tests := []struct {
			name  string
			args  []string
			check func(t *testing.T, got Args)
		}{
			{"version", []string{"--version", "--help", "some message"}, func(t *testing.T, got Args) {
				if !got.Version || !got.Help || !reflect.DeepEqual(got.Messages, []string{"some message"}) {
					t.Fatalf("version/help/messages = %#v", got)
				}
			}},
			{"version shorthand", []string{"-v"}, func(t *testing.T, got Args) {
				if !got.Version {
					t.Fatalf("-v did not set Version")
				}
			}},
			{"help shorthand", []string{"-h"}, func(t *testing.T, got Args) {
				if !got.Help {
					t.Fatalf("-h did not set Help")
				}
			}},
			{"print", []string{"--print"}, func(t *testing.T, got Args) {
				if !got.Print {
					t.Fatalf("--print did not set Print")
				}
			}},
			{"print shorthand", []string{"-p"}, func(t *testing.T, got Args) {
				if !got.Print {
					t.Fatalf("-p did not set Print")
				}
			}},
			{"continue shorthand", []string{"-c"}, func(t *testing.T, got Args) {
				if !got.Continue {
					t.Fatalf("-c did not set Continue")
				}
			}},
			{"resume shorthand", []string{"-r"}, func(t *testing.T, got Args) {
				if !got.Resume {
					t.Fatalf("-r did not set Resume")
				}
			}},
			{"no session", []string{"--no-session"}, func(t *testing.T, got Args) {
				if !got.NoSession {
					t.Fatalf("--no-session did not set NoSession")
				}
			}},
			{"verbose", []string{"--verbose"}, func(t *testing.T, got Args) {
				if !got.Verbose {
					t.Fatalf("--verbose did not set Verbose")
				}
			}},
			{"offline", []string{"--offline"}, func(t *testing.T, got Args) {
				if !got.Offline {
					t.Fatalf("--offline did not set Offline")
				}
			}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				tc.check(t, ParseArgs(tc.args))
			})
		}
	})

	t.Run("value flags", func(t *testing.T) {
		got := ParseArgs([]string{
			"--provider", "anthropic",
			"--model", "claude-sonnet",
			"--api-key", "sk-test-key",
			"--system-prompt", "You are helpful",
			"--append-system-prompt", "Context A",
			"--append-system-prompt", "Context B",
			"--mode", "rpc",
			"--session", "/path/to/session.jsonl",
			"--fork", "1234abcd",
			"--session-dir", "/tmp/sessions",
			"--export", "session.html",
			"--thinking", "high",
			"--models", "gpt-4o,claude-sonnet,gemini-pro",
		})
		if got.Provider != "anthropic" || got.Model != "claude-sonnet" || got.APIKey != "sk-test-key" {
			t.Fatalf("provider/model/api key = %#v", got)
		}
		if got.SystemPrompt != "You are helpful" || !reflect.DeepEqual(got.AppendSystemPrompt, []string{"Context A", "Context B"}) {
			t.Fatalf("system prompt flags = %#v", got)
		}
		if got.Mode != ModeRPC || got.Session != "/path/to/session.jsonl" || got.Fork != "1234abcd" || got.SessionDir != "/tmp/sessions" {
			t.Fatalf("mode/session flags = %#v", got)
		}
		if got.Export != "session.html" || got.Thinking != ThinkingHigh {
			t.Fatalf("export/thinking flags = %#v", got)
		}
		if !reflect.DeepEqual(got.Models, []string{"gpt-4o", "claude-sonnet", "gemini-pro"}) {
			t.Fatalf("models = %#v", got.Models)
		}
	})

	t.Run("print prompt consumption follows Pi", func(t *testing.T) {
		prompt := "---\ntitle: hello\n---\nSay hi."
		got := ParseArgs([]string{"-p", prompt})
		if !got.Print || !reflect.DeepEqual(got.Messages, []string{prompt}) || len(got.UnknownFlags) != 0 {
			t.Fatalf("frontmatter prompt after -p = %#v", got)
		}
		got = ParseArgs([]string{"-p", "--provider", "openai", "Say hi."})
		if !got.Print || got.Provider != "openai" || !reflect.DeepEqual(got.Messages, []string{"Say hi."}) {
			t.Fatalf("option after -p should not be consumed as prompt: %#v", got)
		}
	})
}

func TestParseArgsPiResourceAndToolFlags(t *testing.T) {
	got := ParseArgs([]string{"--no-extensions", "-e", "foo.ts", "--extension", "bar.ts"})
	if !got.NoExtensions || !reflect.DeepEqual(got.Extensions, []string{"foo.ts", "bar.ts"}) {
		t.Fatalf("extension flags = %#v", got)
	}

	got = ParseArgs([]string{"--skill", "./skill-a", "--skill", "./skill-b"})
	if !reflect.DeepEqual(got.Skills, []string{"./skill-a", "./skill-b"}) {
		t.Fatalf("skills = %#v", got.Skills)
	}

	got = ParseArgs([]string{"--prompt-template", "./one", "--prompt-template", "./two"})
	if !reflect.DeepEqual(got.PromptTemplates, []string{"./one", "./two"}) {
		t.Fatalf("prompt templates = %#v", got.PromptTemplates)
	}

	got = ParseArgs([]string{"--theme", "./dark.json", "--theme", "./light.json"})
	if !reflect.DeepEqual(got.Themes, []string{"./dark.json", "./light.json"}) {
		t.Fatalf("themes = %#v", got.Themes)
	}

	got = ParseArgs([]string{"--no-skills", "-np", "--no-themes", "-nc"})
	if !got.NoSkills || !got.NoPromptTemplates || !got.NoThemes || !got.NoContextFiles {
		t.Fatalf("resource disable flags = %#v", got)
	}

	got = ParseArgs([]string{"--no-tools", "--no-builtin-tools", "-t", "read,bash,,edit"})
	if !got.NoTools || !got.NoBuiltinTools || !reflect.DeepEqual(got.Tools, []string{"read", "bash", "edit"}) {
		t.Fatalf("tool flags = %#v", got)
	}
}

func TestParseArgsPiMessagesFilesUnknownsAndDiagnostics(t *testing.T) {
	got := ParseArgs([]string{"@file.txt", "explain this", "@image.png"})
	if !reflect.DeepEqual(got.FileArgs, []string{"file.txt", "image.png"}) || !reflect.DeepEqual(got.Messages, []string{"explain this"}) {
		t.Fatalf("messages/file args = %#v", got)
	}

	got = ParseArgs([]string{"--unknown-flag", "message", "--boolean-flag", "--equals=value"})
	if got.UnknownFlags["unknown-flag"] != "message" || got.UnknownFlags["boolean-flag"] != true || got.UnknownFlags["equals"] != "value" {
		t.Fatalf("unknown flags = %#v", got.UnknownFlags)
	}
	if len(got.Messages) != 0 {
		t.Fatalf("unknown flag value should not become message: %#v", got.Messages)
	}

	got = ParseArgs([]string{"-unknown", "--thinking", "random"})
	if len(got.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want two entries", got.Diagnostics)
	}
	if got.Diagnostics[0].Type != "error" || !strings.Contains(got.Diagnostics[0].Message, "Unknown option: -unknown") {
		t.Fatalf("unknown short diagnostic = %#v", got.Diagnostics)
	}
	if got.Diagnostics[1].Type != "warning" || !strings.Contains(got.Diagnostics[1].Message, `Invalid thinking level "random"`) {
		t.Fatalf("thinking diagnostic = %#v", got.Diagnostics)
	}
}

func TestParseArgsPiListModelsAndComplexCombination(t *testing.T) {
	got := ParseArgs([]string{"--list-models"})
	if got.ListModels != true {
		t.Fatalf("list models = %#v, want true", got.ListModels)
	}
	got = ParseArgs([]string{"--list-models", "sonnet"})
	if got.ListModels != "sonnet" {
		t.Fatalf("list models search = %#v, want sonnet", got.ListModels)
	}
	got = ParseArgs([]string{"--list-models", "@models.txt", "prompt"})
	if got.ListModels != true || !reflect.DeepEqual(got.FileArgs, []string{"models.txt"}) || !reflect.DeepEqual(got.Messages, []string{"prompt"}) {
		t.Fatalf("list models with file arg = %#v", got)
	}

	got = ParseArgs([]string{
		"--provider", "anthropic",
		"--model", "claude-sonnet",
		"--print",
		"--thinking", "high",
		"@prompt.md",
		"Do the task",
	})
	if got.Provider != "anthropic" || got.Model != "claude-sonnet" || !got.Print || got.Thinking != ThinkingHigh {
		t.Fatalf("complex scalar flags = %#v", got)
	}
	if !reflect.DeepEqual(got.FileArgs, []string{"prompt.md"}) || !reflect.DeepEqual(got.Messages, []string{"Do the task"}) {
		t.Fatalf("complex messages/file args = %#v", got)
	}
}
