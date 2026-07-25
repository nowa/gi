package gicodingagent

import (
	attribution "github.com/nowa/gi/gi-coding-agent/internal/attribution"
	llm "github.com/nowa/gi/gi-llm-provider"
)

// ProviderAttributionContext is the immutable session and telemetry state used
// to derive headers for one provider request.
type ProviderAttributionContext = attribution.Context

func GetAttributionHeaders(model llm.Model, installTelemetryEnabled bool) map[string]string {
	return attribution.GetAttributionHeaders(model, installTelemetryEnabled)
}

func BuildSDKStreamHeaders(model llm.Model, installTelemetryEnabled bool, providerHeaders, requestHeaders map[string]string) map[string]string {
	return attribution.BuildSDKStreamHeaders(model, installTelemetryEnabled, providerHeaders, requestHeaders)
}

func MergeProviderAttributionHeaders(
	model llm.Model,
	context ProviderAttributionContext,
	headerSources ...map[string]string,
) map[string]string {
	return attribution.MergeProviderHeaders(
		model,
		context,
		headerSources...,
	)
}
