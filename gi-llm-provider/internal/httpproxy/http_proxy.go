package httpproxy

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const UnsupportedProxyProtocolMessage = "Unsupported proxy protocol. SOCKS and PAC proxy URLs are not supported; use an HTTP or HTTPS proxy URL."

var defaultProxyPorts = map[string]int{
	"ftp":    21,
	"gopher": 70,
	"http":   80,
	"https":  443,
	"ws":     80,
	"wss":    443,
}

func ResolveHTTPProxyURLForTarget(targetURL string) (*url.URL, error) {
	return ResolveHTTPProxyURLForTargetWithLookup(targetURL, proxyEnv)
}

// ResolveHTTPProxyURLForTargetWithLookup resolves proxy variables through a
// caller-owned lookup. This keeps request-scoped provider configuration out of
// the process environment.
func ResolveHTTPProxyURLForTargetWithLookup(
	targetURL string,
	lookup func(string) string,
) (*url.URL, error) {
	if lookup == nil {
		lookup = proxyEnv
	}
	proxy := proxyForURL(targetURL, lookup)
	if proxy == "" {
		return nil, nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
		return nil, fmt.Errorf("Invalid proxy URL %q", proxy)
	}
	if proxyURL.Scheme != "http" && proxyURL.Scheme != "https" {
		return nil, fmt.Errorf("%s Got %s:", UnsupportedProxyProtocolMessage, proxyURL.Scheme)
	}
	return proxyURL, nil
}

func proxyForURL(targetURL string, lookup func(string) string) string {
	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	protocol := parsed.Scheme
	host := parsed.Hostname()
	port := parsed.Port()
	portNumber := defaultProxyPorts[protocol]
	if port != "" {
		if parsedPort, err := strconv.Atoi(port); err == nil {
			portNumber = parsedPort
		}
	}
	if !shouldProxyHost(host, portNumber, lookup) {
		return ""
	}
	proxy := lookupProxyEnv(protocol+"_proxy", lookup)
	if proxy == "" {
		proxy = lookupProxyEnv("all_proxy", lookup)
	}
	if proxy != "" && !strings.Contains(proxy, "://") {
		proxy = protocol + "://" + proxy
	}
	return proxy
}

func shouldProxyHost(host string, port int, lookup func(string) string) bool {
	noProxy := strings.ToLower(lookupProxyEnv("no_proxy", lookup))
	if noProxy == "" {
		return true
	}
	if noProxy == "*" {
		return false
	}
	for _, entry := range splitProxyList(noProxy) {
		if entry == "" {
			continue
		}
		proxyHost := entry
		proxyPort := 0
		if before, after, ok := strings.Cut(entry, ":"); ok {
			if parsedPort, err := strconv.Atoi(after); err == nil {
				proxyHost = before
				proxyPort = parsedPort
			}
		}
		if proxyPort != 0 && proxyPort != port {
			continue
		}
		if !strings.HasPrefix(proxyHost, ".") && !strings.HasPrefix(proxyHost, "*") {
			if host == proxyHost {
				return false
			}
			continue
		}
		if strings.HasPrefix(proxyHost, "*") {
			proxyHost = strings.TrimPrefix(proxyHost, "*")
		}
		if strings.HasSuffix(host, proxyHost) {
			return false
		}
	}
	return true
}

func splitProxyList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func proxyEnv(key string) string {
	if value := os.Getenv(strings.ToLower(key)); value != "" {
		return value
	}
	return os.Getenv(strings.ToUpper(key))
}

func lookupProxyEnv(key string, lookup func(string) string) string {
	if value := lookup(strings.ToLower(key)); value != "" {
		return value
	}
	return lookup(strings.ToUpper(key))
}
