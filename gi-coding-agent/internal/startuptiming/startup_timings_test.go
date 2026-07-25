package startuptiming

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintTimingGroupFiltersInvalidEntries(t *testing.T) {
	var output bytes.Buffer
	printTimingGroup(&output, "Startup Timings", []Entry{
		{Label: "parse args", MS: 4},
		{Label: "clock moved backwards", MS: -1},
		{Label: "dispatch", MS: 6},
	})

	got := output.String()
	for _, want := range []string{
		"--- Startup Timings ---",
		"  parse args: 4ms",
		"  dispatch: 6ms",
		"  TOTAL: 10ms",
		strings.Repeat("-", len("Startup Timings")+8),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "clock moved backwards") {
		t.Fatalf("output contains invalid timing: %q", got)
	}
}

func TestPrintTimingGroupSkipsEmptyGroups(t *testing.T) {
	var output bytes.Buffer
	printTimingGroup(&output, "Startup Timings", []Entry{
		{Label: "invalid", MS: -1},
	})
	if output.Len() != 0 {
		t.Fatalf("output = %q, want empty", output.String())
	}
}
