package gicodingagent

import clip "github.com/nowa/gi/gi-coding-agent/internal/clipboard"

type ClipboardImage = clip.ClipboardImage
type ClipboardImageOptions = clip.ClipboardImageOptions
type ClipboardImageCommandOptions = clip.ClipboardImageCommandOptions
type ClipboardImageCommandResult = clip.ClipboardImageCommandResult
type ClipboardImageOperations = clip.ClipboardImageOperations

func ReadClipboardImage(options ClipboardImageOptions) *ClipboardImage {
	return clip.ReadClipboardImage(options)
}

func IsWaylandSession(env map[string]string) bool {
	return clip.IsWaylandSession(env)
}

func ExtensionForImageMIMEType(mimeType string) string {
	return clip.ExtensionForImageMIMEType(mimeType)
}
