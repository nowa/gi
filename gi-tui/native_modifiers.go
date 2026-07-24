package gitui

// ModifierKey identifies a physical keyboard modifier.
type ModifierKey string

const (
	ModifierShift   ModifierKey = "shift"
	ModifierCommand ModifierKey = "command"
	ModifierControl ModifierKey = "control"
	ModifierOption  ModifierKey = "option"
)

// IsNativeModifierPressed reports the current platform modifier state. It
// returns false when the platform has no native implementation available.
func IsNativeModifierPressed(key ModifierKey) bool {
	switch key {
	case ModifierShift, ModifierCommand, ModifierControl, ModifierOption:
		return nativeModifierPressed(key)
	default:
		return false
	}
}
