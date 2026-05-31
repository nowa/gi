package telemetry

import (
	"os"
	"strings"
)

type Settings interface {
	GetEnableInstallTelemetry() bool
}

func InstallEnabled(settings Settings) bool {
	if value, ok := LookupInstallEnv(); ok {
		return IsTruthyEnvFlag(value)
	}
	if settings == nil {
		return true
	}
	return settings.GetEnableInstallTelemetry()
}

func LookupInstallEnv() (string, bool) {
	if value, ok := os.LookupEnv("GI_TELEMETRY"); ok {
		return value, true
	}
	return os.LookupEnv("PI_TELEMETRY")
}

func IsTruthyEnvFlag(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "1" || strings.EqualFold(trimmed, "true") || strings.EqualFold(trimmed, "yes")
}
