package pathutil

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMarkPathIgnoredByCloudSyncUsesDirectPlatformCommands(
	t *testing.T,
) {
	for _, tc := range []struct {
		name string
		goos string
		want []cloudSyncAttributeCommand
	}{
		{
			name: "darwin",
			goos: "darwin",
			want: []cloudSyncAttributeCommand{
				{
					name: "xattr",
					args: []string{
						"-w",
						"com.dropbox.ignored",
						"1",
						"/store",
					},
				},
				{
					name: "xattr",
					args: []string{
						"-w",
						"com.apple.fileprovider.ignore#P",
						"1",
						"/store",
					},
				},
			},
		},
		{
			name: "linux",
			goos: "linux",
			want: []cloudSyncAttributeCommand{{
				name: "setfattr",
				args: []string{
					"-n",
					"user.com.dropbox.ignored",
					"-v",
					"1",
					"/store",
				},
			}},
		},
		{name: "windows", goos: "windows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []cloudSyncAttributeCommand
			MarkPathIgnoredByCloudSync(
				context.Background(),
				"/store",
				CloudSyncMarkOptions{
					GOOS: tc.goos,
					RunCommand: func(
						_ context.Context,
						name string,
						args []string,
					) error {
						got = append(
							got,
							cloudSyncAttributeCommand{
								name: name,
								args: append(
									[]string(nil),
									args...,
								),
							},
						)
						return nil
					},
				},
			)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("commands = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestMarkPathIgnoredByCloudSyncContinuesAfterFailure(
	t *testing.T,
) {
	var calls int
	MarkPathIgnoredByCloudSync(
		context.Background(),
		"/store",
		CloudSyncMarkOptions{
			GOOS: "darwin",
			RunCommand: func(
				context.Context,
				string,
				[]string,
			) error {
				calls++
				return errors.New("attribute unsupported")
			},
		},
	)
	if calls != 2 {
		t.Fatalf("command calls = %d, want 2", calls)
	}
}
