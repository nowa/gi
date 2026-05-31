package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/gitsource"

type GitSource = gitsource.GitSource

func ParseGitURL(source string) (GitSource, bool) {
	return gitsource.ParseGitURL(source)
}
