package browser

import (
	"errors"
	"reflect"
	"testing"
)

func TestCommandForPlatformNeverUsesShell(t *testing.T) {
	target := "https://example.test/callback?state=a&next=calc.exe"
	tests := []struct {
		platform string
		name     string
		args     []string
	}{
		{
			platform: "darwin",
			name:     "open",
			args:     []string{target},
		},
		{
			platform: "windows",
			name:     "rundll32",
			args: []string{
				"url.dll,FileProtocolHandler",
				target,
			},
		},
		{
			platform: "linux",
			name:     "xdg-open",
			args:     []string{target},
		},
	}
	for _, test := range tests {
		t.Run(test.platform, func(t *testing.T) {
			name, args := commandForPlatform(
				test.platform,
				target,
			)
			if name != test.name ||
				!reflect.DeepEqual(args, test.args) {
				t.Fatalf(
					"command = %q %#v, want %q %#v",
					name,
					args,
					test.name,
					test.args,
				)
			}
		})
	}
}

func TestOpenPassesTargetAsOneArgument(t *testing.T) {
	target := "https://example.test/a?x=1&y=2 | touch /tmp/pwned"
	var gotName string
	var gotArgs []string
	err := open(
		"windows",
		"  "+target+"  ",
		func(name string, args []string) error {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "rundll32" ||
		!reflect.DeepEqual(
			gotArgs,
			[]string{
				"url.dll,FileProtocolHandler",
				target,
			},
		) {
		t.Fatalf("command = %q %#v", gotName, gotArgs)
	}
}

func TestOpenHandlesEmptyTargetAndStartFailure(t *testing.T) {
	calls := 0
	if err := open(
		"linux",
		" \t ",
		func(string, []string) error {
			calls++
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("empty target started %d commands", calls)
	}

	startError := errors.New("launcher missing")
	err := open(
		"linux",
		"https://example.test",
		func(string, []string) error {
			return startError
		},
	)
	if !errors.Is(err, startError) {
		t.Fatalf("start error = %v", err)
	}
}
