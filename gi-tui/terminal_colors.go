package gitui

import (
	"context"
	"math/big"
	"strconv"
	"strings"
)

const (
	osc11BackgroundColorQuery       = "\x1b]11;?\x07"
	terminalColorSchemeQuery        = "\x1b[?996n"
	terminalColorNotificationsOn    = "\x1b[?2031h"
	terminalColorNotificationsOff   = "\x1b[?2031l"
	terminalColorSchemeReportPrefix = "\x1b[?997;"
)

// RGBColor is an 8-bit terminal color.
type RGBColor struct {
	R int
	G int
	B int
}

// TerminalColorScheme is a terminal-reported preference.
type TerminalColorScheme string

const (
	TerminalColorSchemeDark  TerminalColorScheme = "dark"
	TerminalColorSchemeLight TerminalColorScheme = "light"
)

type terminalBackgroundColorResult struct {
	color RGBColor
	ok    bool
}

type pendingTerminalBackgroundQuery struct {
	result chan terminalBackgroundColorResult
	active bool
}

type terminalColorSchemeListenerEntry struct {
	id       int
	listener func(TerminalColorScheme)
}

func hexToRGB(hex string) (RGBColor, bool) {
	normalized := strings.TrimPrefix(hex, "#")
	if len(normalized) != 6 || !isASCIIHex(normalized) {
		return RGBColor{}, false
	}
	value, err := strconv.ParseUint(normalized, 16, 32)
	if err != nil {
		return RGBColor{}, false
	}
	return RGBColor{
		R: int(value >> 16),
		G: int((value >> 8) & 0xff),
		B: int(value & 0xff),
	}, true
}

func parseOSCHexChannel(channel string) (int, bool) {
	if channel == "" || !isASCIIHex(channel) {
		return 0, false
	}
	value, ok := new(big.Int).SetString(channel, 16)
	if !ok {
		return 0, false
	}
	maxValue := new(big.Int).Lsh(big.NewInt(1), uint(4*len(channel)))
	maxValue.Sub(maxValue, big.NewInt(1))
	if maxValue.Sign() <= 0 {
		return 0, false
	}

	scaled := new(big.Int).Mul(value, big.NewInt(255))
	// Match Math.round by adding one half of the positive denominator before
	// integer division.
	scaled.Add(scaled, new(big.Int).Quo(new(big.Int).Set(maxValue), big.NewInt(2)))
	scaled.Quo(scaled, maxValue)
	return int(scaled.Int64()), true
}

func isASCIIHex(value string) bool {
	for _, char := range value {
		if !((char >= '0' && char <= '9') ||
			(char >= 'a' && char <= 'f') ||
			(char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return value != ""
}

// IsOSC11BackgroundColorResponse reports whether data is one complete, strict
// OSC 11 background-color response. The payload itself may still be invalid.
func IsOSC11BackgroundColorResponse(data string) bool {
	if !strings.HasPrefix(data, "\x1b]11;") {
		return false
	}
	payload, ok := trimTerminalStringTerminator(strings.TrimPrefix(data, "\x1b]11;"))
	if !ok {
		return false
	}
	return !strings.ContainsAny(payload, "\x07\x1b")
}

func trimTerminalStringTerminator(data string) (string, bool) {
	switch {
	case strings.HasSuffix(data, "\x07"):
		return strings.TrimSuffix(data, "\x07"), true
	case strings.HasSuffix(data, "\x1b\\"):
		return strings.TrimSuffix(data, "\x1b\\"), true
	default:
		return "", false
	}
}

// ParseOSC11BackgroundColor parses rgb:, rgba:, #RRGGBB, and #RRRRGGGGBBBB
// terminal replies into 8-bit channels.
func ParseOSC11BackgroundColor(data string) (RGBColor, bool) {
	if !IsOSC11BackgroundColorResponse(data) {
		return RGBColor{}, false
	}
	payload, _ := trimTerminalStringTerminator(strings.TrimPrefix(data, "\x1b]11;"))
	value := strings.TrimSpace(payload)
	if strings.HasPrefix(value, "#") {
		switch len(value) {
		case 7:
			return hexToRGB(value)
		case 13:
			red, okRed := parseOSCHexChannel(value[1:5])
			green, okGreen := parseOSCHexChannel(value[5:9])
			blue, okBlue := parseOSCHexChannel(value[9:13])
			if !okRed || !okGreen || !okBlue {
				return RGBColor{}, false
			}
			return RGBColor{R: red, G: green, B: blue}, true
		default:
			return RGBColor{}, false
		}
	}

	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "rgba:"):
		value = value[len("rgba:"):]
	case strings.HasPrefix(lower, "rgb:"):
		value = value[len("rgb:"):]
	}
	channels := strings.Split(value, "/")
	if len(channels) < 3 {
		return RGBColor{}, false
	}
	red, okRed := parseOSCHexChannel(channels[0])
	green, okGreen := parseOSCHexChannel(channels[1])
	blue, okBlue := parseOSCHexChannel(channels[2])
	if !okRed || !okGreen || !okBlue {
		return RGBColor{}, false
	}
	return RGBColor{R: red, G: green, B: blue}, true
}

// ParseTerminalColorSchemeReport parses the color-palette notification report
// emitted in response to CSI ? 996 n.
func ParseTerminalColorSchemeReport(data string) (TerminalColorScheme, bool) {
	if !strings.HasPrefix(data, terminalColorSchemeReportPrefix) || !strings.HasSuffix(data, "n") {
		return "", false
	}
	switch strings.TrimSuffix(strings.TrimPrefix(data, terminalColorSchemeReportPrefix), "n") {
	case "1":
		return TerminalColorSchemeDark, true
	case "2":
		return TerminalColorSchemeLight, true
	default:
		return "", false
	}
}

// OnTerminalColorSchemeChange registers a listener and returns an idempotent
// unsubscribe function. Listener callbacks run without the TUI state lock held.
func (t *TUI) OnTerminalColorSchemeChange(listener func(TerminalColorScheme)) func() {
	if listener == nil {
		return func() {}
	}
	t.mu.Lock()
	t.nextTerminalColorSchemeListenerID++
	id := t.nextTerminalColorSchemeListenerID
	t.terminalColorSchemeListeners = append(t.terminalColorSchemeListeners, terminalColorSchemeListenerEntry{
		id:       id,
		listener: listener,
	})
	t.mu.Unlock()

	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		for index, entry := range t.terminalColorSchemeListeners {
			if entry.id == id {
				t.terminalColorSchemeListeners = append(
					t.terminalColorSchemeListeners[:index],
					t.terminalColorSchemeListeners[index+1:]...,
				)
				return
			}
		}
	}
}

// SetTerminalColorSchemeNotifications enables or disables terminal palette
// change reports. The desired state is retained across TUI restarts.
func (t *TUI) SetTerminalColorSchemeNotifications(enabled bool) error {
	t.mu.Lock()
	if t.terminalColorSchemeNotificationsEnabled == enabled {
		t.mu.Unlock()
		return nil
	}
	t.terminalColorSchemeNotificationsEnabled = enabled
	running := !t.stopped
	terminal := t.terminal
	t.mu.Unlock()

	if !running {
		return nil
	}
	if enabled {
		return terminal.Write(terminalColorNotificationsOn)
	}
	return terminal.Write(terminalColorNotificationsOff)
}

// QueryTerminalBackgroundColor writes an OSC 11 query and waits for its FIFO
// reply. A strict but unparseable response returns ok=false with no error.
func (t *TUI) QueryTerminalBackgroundColor(ctx context.Context) (color RGBColor, ok bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	query := &pendingTerminalBackgroundQuery{
		result: make(chan terminalBackgroundColorResult, 1),
		active: true,
	}

	t.mu.Lock()
	t.pendingTerminalBackgroundQueries = append(t.pendingTerminalBackgroundQueries, query)
	t.pendingTerminalBackgroundReplies++
	t.mu.Unlock()

	if err := t.terminal.Write(osc11BackgroundColorQuery); err != nil {
		t.removePendingTerminalBackgroundQuery(query)
		return RGBColor{}, false, err
	}

	select {
	case result := <-query.result:
		return result.color, result.ok, nil
	case <-ctx.Done():
		t.mu.Lock()
		query.active = false
		t.mu.Unlock()
		return RGBColor{}, false, ctx.Err()
	}
}

func (t *TUI) removePendingTerminalBackgroundQuery(query *pendingTerminalBackgroundQuery) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for index, candidate := range t.pendingTerminalBackgroundQueries {
		if candidate != query {
			continue
		}
		t.pendingTerminalBackgroundQueries = append(
			t.pendingTerminalBackgroundQueries[:index],
			t.pendingTerminalBackgroundQueries[index+1:]...,
		)
		t.pendingTerminalBackgroundReplies = max(0, t.pendingTerminalBackgroundReplies-1)
		query.active = false
		return
	}
}

func (t *TUI) consumeOSC11BackgroundResponse(data string) bool {
	t.mu.Lock()
	if t.pendingTerminalBackgroundReplies <= 0 || !IsOSC11BackgroundColorResponse(data) {
		t.mu.Unlock()
		return false
	}
	t.pendingTerminalBackgroundReplies--
	var query *pendingTerminalBackgroundQuery
	if len(t.pendingTerminalBackgroundQueries) > 0 {
		query = t.pendingTerminalBackgroundQueries[0]
		t.pendingTerminalBackgroundQueries = t.pendingTerminalBackgroundQueries[1:]
	}
	deliver := query != nil && query.active
	if deliver {
		query.active = false
	}
	t.mu.Unlock()

	if deliver {
		color, ok := ParseOSC11BackgroundColor(data)
		query.result <- terminalBackgroundColorResult{color: color, ok: ok}
	}
	return true
}

func (t *TUI) consumeTerminalColorSchemeReport(data string) bool {
	scheme, ok := ParseTerminalColorSchemeReport(data)
	if !ok {
		return false
	}
	t.mu.Lock()
	listeners := make([]func(TerminalColorScheme), 0, len(t.terminalColorSchemeListeners))
	for _, entry := range t.terminalColorSchemeListeners {
		listeners = append(listeners, entry.listener)
	}
	t.mu.Unlock()
	for _, listener := range listeners {
		listener(scheme)
	}
	return true
}

// QueryTerminalColorScheme writes the palette-preference query and waits for
// the next valid scheme report.
func (t *TUI) QueryTerminalColorScheme(ctx context.Context) (TerminalColorScheme, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(chan TerminalColorScheme, 1)
	unsubscribe := t.OnTerminalColorSchemeChange(func(scheme TerminalColorScheme) {
		select {
		case result <- scheme:
		default:
		}
	})
	defer unsubscribe()

	if err := t.terminal.Write(terminalColorSchemeQuery); err != nil {
		return "", false, err
	}
	select {
	case scheme := <-result:
		return scheme, true, nil
	case <-ctx.Done():
		return "", false, ctx.Err()
	}
}
