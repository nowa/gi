package gicodingagent

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestBashTimeoutProtocolPreservesFractionalSeconds(t *testing.T) {
	definition := CreateBashToolDefinition(t.TempDir())
	if got := definition.Parameters.Properties["timeout"].Type; got != "number" {
		t.Fatalf("timeout schema type = %#v, want number", got)
	}

	input, err := parseBashToolDefinitionInput(map[string]any{
		"command": "echo ok",
		"timeout": 0.125,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !input.timeoutSet || input.Timeout != 0.125 {
		t.Fatalf("parsed timeout = %#v", input)
	}

	var remaining time.Duration
	tool := NewBashTool(t.TempDir(), BashToolOptions{Operations: BashOperations{
		Exec: func(_ string, _ string, options BashExecOptions) (BashOperationResult, error) {
			deadline, ok := options.Context.Deadline()
			if !ok {
				t.Fatal("fractional timeout did not reach execution context")
			}
			remaining = time.Until(deadline)
			return BashOperationResult{ExitCode: 0}, nil
		},
	}})
	if _, err := tool.Execute("fractional-timeout", input); err != nil {
		t.Fatal(err)
	}
	if remaining <= 0 || remaining > 125*time.Millisecond {
		t.Fatalf("execution deadline remaining = %s", remaining)
	}
}

func TestBashTimeoutProtocolRejectsExplicitInvalidValues(t *testing.T) {
	calls := 0
	options := BashToolOptions{Operations: BashOperations{
		Exec: func(_ string, _ string, _ BashExecOptions) (BashOperationResult, error) {
			calls++
			return BashOperationResult{ExitCode: 0}, nil
		},
	}}
	tool := NewBashTool(t.TempDir(), options)
	for _, timeout := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := tool.Execute("invalid-timeout", BashToolInput{
			Command: "echo no",
			Timeout: timeout,
		}); err == nil || !strings.Contains(err.Error(), "Invalid timeout") {
			t.Fatalf("timeout %v error = %v", timeout, err)
		}
	}

	definition := CreateBashToolDefinition(t.TempDir(), options)
	if _, err := definition.Execute("zero-timeout", map[string]any{
		"command": "echo no",
		"timeout": 0,
	}); err == nil || !strings.Contains(err.Error(), "Invalid timeout") {
		t.Fatalf("explicit zero timeout error = %v", err)
	}
	if _, err := definition.Execute("string-timeout", map[string]any{
		"command": "echo no",
		"timeout": "1",
	}); err == nil || err.Error() != "timeout must be a number" {
		t.Fatalf("string timeout error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("invalid timeout reached executor %d times", calls)
	}

	if _, _, err := optionalBashTimeout(map[string]any{"timeout": struct{}{}}); err == nil {
		t.Fatal("non-number timeout returned nil error")
	}
}
