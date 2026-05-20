package gicodingagent

import "testing"

func TestTriggerCompactExtensionOnlyCompactsWhenCrossingThreshold(t *testing.T) {
	extension := NewTriggerCompactExtension(100_000)
	calls := 0
	compact := func() { calls++ }

	extension.OnTurnEnd(intPtr(110_000), compact)
	if calls != 0 {
		t.Fatalf("calls after first over-threshold turn = %d", calls)
	}
	extension.OnTurnEnd(intPtr(120_000), compact)
	if calls != 0 {
		t.Fatalf("calls after staying over threshold = %d", calls)
	}
	extension.OnTurnEnd(intPtr(95_000), compact)
	if calls != 0 {
		t.Fatalf("calls after dropping below threshold = %d", calls)
	}
	extension.OnTurnEnd(intPtr(105_000), compact)
	if calls != 1 {
		t.Fatalf("calls after crossing threshold = %d", calls)
	}
}

func intPtr(value int) *int {
	return &value
}
