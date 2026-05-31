package startuptiming

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Entry struct {
	Label string
	MS    int64
}

type Timings struct {
	enabled bool
	last    time.Time
	entries []Entry
}

func NewFromEnv() *Timings {
	enabled := EnvEnabled(os.Getenv("GI_TIMING")) || EnvEnabled(os.Getenv("PI_TIMING"))
	return &Timings{enabled: enabled, last: time.Now()}
}

func EnvEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (t *Timings) Mark(label string) {
	if t == nil || !t.enabled {
		return
	}
	now := time.Now()
	t.entries = append(t.entries, Entry{Label: label, MS: now.Sub(t.last).Milliseconds()})
	t.last = now
}

func (t *Timings) Print(writer io.Writer) {
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
