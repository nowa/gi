package gicodingagent

import (
	"reflect"
	"strings"
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionPostRunPersistsRetryErrorsButExcludesProviderContext(
	t *testing.T,
) {
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	model := llm.MustGetModel("anthropic", "claude-sonnet-4-5")
	retry := AgentSessionRetrySettings{
		Enabled:     true,
		MaxRetries:  2,
		BaseDelayMs: 1,
	}
	var contexts [][]llm.Message
	calls := 0
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            manager.GetCWD(),
		AgentDir:       t.TempDir(),
		Model:          model,
		SessionManager: manager,
		RetrySettings:  &retry,
		Responder: func(
			_ string,
			context []llm.Message,
			_ llm.Model,
		) (llm.Message, error) {
			contexts = append(
				contexts,
				append([]llm.Message(nil), context...),
			)
			calls++
			if calls == 1 {
				return retryAssistantError("overloaded_error"), nil
			}
			return retryAssistantText("recovered"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	var agentEnds []AgentSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "agent_end" {
			agentEnds = append(agentEnds, event)
		}
	})
	if err := session.Prompt("first"); err != nil {
		t.Fatal(err)
	}
	if len(contexts) != 2 {
		t.Fatalf("provider contexts = %d, want 2", len(contexts))
	}
	if got := assistantErrors(contexts[1]); len(got) != 0 {
		t.Fatalf("retry context contains persisted errors: %#v", got)
	}
	if len(agentEnds) != 2 ||
		!agentEnds[0].WillRetry ||
		agentEnds[1].WillRetry {
		t.Fatalf("agent_end events = %#v", agentEnds)
	}
	if got := assistantErrors(session.Messages()); !reflect.DeepEqual(
		got,
		[]string{"overloaded_error"},
	) {
		t.Fatalf("session errors = %#v", got)
	}

	if err := session.Prompt("second"); err != nil {
		t.Fatal(err)
	}
	if len(contexts) != 3 {
		t.Fatalf("provider contexts after next prompt = %d, want 3", len(contexts))
	}
	if got := assistantErrors(contexts[2]); len(got) != 0 {
		t.Fatalf("later provider context contains retry history: %#v", got)
	}
}

func TestAgentSessionPostRunDrainsMessagesQueuedByAgentEndExtension(
	t *testing.T,
) {
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            manager.GetCWD(),
		AgentDir:       t.TempDir(),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
		Responder: func(
			_ string,
			_ []llm.Message,
			_ llm.Model,
		) (llm.Message, error) {
			calls++
			return retryAssistantText("response"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	order := []string{}
	var extensionRunMessages [][]llm.Message
	var publicRunMessages [][]llm.Message
	queued := false
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	mustLoadProtocolFactories(
		t,
		runtime,
		ProtocolExtensionFactory{
			Path: "post-run.gi.json",
			Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On(
					"agent_end",
					func(event ProtocolSessionEvent) (
						ProtocolEventResult,
						error,
					) {
						order = append(order, "extension")
						extensionRunMessages = append(
							extensionRunMessages,
							append([]llm.Message(nil), event.Messages...),
						)
						if !queued {
							queued = true
							if err := session.FollowUp(
								"queued after agent_end",
							); err != nil {
								return ProtocolEventResult{}, err
							}
						}
						return ProtocolEventResult{}, nil
					},
				)
			},
		},
	)
	runtime.BindSession(session)
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "agent_end" {
			order = append(order, "public")
			publicRunMessages = append(
				publicRunMessages,
				append([]llm.Message(nil), event.Messages...),
			)
		}
	})

	if err := session.Prompt("initial"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("provider calls = %d, want 2", calls)
	}
	if !reflect.DeepEqual(
		order,
		[]string{"extension", "public", "extension", "public"},
	) {
		t.Fatalf("agent_end order = %#v", order)
	}
	if session.PendingMessageCount() != 0 {
		t.Fatalf(
			"pending messages = %d, want 0",
			session.PendingMessageCount(),
		)
	}
	messages := session.Messages()
	if !containsMessageText(messages, "queued after agent_end") {
		t.Fatalf("session messages = %#v", messages)
	}
	wantRoles := [][]string{
		{llm.RoleUser, llm.RoleAssistant},
		{llm.RoleUser, llm.RoleAssistant},
	}
	if got := messageRoleGroups(extensionRunMessages); !reflect.DeepEqual(
		got,
		wantRoles,
	) {
		t.Fatalf("extension agent_end messages = %#v", got)
	}
	if got := messageRoleGroups(publicRunMessages); !reflect.DeepEqual(
		got,
		wantRoles,
	) {
		t.Fatalf("public agent_end messages = %#v", got)
	}
}

func TestAgentSessionPostRunQueuedMessageReusesScheduledContinuation(
	t *testing.T,
) {
	for _, test := range []struct {
		name       string
		firstReply func(llm.Model) llm.Message
		configure  func(*AgentSessionOptions)
	}{
		{
			name: "retry",
			firstReply: func(llm.Model) llm.Message {
				return retryAssistantError("overloaded_error")
			},
			configure: func(options *AgentSessionOptions) {
				settings := AgentSessionRetrySettings{
					Enabled:     true,
					MaxRetries:  1,
					BaseDelayMs: 1,
				}
				compaction := agentharness.CompactionSettings{
					Enabled: false,
				}
				options.RetrySettings = &settings
				options.CompactionSettings = &compaction
			},
		},
		{
			name: "threshold compaction",
			firstReply: func(model llm.Model) llm.Message {
				message := retryAssistantText("large response")
				message.Provider = model.Provider
				message.Model = model.ID
				message.Usage = llm.Usage{TotalTokens: 190}
				return message
			},
			configure: func(options *AgentSessionOptions) {
				settings := agentharness.CompactionSettings{
					Enabled:          true,
					ReserveTokens:    100,
					KeepRecentTokens: 1,
				}
				options.CompactionSettings = &settings
				options.CompactionSummarizer = func(
					preparation agentharness.CompactionPreparation,
					_ string,
				) (agentharness.CompactionResult, error) {
					return agentharness.CompactionResult{
						Summary:          "queued continuation summary",
						FirstKeptEntryID: preparation.FirstKeptEntryID,
						TokensBefore:     preparation.TokensBefore,
					}, nil
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, err := InMemorySessionManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			model := llm.MustGetModel(
				"anthropic",
				"claude-sonnet-4-5",
			)
			model.ContextWindow = 250
			var prompts []string
			calls := 0
			options := AgentSessionOptions{
				CWD:            manager.GetCWD(),
				AgentDir:       t.TempDir(),
				Model:          model,
				SessionManager: manager,
				Responder: func(
					prompt string,
					_ []llm.Message,
					model llm.Model,
				) (llm.Message, error) {
					prompts = append(prompts, prompt)
					calls++
					if calls == 1 {
						return test.firstReply(model), nil
					}
					message := retryAssistantText("queued response")
					message.Usage = llm.Usage{TotalTokens: 1}
					return message, nil
				},
			}
			test.configure(&options)
			session, err := CreateAgentSession(options)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Dispose()

			queued := false
			runtime := NewProtocolExtensionRuntime(
				CapabilityLifecycleEvents,
			)
			mustLoadProtocolFactories(
				t,
				runtime,
				ProtocolExtensionFactory{
					Path: "scheduled-continuation.gi.json",
					Factory: func(ctx *ProtocolExtensionContext) error {
						return ctx.On(
							"agent_end",
							func(ProtocolSessionEvent) (
								ProtocolEventResult,
								error,
							) {
								if queued {
									return ProtocolEventResult{}, nil
								}
								queued = true
								return ProtocolEventResult{},
									session.FollowUp("queued after agent_end")
							},
						)
					},
				},
			)
			runtime.BindSession(session)

			if err := session.Prompt("initial"); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(
				prompts,
				[]string{"initial", "queued after agent_end"},
			) {
				t.Fatalf("provider prompts = %#v", prompts)
			}
		})
	}
}

func TestAgentSessionPostRunRoutesOverflowThroughCompactionRecovery(
	t *testing.T,
) {
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	model := autoCompactionTestModel()
	retry := AgentSessionRetrySettings{
		Enabled:     true,
		MaxRetries:  3,
		BaseDelayMs: 1,
	}
	compaction := agentharness.CompactionSettings{
		Enabled:          true,
		ReserveTokens:    20_000,
		KeepRecentTokens: 1,
	}
	compactionCalls := 0
	var contexts [][]llm.Message
	calls := 0
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:                manager.GetCWD(),
		AgentDir:           t.TempDir(),
		Model:              model,
		SessionManager:     manager,
		RetrySettings:      &retry,
		CompactionSettings: &compaction,
		AutoCompactionRunner: func(reason string, willRetry bool) error {
			compactionCalls++
			if reason != "overflow" || !willRetry {
				t.Fatalf(
					"auto compaction = %q/%v, want overflow/true",
					reason,
					willRetry,
				)
			}
			return nil
		},
		Responder: func(
			_ string,
			context []llm.Message,
			_ llm.Model,
		) (llm.Message, error) {
			contexts = append(
				contexts,
				append([]llm.Message(nil), context...),
			)
			calls++
			if calls == 1 {
				return autoCompactionAssistantError(
					"prompt is too long",
					1,
				), nil
			}
			return retryAssistantText("recovered after compaction"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	var events []AgentSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		events = append(events, event)
	})
	if err := session.Prompt("large request"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || compactionCalls != 1 {
		t.Fatalf(
			"provider/compaction calls = %d/%d, want 2/1",
			calls,
			compactionCalls,
		)
	}
	if got := eventsOfType(events, "auto_retry_start"); len(got) != 0 {
		t.Fatalf("overflow used ordinary retry: %#v", got)
	}
	compactionEnds := eventsOfType(events, "compaction_end")
	if len(compactionEnds) != 1 || !compactionEnds[0].WillRetry {
		t.Fatalf("compaction_end events = %#v", compactionEnds)
	}
	if got := assistantErrors(contexts[1]); len(got) != 0 {
		t.Fatalf("overflow retry context contains error: %#v", got)
	}
	if got := assistantErrors(session.Messages()); !reflect.DeepEqual(
		got,
		[]string{"prompt is too long"},
	) {
		t.Fatalf("persisted overflow errors = %#v", got)
	}
}

func TestAgentSessionPostRunEstimatesThresholdAfterZeroUsageResponse(
	t *testing.T,
) {
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	model := llm.MustGetModel("anthropic", "claude-sonnet-4-5")
	model.ContextWindow = 250
	settings := agentharness.CompactionSettings{
		Enabled:          true,
		ReserveTokens:    100,
		KeepRecentTokens: 1,
	}
	calls := 0
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:                manager.GetCWD(),
		AgentDir:           t.TempDir(),
		Model:              model,
		SessionManager:     manager,
		CompactionSettings: &settings,
		Responder: func(
			_ string,
			_ []llm.Message,
			_ llm.Model,
		) (llm.Message, error) {
			calls++
			message := retryAssistantText("response")
			if calls == 1 {
				message.Usage = llm.Usage{TotalTokens: 100}
			}
			return message, nil
		},
		CompactionSummarizer: func(
			preparation agentharness.CompactionPreparation,
			_ string,
		) (agentharness.CompactionResult, error) {
			return agentharness.CompactionResult{
				Summary:          "zero usage summary",
				FirstKeptEntryID: preparation.FirstKeptEntryID,
				TokensBefore:     preparation.TokensBefore,
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	if err := session.Prompt("seed usage"); err != nil {
		t.Fatal(err)
	}
	if got := len(filterFileEntriesByType(
		manager.GetEntries(),
		"compaction",
	)); got != 0 {
		t.Fatalf("compactions after seed = %d, want 0", got)
	}
	if err := session.Prompt(strings.Repeat("context ", 80)); err != nil {
		t.Fatal(err)
	}
	if got := len(filterFileEntriesByType(
		manager.GetEntries(),
		"compaction",
	)); got != 1 {
		t.Fatalf("compactions after zero usage = %d, want 1", got)
	}
}

func TestAgentSessionPostRunAutoCompactsInsideActivePrompt(t *testing.T) {
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	model := llm.MustGetModel("anthropic", "claude-sonnet-4-5")
	model.ContextWindow = 250
	settings := agentharness.CompactionSettings{
		Enabled:          true,
		ReserveTokens:    100,
		KeepRecentTokens: 1,
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:                manager.GetCWD(),
		AgentDir:           t.TempDir(),
		Model:              model,
		SessionManager:     manager,
		CompactionSettings: &settings,
		Responder: func(
			_ string,
			_ []llm.Message,
			model llm.Model,
		) (llm.Message, error) {
			message := retryAssistantText("large response")
			message.Provider = model.Provider
			message.Model = model.ID
			message.Usage = llm.Usage{
				Input:       190,
				TotalTokens: 190,
			}
			return message, nil
		},
		CompactionSummarizer: func(
			preparation agentharness.CompactionPreparation,
			_ string,
		) (agentharness.CompactionResult, error) {
			return agentharness.CompactionResult{
				Summary:          "automatic summary",
				FirstKeptEntryID: preparation.FirstKeptEntryID,
				TokensBefore:     preparation.TokensBefore,
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	var compactionEnds []AgentSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "compaction_end" {
			compactionEnds = append(compactionEnds, event)
		}
	})
	if err := session.Prompt("compact me"); err != nil {
		t.Fatal(err)
	}
	entries := filterFileEntriesByType(
		session.SessionManager.GetEntries(),
		"compaction",
	)
	if len(entries) != 1 {
		t.Fatalf("compaction entries = %#v", entries)
	}
	if len(compactionEnds) != 1 ||
		compactionEnds[0].Reason != "threshold" ||
		compactionEnds[0].WillRetry ||
		compactionEnds[0].Result == nil ||
		compactionEnds[0].Result.EstimatedTokensAfter == 0 {
		t.Fatalf("compaction_end events = %#v", compactionEnds)
	}
	if !session.IsIdle() {
		t.Fatal("session should be idle after automatic compaction")
	}
}

func TestAgentSessionRuntimeSettingsUseSingleManagerOwner(t *testing.T) {
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := NewInMemorySettingsManager(map[string]any{
		"steeringMode": "all",
		"followUpMode": "one-at-a-time",
		"compaction":   map[string]any{"enabled": false},
		"retry":        map[string]any{"enabled": false},
	})
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:             manager.GetCWD(),
		AgentDir:        t.TempDir(),
		Model:           llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SettingsManager: settings,
		SessionManager:  manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()
	if session.SteeringMode != "all" ||
		session.FollowUpMode != "one-at-a-time" ||
		session.CompactionSettings.Enabled ||
		session.RetrySettings.Enabled {
		t.Fatalf(
			"initial runtime settings = %q/%q/%v/%v",
			session.SteeringMode,
			session.FollowUpMode,
			session.CompactionSettings.Enabled,
			session.RetrySettings.Enabled,
		)
	}

	rpc := NewRPCSessionHost(session)
	if err := rpc.SetSteeringMode("one-at-a-time"); err != nil {
		t.Fatal(err)
	}
	if err := rpc.SetFollowUpMode("all"); err != nil {
		t.Fatal(err)
	}
	enabled := true
	if err := rpc.SetAutoCompaction(&enabled); err != nil {
		t.Fatal(err)
	}
	if err := rpc.SetAutoRetry(&enabled); err != nil {
		t.Fatal(err)
	}
	session.SyncRuntimeSettings()

	if settings.GetSteeringMode() != "one-at-a-time" ||
		settings.GetFollowUpMode() != "all" ||
		!settings.GetCompactionEnabled() ||
		!settings.GetRetryEnabled() {
		t.Fatalf("durable settings did not receive RPC changes")
	}
	if session.SteeringMode != "one-at-a-time" ||
		session.FollowUpMode != "all" ||
		!session.CompactionSettings.Enabled ||
		!session.RetrySettings.Enabled {
		t.Fatalf(
			"synced runtime settings = %q/%q/%v/%v",
			session.SteeringMode,
			session.FollowUpMode,
			session.CompactionSettings.Enabled,
			session.RetrySettings.Enabled,
		)
	}
}

func assistantErrors(messages []llm.Message) []string {
	var result []string
	for _, message := range messages {
		if message.Role == llm.RoleAssistant &&
			message.StopReason == llm.StopReasonError {
			result = append(result, message.ErrorMessage)
		}
	}
	return result
}

func containsMessageText(messages []llm.Message, want string) bool {
	for _, message := range messages {
		for _, part := range message.Content {
			if part.Type == llm.ContentText && part.Text == want {
				return true
			}
		}
	}
	return false
}

func messageRoleGroups(groups [][]llm.Message) [][]string {
	result := make([][]string, len(groups))
	for index, messages := range groups {
		result[index] = make([]string, len(messages))
		for messageIndex, message := range messages {
			result[index][messageIndex] = message.Role
		}
	}
	return result
}
