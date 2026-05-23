package gicodingagent

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type startupTimingEntry struct {
	Label string
	MS    int64
}

type startupTimings struct {
	enabled bool
	last    time.Time
	entries []startupTimingEntry
}

func newStartupTimingsFromEnv() *startupTimings {
	enabled := timingEnvEnabled(os.Getenv("GI_TIMING")) || timingEnvEnabled(os.Getenv("PI_TIMING"))
	return &startupTimings{enabled: enabled, last: time.Now()}
}

func timingEnvEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (t *startupTimings) Mark(label string) {
	if t == nil || !t.enabled {
		return
	}
	now := time.Now()
	t.entries = append(t.entries, startupTimingEntry{Label: label, MS: now.Sub(t.last).Milliseconds()})
	t.last = now
}

func (t *startupTimings) Print(writer io.Writer) {
	if t == nil || !t.enabled || len(t.entries) == 0 || writer == nil {
		return
	}
	var total int64
	_, _ = fmt.Fprintln(writer, "\n--- Startup Timings ---")
	for _, entry := range t.entries {
		total += entry.MS
		_, _ = fmt.Fprintf(writer, "  %s: %dms\n", entry.Label, entry.MS)
	}
	_, _ = fmt.Fprintf(writer, "  TOTAL: %dms\n", total)
	_, _ = fmt.Fprintln(writer, "------------------------")
}
