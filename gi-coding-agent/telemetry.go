package gicodingagent

import (
	"os"
	"strings"
)

func IsInstallTelemetryEnabled(settingsManager *SettingsManager) bool {
	if value, ok := lookupInstallTelemetryEnv(); ok {
		return isTruthyEnvFlag(value)
	}
	if settingsManager == nil {
		return true
	}
	return settingsManager.GetEnableInstallTelemetry()
}

func lookupInstallTelemetryEnv() (string, bool) {
	if value, ok := os.LookupEnv("GI_TELEMETRY"); ok {
		return value, true
	}
	return os.LookupEnv("PI_TELEMETRY")
}

func isTruthyEnvFlag(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "1" || strings.EqualFold(trimmed, "true") || strings.EqualFold(trimmed, "yes")
}
