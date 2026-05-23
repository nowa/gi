package gicodingagent

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const LatestGiVersionURL = "https://api.github.com/repos/nowa/gi/releases/latest"
const LatestPiVersionURL = LatestGiVersionURL

type LatestGiRelease struct {
	Version     string
	PackageName string
}

type LatestPiRelease = LatestGiRelease

type VersionCheckOptions struct {
	URL        string
	Timeout    time.Duration
	HTTPClient *http.Client
	Skip       bool
	Offline    bool
}

type parsedPackageVersion struct {
	major      int
	minor      int
	patch      int
	prerelease string
}

var packageVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+.*)?$`)

func ComparePackageVersions(leftVersion, rightVersion string) (int, bool) {
	left, ok := parsePackageVersion(leftVersion)
	if !ok {
		return 0, false
	}
	right, ok := parsePackageVersion(rightVersion)
	if !ok {
		return 0, false
	}
	if left.major != right.major {
		return left.major - right.major, true
	}
	if left.minor != right.minor {
		return left.minor - right.minor, true
	}
	if left.patch != right.patch {
		return left.patch - right.patch, true
	}
	if left.prerelease == right.prerelease {
		return 0, true
	}
	if left.prerelease == "" {
		return 1, true
	}
	if right.prerelease == "" {
		return -1, true
	}
	return strings.Compare(left.prerelease, right.prerelease), true
}

func IsNewerPackageVersion(candidateVersion, currentVersion string) bool {
	if comparison, ok := ComparePackageVersions(candidateVersion, currentVersion); ok {
		return comparison > 0
	}
	return strings.TrimSpace(candidateVersion) != strings.TrimSpace(currentVersion)
}

func GetLatestGiRelease(currentVersion string, options VersionCheckOptions) (LatestGiRelease, bool) {
	if options.Skip || options.Offline ||
		os.Getenv("GI_SKIP_VERSION_CHECK") != "" ||
		os.Getenv("GI_OFFLINE") != "" ||
		os.Getenv("PI_SKIP_VERSION_CHECK") != "" ||
		os.Getenv("PI_OFFLINE") != "" {
		return LatestGiRelease{}, false
	}
	url := options.URL
	if url == "" {
		url = LatestGiVersionURL
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LatestGiRelease{}, false
	}
	request.Header.Set("User-Agent", GetGiUserAgent(currentVersion))
	request.Header.Set("accept", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return LatestGiRelease{}, false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return LatestGiRelease{}, false
	}
	var payload struct {
		Version     any `json:"version"`
		TagName     any `json:"tag_name"`
		PackageName any `json:"packageName"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return LatestGiRelease{}, false
	}
	version, ok := payload.Version.(string)
	if !ok || strings.TrimSpace(version) == "" {
		version, ok = payload.TagName.(string)
	}
	version = strings.TrimSpace(version)
	if !ok || version == "" {
		return LatestGiRelease{}, false
	}
	release := LatestGiRelease{Version: version}
	if packageName, ok := payload.PackageName.(string); ok {
		release.PackageName = strings.TrimSpace(packageName)
	}
	return release, true
}

func GetLatestPiRelease(currentVersion string, options VersionCheckOptions) (LatestPiRelease, bool) {
	return GetLatestGiRelease(currentVersion, options)
}

func GetLatestGiVersion(currentVersion string, options VersionCheckOptions) (string, bool) {
	release, ok := GetLatestGiRelease(currentVersion, options)
	if !ok {
		return "", false
	}
	return release.Version, true
}

func GetLatestPiVersion(currentVersion string, options VersionCheckOptions) (string, bool) {
	return GetLatestGiVersion(currentVersion, options)
}

func CheckForNewGiVersion(currentVersion string, options VersionCheckOptions) (string, bool) {
	latestVersion, ok := GetLatestGiVersion(currentVersion, options)
	if !ok || !IsNewerPackageVersion(latestVersion, currentVersion) {
		return "", false
	}
	return latestVersion, true
}

func CheckForNewPiVersion(currentVersion string, options VersionCheckOptions) (string, bool) {
	return CheckForNewGiVersion(currentVersion, options)
}

func parsePackageVersion(version string) (parsedPackageVersion, bool) {
	match := packageVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return parsedPackageVersion{}, false
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return parsedPackageVersion{major: major, minor: minor, patch: patch, prerelease: match[4]}, true
}
