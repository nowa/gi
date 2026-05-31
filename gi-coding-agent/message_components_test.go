package gicodingagent

import (
	"reflect"
	"strings"
	"testing"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestAssistantMessageComponentOSC133Markers(t *testing.T) {
	t.Run("adds OSC 133 zone markers to assistant messages without tool calls", func(t *testing.T) {
		component := NewAssistantMessageComponent([]AssistantContentBlock{{Type: "text", Text: "hello"}})
		lines := component.Render(40)

		if len(lines) == 0 {
			t.Fatalf("lines should not be empty")
		}
		if !strings.Contains(lines[0], OSC133ZoneStart) {
			t.Fatalf("first line = %q", lines[0])
		}
		if !strings.HasPrefix(lines[len(lines)-1], OSC133ZoneEnd+OSC133ZoneFinal) {
			t.Fatalf("last line = %q", lines[len(lines)-1])
		}
	})

	t.Run("does not add OSC 133 zone markers when assistant message contains tool calls", func(t *testing.T) {
		component := NewAssistantMessageComponent([]AssistantContentBlock{
			{Type: "text", Text: "calling tool"},
			{Type: "toolCall", ID: "tool-1", Name: "read", Arguments: map[string]any{"path": "file.txt"}},
		})
		rendered := strings.Join(component.Render(60), "\n")

		for _, marker := range []string{OSC133ZoneStart, OSC133ZoneEnd, OSC133ZoneFinal} {
			if strings.Contains(rendered, marker) {
				t.Fatalf("rendered should not contain %q: %q", marker, rendered)
			}
		}
	})

	t.Run("renders hidden thinking through the Pi assistant message style", func(t *testing.T) {
		component := NewAssistantMessageComponent([]AssistantContentBlock{
			{Type: "thinking", Thinking: "private plan"},
			{Type: "text", Text: "final answer"},
		})
		component.HideThinkingBlock = true
		component.HiddenThinkingLabel = "Reasoning hidden"

		rendered := StripAnsi(strings.Join(component.Render(80), "\n"))
		if !strings.Contains(rendered, "Reasoning hidden") || !strings.Contains(rendered, "final answer") {
			t.Fatalf("rendered missing hidden label/text:\n%s", rendered)
		}
		if strings.Contains(rendered, "private plan") {
			t.Fatalf("rendered leaked hidden thinking:\n%s", rendered)
		}
	})

	t.Run("does not invent hidden thinking label for plain text", func(t *testing.T) {
		component := NewAssistantMessageComponent([]AssistantContentBlock{{Type: "text", Text: "final answer"}})
		component.HideThinkingBlock = true

		rendered := StripAnsi(strings.Join(component.Render(80), "\n"))
		if strings.Contains(rendered, "Thinking...") {
			t.Fatalf("plain assistant text should not show hidden thinking label:\n%s", rendered)
		}
		if !strings.Contains(rendered, "final answer") {
			t.Fatalf("plain assistant text missing:\n%s", rendered)
		}
	})

	t.Run("does not render empty thinking blocks", func(t *testing.T) {
		component := NewAssistantMessageComponent([]AssistantContentBlock{
			{Type: "thinking", Thinking: "   \n\t"},
			{Type: "text", Text: "final answer"},
		})
		component.HideThinkingBlock = true

		rendered := StripAnsi(strings.Join(component.Render(80), "\n"))
		if strings.Contains(rendered, "Thinking...") {
			t.Fatalf("empty thinking block should not show hidden thinking label:\n%s", rendered)
		}
		if !strings.Contains(rendered, "final answer") {
			t.Fatalf("assistant text missing:\n%s", rendered)
		}
	})

	t.Run("matches Pi spacing around thinking blocks", func(t *testing.T) {
		thinkingThenText := NewAssistantMessageComponent([]AssistantContentBlock{
			{Type: "thinking", Thinking: "private plan"},
			{Type: "text", Text: "final answer"},
		})
		thinkingThenText.HideThinkingBlock = true
		if got := normalizedAssistantRenderLines(thinkingThenText.Render(40)); !reflect.DeepEqual(got, []string{
			"",
			" Thinking...",
			"",
			" final answer",
		}) {
			t.Fatalf("thinking then text lines = %#v", got)
		}

		textThenThinking := NewAssistantMessageComponent([]AssistantContentBlock{
			{Type: "text", Text: "final answer"},
			{Type: "thinking", Thinking: "private plan"},
		})
		textThenThinking.HideThinkingBlock = true
		if got := normalizedAssistantRenderLines(textThenThinking.Render(40)); !reflect.DeepEqual(got, []string{
			"",
			" final answer",
			" Thinking...",
		}) {
			t.Fatalf("text then thinking lines = %#v", got)
		}
	})

	t.Run("does not render tool calls as assistant text", func(t *testing.T) {
		component := NewAssistantMessageComponent([]AssistantContentBlock{
			{Type: "toolCall", ID: "tool-1", Name: "read", Arguments: map[string]any{"path": "file.txt"}},
		})

		rendered := strings.Join(component.Render(80), "\n")
		if strings.Contains(rendered, "read") {
			t.Fatalf("tool call rendered as assistant text: %q", rendered)
		}
	})
}

func TestUserMessageComponentOSC133Markers(t *testing.T) {
	component := NewUserMessageComponent("hello")
	lines := component.Render(20)

	if len(lines) != 3 {
		t.Fatalf("line count = %d, lines=%#v", len(lines), lines)
	}
	if !strings.Contains(lines[0], OSC133ZoneStart) || !strings.HasSuffix(lines[0], TerminalBGReset) {
		t.Fatalf("first line = %q", lines[0])
	}
	if strings.Contains(lines[0], OSC133ZoneEnd) {
		t.Fatalf("first line should not contain closing marker: %q", lines[0])
	}
	if !strings.Contains(lines[1], "hello") {
		t.Fatalf("content line = %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], OSC133ZoneEnd+OSC133ZoneFinal) || !strings.HasSuffix(lines[2], TerminalBGReset) {
		t.Fatalf("closing line = %q", lines[2])
	}
}

func TestBorderedLoaderComponentMatchesPiStructureAndCancellation(t *testing.T) {
	t.Run("cancellable loader renders borders hint and aborts on cancel", func(t *testing.T) {
		loader := NewBorderedLoaderComponent(nil, "Creating gist...")
		aborted := false
		loader.SetOnAbort(func() { aborted = true })

		rendered := StripAnsi(strings.Join(loader.Render(48), "\n"))
		if strings.Count(rendered, "─") < 2 || !strings.Contains(rendered, "Creating gist...") || !strings.Contains(rendered, "cancel") {
			t.Fatalf("bordered loader render mismatch:\n%s", rendered)
		}

		loader.HandleInput("\x1b")
		if !loader.Cancelled() || !loader.Aborted() || loader.Signal().Err() == nil || !aborted {
			t.Fatalf("loader cancellation state = cancelled:%v aborted:%v err:%v callback:%v", loader.Cancelled(), loader.Aborted(), loader.Signal().Err(), aborted)
		}
		loader.Dispose()
	})

	t.Run("non-cancellable loader omits cancel hint and ignores escape", func(t *testing.T) {
		loader := NewNonCancellableBorderedLoaderComponent(nil, "Saving...")

		rendered := StripAnsi(strings.Join(loader.Render(48), "\n"))
		if strings.Count(rendered, "─") < 2 || !strings.Contains(rendered, "Saving...") {
			t.Fatalf("non-cancellable render mismatch:\n%s", rendered)
		}
		if strings.Contains(rendered, "cancel") {
			t.Fatalf("non-cancellable loader should not render cancel hint:\n%s", rendered)
		}

		loader.HandleInput("\x1b")
		if loader.Cancelled() || loader.Aborted() || loader.Signal().Err() != nil {
			t.Fatalf("non-cancellable loader should ignore escape: cancelled:%v aborted:%v err:%v", loader.Cancelled(), loader.Aborted(), loader.Signal().Err())
		}
		loader.Dispose()
	})
}

func normalizedAssistantRenderLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimRight(StripAnsi(line), " "))
	}
	return out
}

func TestCLIEarendilAnnouncementComponentEmbedsPiImageAsset(t *testing.T) {
	if len(cliEarendilClankolasPNG) == 0 {
		t.Fatal("clankolas image asset was not embedded")
	}
	dims, err := gitui.GetImageDimensions(cliEarendilClankolasPNG)
	if err != nil {
		t.Fatal(err)
	}
	if dims.Width != 640 || dims.Height != 537 {
		t.Fatalf("clankolas dimensions = %dx%d, want 640x537", dims.Width, dims.Height)
	}

	gitui.SetCapabilities(gitui.TerminalCapabilities{Images: false, Protocol: gitui.ImageProtocolNone})
	defer gitui.ResetCapabilitiesCache()
	rendered := StripAnsi(strings.Join(newCLIEarendilAnnouncementComponent().Render(80), "\n"))
	for _, expected := range []string{
		"gi has joined Earendil",
		cliEarendilBlogURL,
		"[Image: clankolas.png [image/png] 640x537]",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("announcement missing %q:\n%s", expected, rendered)
		}
	}
}

func TestCLIDaxnutsComponentPiStyleImageAndFinalText(t *testing.T) {
	if got, want := len(cliDaxnutsHex), cliDaxnutsImageWidth*cliDaxnutsImageHeight*6; got != want {
		t.Fatalf("daxnuts hex length = %d, want %d", got, want)
	}
	component := &cliDaxnutsComponent{
		image:       buildCLIDaxnutsImage(),
		tick:        25,
		maxTicks:    25,
		cachedWidth: -1,
		cachedTick:  -1,
	}
	lines := component.Render(100)
	rendered := strings.Join(lines, "\n")
	plain := StripAnsi(rendered)

	for _, expected := range []string{
		"▄",
		"Free Kimi K2.5 via OpenCode Zen",
		"\"Powered by daxnuts\"",
		"— @thdxr",
		"Try OpenCode",
		"https://mistral.ai/news/mistral-vibe-2-0",
	} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("rendered missing %q:\n%s", expected, plain)
		}
	}
	for _, expected := range []string{"\x1b[38;2;", "\x1b[48;2;"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered missing truecolor code %q", expected)
		}
	}
	for index, line := range lines {
		if width := gitui.VisibleWidth(line); width > 100 {
			t.Fatalf("line %d width = %d, want <= 100", index, width)
		}
	}
}
