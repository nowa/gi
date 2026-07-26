package gicodingagent

import (
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestCLICustomMessageComponentProvidesOutputPaddingToRenderersAndUpdatesIt(
	t *testing.T,
) {
	var optionsSeen []map[string]any
	renderer := func(_ any, options any) []string {
		optionMap, _ := options.(map[string]any)
		snapshot := make(map[string]any, len(optionMap))
		for key, value := range optionMap {
			snapshot[key] = value
		}
		optionsSeen = append(optionsSeen, snapshot)
		padding, _ := optionMap["outputPad"].(int)
		return []string{
			strings.Repeat(" ", padding) + "custom",
		}
	}
	component := newCLICustomMessageComponent(
		llm.Message{
			Role:       "custom",
			CustomType: "test",
			Content:    []llm.ContentPart{llm.Text("custom")},
		},
		renderer,
		1,
	)

	rendered := component.Render(40)
	if len(rendered) != 1 || !strings.HasPrefix(rendered[0], " custom") {
		t.Fatalf("padded render = %#v", rendered)
	}
	if len(optionsSeen) != 1 ||
		optionsSeen[0]["expanded"] != false ||
		optionsSeen[0]["outputPad"] != 1 ||
		optionsSeen[0]["width"] != 40 {
		t.Fatalf("initial options = %#v", optionsSeen)
	}

	component.SetOutputPad(0)
	rendered = component.Render(40)
	if len(rendered) != 1 || !strings.HasPrefix(rendered[0], "custom") {
		t.Fatalf("unpadded render = %#v", rendered)
	}
	if len(optionsSeen) != 2 ||
		optionsSeen[1]["expanded"] != false ||
		optionsSeen[1]["outputPad"] != 0 {
		t.Fatalf("updated options = %#v", optionsSeen)
	}

	component.SetExpanded(true)
	_ = component.Render(40)
	if optionsSeen[2]["expanded"] != true ||
		optionsSeen[2]["outputPad"] != 0 {
		t.Fatalf("expanded options = %#v", optionsSeen)
	}
}
