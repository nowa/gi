package gillmprovider

import (
	"net/url"
	"strings"

	"github.com/nowa/gi/gi-llm-provider/internal/httpproxy"
)

const UnsupportedProxyProtocolMessage = httpproxy.UnsupportedProxyProtocolMessage

func ResolveHTTPProxyURLForTarget(targetURL string) (*url.URL, error) {
	return httpproxy.ResolveHTTPProxyURLForTarget(targetURL)
}

// ResolveHTTPProxyURLForTargetWithEnv resolves request-scoped proxy variables
// before falling back to the ambient process environment.
func ResolveHTTPProxyURLForTargetWithEnv(
	targetURL string,
	env ProviderEnv,
) (*url.URL, error) {
	return httpproxy.ResolveHTTPProxyURLForTargetWithLookup(
		targetURL,
		func(name string) string {
			lower := strings.ToLower(name)
			upper := strings.ToUpper(name)
			if value := env[lower]; value != "" {
				return value
			}
			if value := env[upper]; value != "" {
				return value
			}
			return GetProviderEnvValue(name, nil)
		},
	)
}
