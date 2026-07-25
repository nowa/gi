package gicodingagent

import (
	"strings"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestCLIInteractiveTUIHostShowsCacheMissNoticeWithoutPersistingIt(t *testing.T) {
	host, session := newCacheMissNoticeTestHost(t, true)
	first, second := cacheStatsHealthyTurns()
	missed := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheWrite: 110_000,
		cost:       llm.UsageCost{CacheWrite: 0.4125},
		timestamp:  600_000,
	})
	missed.StopReason = llm.StopReasonStop
	session.SessionManager.AppendMessage(first)
	session.SessionManager.AppendMessage(second)
	session.SessionManager.AppendMessage(missed)
	before := session.SessionManager.GetEntries()

	host.handleLiveMessageEnd(AgentSessionEvent{
		Type:    "message_end",
		Message: &missed,
	})

	rendered := StripAnsi(strings.Join(host.chat.Render(120), "\n"))
	if !strings.Contains(rendered, "Cache miss after 9m idle") ||
		!strings.Contains(rendered, "105k tokens re-billed") {
		t.Fatalf("cache miss notice missing:\n%s", rendered)
	}
	after := session.SessionManager.GetEntries()
	if len(after) != len(before) {
		t.Fatalf("notice changed persisted entry count: before=%d after=%d", len(before), len(after))
	}
	for _, entry := range after {
		if entry.Type != "message" {
			t.Fatalf("notice persisted as session entry: %#v", entry)
		}
	}
}

func TestCLIInteractiveTUIHostDoesNotShowCacheMissNoticeWhenDisabled(t *testing.T) {
	host, session := newCacheMissNoticeTestHost(t, false)
	first, second := cacheStatsHealthyTurns()
	missed := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheWrite: 110_000,
		cost:       llm.UsageCost{CacheWrite: 0.4125},
		timestamp:  600_000,
	})
	session.SessionManager.AppendMessage(first)
	session.SessionManager.AppendMessage(second)
	session.SessionManager.AppendMessage(missed)

	host.handleLiveMessageEnd(AgentSessionEvent{
		Type:    "message_end",
		Message: &missed,
	})

	if host.chat.ChildCount() != 0 {
		t.Fatalf("disabled cache notice rendered %d components", host.chat.ChildCount())
	}
}

func TestCLIInteractiveTUIHostRebuildsCacheMissNoticesFromEntryIDs(t *testing.T) {
	host, session := newCacheMissNoticeTestHost(t, true)
	first, second := cacheStatsHealthyTurns()
	missed := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheWrite: 110_000,
		cost:       llm.UsageCost{CacheWrite: 0.4125},
		timestamp:  600_000,
	})
	session.SessionManager.AppendMessage(first)
	session.SessionManager.AppendMessage(second)
	missedID := session.SessionManager.AppendMessage(missed)
	beforeCount := len(session.SessionManager.GetEntries())

	misses := collectCacheMisses(
		session.SessionManager.GetEntries(),
		session.ModelRuntime,
	)
	if _, ok := misses[missedID]; !ok {
		t.Fatalf("cache miss not keyed by persisted entry ID %q: %#v", missedID, misses)
	}

	host.rerenderSessionMessages()

	rendered := StripAnsi(strings.Join(host.chat.Render(120), "\n"))
	if !strings.Contains(rendered, "Cache miss after 9m idle") {
		t.Fatalf("rebuilt cache miss notice missing:\n%s", rendered)
	}
	if afterCount := len(session.SessionManager.GetEntries()); afterCount != beforeCount {
		t.Fatalf("rebuild persisted notice: before=%d after=%d", beforeCount, afterCount)
	}
}

func TestCLIInteractiveTUIHostFormatsCacheMissNoticeReasons(t *testing.T) {
	tests := []struct {
		name string
		miss CacheMiss
		want string
	}{
		{
			name: "model switch takes precedence",
			miss: CacheMiss{
				MissedTokens: 50_000,
				MissedCost:   0.25,
				Idle:         10 * time.Minute,
				ModelChanged: true,
			},
			want: "Cache miss after model switch: 50k tokens re-billed (~$0.25)",
		},
		{
			name: "idle gap",
			miss: CacheMiss{
				MissedTokens: 50_000,
				MissedCost:   0.25,
				Idle:         7*time.Minute + 31*time.Second,
			},
			want: "Cache miss after 8m idle: 50k tokens re-billed (~$0.25)",
		},
		{
			name: "generic",
			miss: CacheMiss{
				MissedTokens: 20_000,
			},
			want: "Cache miss: 20k tokens re-billed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := &CLIInteractiveTUIHost{chat: gitui.NewContainer()}
			host.addCacheMissNotice(test.miss)
			rendered := StripAnsi(strings.Join(host.chat.Render(120), "\n"))
			if !strings.Contains(rendered, test.want) {
				t.Fatalf("notice = %q, want %q", rendered, test.want)
			}
		})
	}

	host := &CLIInteractiveTUIHost{chat: gitui.NewContainer()}
	host.addCacheMissNotice(CacheMiss{MissedTokens: 19_999, MissedCost: 0.099})
	if host.chat.ChildCount() != 0 {
		t.Fatalf("insignificant miss rendered %d components", host.chat.ChildCount())
	}
}

func newCacheMissNoticeTestHost(
	t *testing.T,
	showNotices bool,
) (*CLIInteractiveTUIHost, *AgentSession) {
	t.Helper()
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := NewInMemorySettingsManager(map[string]any{
		"showCacheMissNotices": showNotices,
	})
	session := &AgentSession{
		SessionManager:  manager,
		SettingsManager: settings,
	}
	runtimeHost := &agentSessionPrintModeHost{
		session:         session,
		settingsManager: settings,
	}
	return &CLIInteractiveTUIHost{
		runtimeHost: runtimeHost,
		chat:        gitui.NewContainer(),
	}, session
}
