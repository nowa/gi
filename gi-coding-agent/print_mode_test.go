package gicodingagent

import (
	"bytes"
	"reflect"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestRunPrintModePiShutdownLifecycle(t *testing.T) {
	images := []llm.ContentPart{llm.Image("abc", "image/png")}
	host := newFakePrintModeHost(llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text("done")},
		StopReason: llm.StopReasonStop,
	})
	exitCode := RunPrintMode(host, PrintModeOptions{
		Mode:           "text",
		InitialMessage: "Say done",
		InitialImages:  images,
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if len(host.session.prompts) != 1 || host.session.prompts[0].message != "Say done" || !reflect.DeepEqual(host.session.prompts[0].options.Images, images) {
		t.Fatalf("prompts = %#v", host.session.prompts)
	}
	if host.shutdownEvents != 1 || host.shutdownReason != "quit" {
		t.Fatalf("shutdown events = %d reason = %q", host.shutdownEvents, host.shutdownReason)
	}
}

func TestRunPrintModePiJSONShutdownLifecycle(t *testing.T) {
	host := newFakePrintModeHost(llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text("done")},
		StopReason: llm.StopReasonStop,
	})
	exitCode := RunPrintMode(host, PrintModeOptions{
		Mode:     "json",
		Messages: []string{"hello"},
	})
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if len(host.session.prompts) != 1 || host.session.prompts[0].message != "hello" {
		t.Fatalf("prompts = %#v", host.session.prompts)
	}
	if host.shutdownEvents != 1 || host.shutdownReason != "quit" {
		t.Fatalf("shutdown events = %d reason = %q", host.shutdownEvents, host.shutdownReason)
	}
}

func TestRunPrintModePiAssistantError(t *testing.T) {
	host := newFakePrintModeHost(llm.Message{
		Role:         llm.RoleAssistant,
		StopReason:   llm.StopReasonError,
		ErrorMessage: "provider failure",
	})
	var stderr bytes.Buffer
	exitCode := RunPrintMode(host, PrintModeOptions{
		Mode:   "text",
		Stderr: &stderr,
	})
	if exitCode != 1 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if stderr.String() != "provider failure\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if host.shutdownEvents != 1 || host.shutdownReason != "quit" {
		t.Fatalf("shutdown events = %d reason = %q", host.shutdownEvents, host.shutdownReason)
	}
}

type fakePrintModeHost struct {
	session        *fakePrintModeSession
	shutdownEvents int
	shutdownReason string
}

func newFakePrintModeHost(message llm.Message) *fakePrintModeHost {
	return &fakePrintModeHost{session: &fakePrintModeSession{messages: []llm.Message{message}}}
}

func (h *fakePrintModeHost) PrintModeSession() PrintModeSession {
	return h.session
}

func (h *fakePrintModeHost) Dispose() error {
	h.shutdownEvents++
	h.shutdownReason = "quit"
	return nil
}

type fakePrintModeSession struct {
	prompts  []fakePrintModePrompt
	messages []llm.Message
}

type fakePrintModePrompt struct {
	message string
	options PrintModePromptOptions
}

func (s *fakePrintModeSession) Prompt(message string, options PrintModePromptOptions) error {
	s.prompts = append(s.prompts, fakePrintModePrompt{message: message, options: options})
	return nil
}

func (s *fakePrintModeSession) WaitForIdle() error {
	return nil
}

func (s *fakePrintModeSession) Messages() []llm.Message {
	return s.messages
}
