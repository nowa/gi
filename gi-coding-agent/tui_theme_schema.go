package gicodingagent

import _ "embed"

//go:embed theme-schema.json
var tuiThemeSchemaJSON []byte

// TUIThemeSchemaJSON returns Gi's Pi-compatible TUI theme JSON schema.
func TUIThemeSchemaJSON() []byte {
	return append([]byte(nil), tuiThemeSchemaJSON...)
}
