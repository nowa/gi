package gicodingagent

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionPromptExecutesExtensionCommandWithArgs(t *testing.T) {
	session := newPromptExpansionSession(t, t.TempDir(), t.TempDir(), nil)
	runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
	var gotArgs string
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{
		Path: "deploy.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterCommand("deploy", ProtocolCommandDefinition{
				Description: "Deploy",
				Handler: func(args string) error {
					gotArgs = args
					return nil
				},
			})
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt("/deploy prod --force"); err != nil {
		t.Fatal(err)
	}
	if gotArgs != "prod --force" {
		t.Fatalf("command args = %q, want %q", gotArgs, "prod --force")
	}
	if messages := session.Messages(); len(messages) != 0 {
		t.Fatalf("extension command should not persist prompt messages: %#v", messages)
	}
}

func TestAgentSessionPromptExpandsPromptTemplateBeforePersistingAndSending(t *testing.T) {
	cwd := t.TempDir()
	writePromptExpansionFile(t, filepath.Join(cwd, ConfigDirName, "prompts", "review.md"), "---\ndescription: Review\n---\nReview $1 with $ARGUMENTS\n")
	var seenPrompt string
	session := newPromptExpansionSession(t, cwd, t.TempDir(), func(prompt string) {
		seenPrompt = prompt
	})

	if err := session.Prompt("/review file.go carefully"); err != nil {
		t.Fatal(err)
	}
	want := "Review file.go with file.go carefully"
	if seenPrompt != want {
		t.Fatalf("responder prompt = %q, want %q", seenPrompt, want)
	}
	assertFirstUserMessageText(t, session, want)
}

func TestAgentSessionPromptExpandsSkillCommandBeforePersistingAndSending(t *testing.T) {
	cwd := t.TempDir()
	skillPath := filepath.Join(cwd, ConfigDirName, "skills", "demo", "SKILL.md")
	writePromptExpansionFile(t, skillPath, "---\nname: demo\ndescription: Demo skill.\n---\nUse demo instructions.\n")
	var seenPrompt string
	session := newPromptExpansionSession(t, cwd, t.TempDir(), func(prompt string) {
		seenPrompt = prompt
	})

	if err := session.Prompt("/skill:demo extra instructions"); err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`<skill name="demo" location="` + skillPath + `">`,
		"References are relative to " + filepath.Dir(skillPath) + ".",
		"Use demo instructions.",
		"</skill>\n\nextra instructions",
	} {
		if !strings.Contains(seenPrompt, needle) {
			t.Fatalf("expanded skill prompt missing %q in %q", needle, seenPrompt)
		}
	}
	assertFirstUserMessageText(t, session, seenPrompt)
}

func TestAgentSessionPromptRunsInputTransformBeforePromptExpansion(t *testing.T) {
	cwd := t.TempDir()
	writePromptExpansionFile(t, filepath.Join(cwd, ConfigDirName, "prompts", "review.md"), "---\ndescription: Review\n---\nReview $1\n")
	var seenPrompt string
	session := newPromptExpansionSession(t, cwd, t.TempDir(), func(prompt string) {
		seenPrompt = prompt
	})
	runtime := NewProtocolExtensionRuntime(CapabilityInputEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{
		Path: "input.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.OnInput(func(event ProtocolInputEvent) (ProtocolInputResult, error) {
				if event.Text == "alias" && event.Source == "interactive" {
					return ProtocolInputTransform("/review transformed.go"), nil
				}
				return ProtocolInputContinue(), nil
			})
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt("alias"); err != nil {
		t.Fatal(err)
	}
	if seenPrompt != "Review transformed.go" {
		t.Fatalf("responder prompt = %q, want transformed template expansion", seenPrompt)
	}
	assertFirstUserMessageText(t, session, seenPrompt)
}

func TestAgentSessionQueuedMessagesExpandPromptsAndPreserveImages(t *testing.T) {
	cwd := t.TempDir()
	writePromptExpansionFile(t, filepath.Join(cwd, ConfigDirName, "prompts", "review.md"), "---\ndescription: Review\n---\nReview $1\n")
	session := newPromptExpansionSession(t, cwd, t.TempDir(), nil)
	image := llm.Image("image-data", "image/png")

	if err := session.SteerWithImages("/review file.go", []llm.ContentPart{image}); err != nil {
		t.Fatal(err)
	}
	steering := session.GetSteeringQueue()
	if len(steering) != 1 || steering[0].Text != "Review file.go" || !sameContentParts(steering[0].Images, []llm.ContentPart{image}) {
		t.Fatalf("steering queue = %#v", steering)
	}
	if got := session.GetSteeringMessages(); len(got) != 1 || got[0] != "Review file.go" {
		t.Fatalf("steering messages = %#v", got)
	}

	if err := session.FollowUpWithImages("plain follow-up", []llm.ContentPart{image}); err != nil {
		t.Fatal(err)
	}
	followUp := session.GetFollowUpQueue()
	if len(followUp) != 1 || followUp[0].Text != "plain follow-up" || !sameContentParts(followUp[0].Images, []llm.ContentPart{image}) {
		t.Fatalf("follow-up queue = %#v", followUp)
	}
}

func TestAgentSessionQueuedMessagesRejectExtensionCommands(t *testing.T) {
	session := newPromptExpansionSession(t, t.TempDir(), t.TempDir(), nil)
	runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{
		Path: "deploy.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterCommand("deploy", ProtocolCommandDefinition{Description: "Deploy", Handler: func(string) error {
				return nil
			}})
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	for _, call := range []struct {
		name string
		err  error
	}{
		{name: "steer", err: session.Steer("/deploy prod")},
		{name: "follow-up", err: session.FollowUp("/deploy prod")},
	} {
		if call.err == nil || !strings.Contains(call.err.Error(), `Extension command "/deploy" cannot be queued`) {
			t.Fatalf("%s error = %v", call.name, call.err)
		}
	}
	if session.PendingMessageCount() != 0 {
		t.Fatalf("pending count = %d", session.PendingMessageCount())
	}
}

func TestAgentSessionExtensionQueuedSlashFollowUpIsRawPiRegression(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var prompts []string
	var commandRuns []string
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		prompts = append(prompts, prompt)
		if calls == 1 {
			close(started)
			<-release
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("first turn complete")}}, nil
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("queued follow-up handled by model")}}, nil
	})
	runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{
		Path: "testcmd.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterCommand("testcmd", ProtocolCommandDefinition{
				Description: "Test command",
				Handler: func(args string) error {
					commandRuns = append(commandRuns, args)
					return nil
				},
			})
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started
	if err := runtime.SendUserMessage("/testcmd queued", ProtocolSendUserMessageOptions{DeliverAs: "followUp"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if len(commandRuns) != 0 {
		t.Fatalf("command runs = %#v", commandRuns)
	}
	if !reflect.DeepEqual(prompts, []string{"start", "/testcmd queued"}) {
		t.Fatalf("prompts = %#v", prompts)
	}
	messages := session.Messages()
	if len(messages) < 3 || messages[2].Role != llm.RoleUser || rpcMessageText(messages[2]) != "/testcmd queued" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestAgentSessionQueuedFollowUpRunsAfterCurrentResponse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var prompts []string
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		prompts = append(prompts, prompt)
		if calls == 1 {
			close(started)
			<-release
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("first done")}}, nil
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("follow-up done")}}, nil
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started

	image := llm.Image("follow-image", "image/png")
	if err := session.FollowUpWithImages("after current run", []llm.ContentPart{image}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(prompts, []string{"start", "after current run"}) {
		t.Fatalf("prompts = %#v", prompts)
	}
	if session.PendingMessageCount() != 0 || len(session.GetFollowUpQueue()) != 0 {
		t.Fatalf("pending=%d follow-up=%#v", session.PendingMessageCount(), session.GetFollowUpQueue())
	}
	messages := session.Messages()
	if len(messages) != 4 || messages[2].Role != llm.RoleUser || rpcMessageText(messages[2]) != "after current run" {
		t.Fatalf("messages = %#v", messages)
	}
	if len(messages[2].Content) != 2 || !sameContentParts(messages[2].Content[1:], []llm.ContentPart{image}) {
		t.Fatalf("follow-up content = %#v", messages[2].Content)
	}
}

func TestAgentSessionQueuedSteeringAllModeBatchesBeforeNextResponse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var contexts [][]string
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		contexts = append(contexts, userTexts(context))
		if calls == 1 {
			close(started)
			<-release
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("first done")}}, nil
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("steered")}}, nil
	})
	session.SteeringMode = "all"
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started

	if err := session.Steer("steer 1"); err != nil {
		t.Fatal(err)
	}
	if err := session.Steer("steer 2"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	wantContexts := [][]string{{"start"}, {"start", "steer 1", "steer 2"}}
	if !reflect.DeepEqual(contexts, wantContexts) {
		t.Fatalf("contexts = %#v, want %#v", contexts, wantContexts)
	}
	if session.PendingMessageCount() != 0 || len(session.GetSteeringQueue()) != 0 {
		t.Fatalf("pending=%d steering=%#v", session.PendingMessageCount(), session.GetSteeringQueue())
	}
}

func TestAgentSessionQueuedFollowUpAllModeBatchesAfterCurrentResponse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var contexts [][]string
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		contexts = append(contexts, userTexts(context))
		if calls == 1 {
			close(started)
			<-release
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	session.FollowUpMode = "all"
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started

	if err := session.FollowUp("follow-up 1"); err != nil {
		t.Fatal(err)
	}
	if err := session.FollowUp("follow-up 2"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	wantContexts := [][]string{{"start"}, {"start", "follow-up 1", "follow-up 2"}}
	if !reflect.DeepEqual(contexts, wantContexts) {
		t.Fatalf("contexts = %#v, want %#v", contexts, wantContexts)
	}
}

func TestAgentSessionQueuedSteeringOneAtATimeDeliversInOrder(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var contexts [][]string
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		contexts = append(contexts, userTexts(context))
		if calls == 1 {
			close(started)
			<-release
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started

	if err := session.Steer("steer 1"); err != nil {
		t.Fatal(err)
	}
	if err := session.Steer("steer 2"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	wantContexts := [][]string{{"start"}, {"start", "steer 1"}, {"start", "steer 1", "steer 2"}}
	if !reflect.DeepEqual(contexts, wantContexts) {
		t.Fatalf("contexts = %#v, want %#v", contexts, wantContexts)
	}
}

func TestAgentSessionQueuedFollowUpOneAtATimeDeliversInOrder(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var contexts [][]string
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		contexts = append(contexts, userTexts(context))
		if calls == 1 {
			close(started)
			<-release
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started

	if err := session.FollowUp("follow-up 1"); err != nil {
		t.Fatal(err)
	}
	if err := session.FollowUp("follow-up 2"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	wantContexts := [][]string{{"start"}, {"start", "follow-up 1"}, {"start", "follow-up 1", "follow-up 2"}}
	if !reflect.DeepEqual(contexts, wantContexts) {
		t.Fatalf("contexts = %#v, want %#v", contexts, wantContexts)
	}
}

func TestAgentSessionQueuedMessageStartSeesPendingDrained(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		if calls == 1 {
			close(started)
			<-release
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	var pendingAtStart []int
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "message_start" && event.Message != nil && event.Message.Role == llm.RoleUser && rpcMessageText(*event.Message) == "queued" {
			pendingAtStart = append(pendingAtStart, session.PendingMessageCount())
		}
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started

	if err := session.FollowUp("queued"); err != nil {
		t.Fatal(err)
	}
	if session.PendingMessageCount() != 1 {
		t.Fatalf("pending before release = %d, want 1", session.PendingMessageCount())
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pendingAtStart, []int{0}) {
		t.Fatalf("pending at message_start = %#v, want [0]", pendingAtStart)
	}
}

func TestAgentSessionExtensionOriginSteeringDeliversBeforeNextResponse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var contexts [][]string
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		contexts = append(contexts, userTexts(context))
		if calls == 1 {
			close(started)
			<-release
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	var extensionContext *ProtocolExtensionContext
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{
		Path: "extension-steer.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			extensionContext = ctx
			return nil
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started

	if err := extensionContext.SendUserMessage("extension steer", ProtocolSendUserMessageOptions{DeliverAs: "steer"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	wantContexts := [][]string{{"start"}, {"start", "extension steer"}}
	if !reflect.DeepEqual(contexts, wantContexts) {
		t.Fatalf("contexts = %#v, want %#v", contexts, wantContexts)
	}
}

func TestAgentSessionExtensionSendUserMessageWhileIdleTriggersTurn(t *testing.T) {
	var prompts []string
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		prompts = append(prompts, prompt)
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("response")}}, nil
	})
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	var extensionContext *ProtocolExtensionContext
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{
		Path: "extension-prompt.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			extensionContext = ctx
			return nil
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	if err := extensionContext.SendUserMessage("from extension", ProtocolSendUserMessageOptions{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(prompts, []string{"from extension"}) {
		t.Fatalf("prompts = %#v", prompts)
	}
	messages := session.Messages()
	if len(messages) != 2 || messages[0].Role != llm.RoleUser || messages[1].Role != llm.RoleAssistant {
		t.Fatalf("messages = %#v", messages)
	}
	if rpcMessageText(messages[0]) != "from extension" {
		t.Fatalf("first message text = %q", rpcMessageText(messages[0]))
	}
}

func TestAgentSessionExtensionCustomMessageSteerQueuesAndPersistsCustomEntry(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var contexts [][]string
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		contexts = append(contexts, userTexts(context))
		if calls == 1 {
			close(started)
			<-release
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	extensionContext := bindCustomMessageExtension(t, session)
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started

	err := extensionContext.SendCustomMessage(ProtocolCustomMessage{
		CustomType: "queue-test",
		Content:    "steer custom",
		Display:    true,
		Details:    map[string]any{"value": float64(1)},
	}, ProtocolSendCustomMessageOptions{DeliverAs: "steer"})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	wantContexts := [][]string{{"start"}, {"start", "steer custom"}}
	if !reflect.DeepEqual(contexts, wantContexts) {
		t.Fatalf("contexts = %#v, want %#v", contexts, wantContexts)
	}
	if !hasCustomMessageEntry(session.SessionManager.GetEntries(), "queue-test", "steer custom") {
		t.Fatalf("entries = %#v", session.SessionManager.GetEntries())
	}
}

func TestAgentSessionExtensionCustomMessageFollowUpQueuesAndPersistsCustomEntry(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var contexts [][]string
	calls := 0
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		contexts = append(contexts, userTexts(context))
		if calls == 1 {
			close(started)
			<-release
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	extensionContext := bindCustomMessageExtension(t, session)
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("start")
	}()
	<-started

	err := extensionContext.SendCustomMessage(ProtocolCustomMessage{
		CustomType: "queue-test",
		Content:    "follow-up custom",
		Display:    true,
		Details:    map[string]any{"value": float64(1)},
	}, ProtocolSendCustomMessageOptions{DeliverAs: "followUp"})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	wantContexts := [][]string{{"start"}, {"start", "follow-up custom"}}
	if !reflect.DeepEqual(contexts, wantContexts) {
		t.Fatalf("contexts = %#v, want %#v", contexts, wantContexts)
	}
	if !hasCustomMessageEntry(session.SessionManager.GetEntries(), "queue-test", "follow-up custom") {
		t.Fatalf("entries = %#v", session.SessionManager.GetEntries())
	}
}

func TestAgentSessionExtensionCustomMessageNextTurnInjectsNextPrompt(t *testing.T) {
	var contexts [][]string
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		contexts = append(contexts, userTexts(context))
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	extensionContext := bindCustomMessageExtension(t, session)

	err := extensionContext.SendCustomMessage(ProtocolCustomMessage{
		CustomType: "next-turn",
		Content:    "carry this",
		Display:    true,
		Details:    map[string]any{"value": float64(1)},
	}, ProtocolSendCustomMessageOptions{DeliverAs: "nextTurn"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt("normal prompt"); err != nil {
		t.Fatal(err)
	}

	wantContexts := [][]string{{"normal prompt", "carry this"}}
	if !reflect.DeepEqual(contexts, wantContexts) {
		t.Fatalf("contexts = %#v, want %#v", contexts, wantContexts)
	}
	if !hasCustomMessageEntry(session.SessionManager.GetEntries(), "next-turn", "carry this") {
		t.Fatalf("entries = %#v", session.SessionManager.GetEntries())
	}
}

func TestAgentSessionPromptPreflightRejectsWithoutModelOrAuth(t *testing.T) {
	t.Run("without model", func(t *testing.T) {
		manager, err := InMemorySessionManager(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		called := false
		session, err := CreateAgentSession(AgentSessionOptions{
			CWD:            t.TempDir(),
			AgentDir:       filepath.Join(t.TempDir(), "agent"),
			SessionManager: manager,
			Responder: func(string, []llm.Message, llm.Model) (llm.Message, error) {
				called = true
				return llm.AssistantMessage([]llm.ContentPart{llm.Text("unexpected")}, llm.StopReasonStop, llm.Model{}), nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		err = session.Prompt("hi")
		if err == nil || !strings.Contains(err.Error(), "No model selected.") {
			t.Fatalf("prompt error = %v", err)
		}
		if called {
			t.Fatal("responder was called")
		}
		if messages := session.Messages(); len(messages) != 0 {
			t.Fatalf("messages = %#v, want none", messages)
		}
	})

	t.Run("without configured auth", func(t *testing.T) {
		manager, err := InMemorySessionManager(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		called := false
		model := llm.Model{ID: "needs-auth", Provider: "openai", API: "openai-responses", Input: []string{"text"}}
		session, err := CreateAgentSession(AgentSessionOptions{
			CWD:            t.TempDir(),
			AgentDir:       filepath.Join(t.TempDir(), "agent"),
			Model:          model,
			SessionManager: manager,
			Preflight: func(model llm.Model) error {
				return errors.New(formatNoAPIKeyFoundMessage(model.Provider))
			},
			Responder: func(string, []llm.Message, llm.Model) (llm.Message, error) {
				called = true
				return llm.AssistantMessage([]llm.ContentPart{llm.Text("unexpected")}, llm.StopReasonStop, model), nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		err = session.Prompt("hi")
		if err == nil || !strings.Contains(err.Error(), "No API key found for openai.") {
			t.Fatalf("prompt error = %v", err)
		}
		if called {
			t.Fatal("responder was called")
		}
		if messages := session.Messages(); len(messages) != 0 {
			t.Fatalf("messages = %#v, want none", messages)
		}
	})
}

func newPromptExpansionSession(t *testing.T, cwd, agentDir string, capture func(string)) *AgentSession {
	t.Helper()
	return newPromptExpansionSessionWithResponder(t, cwd, agentDir, func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		if capture != nil {
			capture(prompt)
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("ok")}}, nil
	})
}

func newPromptExpansionSessionWithResponder(t *testing.T, cwd, agentDir string, responder AgentSessionResponder) *AgentSession {
	t.Helper()
	manager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       agentDir,
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
		Responder:      responder,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Dispose)
	return session
}

func writePromptExpansionFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFirstUserMessageText(t *testing.T, session *AgentSession, want string) {
	t.Helper()
	messages := session.Messages()
	if len(messages) < 1 || messages[0].Role != llm.RoleUser || len(messages[0].Content) == 0 {
		t.Fatalf("messages = %#v", messages)
	}
	if got := messages[0].Content[0].Text; got != want {
		t.Fatalf("first user text = %q, want %q", got, want)
	}
}

func sameContentParts(a, b []llm.ContentPart) bool {
	return reflect.DeepEqual(a, b)
}

func userTexts(messages []llm.Message) []string {
	var result []string
	for _, message := range messages {
		if message.Role == llm.RoleUser {
			result = append(result, rpcMessageText(message))
		}
	}
	return result
}

func bindCustomMessageExtension(t *testing.T, session *AgentSession) *ProtocolExtensionContext {
	t.Helper()
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	var extensionContext *ProtocolExtensionContext
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{
		Path: "custom-message.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			extensionContext = ctx
			return nil
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}
	if extensionContext == nil {
		t.Fatal("extension context not captured")
	}
	return extensionContext
}

func hasCustomMessageEntry(entries []FileEntry, customType, content string) bool {
	for _, entry := range entries {
		if entry.Type == "custom_message" && entry.CustomType == customType && customMessageText(entry.Content) == content {
			return true
		}
	}
	return false
}
