package gicodingagent

import (
	"context"

	"github.com/nowa/gi/gi-coding-agent/internal/ansiutil"
	"github.com/nowa/gi/gi-coding-agent/internal/frontmatter"
	"github.com/nowa/gi/gi-coding-agent/internal/pathutil"
	"github.com/nowa/gi/gi-coding-agent/internal/versioncheck"
)

type FrontmatterResult = frontmatter.Result

func ParseFrontmatter(content string) (FrontmatterResult, error) {
	return frontmatter.Parse(content)
}

func StripFrontmatter(content string) string {
	return frontmatter.Strip(content)
}

func normalizeNewlines(value string) string {
	return frontmatter.NormalizeNewlines(value)
}

func ExpandPath(path string) string {
	return pathutil.ExpandPath(path)
}

func ResolveToCwd(path, cwd string) string {
	return pathutil.ResolveToCwd(path, cwd)
}

func ResolveReadPath(path, cwd string) (string, error) {
	return pathutil.ResolveReadPath(path, cwd)
}

func CanonicalizePath(path string) string {
	return pathutil.CanonicalizePath(path)
}

func GetCwdRelativePath(path, cwd string) (string, bool) {
	return pathutil.GetCwdRelativePath(path, cwd)
}

func IsLocalPath(value string) bool {
	return pathutil.IsLocalPath(value)
}

func MarkPathIgnoredByCloudSync(path string) {
	pathutil.MarkPathIgnoredByCloudSync(
		context.Background(),
		path,
		pathutil.CloudSyncMarkOptions{},
	)
}

func GetGiUserAgent(version string) string {
	return versioncheck.GetGiUserAgent(version)
}

func GetPiUserAgent(version string) string {
	return versioncheck.GetPiUserAgent(version)
}

func normalizeUserPathText(value string) string {
	return pathutil.NormalizeUserPathText(value)
}

func comparableUserPathText(value string) string {
	return pathutil.ComparableUserPathText(value)
}

func comparableUserPathTextWithCase(value string) string {
	return pathutil.ComparableUserPathTextWithCase(value)
}

func comparableUserPathTextFolded(value string) string {
	return pathutil.ComparableUserPathTextFolded(value)
}

func StripAnsi(value string) string {
	return ansiutil.Strip(value)
}
