package gicodingagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

const (
	tuiResetAll = "\x1b[0m"
	tuiResetFG  = "\x1b[39m"
	tuiResetBG  = "\x1b[49m"
)

var tuiDarkThemeFG = map[string]string{
	"accent":             "\x1b[38;2;138;190;183m",
	"border":             "\x1b[38;2;95;135;255m",
	"borderAccent":       "\x1b[38;2;0;215;255m",
	"borderMuted":        "\x1b[38;2;80;80;80m",
	"success":            "\x1b[38;2;181;189;104m",
	"error":              "\x1b[38;2;204;102;102m",
	"warning":            "\x1b[38;2;255;255;0m",
	"muted":              "\x1b[38;2;128;128;128m",
	"dim":                "\x1b[38;2;102;102;102m",
	"text":               "\x1b[38;2;212;212;212m",
	"thinkingText":       "\x1b[38;2;128;128;128m",
	"userMessageText":    "\x1b[38;2;212;212;212m",
	"customMessageText":  "\x1b[38;2;212;212;212m",
	"customMessageLabel": "\x1b[38;2;149;117;205m",
	"toolTitle":          "\x1b[38;2;212;212;212m",
	"toolOutput":         "\x1b[38;2;128;128;128m",
	"mdHeading":          "\x1b[38;2;240;198;116m",
	"mdLink":             "\x1b[38;2;129;162;190m",
	"mdLinkUrl":          "\x1b[38;2;102;102;102m",
	"mdCode":             "\x1b[38;2;138;190;183m",
	"mdCodeBlock":        "\x1b[38;2;181;189;104m",
	"mdCodeBlockBorder":  "\x1b[38;2;128;128;128m",
	"mdQuote":            "\x1b[38;2;128;128;128m",
	"mdQuoteBorder":      "\x1b[38;2;128;128;128m",
	"mdHr":               "\x1b[38;2;128;128;128m",
	"mdListBullet":       "\x1b[38;2;138;190;183m",
	"toolDiffAdded":      "\x1b[38;2;181;189;104m",
	"toolDiffRemoved":    "\x1b[38;2;204;102;102m",
	"toolDiffContext":    "\x1b[38;2;128;128;128m",
	"syntaxComment":      "\x1b[38;2;106;153;85m",
	"syntaxKeyword":      "\x1b[38;2;86;156;214m",
	"syntaxFunction":     "\x1b[38;2;220;220;170m",
	"syntaxVariable":     "\x1b[38;2;156;220;254m",
	"syntaxString":       "\x1b[38;2;206;145;120m",
	"syntaxNumber":       "\x1b[38;2;181;206;168m",
	"syntaxType":         "\x1b[38;2;78;201;176m",
	"syntaxOperator":     "\x1b[38;2;212;212;212m",
	"syntaxPunctuation":  "\x1b[38;2;212;212;212m",
	"thinkingOff":        "\x1b[38;2;80;80;80m",
	"thinkingMinimal":    "\x1b[38;2;110;110;110m",
	"thinkingLow":        "\x1b[38;2;95;135;175m",
	"thinkingMedium":     "\x1b[38;2;129;162;190m",
	"thinkingHigh":       "\x1b[38;2;178;148;187m",
	"thinkingXhigh":      "\x1b[38;2;209;131;232m",
	"bashMode":           "\x1b[38;2;181;189;104m",
}

var tuiDarkThemeBG = map[string]string{
	"selectedBg":      "\x1b[48;2;58;58;74m",
	"userMessageBg":   "\x1b[48;2;52;53;65m",
	"customMessageBg": "\x1b[48;2;45;40;56m",
	"toolPendingBg":   "\x1b[48;2;40;40;50m",
	"toolSuccessBg":   "\x1b[48;2;40;50;40m",
	"toolErrorBg":     "\x1b[48;2;60;40;40m",
}

var tuiLightThemeFG = map[string]string{
	"accent":             "\x1b[38;2;90;128;128m",
	"border":             "\x1b[38;2;84;125;167m",
	"borderAccent":       "\x1b[38;2;90;128;128m",
	"borderMuted":        "\x1b[38;2;176;176;176m",
	"success":            "\x1b[38;2;88;132;88m",
	"error":              "\x1b[38;2;170;85;85m",
	"warning":            "\x1b[38;2;154;115;38m",
	"muted":              "\x1b[38;2;108;108;108m",
	"dim":                "\x1b[38;2;118;118;118m",
	"text":               "\x1b[38;2;31;35;40m",
	"thinkingText":       "\x1b[38;2;108;108;108m",
	"userMessageText":    "\x1b[38;2;31;35;40m",
	"customMessageText":  "\x1b[38;2;31;35;40m",
	"customMessageLabel": "\x1b[38;2;126;87;194m",
	"toolTitle":          "\x1b[38;2;31;35;40m",
	"toolOutput":         "\x1b[38;2;108;108;108m",
	"mdHeading":          "\x1b[38;2;154;115;38m",
	"mdLink":             "\x1b[38;2;84;125;167m",
	"mdLinkUrl":          "\x1b[38;2;118;118;118m",
	"mdCode":             "\x1b[38;2;90;128;128m",
	"mdCodeBlock":        "\x1b[38;2;88;132;88m",
	"mdCodeBlockBorder":  "\x1b[38;2;108;108;108m",
	"mdQuote":            "\x1b[38;2;108;108;108m",
	"mdQuoteBorder":      "\x1b[38;2;108;108;108m",
	"mdHr":               "\x1b[38;2;108;108;108m",
	"mdListBullet":       "\x1b[38;2;88;132;88m",
	"toolDiffAdded":      "\x1b[38;2;88;132;88m",
	"toolDiffRemoved":    "\x1b[38;2;170;85;85m",
	"toolDiffContext":    "\x1b[38;2;108;108;108m",
	"syntaxComment":      "\x1b[38;2;0;128;0m",
	"syntaxKeyword":      "\x1b[38;2;0;0;255m",
	"syntaxFunction":     "\x1b[38;2;121;94;38m",
	"syntaxVariable":     "\x1b[38;2;0;16;128m",
	"syntaxString":       "\x1b[38;2;163;21;21m",
	"syntaxNumber":       "\x1b[38;2;9;134;88m",
	"syntaxType":         "\x1b[38;2;38;127;153m",
	"syntaxOperator":     "\x1b[38;2;0;0;0m",
	"syntaxPunctuation":  "\x1b[38;2;0;0;0m",
	"thinkingOff":        "\x1b[38;2;176;176;176m",
	"thinkingMinimal":    "\x1b[38;2;118;118;118m",
	"thinkingLow":        "\x1b[38;2;84;125;167m",
	"thinkingMedium":     "\x1b[38;2;90;128;128m",
	"thinkingHigh":       "\x1b[38;2;135;95;135m",
	"thinkingXhigh":      "\x1b[38;2;139;0;139m",
	"bashMode":           "\x1b[38;2;88;132;88m",
}

var tuiLightThemeBG = map[string]string{
	"selectedBg":      "\x1b[48;2;208;208;224m",
	"userMessageBg":   "\x1b[48;2;232;232;232m",
	"customMessageBg": "\x1b[48;2;237;231;246m",
	"toolPendingBg":   "\x1b[48;2;232;232;240m",
	"toolSuccessBg":   "\x1b[48;2;232;240;232m",
	"toolErrorBg":     "\x1b[48;2;240;232;232m",
}

type tuiThemePalette struct {
	name string
	fg   map[string]string
	bg   map[string]string
}

var (
	tuiThemeMu     sync.RWMutex
	activeTUITheme = tuiBuiltinThemePalette("dark", tuiDarkThemeFG, tuiDarkThemeBG)
)

func tuiCloneStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func tuiActiveThemeSnapshot() tuiThemePalette {
	tuiThemeMu.RLock()
	defer tuiThemeMu.RUnlock()
	return tuiThemePalette{
		name: activeTUITheme.name,
		fg:   tuiCloneStringMap(activeTUITheme.fg),
		bg:   tuiCloneStringMap(activeTUITheme.bg),
	}
}

func tuiSetActiveThemePalette(theme tuiThemePalette) {
	if theme.fg == nil {
		theme.fg = map[string]string{}
	}
	if theme.bg == nil {
		theme.bg = map[string]string{}
	}
	tuiThemeMu.Lock()
	activeTUITheme = tuiThemePalette{
		name: theme.name,
		fg:   tuiCloneStringMap(theme.fg),
		bg:   tuiCloneStringMap(theme.bg),
	}
	tuiThemeMu.Unlock()
}

func tuiSetActiveTheme(name string, available []TUIThemeInfo) error {
	theme, err := tuiLoadThemePalette(name, available)
	if err != nil {
		return err
	}
	tuiSetActiveThemePalette(theme)
	return nil
}

func tuiLoadThemePalette(name string, available []TUIThemeInfo) (tuiThemePalette, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "system" {
		name = tuiDefaultThemeName()
	}
	switch name {
	case "dark":
		return tuiBuiltinThemePalette("dark", tuiDarkThemeFG, tuiDarkThemeBG), nil
	case "light":
		return tuiBuiltinThemePalette("light", tuiLightThemeFG, tuiLightThemeBG), nil
	}
	for _, theme := range available {
		if theme.Name == name && strings.TrimSpace(theme.Path) != "" {
			return tuiLoadThemePaletteFromPath(name, theme.Path)
		}
	}
	return tuiThemePalette{}, fmt.Errorf("theme not found: %s", name)
}

func tuiDefaultThemeName() string {
	colorfgbg := os.Getenv("COLORFGBG")
	if colorfgbg != "" {
		parts := strings.Split(colorfgbg, ";")
		if len(parts) >= 2 {
			if bg, err := strconv.Atoi(parts[1]); err == nil && bg >= 8 {
				return "light"
			}
		}
	}
	return "dark"
}

type tuiColorMode string

const (
	tuiColorModeTruecolor tuiColorMode = "truecolor"
	tuiColorMode256       tuiColorMode = "256color"
)

func tuiDetectColorMode() tuiColorMode {
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return tuiColorModeTruecolor
	}
	if os.Getenv("WT_SESSION") != "" {
		return tuiColorModeTruecolor
	}
	term := os.Getenv("TERM")
	switch {
	case term == "" || term == "dumb" || term == "linux":
		return tuiColorMode256
	case os.Getenv("TERM_PROGRAM") == "Apple_Terminal":
		return tuiColorMode256
	case term == "screen" || strings.HasPrefix(term, "screen-") || strings.HasPrefix(term, "screen."):
		return tuiColorMode256
	default:
		return tuiColorModeTruecolor
	}
}

func tuiBuiltinThemePalette(name string, fg, bg map[string]string) tuiThemePalette {
	if tuiDetectColorMode() == tuiColorModeTruecolor {
		return tuiThemePalette{name: name, fg: tuiCloneStringMap(fg), bg: tuiCloneStringMap(bg)}
	}
	return tuiThemePalette{
		name: name,
		fg:   tuiConvertTruecolorANSIMap(fg, true),
		bg:   tuiConvertTruecolorANSIMap(bg, false),
	}
}

func tuiConvertTruecolorANSIMap(input map[string]string, foreground bool) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		if converted, ok := tuiTruecolorANSITo256(value, foreground); ok {
			output[key] = converted
			continue
		}
		output[key] = value
	}
	return output
}

func tuiTruecolorANSITo256(value string, foreground bool) (string, bool) {
	prefix := "\x1b[38;2;"
	format := "\x1b[38;5;%dm"
	if !foreground {
		prefix = "\x1b[48;2;"
		format = "\x1b[48;5;%dm"
	}
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, "m") {
		return "", false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(value, prefix), "m"), ";")
	if len(parts) != 3 {
		return "", false
	}
	r, errR := strconv.Atoi(parts[0])
	g, errG := strconv.Atoi(parts[1])
	b, errB := strconv.Atoi(parts[2])
	if errR != nil || errG != nil || errB != nil {
		return "", false
	}
	return fmt.Sprintf(format, tuiRGBTo256(r, g, b)), true
}

func tuiLoadThemePaletteFromPath(name, path string) (tuiThemePalette, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return tuiThemePalette{}, err
	}
	var theme themeFile
	if err := json.Unmarshal(content, &theme); err != nil {
		return tuiThemePalette{}, err
	}
	if strings.TrimSpace(theme.Name) == "" {
		return tuiThemePalette{}, errors.New("theme name is required")
	}
	fg := make(map[string]string, len(tuiDarkThemeFG))
	for token := range tuiDarkThemeFG {
		value, ok := theme.Colors[token]
		if !ok {
			return tuiThemePalette{}, fmt.Errorf("theme %q missing color token %q", name, token)
		}
		ansi, err := tuiThemeANSI(value, theme.Vars, true, map[string]bool{})
		if err != nil {
			return tuiThemePalette{}, fmt.Errorf("theme %q color %q: %w", name, token, err)
		}
		fg[token] = ansi
	}
	bg := make(map[string]string, len(tuiDarkThemeBG))
	for token := range tuiDarkThemeBG {
		value, ok := theme.Colors[token]
		if !ok {
			return tuiThemePalette{}, fmt.Errorf("theme %q missing background token %q", name, token)
		}
		ansi, err := tuiThemeANSI(value, theme.Vars, false, map[string]bool{})
		if err != nil {
			return tuiThemePalette{}, fmt.Errorf("theme %q background %q: %w", name, token, err)
		}
		bg[token] = ansi
	}
	return tuiThemePalette{name: theme.Name, fg: fg, bg: bg}, nil
}

func tuiThemeANSI(value any, vars map[string]any, foreground bool, seen map[string]bool) (string, error) {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return "", nil
		}
		if strings.HasPrefix(typed, "#") {
			r, g, b, err := tuiHexToRGB(typed)
			if err != nil {
				return "", err
			}
			if tuiDetectColorMode() == tuiColorMode256 {
				if foreground {
					return fmt.Sprintf("\x1b[38;5;%dm", tuiRGBTo256(r, g, b)), nil
				}
				return fmt.Sprintf("\x1b[48;5;%dm", tuiRGBTo256(r, g, b)), nil
			}
			if foreground {
				return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b), nil
			}
			return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b), nil
		}
		if seen[typed] {
			return "", fmt.Errorf("circular variable reference %q", typed)
		}
		next, ok := vars[typed]
		if !ok {
			return "", fmt.Errorf("unknown variable %q", typed)
		}
		seen[typed] = true
		return tuiThemeANSI(next, vars, foreground, seen)
	case float64:
		if typed != float64(int(typed)) {
			return "", fmt.Errorf("color index must be an integer: %v", typed)
		}
		return tuiThemeIndexedANSI(int(typed), foreground)
	case int:
		return tuiThemeIndexedANSI(typed, foreground)
	default:
		return "", fmt.Errorf("unsupported color value %T", value)
	}
}

func tuiThemeIndexedANSI(index int, foreground bool) (string, error) {
	if index < 0 || index > 255 {
		return "", fmt.Errorf("color index out of range: %d", index)
	}
	if foreground {
		return fmt.Sprintf("\x1b[38;5;%dm", index), nil
	}
	return fmt.Sprintf("\x1b[48;5;%dm", index), nil
}

func tuiHexToRGB(hex string) (int, int, int, error) {
	cleaned := strings.TrimPrefix(hex, "#")
	if len(cleaned) != 6 {
		return 0, 0, 0, fmt.Errorf("invalid hex color %q", hex)
	}
	value, err := strconv.ParseUint(cleaned, 16, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid hex color %q", hex)
	}
	return int(value >> 16 & 0xff), int(value >> 8 & 0xff), int(value & 0xff), nil
}

var (
	tuiColorCubeValues = []int{0, 95, 135, 175, 215, 255}
	tuiGrayRampValues  = []int{8, 18, 28, 38, 48, 58, 68, 78, 88, 98, 108, 118, 128, 138, 148, 158, 168, 178, 188, 198, 208, 218, 228, 238}
)

func tuiRGBTo256(r, g, b int) int {
	rIndex := tuiClosestColorCubeIndex(r)
	gIndex := tuiClosestColorCubeIndex(g)
	bIndex := tuiClosestColorCubeIndex(b)
	cubeR := tuiColorCubeValues[rIndex]
	cubeG := tuiColorCubeValues[gIndex]
	cubeB := tuiColorCubeValues[bIndex]
	cubeIndex := 16 + 36*rIndex + 6*gIndex + bIndex
	cubeDistance := tuiColorDistance(r, g, b, cubeR, cubeG, cubeB)

	gray := int(0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b) + 0.5)
	grayIndex := tuiClosestGrayRampIndex(gray)
	grayValue := tuiGrayRampValues[grayIndex]
	grayDistance := tuiColorDistance(r, g, b, grayValue, grayValue, grayValue)

	spread := max(r, max(g, b)) - min(r, min(g, b))
	if spread < 10 && grayDistance < cubeDistance {
		return 232 + grayIndex
	}
	return cubeIndex
}

func tuiClosestColorCubeIndex(value int) int {
	bestIndex := 0
	bestDistance := absInt(value - tuiColorCubeValues[0])
	for index := 1; index < len(tuiColorCubeValues); index++ {
		distance := absInt(value - tuiColorCubeValues[index])
		if distance < bestDistance {
			bestDistance = distance
			bestIndex = index
		}
	}
	return bestIndex
}

func tuiClosestGrayRampIndex(gray int) int {
	bestIndex := 0
	bestDistance := absInt(gray - tuiGrayRampValues[0])
	for index := 1; index < len(tuiGrayRampValues); index++ {
		distance := absInt(gray - tuiGrayRampValues[index])
		if distance < bestDistance {
			bestDistance = distance
			bestIndex = index
		}
	}
	return bestIndex
}

func tuiColorDistance(r1, g1, b1, r2, g2, b2 int) float64 {
	dr := float64(r1 - r2)
	dg := float64(g1 - g2)
	db := float64(b1 - b2)
	return dr*dr*0.299 + dg*dg*0.587 + db*db*0.114
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func tuiThemeFG(color, text string) string {
	if text == "" {
		return text
	}
	tuiThemeMu.RLock()
	prefix, ok := activeTUITheme.fg[color]
	tuiThemeMu.RUnlock()
	if !ok {
		return text
	}
	if prefix == "" {
		prefix = tuiResetFG
	}
	return prefix + text + tuiResetFG
}

func tuiThemeBG(color, text string) string {
	if text == "" {
		return text
	}
	tuiThemeMu.RLock()
	prefix, ok := activeTUITheme.bg[color]
	tuiThemeMu.RUnlock()
	if !ok {
		return text
	}
	if prefix == "" {
		prefix = tuiResetBG
	}
	return prefix + text + tuiResetBG
}

func tuiThemeBold(text string) string {
	if text == "" {
		return text
	}
	return "\x1b[1m" + text + tuiResetAll
}

func tuiThemeBoldAccent(text string) string {
	if text == "" {
		return text
	}
	tuiThemeMu.RLock()
	prefix, ok := activeTUITheme.fg["accent"]
	tuiThemeMu.RUnlock()
	if !ok {
		return tuiThemeBold(text)
	}
	if prefix == "" {
		prefix = tuiResetFG
	}
	return "\x1b[1m" + prefix + text + tuiResetAll
}

func tuiThemeDim(text string) string {
	return tuiThemeFG("dim", text)
}

func tuiThemeMuted(text string) string {
	return tuiThemeFG("muted", text)
}

func tuiThemeAccent(text string) string {
	return tuiThemeFG("accent", text)
}

func tuiThemeWarning(text string) string {
	return tuiThemeFG("warning", text)
}

func tuiThemeError(text string) string {
	return tuiThemeFG("error", text)
}

func tuiThemeSuccess(text string) string {
	return tuiThemeFG("success", text)
}

func tuiThemeBorder(text string) string {
	return tuiThemeFG("border", text)
}

func tuiThemeBorderMuted(text string) string {
	return tuiThemeFG("borderMuted", text)
}

func tuiThemeBashMode(text string) string {
	return tuiThemeFG("bashMode", text)
}

func tuiThemeToolTitle(text string) string {
	return tuiThemeFG("toolTitle", text)
}

func tuiThemeToolOutput(text string) string {
	return tuiThemeFG("toolOutput", text)
}

func tuiThemeThinkingBorder(level string) func(string) string {
	switch level {
	case "minimal":
		return func(text string) string { return tuiThemeFG("thinkingMinimal", text) }
	case "low":
		return func(text string) string { return tuiThemeFG("thinkingLow", text) }
	case "medium":
		return func(text string) string { return tuiThemeFG("thinkingMedium", text) }
	case "high":
		return func(text string) string { return tuiThemeFG("thinkingHigh", text) }
	case "xhigh":
		return func(text string) string { return tuiThemeFG("thinkingXhigh", text) }
	default:
		return func(text string) string { return tuiThemeFG("thinkingOff", text) }
	}
}

func tuiThemeKeyHint(key, description string) string {
	if description == "" {
		return tuiThemeDim(key)
	}
	return tuiThemeDim(key) + tuiThemeMuted(" "+description)
}

func tuiThemeSelectList() gitui.SelectListTheme {
	return gitui.SelectListTheme{
		SelectedPrefix: tuiThemeAccent,
		SelectedText:   tuiThemeAccent,
		Description:    tuiThemeMuted,
		ScrollInfo:     tuiThemeMuted,
		NoMatch:        tuiThemeMuted,
	}
}

func tuiThemeEditor() gitui.EditorTheme {
	return gitui.EditorTheme{
		Border:     tuiThemeBorderMuted,
		SelectList: tuiThemeSelectList(),
	}
}

func tuiThemeSettingsList() gitui.SettingsListTheme {
	return gitui.SettingsListTheme{
		Label: func(text string, selected bool) string {
			if selected {
				return tuiThemeAccent(text)
			}
			return text
		},
		CurrentValue: func(text string, selected bool) string {
			if selected {
				return tuiThemeAccent(text)
			}
			return tuiThemeMuted(text)
		},
		Description: tuiThemeDim,
		Hint:        tuiThemeDim,
		Selected:    tuiThemeAccent,
		Value:       tuiThemeMuted,
		Cursor:      tuiThemeAccent("→ "),
	}
}

func tuiThemeMarkdown() gitui.MarkdownTheme {
	return gitui.MarkdownTheme{
		Heading:         func(text string) string { return tuiThemeFG("mdHeading", text) },
		Link:            func(text string) string { return tuiThemeFG("mdLink", text) },
		LinkURL:         func(text string) string { return tuiThemeFG("mdLinkUrl", text) },
		Code:            func(text string) string { return tuiThemeFG("mdCode", text) },
		CodeBlock:       func(text string) string { return tuiThemeFG("mdCodeBlock", text) },
		CodeBlockBorder: func(text string) string { return tuiThemeFG("mdCodeBlockBorder", text) },
		Quote:           func(text string) string { return tuiThemeFG("mdQuote", text) },
		QuoteBorder:     func(text string) string { return tuiThemeFG("mdQuoteBorder", text) },
		HR:              func(text string) string { return tuiThemeFG("mdHr", text) },
		ListBullet:      func(text string) string { return tuiThemeFG("mdListBullet", text) },
		Bold:            tuiThemeBold,
		Italic:          func(text string) string { return "\x1b[3m" + text + "\x1b[23m" },
		Underline:       func(text string) string { return "\x1b[4m" + text + "\x1b[24m" },
		Strikethrough:   func(text string) string { return "\x1b[9m" + text + "\x1b[29m" },
	}
}
