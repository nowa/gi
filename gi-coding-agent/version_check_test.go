package gicodingagent

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestVersionChecksComparePackageVersions(t *testing.T) {
	if got, ok := ComparePackageVersions("0.70.6", "0.70.5"); !ok || got <= 0 {
		t.Fatalf("0.70.6 vs 0.70.5 = %d, %v", got, ok)
	}
	if got, ok := ComparePackageVersions("0.70.5", "0.70.5"); !ok || got != 0 {
		t.Fatalf("0.70.5 vs 0.70.5 = %d, %v", got, ok)
	}
	if got, ok := ComparePackageVersions("0.70.4", "0.70.5"); !ok || got >= 0 {
		t.Fatalf("0.70.4 vs 0.70.5 = %d, %v", got, ok)
	}
	if IsNewerPackageVersion("0.70.5", "0.70.5") {
		t.Fatalf("same version should not be newer")
	}
	if !IsNewerPackageVersion("0.70.6", "0.70.5") {
		t.Fatalf("candidate should be newer")
	}
}

func TestVersionChecksReturnOnlyNewerVersions(t *testing.T) {
	client := latestVersionClient(t, func(_ *http.Request) map[string]any {
		return map[string]any{"version": "1.2.3"}
	})
	options := VersionCheckOptions{URL: LatestPiVersionURL, HTTPClient: client}

	if got, ok := CheckForNewPiVersion("1.2.3", options); ok || got != "" {
		t.Fatalf("same latest = %q, %v", got, ok)
	}
	if got, ok := CheckForNewPiVersion("1.2.2", options); !ok || got != "1.2.3" {
		t.Fatalf("new latest = %q, %v", got, ok)
	}
}

func TestVersionChecksUsePiDevAPIWithPiUserAgent(t *testing.T) {
	var userAgent string
	var accept string
	client := latestVersionClient(t, func(r *http.Request) map[string]any {
		userAgent = r.Header.Get("User-Agent")
		accept = r.Header.Get("accept")
		return map[string]any{"version": "1.2.4"}
	})

	if got, ok := GetLatestPiVersion("1.2.3", VersionCheckOptions{URL: LatestPiVersionURL, HTTPClient: client}); !ok || got != "1.2.4" {
		t.Fatalf("latest version = %q, %v", got, ok)
	}
	if !strings.HasPrefix(userAgent, "pi/1.2.3 ") {
		t.Fatalf("user agent = %q", userAgent)
	}
	if accept != "application/json" {
		t.Fatalf("accept = %q", accept)
	}
}

func TestVersionChecksReturnActivePackageName(t *testing.T) {
	client := latestVersionClient(t, func(_ *http.Request) map[string]any {
		return map[string]any{"packageName": "@new-scope/pi", "version": "1.2.4"}
	})

	release, ok := GetLatestPiRelease("1.2.3", VersionCheckOptions{URL: LatestPiVersionURL, HTTPClient: client})
	if !ok || release.Version != "1.2.4" || release.PackageName != "@new-scope/pi" {
		t.Fatalf("release = %#v, %v", release, ok)
	}
}

func TestVersionChecksSkipAPICallsWhenDisabled(t *testing.T) {
	called := false
	client := latestVersionClient(t, func(_ *http.Request) map[string]any {
		called = true
		return map[string]any{"version": "1.2.4"}
	})

	if got, ok := GetLatestPiVersion("1.2.3", VersionCheckOptions{URL: LatestPiVersionURL, HTTPClient: client, Skip: true}); ok || got != "" {
		t.Fatalf("latest version = %q, %v", got, ok)
	}
	if called {
		t.Fatalf("server should not have been called")
	}

	t.Setenv("PI_SKIP_VERSION_CHECK", "1")
	if got, ok := GetLatestPiVersion("1.2.3", VersionCheckOptions{URL: LatestPiVersionURL, HTTPClient: client}); ok || got != "" {
		t.Fatalf("latest version with env skip = %q, %v", got, ok)
	}
}

func latestVersionClient(t *testing.T, payload func(*http.Request) map[string]any) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		content, err := json.Marshal(payload(request))
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"content-type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(content))),
			Request:    request,
		}, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
