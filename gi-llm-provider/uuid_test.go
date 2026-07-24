package gillmprovider

import (
	"regexp"
	"testing"
)

func TestUUIDv7UsesRFC9562LayoutAndPreservesMonotonicOrder(t *testing.T) {
	randomValues := [][]byte{
		{0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xfe, 0x01, 0x11, 0x22, 0x33, 0x44, 0x55},
		make([]byte, 16),
		make([]byte, 16),
	}
	randomBytes := func(bytes []byte) error {
		copy(bytes, randomValues[0])
		randomValues = randomValues[1:]
		return nil
	}
	generator := NewUUIDv7Generator(randomBytes, func() int64 { return 0x0123456789ab })

	first := generator.Generate()
	second := generator.Generate()
	third := generator.Generate()

	if first != "01234567-89ab-7fff-bfff-f91122334455" {
		t.Fatalf("first = %s", first)
	}
	if second != "01234567-89ab-7fff-bfff-fc0000000000" {
		t.Fatalf("second = %s", second)
	}
	if third != "01234567-89ac-7000-8000-000000000000" {
		t.Fatalf("third = %s", third)
	}
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(first) || !pattern.MatchString(second) || !pattern.MatchString(third) {
		t.Fatalf("UUIDs do not match v7 layout: %s %s %s", first, second, third)
	}
	if !(first < second && second < third) {
		t.Fatalf("UUIDs are not monotonic: %s %s %s", first, second, third)
	}
}
