package gicodingagent

import clip "github.com/nowa/gi/gi-coding-agent/internal/clipboard"

const defaultClipboardCopyTimeout = clip.DefaultCopyTimeout

type ClipboardCopyOptions = clip.ClipboardCopyOptions
type ClipboardTextCommandOptions = clip.ClipboardTextCommandOptions
type ClipboardCopyOperations = clip.ClipboardCopyOperations

func CopyToClipboard(text string, options ClipboardCopyOptions) error {
	return clip.CopyToClipboard(text, options)
}
