package gicodingagent

import (
	"path/filepath"
	"regexp"
	"sync"
	"testing"
)

var trackingIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestSettingsManagerAnalyticsPiCases(t *testing.T) {
	t.Run("defaults to disabled with no tracking identifier", func(t *testing.T) {
		manager := NewInMemorySettingsManager(nil)
		if manager.GetEnableAnalytics() {
			t.Fatal("analytics should default to disabled")
		}
		if trackingID := manager.GetTrackingID(); trackingID != "" {
			t.Fatalf("tracking ID = %q, want empty", trackingID)
		}
	})

	t.Run("generates a tracking identifier on opt-in", func(t *testing.T) {
		manager := NewInMemorySettingsManager(nil)
		manager.SetEnableAnalytics(true)
		if !manager.GetEnableAnalytics() {
			t.Fatal("analytics should be enabled")
		}
		if trackingID := manager.GetTrackingID(); !trackingIDPattern.MatchString(trackingID) {
			t.Fatalf("tracking ID = %q, want UUID", trackingID)
		}
	})

	t.Run("does not generate a tracking identifier on opt-out", func(t *testing.T) {
		manager := NewInMemorySettingsManager(nil)
		manager.SetEnableAnalytics(false)
		if manager.GetEnableAnalytics() {
			t.Fatal("analytics should be disabled")
		}
		if trackingID := manager.GetTrackingID(); trackingID != "" {
			t.Fatalf("tracking ID = %q, want empty", trackingID)
		}
	})

	t.Run("keeps the tracking identifier when toggling analytics", func(t *testing.T) {
		manager := NewInMemorySettingsManager(nil)
		manager.SetEnableAnalytics(true)
		trackingID := manager.GetTrackingID()
		manager.SetEnableAnalytics(false)
		manager.SetEnableAnalytics(true)
		if got := manager.GetTrackingID(); got != trackingID {
			t.Fatalf("tracking ID = %q, want %q", got, trackingID)
		}
	})
}

func TestSettingsManagerAnalyticsConcurrentOptInKeepsSingleTrackingID(t *testing.T) {
	manager := NewInMemorySettingsManager(nil)
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			manager.SetEnableAnalytics(true)
		}()
	}
	wait.Wait()

	if !manager.GetEnableAnalytics() {
		t.Fatal("analytics should be enabled")
	}
	if trackingID := manager.GetTrackingID(); !trackingIDPattern.MatchString(trackingID) {
		t.Fatalf("tracking ID = %q, want UUID", trackingID)
	}
}

func TestSettingsManagerAppliesFirstTimeSetupAsOneStateTransition(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	manager := NewSettingsManager(projectDir, agentDir)
	result := FirstTimeSetupResult{Theme: TerminalThemeLight, ShareAnalytics: true}
	if err := manager.ApplyFirstTimeSetup(result); err != nil {
		t.Fatal(err)
	}

	saved := readSettingsJSON(t, filepath.Join(agentDir, "settings.json"))
	if saved["theme"] != "light" || saved["enableAnalytics"] != true {
		t.Fatalf("saved first-time setup = %#v", saved)
	}
	trackingID, _ := saved["trackingId"].(string)
	if !trackingIDPattern.MatchString(trackingID) {
		t.Fatalf("saved tracking ID = %q, want UUID", trackingID)
	}
}
