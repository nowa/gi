package gicodingagent

import (
	"reflect"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
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

func TestBuildInitialMessageCarriesFileImagesWithCopy(t *testing.T) {
	parsed := initialMessageArgs("Describe it")
	images := []llm.ContentPart{llm.Image("abc", "image/png")}

	result := BuildInitialMessage(InitialMessageInput{
		Parsed:     &parsed,
		FileImages: images,
	})

	if result.InitialMessage != "Describe it" {
		t.Fatalf("initial message = %q", result.InitialMessage)
	}
	if !reflect.DeepEqual(result.InitialImages, images) {
		t.Fatalf("images = %#v, want %#v", result.InitialImages, images)
	}
	images[0].Data = "mutated"
	if result.InitialImages[0].Data != "abc" {
		t.Fatalf("images alias input slice: %#v", result.InitialImages)
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
