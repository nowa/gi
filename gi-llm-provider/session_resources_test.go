package gillmprovider

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSessionResourceCleanupRegistryPiParity(t *testing.T) {
	resetSessionResourceCleanupsForTest()
	defer resetSessionResourceCleanupsForTest()

	var calls []string
	unregisterFirst := RegisterSessionResourceCleanup(func(sessionID string) error {
		calls = append(calls, "first:"+sessionID)
		return nil
	})
	RegisterSessionResourceCleanup(func(sessionID string) error {
		calls = append(calls, "second:"+sessionID)
		return nil
	})

	if err := CleanupSessionResources("session-1"); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if !sameStringSet(calls, []string{"first:session-1", "second:session-1"}) {
		t.Fatalf("calls = %#v", calls)
	}

	calls = nil
	unregisterFirst()
	if err := CleanupSessionResources("session-2"); err != nil {
		t.Fatalf("cleanup after unregister: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"second:session-2"}) {
		t.Fatalf("calls after unregister = %#v", calls)
	}
}

func TestCleanupSessionResourcesAggregatesErrors(t *testing.T) {
	resetSessionResourceCleanupsForTest()
	defer resetSessionResourceCleanupsForTest()

	firstErr := errors.New("first failed")
	secondErr := errors.New("second failed")
	RegisterSessionResourceCleanup(func(string) error { return firstErr })
	RegisterSessionResourceCleanup(func(string) error { return secondErr })

	err := CleanupSessionResources("")
	if err == nil {
		t.Fatal("cleanup returned nil error")
	}
	if !IsSessionResourceCleanupError(err) {
		t.Fatalf("error type = %T", err)
	}
	if !strings.Contains(err.Error(), "first failed") || !strings.Contains(err.Error(), "second failed") {
		t.Fatalf("error text = %q", err.Error())
	}
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("aggregate does not unwrap cleanup errors: %v", err)
	}
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := map[string]int{}
	for _, item := range left {
		seen[item]++
	}
	for _, item := range right {
		seen[item]--
		if seen[item] < 0 {
			return false
		}
	}
	return true
}
