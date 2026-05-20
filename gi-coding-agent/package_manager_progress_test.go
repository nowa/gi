package gicodingagent

import (
	"runtime"
	"strings"
	"testing"
)

func TestPackageManagerProgressPiParity(t *testing.T) {
	t.Run("resolve extension sources does not emit progress for local paths", func(t *testing.T) {
		var events []PackageProgressEvent
		manager := NewDefaultPackageManager(PackageManagerOptions{
			CWD:             t.TempDir(),
			AgentDir:        t.TempDir(),
			SettingsManager: NewInMemorySettingsManager(nil),
			Progress:        func(event PackageProgressEvent) { events = append(events, event) },
		})

		if _, err := manager.ResolveExtensionSources([]string{t.TempDir()}, ResolveExtensionSourcesOptions{}); err != nil {
			t.Fatal(err)
		}
		if len(events) != 0 {
			t.Fatalf("events = %#v", events)
		}
	})

	t.Run("install attempt emits start and error progress", func(t *testing.T) {
		var events []PackageProgressEvent
		manager := NewDefaultPackageManager(PackageManagerOptions{
			CWD:             t.TempDir(),
			AgentDir:        t.TempDir(),
			SettingsManager: NewInMemorySettingsManager(nil),
		})
		manager.SetProgressCallback(func(event PackageProgressEvent) { events = append(events, event) })

		err := manager.Install("npm:nonexistent-package@1.0.0", false)
		if err == nil || !strings.Contains(err.Error(), "npm packages are not supported") {
			t.Fatalf("install error = %v", err)
		}
		if !packageProgressHas(events, "start", "install") || !packageProgressHas(events, "error", "install") {
			t.Fatalf("events = %#v", events)
		}
	})

	t.Run("install recognizes github HTTPS URL without git prefix", func(t *testing.T) {
		var events []PackageProgressEvent
		manager := NewDefaultPackageManager(PackageManagerOptions{
			CWD:             t.TempDir(),
			AgentDir:        t.TempDir(),
			SettingsManager: NewInMemorySettingsManager(nil),
			Progress:        func(event PackageProgressEvent) { events = append(events, event) },
		})

		if err := manager.Install("https://github.com/nonexistent/repo", false); err != nil {
			t.Fatal(err)
		}
		parsed := manager.ParseSource("https://github.com/nonexistent/repo")
		if parsed.Type != "git" || parsed.Host != "github.com" || parsed.Path != "nonexistent/repo" {
			t.Fatalf("parsed = %#v", parsed)
		}
		if !packageProgressHas(events, "start", "install") || !packageProgressHas(events, "done", "install") {
			t.Fatalf("events = %#v", events)
		}
	})
}

func TestPackageManagerRunCommandCaptureWaitsForCompleteStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX sh")
	}
	output, err := runPackageCommandCapture("sh", []string{"-c", "printf abc; sleep 0.05; printf 123"}, PackageCommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if output != "abc123" {
		t.Fatalf("output = %q", output)
	}
}

func packageProgressHas(events []PackageProgressEvent, eventType, action string) bool {
	for _, event := range events {
		if event.Type == eventType && event.Action == action {
			return true
		}
	}
	return false
}
