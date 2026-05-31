package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/versioncheck"

const LatestGiVersionURL = versioncheck.LatestGiVersionURL
const LatestPiVersionURL = versioncheck.LatestPiVersionURL

type LatestGiRelease = versioncheck.LatestGiRelease
type LatestPiRelease = versioncheck.LatestPiRelease
type VersionCheckOptions = versioncheck.Options

func ComparePackageVersions(leftVersion, rightVersion string) (int, bool) {
	return versioncheck.ComparePackageVersions(leftVersion, rightVersion)
}

func IsNewerPackageVersion(candidateVersion, currentVersion string) bool {
	return versioncheck.IsNewerPackageVersion(candidateVersion, currentVersion)
}

func GetLatestGiRelease(currentVersion string, options VersionCheckOptions) (LatestGiRelease, bool) {
	return versioncheck.GetLatestGiRelease(currentVersion, options)
}

func GetLatestPiRelease(currentVersion string, options VersionCheckOptions) (LatestPiRelease, bool) {
	return versioncheck.GetLatestPiRelease(currentVersion, options)
}

func GetLatestGiVersion(currentVersion string, options VersionCheckOptions) (string, bool) {
	return versioncheck.GetLatestGiVersion(currentVersion, options)
}

func GetLatestPiVersion(currentVersion string, options VersionCheckOptions) (string, bool) {
	return versioncheck.GetLatestPiVersion(currentVersion, options)
}

func CheckForNewGiVersion(currentVersion string, options VersionCheckOptions) (string, bool) {
	return versioncheck.CheckForNewGiVersion(currentVersion, options)
}

func CheckForNewPiVersion(currentVersion string, options VersionCheckOptions) (string, bool) {
	return versioncheck.CheckForNewPiVersion(currentVersion, options)
}
