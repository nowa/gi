package gitui

import (
	"strings"
	"testing"
)

func TestKeysLegacyControlAndWindowsTerminalBackspace(t *testing.T) {
	SetKittyProtocolActive(false)
	t.Setenv("WT_SESSION", "")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "")

	for _, tc := range []struct {
		data string
		spec string
	}{
		{"\x00", "ctrl+space"},
		{"\x1c", "ctrl+\\"},
		{"\x1d", "ctrl+]"},
		{"\x1f", "ctrl+-"},
		{"\x1f", "ctrl+_"},
		{"\x1b\x1b", "ctrl+alt+["},
		{"\x1b\x1c", "ctrl+alt+\\"},
		{"\x1b\x1d", "ctrl+alt+]"},
		{"\x1b\x1f", "ctrl+alt+-"},
		{"\b", "backspace"},
		{"\b", "ctrl+h"},
		{"\x02", "ctrl+b"},
		{"\x06", "ctrl+f"},
		{"\x10", "ctrl+p"},
		{"\x0e", "ctrl+n"},
	} {
		if !MatchesKey(tc.data, tc.spec) {
			t.Fatalf("MatchesKey(%q, %q) = false", tc.data, tc.spec)
		}
	}
	if MatchesKey("\b", "ctrl+backspace") {
		t.Fatalf("plain backspace should not match ctrl+backspace outside local Windows Terminal")
	}

	t.Setenv("WT_SESSION", "test-session")
	if !MatchesKey("\b", "ctrl+backspace") || MatchesKey("\b", "backspace") {
		t.Fatalf("local Windows Terminal backspace should be ctrl+backspace")
	}

	t.Setenv("SSH_CONNECTION", "1 2 3 4")
	if MatchesKey("\b", "ctrl+backspace") || !MatchesKey("\b", "backspace") {
		t.Fatalf("Windows Terminal over SSH backspace should stay plain backspace")
	}
}

func TestKeyHelperMatchesPiKeyObject(t *testing.T) {
	if Key.Escape != "escape" || Key.Esc != "esc" || Key.Enter != "enter" || Key.Return != "return" {
		t.Fatalf("special key aliases = escape:%q esc:%q enter:%q return:%q", Key.Escape, Key.Esc, Key.Enter, Key.Return)
	}
	if Key.PageUp != "pageUp" || Key.PageDown != "pageDown" || Key.F12 != "f12" || Key.LeftBracket != "[" {
		t.Fatalf("navigation/symbol key aliases not Pi-shaped")
	}
	if got := Key.Ctrl("c"); got != "ctrl+c" {
		t.Fatalf("Key.Ctrl = %q, want ctrl+c", got)
	}
	if got := Key.CtrlAlt(Key.RightBracket); got != "ctrl+alt+]" {
		t.Fatalf("Key.CtrlAlt = %q, want ctrl+alt+]", got)
	}
	if got := Key.CtrlShiftAlt("p"); got != "ctrl+shift+alt+p" {
		t.Fatalf("Key.CtrlShiftAlt = %q, want ctrl+shift+alt+p", got)
	}
	if !MatchesKey("\x1b", Key.Escape) || !MatchesKey("\x1b", Key.Esc) {
		t.Fatalf("Key escape aliases should match Escape input")
	}
	if !MatchesKey("\x1b[5~", Key.PageUp) || !MatchesKey("\x1b[24~", Key.F12) {
		t.Fatalf("Key navigation aliases should match parsed terminal input")
	}
}

func TestKeysKittyAlternateLayoutAndSuperModifierMatrix(t *testing.T) {
	SetKittyProtocolActive(true)
	defer SetKittyProtocolActive(false)

	for _, tc := range []struct {
		name string
		data string
		want string
	}{
		{name: "cyrillic ctrl c uses base layout", data: "\x1b[1089::99;5u", want: "ctrl+c"},
		{name: "cyrillic ctrl d uses base layout", data: "\x1b[1074::100;5u", want: "ctrl+d"},
		{name: "cyrillic ctrl z uses base layout", data: "\x1b[1103::122;5u", want: "ctrl+z"},
		{name: "cyrillic ctrl shift p uses base layout", data: "\x1b[1079::112;6u", want: "ctrl+shift+p"},
		{name: "latin direct codepoint still matches", data: "\x1b[99;5u", want: "ctrl+c"},
		{name: "shifted key field", data: "\x1b[99:67:99;2u", want: "shift+c"},
		{name: "release event still matches base layout", data: "\x1b[1089::99;5:3u", want: "ctrl+c"},
		{name: "full shifted base repeat format", data: "\x1b[1089:1057:99;6:2u", want: "ctrl+shift+c"},
		{name: "dvorak prefers latin codepoint", data: "\x1b[107::118;5u", want: "ctrl+k"},
		{name: "dvorak prefers symbol codepoint", data: "\x1b[47::91;5u", want: "ctrl+/"},
		{name: "super printable", data: "\x1b[107;9u", want: "super+k"},
		{name: "super enter", data: "\x1b[13;9u", want: "super+enter"},
		{name: "ctrl super printable", data: "\x1b[107;13u", want: "ctrl+super+k"},
		{name: "ctrl shift super printable", data: "\x1b[107;14u", want: "ctrl+shift+super+k"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !MatchesKey(tc.data, tc.want) {
				t.Fatalf("MatchesKey(%q, %q) = false", tc.data, tc.want)
			}
			event := ParseKey(tc.data)
			if event.Unknown {
				t.Fatalf("ParseKey(%q) returned unknown", tc.data)
			}
			assertKeySpec(t, event, tc.want)
		})
	}

	for _, tc := range []struct {
		data string
		spec string
	}{
		{data: "\x1b[1089::99;5u", spec: "ctrl+d"},
		{data: "\x1b[1089::99;5u", spec: "ctrl+shift+c"},
		{data: "\x1b[107::118;5u", spec: "ctrl+v"},
		{data: "\x1b[47::91;5u", spec: "ctrl+["},
		{data: "\x1b[107;13u", spec: "super+k"},
	} {
		if MatchesKey(tc.data, tc.spec) {
			t.Fatalf("MatchesKey(%q, %q) = true, want false", tc.data, tc.spec)
		}
	}
}

func assertKeySpec(t *testing.T, event KeyEvent, spec string) {
	t.Helper()
	parts := strings.Split(spec, "+")
	key := parts[len(parts)-1]
	wantCtrl, wantAlt, wantShift, wantSuper := false, false, false, false
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "ctrl":
			wantCtrl = true
		case "alt":
			wantAlt = true
		case "shift":
			wantShift = true
		case "super":
			wantSuper = true
		}
	}
	if string(event.Key) != key {
		t.Fatalf("ParseKey key = %q, want %q for %q", event.Key, key, spec)
	}
	if event.Ctrl != wantCtrl || event.Alt != wantAlt || event.Shift != wantShift || event.Super != wantSuper {
		t.Fatalf("ParseKey modifiers = ctrl:%v alt:%v shift:%v super:%v, want %q", event.Ctrl, event.Alt, event.Shift, event.Super, spec)
	}
}

func TestKeysLegacySequencesAndKittyAltGate(t *testing.T) {
	SetKittyProtocolActive(false)
	for _, tc := range []struct {
		data string
		spec string
	}{
		{"\x1bOA", "up"},
		{"\x1bOB", "down"},
		{"\x1bOC", "right"},
		{"\x1bOD", "left"},
		{"\x1bOH", "home"},
		{"\x1bOF", "end"},
		{"\x1bOE", "clear"},
		{"\x1bOP", "f1"},
		{"\x1bOQ", "f2"},
		{"\x1bOR", "f3"},
		{"\x1bOS", "f4"},
		{"\x1bOM", "enter"},
		{"\x1b[11~", "f1"},
		{"\x1b[12~", "f2"},
		{"\x1b[13~", "f3"},
		{"\x1b[14~", "f4"},
		{"\x1b[15~", "f5"},
		{"\x1b[24~", "f12"},
		{"\x1b[[A", "f1"},
		{"\x1b[[B", "f2"},
		{"\x1b[[C", "f3"},
		{"\x1b[[D", "f4"},
		{"\x1b[[E", "f5"},
		{"\x1b[E", "clear"},
		{"\x1b[[5~", "pageUp"},
		{"\x1b[[6~", "pageDown"},
		{"\x1b[a", "shift+up"},
		{"\x1b[b", "shift+down"},
		{"\x1b[c", "shift+right"},
		{"\x1b[d", "shift+left"},
		{"\x1b[e", "shift+clear"},
		{"\x1bOa", "ctrl+up"},
		{"\x1bOb", "ctrl+down"},
		{"\x1bOc", "ctrl+right"},
		{"\x1bOd", "ctrl+left"},
		{"\x1bOe", "ctrl+clear"},
		{"\x1b[2$", "shift+insert"},
		{"\x1b[2^", "ctrl+insert"},
		{"\x1b[3$", "shift+delete"},
		{"\x1b[3^", "ctrl+delete"},
		{"\x1b[5$", "shift+pageUp"},
		{"\x1b[5^", "ctrl+pageUp"},
		{"\x1b[6$", "shift+pageDown"},
		{"\x1b[6^", "ctrl+pageDown"},
		{"\x1b[7$", "shift+home"},
		{"\x1b[7^", "ctrl+home"},
		{"\x1b[8$", "shift+end"},
		{"\x1b[8^", "ctrl+end"},
		{"\x1b ", "alt+space"},
		{"\x1b\r", "alt+enter"},
		{"\x1b\b", "alt+backspace"},
		{"\x1b\x7f", "alt+backspace"},
		{"\x1bb", "alt+left"},
		{"\x1bB", "alt+left"},
		{"\x1bf", "alt+right"},
		{"\x1bF", "alt+right"},
		{"\x1bn", "alt+down"},
		{"\x1bp", "alt+up"},
		{"\x1ba", "alt+a"},
		{"\x1b1", "alt+1"},
	} {
		if !MatchesKey(tc.data, tc.spec) {
			t.Fatalf("MatchesKey(%q, %q) = false", tc.data, tc.spec)
		}
	}
	for _, tc := range []struct {
		data string
		spec string
	}{
		{"\x1bb", "alt+b"},
		{"\x1bf", "alt+f"},
		{"\x1bn", "alt+n"},
		{"\x1bp", "alt+p"},
	} {
		if !MatchesKey(tc.data, tc.spec) {
			t.Fatalf("ambiguous Pi legacy alt sequence MatchesKey(%q, %q) = false", tc.data, tc.spec)
		}
	}

	SetKittyProtocolActive(true)
	defer SetKittyProtocolActive(false)
	for _, data := range []string{"\x1b ", "\x1bB", "\x1bF", "\x1ba", "\x1b1"} {
		if !ParseKey(data).Unknown {
			t.Fatalf("kitty-active legacy alt sequence %q should be unknown", data)
		}
	}
	if !MatchesKey("\x1b\b", "alt+backspace") {
		t.Fatalf("ESC+Ctrl-H alt+backspace should remain available while Kitty protocol is active")
	}
	if !MatchesKey("\x1b\x7f", "alt+backspace") {
		t.Fatalf("ESC+DEL alt+backspace should remain available while Kitty protocol is active")
	}
	for _, tc := range []struct {
		data string
		spec string
	}{
		{"\x1bb", "alt+left"},
		{"\x1bf", "alt+right"},
		{"\x1bn", "alt+down"},
		{"\x1bp", "alt+up"},
		{"\x1b[[A", "f1"},
	} {
		if !MatchesKey(tc.data, tc.spec) {
			t.Fatalf("Kitty-active exact legacy sequence MatchesKey(%q, %q) = false", tc.data, tc.spec)
		}
	}
}

func TestKeysPiModifyOtherAndAltMatrices(t *testing.T) {
	SetKittyProtocolActive(false)
	for _, tc := range []struct {
		data string
		spec string
	}{
		{"\x1b[27;5;99~", "ctrl+c"},
		{"\x1b[27;5;100~", "ctrl+d"},
		{"\x1b[27;5;122~", "ctrl+z"},
		{"\x1b[27;5;13~", "ctrl+enter"},
		{"\x1b[27;2;13~", "shift+enter"},
		{"\x1b[27;3;13~", "alt+enter"},
		{"\x1b[27;2;9~", "shift+tab"},
		{"\x1b[27;5;9~", "ctrl+tab"},
		{"\x1b[27;3;9~", "alt+tab"},
		{"\x1b[27;1;127~", "backspace"},
		{"\x1b[27;5;127~", "ctrl+backspace"},
		{"\x1b[27;3;127~", "alt+backspace"},
		{"\x1b[27;1;27~", "escape"},
		{"\x1b[27;1;32~", "space"},
		{"\x1b[27;5;32~", "ctrl+space"},
		{"\x1b[27;5;47~", "ctrl+/"},
		{"\x1b[27;5;49~", "ctrl+1"},
		{"\x1b[27;2;49~", "shift+1"},
		{"\x1b[27;2;69~", "shift+e"},
		{"\x1b[27;6;69~", "ctrl+shift+e"},
		{"\x1b[27;7;104~", "ctrl+alt+h"},
	} {
		if !MatchesKey(tc.data, tc.spec) {
			t.Fatalf("MatchesKey(%q, %q) = false", tc.data, tc.spec)
		}
	}

	for _, tc := range []struct {
		data string
		spec string
	}{
		{"\x1b\x03", "ctrl+alt+c"},
		{"\x1by", "alt+y"},
		{"\x1bz", "alt+z"},
	} {
		if !MatchesKey(tc.data, tc.spec) {
			t.Fatalf("legacy alt MatchesKey(%q, %q) = false", tc.data, tc.spec)
		}
	}

	SetKittyProtocolActive(true)
	defer SetKittyProtocolActive(false)
	for _, tc := range []struct {
		data string
		spec string
	}{
		{"\x1b\x03", "ctrl+alt+c"},
		{"\x1by", "alt+y"},
		{"\x1bz", "alt+z"},
	} {
		if MatchesKey(tc.data, tc.spec) || !ParseKey(tc.data).Unknown {
			t.Fatalf("kitty-active legacy alt %q should not match %q", tc.data, tc.spec)
		}
	}
}

func TestKeysKittyLinefeedAndUnsupportedModifiers(t *testing.T) {
	SetKittyProtocolActive(true)
	defer SetKittyProtocolActive(false)

	if !MatchesKey("\n", "shift+enter") || MatchesKey("\n", "enter") {
		t.Fatalf("kitty-active linefeed should be shift+enter only")
	}
	if !MatchesKey("\x1b\r", "shift+enter") || MatchesKey("\x1b\r", "alt+enter") {
		t.Fatalf("kitty-active ESC+CR should be shift+enter custom mapping, not alt+enter")
	}
	if event := ParseKey("\x1b\r"); event.Key != KeyEnter || !event.Shift || event.Alt || event.Unknown {
		t.Fatalf("ParseKey(ESC+CR) with Kitty active = %+v, want shift+enter", event)
	}
	if !ParseKey("\x1b[99;17u").Unknown {
		t.Fatalf("unsupported Kitty modifier bits should parse as unknown")
	}
	if !MatchesKey("\x1b[97;65u", "a") || !MatchesKey("\x1b[97;66u", "shift+a") || !MatchesKey("\x1b[97;193u", "a") {
		t.Fatalf("Kitty CapsLock/NumLock modifier bits should be ignored")
	}
	event := ParseKey("\x1b[97;65u")
	if event.Unknown || event.Key != "a" || event.Rune != 'a' || event.Ctrl || event.Alt || event.Shift || event.Super {
		t.Fatalf("ParseKey with Kitty lock modifier bits = %+v, want plain a", event)
	}
	if r, ok := DecodeKittyPrintable("\x1b[97;65u"); !ok || r != 'a' {
		t.Fatalf("DecodeKittyPrintable with Kitty lock modifier bits = (%q,%v), want ('a',true)", r, ok)
	}
	if r, ok := DecodeKittyPrintable("\x1b[99:67:99;2u"); !ok || r != 'C' {
		t.Fatalf("DecodeKittyPrintable with shifted key field = (%q,%v), want ('C',true)", r, ok)
	}
	if !MatchesKey("\x1b[57415u", "=") {
		t.Fatalf("Kitty keypad equals should normalize to printable '='")
	}
	if r, ok := DecodeKittyPrintable("\x1b[57415u"); !ok || r != '=' {
		t.Fatalf("DecodeKittyPrintable keypad equals = (%q,%v), want ('=',true)", r, ok)
	}
	for _, data := range []string{"\x1b[13u", "\x1b[57414u", "\x1b[99;5u", "\x1b[99;7u", "\x1b[99;9u"} {
		if r, ok := DecodeKittyPrintable(data); ok {
			t.Fatalf("DecodeKittyPrintable(%q) = (%q,true), want false", data, r)
		}
	}
}

func TestKeysCSIuEventTypesDigitsAndCtrlAlt(t *testing.T) {
	SetKittyProtocolActive(true)
	if !MatchesKey("\x1b[49u", "1") || !MatchesKey("\x1b[49;5u", "ctrl+1") || MatchesKey("\x1b[49;5u", "ctrl+2") {
		t.Fatalf("Kitty digit bindings should match logical digits and modifiers")
	}
	if !MatchesKey("\x1b[1089:1057:99;6:2u", "ctrl+shift+c") {
		t.Fatalf("full Kitty CSI-u format should match base-layout ctrl+shift+c")
	}
	if !IsKeyRepeat("\x1b[1089:1057:99;6:2u") {
		t.Fatalf("event type 2 should be parsed as repeat")
	}
	if !IsKeyRelease("\x1b[1089::99;5:3u") {
		t.Fatalf("event type 3 should be parsed as release")
	}
	if !MatchesKey("\x1b[57414u", "enter") || !MatchesKey("\x1b[57414;3u", "alt+enter") {
		t.Fatalf("Kitty keypad enter should normalize to enter")
	}
	if !MatchesKey("\x1b[57417u", "left") || !MatchesKey("\x1b[57426u", "delete") {
		t.Fatalf("Kitty keypad navigation should normalize")
	}
	if !MatchesKey("\x1b[1;5:3D", "ctrl+left") || !IsKeyRelease("\x1b[1;5:3D") {
		t.Fatalf("Kitty arrow release sequence should preserve key and event type")
	}
	if !MatchesKey("\x1b[1;5:2C", "ctrl+right") || !IsKeyRepeat("\x1b[1;5:2C") {
		t.Fatalf("Kitty arrow repeat sequence should preserve key and event type")
	}
	if !MatchesKey("\x1b[3;5:3~", "ctrl+delete") || !IsKeyRelease("\x1b[3;5:3~") {
		t.Fatalf("Kitty functional release sequence should preserve key and event type")
	}
	if !MatchesKey("\x1b[5:2~", "pageUp") || !IsKeyRepeat("\x1b[5:2~") {
		t.Fatalf("Kitty functional repeat sequence without modifiers should preserve key and event type")
	}
	if !MatchesKey("\x1b[1;3:3H", "alt+home") || !IsKeyRelease("\x1b[1;3:3H") {
		t.Fatalf("Kitty home/end release sequence should preserve key and event type")
	}

	SetKittyProtocolActive(false)
	if !MatchesKey("\x1b[104;7u", "ctrl+alt+h") || !MatchesKey("\x1b[27;7;104~", "ctrl+alt+h") {
		t.Fatalf("ctrl+alt letter should work through CSI-u and modifyOtherKeys")
	}
}

func TestKeysC1CSIAndSS3Equivalents(t *testing.T) {
	SetKittyProtocolActive(true)
	defer SetKittyProtocolActive(false)

	for _, tc := range []struct {
		data string
		spec string
	}{
		{"\x9bA", "up"},
		{"\u009bB", "down"},
		{"\x9b1;5D", "ctrl+left"},
		{"\u009b1;3:3H", "alt+home"},
		{"\x9b49u", "1"},
		{"\u009b1089:1057:99;6:2u", "ctrl+shift+c"},
		{"\x8fA", "up"},
		{"\u008fM", "enter"},
	} {
		if !MatchesKey(tc.data, tc.spec) {
			t.Fatalf("MatchesKey(C1 %q, %q) = false", tc.data, tc.spec)
		}
		event := ParseKey(tc.data)
		if event.Unknown || event.Raw != tc.data {
			t.Fatalf("ParseKey(C1 %q) = %+v, want parsed event preserving raw input", tc.data, event)
		}
	}

	if !IsKeyRelease("\u009b1;3:3H") {
		t.Fatalf("UTF-8 C1 CSI should preserve event type")
	}
	if r, ok := DecodeKittyPrintable("\x9b49u"); !ok || r != '1' {
		t.Fatalf("DecodeKittyPrintable raw C1 CSI-u = (%q,%v), want ('1',true)", r, ok)
	}
}

func TestDecodePrintableKeyIncludesXtermModifyOtherKeys(t *testing.T) {
	SetKittyProtocolActive(false)
	for _, tc := range []struct {
		data string
		want rune
	}{
		{"\x1b[27;2;69~", 'E'},
		{"\x1b[27;2;196~", 'Ä'},
		{"\x1b[27;2;32~", ' '},
		{"\x1b[69;2u", 'E'},
	} {
		got, ok := DecodePrintableKey(tc.data)
		if !ok || got != tc.want {
			t.Fatalf("DecodePrintableKey(%q) = (%q,%v), want (%q,true)", tc.data, got, ok, tc.want)
		}
	}
	for _, data := range []string{"\x1b[27;2;13~", "\x1b[27;6;69~"} {
		if got, ok := DecodePrintableKey(data); ok {
			t.Fatalf("DecodePrintableKey(%q) = (%q,true), want false", data, got)
		}
	}
}
