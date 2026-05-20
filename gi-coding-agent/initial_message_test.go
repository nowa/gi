package gicodingagent

import (
	"reflect"
	"testing"
)

func TestBuildInitialMessageMergesStdinWithFirstCLIMessage(t *testing.T) {
	parsed := initialMessageArgs("Summarize the text given")
	stdin := "README contents\n"

	result := BuildInitialMessage(InitialMessageInput{
		Parsed:       &parsed,
		StdinContent: &stdin,
	})

	if result.InitialMessage != "README contents\nSummarize the text given" {
		t.Fatalf("initial message = %q", result.InitialMessage)
	}
	if len(parsed.Messages) != 0 {
		t.Fatalf("messages = %#v", parsed.Messages)
	}
}

func TestBuildInitialMessageUsesStdinWhenNoCLIMessage(t *testing.T) {
	parsed := initialMessageArgs()
	stdin := "README contents"

	result := BuildInitialMessage(InitialMessageInput{
		Parsed:       &parsed,
		StdinContent: &stdin,
	})

	if result.InitialMessage != "README contents" {
		t.Fatalf("initial message = %q", result.InitialMessage)
	}
	if len(parsed.Messages) != 0 {
		t.Fatalf("messages = %#v", parsed.Messages)
	}
}

func TestBuildInitialMessageCombinesStdinFileTextAndFirstCLIMessage(t *testing.T) {
	parsed := initialMessageArgs("Explain it", "Second message")
	stdin := "stdin\n"

	result := BuildInitialMessage(InitialMessageInput{
		Parsed:       &parsed,
		StdinContent: &stdin,
		FileText:     "file\n",
	})

	if result.InitialMessage != "stdin\nfile\nExplain it" {
		t.Fatalf("initial message = %q", result.InitialMessage)
	}
	if !reflect.DeepEqual(parsed.Messages, []string{"Second message"}) {
		t.Fatalf("messages = %#v", parsed.Messages)
	}
}

func initialMessageArgs(messages ...string) Args {
	return Args{
		Messages:     append([]string(nil), messages...),
		FileArgs:     []string{},
		UnknownFlags: map[string]any{},
		Diagnostics:  []Diagnostic{},
	}
}
