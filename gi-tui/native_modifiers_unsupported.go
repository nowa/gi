//go:build !darwin || !cgo

package gitui

func nativeModifierPressed(ModifierKey) bool {
	return false
}
