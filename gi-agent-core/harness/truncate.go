package harness

import "github.com/nowa/gi/gi-agent-core/harness/utils"

const (
	DefaultMaxLines    = utils.DefaultMaxLines
	DefaultMaxBytes    = utils.DefaultMaxBytes
	GrepMaxLineLength  = utils.GrepMaxLineLength
	TruncatedByLines   = utils.TruncatedByLines
	TruncatedByBytes   = utils.TruncatedByBytes
	TruncatedByNothing = utils.TruncatedByNothing
)

type TruncationOptions = utils.TruncationOptions
type TruncationResult = utils.TruncationResult

func TruncateHead(content string, options TruncationOptions) TruncationResult {
	return utils.TruncateHead(content, options)
}

func TruncateTail(content string, options TruncationOptions) TruncationResult {
	return utils.TruncateTail(content, options)
}

func TruncateLine(line string, maxChars int) (string, bool) {
	return utils.TruncateLine(line, maxChars)
}
