package gicodingagent

import (
	attribution "github.com/nowa/gi/gi-coding-agent/internal/attribution"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func GetAttributionHeaders(model llm.Model, installTelemetryEnabled bool) map[string]string {
	return attribution.GetAttributionHeaders(model, installTelemetryEnabled)
}

func BuildSDKStreamHeaders(model llm.Model, installTelemetryEnabled bool, providerHeaders, requestHeaders map[string]string) map[string]string {
	return attribution.BuildSDKStreamHeaders(model, installTelemetryEnabled, providerHeaders, requestHeaders)
}
