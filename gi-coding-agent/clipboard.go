package gicodingagent

import (
	"context"

	clip "github.com/nowa/gi/gi-coding-agent/internal/clipboard"
)

const defaultClipboardCopyTimeout = clip.DefaultCopyTimeout

type ClipboardCopyOptions = clip.ClipboardCopyOptions
type ClipboardTextReadOptions = clip.ClipboardTextReadOptions
type ClipboardTextCommandOptions = clip.ClipboardTextCommandOptions
type ClipboardTextReadOperations = clip.ClipboardTextReadOperations
type ClipboardCopyOperations = clip.ClipboardCopyOperations

func ReadClipboardText(
	ctx context.Context,
	options ClipboardTextReadOptions,
) (string, bool) {
	return clip.ReadClipboardText(ctx, options)
}

func CopyToClipboard(text string, options ClipboardCopyOptions) error {
	return clip.CopyToClipboard(text, options)
}
