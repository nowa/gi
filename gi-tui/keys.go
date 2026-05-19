package gitui

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

type KeyID string
type KeyEventType string

const (
	KeyPress   KeyEventType = "press"
	KeyRepeat  KeyEventType = "repeat"
	KeyRelease KeyEventType = "release"
)

const (
	KeyUp        KeyID = "up"
	KeyDown      KeyID = "down"
	KeyLeft      KeyID = "left"
	KeyRight     KeyID = "right"
	KeyEnter     KeyID = "enter"
	KeyEscape    KeyID = "escape"
	KeyTab       KeyID = "tab"
	KeyBacktab   KeyID = "shift+tab"
	KeyBackspace KeyID = "backspace"
	KeyDelete    KeyID = "delete"
	KeyHome      KeyID = "home"
	KeyEnd       KeyID = "end"
)

type KeyHelper struct {
	Escape, Esc, Enter, Return, Tab, Space             string
	Backspace, Delete, Insert, Clear                   string
	Home, End, PageUp, PageDown, Up, Down, Left, Right string
	F1, F2, F3, F4, F5, F6, F7, F8, F9, F10, F11, F12  string

	Backtick, Hyphen, Equals, LeftBracket, RightBracket string
	Backslash, Semicolon, Quote, Comma, Period, Slash   string
	Exclamation, At, Hash, Dollar, Percent, Caret       string
	Ampersand, Asterisk, LeftParen, RightParen          string
	Underscore, Plus, Pipe, Tilde                       string
	LeftBrace, RightBrace, Colon, LessThan, GreaterThan string
	Question                                            string
}

var Key = KeyHelper{
	Escape: "escape", Esc: "esc", Enter: "enter", Return: "return", Tab: "tab", Space: "space",
	Backspace: "backspace", Delete: "delete", Insert: "insert", Clear: "clear",
	Home: "home", End: "end", PageUp: "pageUp", PageDown: "pageDown", Up: "up", Down: "down", Left: "left", Right: "right",
	F1: "f1", F2: "f2", F3: "f3", F4: "f4", F5: "f5", F6: "f6", F7: "f7", F8: "f8", F9: "f9", F10: "f10", F11: "f11", F12: "f12",

	Backtick: "`", Hyphen: "-", Equals: "=", LeftBracket: "[", RightBracket: "]",
	Backslash: "\\", Semicolon: ";", Quote: "'", Comma: ",", Period: ".", Slash: "/",
	Exclamation: "!", At: "@", Hash: "#", Dollar: "$", Percent: "%", Caret: "^",
	Ampersand: "&", Asterisk: "*", LeftParen: "(", RightParen: ")",
	Underscore: "_", Plus: "+", Pipe: "|", Tilde: "~",
	LeftBrace: "{", RightBrace: "}", Colon: ":", LessThan: "<", GreaterThan: ">",
	Question: "?",
}

func (KeyHelper) Ctrl(key string) string         { return "ctrl+" + key }
func (KeyHelper) Shift(key string) string        { return "shift+" + key }
func (KeyHelper) Alt(key string) string          { return "alt+" + key }
func (KeyHelper) Super(key string) string        { return "super+" + key }
func (KeyHelper) CtrlShift(key string) string    { return "ctrl+shift+" + key }
func (KeyHelper) ShiftCtrl(key string) string    { return "shift+ctrl+" + key }
func (KeyHelper) CtrlAlt(key string) string      { return "ctrl+alt+" + key }
func (KeyHelper) AltCtrl(key string) string      { return "alt+ctrl+" + key }
func (KeyHelper) ShiftAlt(key string) string     { return "shift+alt+" + key }
func (KeyHelper) AltShift(key string) string     { return "alt+shift+" + key }
func (KeyHelper) CtrlSuper(key string) string    { return "ctrl+super+" + key }
func (KeyHelper) SuperCtrl(key string) string    { return "super+ctrl+" + key }
func (KeyHelper) ShiftSuper(key string) string   { return "shift+super+" + key }
func (KeyHelper) SuperShift(key string) string   { return "super+shift+" + key }
func (KeyHelper) AltSuper(key string) string     { return "alt+super+" + key }
func (KeyHelper) SuperAlt(key string) string     { return "super+alt+" + key }
func (KeyHelper) CtrlShiftAlt(key string) string { return "ctrl+shift+alt+" + key }
func (KeyHelper) CtrlShiftSuper(key string) string {
	return "ctrl+shift+super+" + key
}

const keyLockMask = 64 | 128

type KeyEvent struct {
	Key     KeyID
	Rune    rune
	Ctrl    bool
	Alt     bool
	Shift   bool
	Super   bool
	Type    KeyEventType
	Raw     string
	Unknown bool
}

var kittyProtocolActive atomic.Bool

func SetKittyProtocolActive(active bool) { kittyProtocolActive.Store(active) }
func IsKittyProtocolActive() bool        { return kittyProtocolActive.Load() }

func IsKeyRelease(data string) bool {
	event := ParseKey(data)
	return event.Type == KeyRelease
}

func IsKeyRepeat(data string) bool {
	event := ParseKey(data)
	return event.Type == KeyRepeat
}

func DecodeKittyPrintable(data string) (rune, bool) {
	event := ParseKey(data)
	if event.Rune == 0 || !isPlainPrintableRune(event.Rune) || event.Ctrl || event.Alt || event.Super || event.Unknown {
		return 0, false
	}
	return event.Rune, true
}

func DecodePrintableKey(data string) (rune, bool) {
	event := ParseKey(data)
	if event.Rune == 0 || !isPlainPrintableRune(event.Rune) || event.Ctrl || event.Alt || event.Super || event.Unknown {
		return 0, false
	}
	return event.Rune, true
}

func isPlainPrintableText(data string) bool {
	if data == "" || !utf8.ValidString(data) {
		return false
	}
	for _, r := range data {
		if !isPlainPrintableRune(r) {
			return false
		}
	}
	return true
}

func isPlainPrintableRune(r rune) bool {
	return r >= 32 && r != 0x7f && (r < 0x80 || r > 0x9f)
}

func ParseKey(data string) KeyEvent {
	if data == "" {
		return unknownKey(data)
	}
	if normalized, ok := normalizeC1KeySequence(data); ok {
		event := ParseKey(normalized)
		event.Raw = data
		return event
	}
	if strings.HasPrefix(data, "\x1b[") && strings.HasSuffix(data, "u") {
		if event, ok := parseCSIu(data); ok {
			return event
		}
	}
	if strings.HasPrefix(data, "\x1b[27;") && strings.HasSuffix(data, "~") {
		if event, ok := parseModifyOtherKeys(data); ok {
			return event
		}
	}
	if event, ok := parseLegacyExactSequence(data); ok {
		return event
	}
	if strings.HasPrefix(data, "\x1b[") {
		if event, ok := parseCSIKey(data); ok {
			return event
		}
	}
	switch data {
	case "\x1b[A":
		return KeyEvent{Key: KeyUp, Raw: data, Type: KeyPress}
	case "\x1b[B":
		return KeyEvent{Key: KeyDown, Raw: data, Type: KeyPress}
	case "\x1b[C":
		return KeyEvent{Key: KeyRight, Raw: data, Type: KeyPress}
	case "\x1b[D":
		return KeyEvent{Key: KeyLeft, Raw: data, Type: KeyPress}
	case "\r", "\r\n":
		return KeyEvent{Key: KeyEnter, Raw: data, Type: KeyPress}
	case "\n":
		event := KeyEvent{Key: KeyEnter, Raw: data, Type: KeyPress}
		if IsKittyProtocolActive() {
			event.Shift = true
		}
		return event
	case "\x1b":
		return KeyEvent{Key: KeyEscape, Raw: data, Type: KeyPress}
	case "\t":
		return KeyEvent{Key: KeyTab, Raw: data, Type: KeyPress}
	case "\x1b[Z":
		return KeyEvent{Key: KeyBacktab, Shift: true, Raw: data, Type: KeyPress}
	case "\x7f":
		return KeyEvent{Key: KeyBackspace, Raw: data, Type: KeyPress}
	case "\b":
		event := KeyEvent{Key: KeyBackspace, Raw: data, Type: KeyPress}
		if isLocalWindowsTerminal() {
			event.Ctrl = true
		}
		return event
	case "\x1b[3~":
		return KeyEvent{Key: KeyDelete, Raw: data, Type: KeyPress}
	case "\x1b[H", "\x1b[1~":
		return KeyEvent{Key: KeyHome, Raw: data, Type: KeyPress}
	case "\x1b[F", "\x1b[4~":
		return KeyEvent{Key: KeyEnd, Raw: data, Type: KeyPress}
	case " ":
		return KeyEvent{Key: "space", Rune: ' ', Raw: data, Type: KeyPress}
	case "\x00":
		return KeyEvent{Key: "space", Ctrl: true, Raw: data, Type: KeyPress}
	case "\x03":
		return KeyEvent{Key: "c", Ctrl: true, Raw: data, Type: KeyPress}
	case "\x04":
		return KeyEvent{Key: "d", Ctrl: true, Raw: data, Type: KeyPress}
	case "\x1c":
		return KeyEvent{Key: "\\", Ctrl: true, Raw: data, Type: KeyPress}
	case "\x1d":
		return KeyEvent{Key: "]", Ctrl: true, Raw: data, Type: KeyPress}
	case "\x1f":
		return KeyEvent{Key: "-", Ctrl: true, Raw: data, Type: KeyPress}
	}
	if len(data) == 1 {
		b := data[0]
		if b >= 1 && b <= 26 {
			return KeyEvent{Key: KeyID(string(rune('a' + b - 1))), Ctrl: true, Raw: data, Type: KeyPress}
		}
	}
	if strings.HasPrefix(data, "\x1b") && len(data) > 1 {
		if IsKittyProtocolActive() && data == "\x1b\r" {
			return KeyEvent{Key: KeyEnter, Shift: true, Raw: data, Type: KeyPress}
		}
		if data == "\x1b\b" || data == "\x1b\x7f" {
			return KeyEvent{Key: KeyBackspace, Alt: true, Raw: data, Type: KeyPress}
		}
		if IsKittyProtocolActive() {
			return unknownKey(data)
		}
		if data == "\x1b\x1b" {
			return KeyEvent{Key: "[", Ctrl: true, Alt: true, Raw: data, Type: KeyPress}
		}
		if event, ok := parseLegacyAltSequence(data); ok {
			return event
		}
		inner := ParseKey(data[1:])
		if inner.Unknown {
			return unknownKey(data)
		}
		inner.Alt = true
		inner.Raw = data
		return inner
	}
	r, n := utf8.DecodeRuneInString(data)
	if r != utf8.RuneError && n == len(data) {
		return KeyEvent{Key: KeyID(strings.ToLower(string(r))), Rune: r, Shift: isUpperASCII(r), Raw: data, Type: KeyPress}
	}
	return KeyEvent{Raw: data, Unknown: true, Type: KeyPress}
}

func normalizeC1KeySequence(data string) (string, bool) {
	code, prefixLen, ok := c1Prefix(data)
	if !ok {
		return "", false
	}
	switch code {
	case 0x8f:
		return "\x1bO" + data[prefixLen:], true
	case 0x9b:
		return "\x1b[" + data[prefixLen:], true
	default:
		return "", false
	}
}

func unknownKey(data string) KeyEvent {
	return KeyEvent{Raw: data, Unknown: true, Type: KeyPress}
}

func parseLegacyExactSequence(data string) (KeyEvent, bool) {
	event := KeyEvent{Raw: data, Type: KeyPress}
	switch data {
	case "\x1bOA":
		event.Key = KeyUp
	case "\x1bOB":
		event.Key = KeyDown
	case "\x1bOC":
		event.Key = KeyRight
	case "\x1bOD":
		event.Key = KeyLeft
	case "\x1bOH":
		event.Key = KeyHome
	case "\x1bOF":
		event.Key = KeyEnd
	case "\x1bOE":
		event.Key = "clear"
	case "\x1bOP":
		event.Key = "f1"
	case "\x1bOQ":
		event.Key = "f2"
	case "\x1bOR":
		event.Key = "f3"
	case "\x1bOS":
		event.Key = "f4"
	case "\x1bOM":
		event.Key = KeyEnter
	case "\x1b[a":
		event.Key = KeyUp
		event.Shift = true
	case "\x1b[b":
		event.Key = KeyDown
		event.Shift = true
	case "\x1b[c":
		event.Key = KeyRight
		event.Shift = true
	case "\x1b[d":
		event.Key = KeyLeft
		event.Shift = true
	case "\x1b[e":
		event.Key = "clear"
		event.Shift = true
	case "\x1bOa":
		event.Key = KeyUp
		event.Ctrl = true
	case "\x1bOb":
		event.Key = KeyDown
		event.Ctrl = true
	case "\x1bOc":
		event.Key = KeyRight
		event.Ctrl = true
	case "\x1bOd":
		event.Key = KeyLeft
		event.Ctrl = true
	case "\x1bOe":
		event.Key = "clear"
		event.Ctrl = true
	case "\x1b[2$":
		event.Key = "insert"
		event.Shift = true
	case "\x1b[2^":
		event.Key = "insert"
		event.Ctrl = true
	case "\x1b[3$":
		event.Key = KeyDelete
		event.Shift = true
	case "\x1b[3^":
		event.Key = KeyDelete
		event.Ctrl = true
	case "\x1b[5$":
		event.Key = "pageUp"
		event.Shift = true
	case "\x1b[5^":
		event.Key = "pageUp"
		event.Ctrl = true
	case "\x1b[6$":
		event.Key = "pageDown"
		event.Shift = true
	case "\x1b[6^":
		event.Key = "pageDown"
		event.Ctrl = true
	case "\x1b[7$":
		event.Key = KeyHome
		event.Shift = true
	case "\x1b[7^":
		event.Key = KeyHome
		event.Ctrl = true
	case "\x1b[8$":
		event.Key = KeyEnd
		event.Shift = true
	case "\x1b[8^":
		event.Key = KeyEnd
		event.Ctrl = true
	case "\x1b[[5~":
		event.Key = "pageUp"
	case "\x1b[[6~":
		event.Key = "pageDown"
	case "\x1b[[A":
		event.Key = "f1"
	case "\x1b[[B":
		event.Key = "f2"
	case "\x1b[[C":
		event.Key = "f3"
	case "\x1b[[D":
		event.Key = "f4"
	case "\x1b[[E":
		event.Key = "f5"
	case "\x1bb":
		event.Key = KeyLeft
		event.Alt = true
	case "\x1bf":
		event.Key = KeyRight
		event.Alt = true
	case "\x1bn":
		event.Key = KeyDown
		event.Alt = true
	case "\x1bp":
		event.Key = KeyUp
		event.Alt = true
	default:
		return KeyEvent{}, false
	}
	return event, true
}

func parseLegacyAltSequence(data string) (KeyEvent, bool) {
	event := KeyEvent{Raw: data, Alt: true, Type: KeyPress}
	switch data {
	case "\x1bB":
		event.Key = KeyLeft
	case "\x1bF":
		event.Key = KeyRight
	case "\x1bp":
		event.Key = KeyUp
	default:
		return KeyEvent{}, false
	}
	return event, true
}

func parseCSIKey(data string) (KeyEvent, bool) {
	if len(data) < 3 || !strings.HasPrefix(data, "\x1b[") {
		return KeyEvent{}, false
	}
	final := data[len(data)-1]
	body := data[2 : len(data)-1]
	parts := []string{}
	if body != "" {
		parts = strings.Split(body, ";")
	}
	modifiers, eventType := 1, 1
	if len(parts) >= 2 {
		modifiers, eventType = csiModifierAndEvent(parts[1])
	}
	var key KeyID
	switch final {
	case 'A':
		key = KeyUp
	case 'B':
		key = KeyDown
	case 'C':
		key = KeyRight
	case 'D':
		key = KeyLeft
	case 'H':
		key = KeyHome
	case 'F':
		key = KeyEnd
	case 'E':
		key = "clear"
	case '~':
		if len(parts) == 0 {
			return KeyEvent{}, false
		}
		code, codeEventType, ok := csiCodeAndEvent(parts[0])
		if !ok {
			return KeyEvent{}, false
		}
		if len(parts) < 2 {
			eventType = codeEventType
		}
		switch code {
		case 1, 7:
			key = KeyHome
		case 2:
			key = "insert"
		case 3:
			key = KeyDelete
		case 4, 8:
			key = KeyEnd
		case 5:
			key = "pageUp"
		case 6:
			key = "pageDown"
		case 11:
			key = "f1"
		case 12:
			key = "f2"
		case 13:
			key = "f3"
		case 14:
			key = "f4"
		case 15:
			key = "f5"
		case 17:
			key = "f6"
		case 18:
			key = "f7"
		case 19:
			key = "f8"
		case 20:
			key = "f9"
		case 21:
			key = "f10"
		case 23:
			key = "f11"
		case 24:
			key = "f12"
		default:
			return KeyEvent{}, false
		}
	default:
		return KeyEvent{}, false
	}
	event := KeyEvent{Key: key, Raw: data, Type: keyEventTypeFromCode(eventType)}
	applyModifierBits(&event, modifiers)
	return event, true
}

func parseCSIu(data string) (KeyEvent, bool) {
	body := strings.TrimSuffix(strings.TrimPrefix(data, "\x1b["), "u")
	parts := strings.Split(body, ";")
	if len(parts) == 0 {
		return KeyEvent{}, false
	}
	keyParts := strings.Split(parts[0], ":")
	code, err := strconv.Atoi(emptyZero(keyParts[0]))
	if err != nil {
		return KeyEvent{}, false
	}
	shifted := 0
	base := 0
	if len(keyParts) > 1 && keyParts[1] != "" {
		shifted, _ = strconv.Atoi(keyParts[1])
	}
	if len(keyParts) > 2 && keyParts[2] != "" {
		base, _ = strconv.Atoi(keyParts[2])
	}
	modifiers := 1
	if len(parts) >= 2 {
		modPart := strings.Split(parts[1], ":")
		modifiers, _ = strconv.Atoi(emptyZero(modPart[0]))
		if len(modPart) > 1 {
			eventType, _ := strconv.Atoi(emptyZero(modPart[1]))
			return keyEventFromCode(data, code, shifted, base, modifiers, eventType), true
		}
	}
	eventType := 1
	if len(parts) >= 3 {
		eventType, _ = strconv.Atoi(parts[2])
	}
	return keyEventFromCode(data, code, shifted, base, modifiers, eventType), true
}

func parseModifyOtherKeys(data string) (KeyEvent, bool) {
	body := strings.TrimSuffix(strings.TrimPrefix(data, "\x1b["), "~")
	parts := strings.Split(body, ";")
	if len(parts) != 3 || parts[0] != "27" {
		return KeyEvent{}, false
	}
	modifiers, err := strconv.Atoi(parts[1])
	if err != nil {
		return KeyEvent{}, false
	}
	code, err := strconv.Atoi(parts[2])
	if err != nil {
		return KeyEvent{}, false
	}
	return keyEventFromCode(data, code, 0, 0, modifiers, 1), true
}

func keyEventFromCode(data string, code, shifted, base, modifiers, eventType int) KeyEvent {
	if hasUnsupportedModifiers(modifiers) {
		return unknownKey(data)
	}
	keyCode := chooseLogicalCode(code, base)
	if shifted != 0 && keyCode == code {
		keyCode = code
	}
	key, r := keyIDFromCode(keyCode)
	effectiveModifiers := (modifiers - 1) &^ keyLockMask
	if shifted != 0 && effectiveModifiers&1 != 0 {
		if _, shiftedRune := keyIDFromCode(shifted); shiftedRune >= 32 {
			r = shiftedRune
		}
	}
	event := KeyEvent{Rune: r, Key: key, Raw: data, Type: KeyPress}
	if modifiers > 1 {
		applyModifierBits(&event, modifiers)
	}
	event.Type = keyEventTypeFromCode(eventType)
	return event
}

func keyEventTypeFromCode(eventType int) KeyEventType {
	switch eventType {
	case 2:
		return KeyRepeat
	case 3:
		return KeyRelease
	}
	return KeyPress
}

func hasUnsupportedModifiers(modifiers int) bool {
	if modifiers < 1 {
		return true
	}
	return (modifiers-1)&^(0x0f|keyLockMask) != 0
}

func csiModifierAndEvent(part string) (modifiers int, eventType int) {
	modifiers = 1
	eventType = 1
	fields := strings.Split(part, ":")
	if len(fields) > 0 {
		if value, err := strconv.Atoi(emptyZero(fields[0])); err == nil {
			modifiers = value
		}
	}
	if len(fields) > 1 {
		if value, err := strconv.Atoi(emptyZero(fields[1])); err == nil {
			eventType = value
		}
	}
	return modifiers, eventType
}

func csiCodeAndEvent(part string) (code int, eventType int, ok bool) {
	fields := strings.Split(part, ":")
	code, err := strconv.Atoi(emptyZero(fields[0]))
	if err != nil {
		return 0, 1, false
	}
	eventType = 1
	if len(fields) > 1 {
		if value, err := strconv.Atoi(emptyZero(fields[1])); err == nil {
			eventType = value
		}
	}
	return code, eventType, true
}

func applyModifierBits(event *KeyEvent, modifiers int) {
	if modifiers <= 1 {
		return
	}
	m := (modifiers - 1) &^ keyLockMask
	event.Shift = m&1 != 0
	event.Alt = m&2 != 0
	event.Ctrl = m&4 != 0
	event.Super = m&8 != 0
}

func chooseLogicalCode(code, base int) int {
	if mapped, ok := keypadCodeMap[code]; ok {
		return mapped
	}
	if base == 0 {
		return code
	}
	if isLatinOrSymbol(code) {
		return code
	}
	return base
}

func isLatinOrSymbol(code int) bool {
	return (code >= 0x20 && code <= 0x7e)
}

var keypadCodeMap = map[int]int{
	57399: '0',
	57400: '1',
	57401: '2',
	57402: '3',
	57403: '4',
	57404: '5',
	57405: '6',
	57406: '7',
	57407: '8',
	57408: '9',
	57409: '.',
	57410: '/',
	57411: '*',
	57412: '-',
	57413: '+',
	57414: '\r',
	57415: '=',
	57416: ',',
	57417: -1,
	57418: -2,
	57419: -3,
	57420: -4,
	57421: -5,
	57422: -6,
	57423: -7,
	57424: -8,
	57425: -9,
	57426: -10,
}

func keyIDFromCode(code int) (KeyID, rune) {
	switch code {
	case -1:
		return KeyLeft, 0
	case -2:
		return KeyRight, 0
	case -3:
		return KeyUp, 0
	case -4:
		return KeyDown, 0
	case -5:
		return "pageUp", 0
	case -6:
		return "pageDown", 0
	case -7:
		return KeyHome, 0
	case -8:
		return KeyEnd, 0
	case -9:
		return "insert", 0
	case -10:
		return KeyDelete, 0
	case 13:
		return KeyEnter, '\r'
	case 57414:
		return KeyEnter, 0
	case 9:
		return KeyTab, '\t'
	case 27:
		return KeyEscape, 0
	case 32:
		return "space", ' '
	case 127:
		return KeyBackspace, 0
	}
	r := rune(code)
	return KeyID(strings.ToLower(string(r))), r
}

func emptyZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func isUpperASCII(r rune) bool { return r >= 'A' && r <= 'Z' }

func MatchesKey(data, spec string) bool {
	event := ParseKey(data)
	if event.Unknown {
		return false
	}
	parts := strings.Split(strings.ToLower(spec), "+")
	key := parts[len(parts)-1]
	wantCtrl, wantAlt, wantShift, wantSuper := false, false, false, false
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "ctrl", "control":
			wantCtrl = true
		case "alt", "option", "meta":
			wantAlt = true
		case "shift":
			wantShift = true
		case "super", "cmd", "command":
			wantSuper = true
		}
	}
	switch key {
	case "return":
		key = "enter"
	case "esc":
		key = "escape"
	}
	if data == "\b" && key == "h" && wantCtrl && !wantAlt && !wantShift && !wantSuper {
		return true
	}
	if !IsKittyProtocolActive() && wantAlt && !wantCtrl && !wantShift && !wantSuper && len(key) == 1 {
		ch := key[0]
		if ((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) && data == "\x1b"+key {
			return true
		}
	}
	if event.Ctrl != wantCtrl || event.Alt != wantAlt || event.Shift != wantShift || event.Super != wantSuper {
		return false
	}
	if string(event.Key) == "-" && key == "_" && wantCtrl {
		key = "-"
	}
	return strings.EqualFold(string(event.Key), key) || (event.Rune != 0 && strings.EqualFold(string(event.Rune), key))
}

func isLocalWindowsTerminal() bool {
	if os.Getenv("WT_SESSION") == "" {
		return false
	}
	return os.Getenv("SSH_CONNECTION") == "" && os.Getenv("SSH_CLIENT") == "" && os.Getenv("SSH_TTY") == ""
}
