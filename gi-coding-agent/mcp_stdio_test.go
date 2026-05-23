package gicodingagent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunMCPStdioRequestTimeoutIncludesStderr(t *testing.T) {
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = runMCPStdioListTools(mcpStdioOptions{
		Command: []string{helper, "-test.run=TestRunMCPStdioTimeoutHelper"},
		Env:     map[string]string{"GI_MCP_TIMEOUT_HELPER": "1"},
		Timeout: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	message := err.Error()
	if !strings.Contains(message, context.DeadlineExceeded.Error()) || !strings.Contains(message, "mcp timeout helper waiting") {
		t.Fatalf("timeout error = %q", message)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestRunMCPStdioTimeoutHelper(t *testing.T) {
	if os.Getenv("GI_MCP_TIMEOUT_HELPER") != "1" {
		return
	}
	_, _ = os.Stderr.WriteString("mcp timeout helper waiting\n")
	for {
		time.Sleep(time.Hour)
	}
}
