//go:build darwin && cgo

package gitui

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>

static int gi_modifier_pressed(int modifier) {
	CGEventFlags flags = CGEventSourceFlagsState(kCGEventSourceStateCombinedSessionState);
	switch (modifier) {
		case 1:
			return (flags & kCGEventFlagMaskShift) != 0;
		case 2:
			return (flags & kCGEventFlagMaskCommand) != 0;
		case 3:
			return (flags & kCGEventFlagMaskControl) != 0;
		case 4:
			return (flags & kCGEventFlagMaskAlternate) != 0;
		default:
			return 0;
	}
}
*/
import "C"

func nativeModifierPressed(key ModifierKey) bool {
	var modifier C.int
	switch key {
	case ModifierShift:
		modifier = 1
	case ModifierCommand:
		modifier = 2
	case ModifierControl:
		modifier = 3
	case ModifierOption:
		modifier = 4
	default:
		return false
	}
	return C.gi_modifier_pressed(modifier) != 0
}
