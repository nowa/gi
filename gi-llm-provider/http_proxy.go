package gillmprovider

import (
	"net/url"

	"github.com/nowa/gi/gi-llm-provider/internal/httpproxy"
)

const UnsupportedProxyProtocolMessage = httpproxy.UnsupportedProxyProtocolMessage

func ResolveHTTPProxyURLForTarget(targetURL string) (*url.URL, error) {
	return httpproxy.ResolveHTTPProxyURLForTarget(targetURL)
}
