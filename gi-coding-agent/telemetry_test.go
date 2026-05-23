package gicodingagent

import (
	"os"
	"testing"
)

func TestIsInstallTelemetryEnabledUsesGiEnvBeforeSettings(t *testing.T) {
	clearInstallTelemetryEnv(t)
	settings := NewInMemorySettingsManager(map[string]any{"enableInstallTelemetry": true})

	t.Setenv("GI_TELEMETRY", "0")
	if IsInstallTelemetryEnabled(settings) {
		t.Fatal("GI_TELEMETRY=0 should disable install telemetry")
	}

	t.Setenv("GI_TELEMETRY", "yes")
	if !IsInstallTelemetryEnabled(settings) {
		t.Fatal("GI_TELEMETRY=yes should enable install telemetry")
	}
}

func TestIsInstallTelemetryEnabledFallsBackToSettingsAndLegacyEnv(t *testing.T) {
	clearInstallTelemetryEnv(t)
	if !IsInstallTelemetryEnabled(nil) {
		t.Fatal("nil settings should default telemetry on")
	}
	if IsInstallTelemetryEnabled(NewInMemorySettingsManager(map[string]any{"enableInstallTelemetry": false})) {
		t.Fatal("settings should disable install telemetry when env is unset")
	}

	t.Setenv("PI_TELEMETRY", "true")
	if !IsInstallTelemetryEnabled(NewInMemorySettingsManager(map[string]any{"enableInstallTelemetry": false})) {
		t.Fatal("legacy PI_TELEMETRY should remain a migration fallback")
	}
}

func clearInstallTelemetryEnv(t *testing.T) {
	t.Helper()
	giValue, giOK := os.LookupEnv("GI_TELEMETRY")
	piValue, piOK := os.LookupEnv("PI_TELEMETRY")
	if err := os.Unsetenv("GI_TELEMETRY"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("PI_TELEMETRY"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if giOK {
			_ = os.Setenv("GI_TELEMETRY", giValue)
		} else {
			_ = os.Unsetenv("GI_TELEMETRY")
		}
		if piOK {
			_ = os.Setenv("PI_TELEMETRY", piValue)
		} else {
			_ = os.Unsetenv("PI_TELEMETRY")
		}
	})
}
