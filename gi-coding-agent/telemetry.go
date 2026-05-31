package gicodingagent

import telemetry "github.com/nowa/gi/gi-coding-agent/internal/telemetry"

func IsInstallTelemetryEnabled(settingsManager *SettingsManager) bool {
	if settingsManager == nil {
		return telemetry.InstallEnabled(nil)
	}
	return telemetry.InstallEnabled(settingsManager)
}

func lookupInstallTelemetryEnv() (string, bool) {
	return telemetry.LookupInstallEnv()
}

func isTruthyEnvFlag(value string) bool {
	return telemetry.IsTruthyEnvFlag(value)
}
