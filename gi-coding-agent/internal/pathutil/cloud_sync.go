package pathutil

import (
	"context"
	"os/exec"
	"runtime"
	"time"
)

const DefaultCloudSyncMarkTimeout = 2 * time.Second

// CloudSyncMarkOptions isolates platform and process boundaries for
// MarkPathIgnoredByCloudSync.
type CloudSyncMarkOptions struct {
	GOOS       string
	Timeout    time.Duration
	RunCommand func(context.Context, string, []string) error
}

type cloudSyncAttributeCommand struct {
	name string
	args []string
}

// MarkPathIgnoredByCloudSync applies supported best-effort cloud-provider
// ignore attributes. Unsupported platforms and command failures are silent,
// matching the advisory nature of the metadata.
func MarkPathIgnoredByCloudSync(
	ctx context.Context,
	path string,
	options CloudSyncMarkOptions,
) {
	if path == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	goos := options.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultCloudSyncMarkTimeout
	}
	runCommand := options.RunCommand
	if runCommand == nil {
		runCommand = runCloudSyncAttributeCommand
	}
	markCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for _, command := range cloudSyncAttributeCommands(goos, path) {
		_ = runCommand(markCtx, command.name, command.args)
		if markCtx.Err() != nil {
			return
		}
	}
}

func cloudSyncAttributeCommands(
	goos string,
	path string,
) []cloudSyncAttributeCommand {
	switch goos {
	case "darwin":
		return []cloudSyncAttributeCommand{
			{
				name: "xattr",
				args: []string{
					"-w",
					"com.dropbox.ignored",
					"1",
					path,
				},
			},
			{
				name: "xattr",
				args: []string{
					"-w",
					"com.apple.fileprovider.ignore#P",
					"1",
					path,
				},
			},
		}
	case "linux":
		return []cloudSyncAttributeCommand{{
			name: "setfattr",
			args: []string{
				"-n",
				"user.com.dropbox.ignored",
				"-v",
				"1",
				path,
			},
		}}
	default:
		return nil
	}
}

func runCloudSyncAttributeCommand(
	ctx context.Context,
	name string,
	args []string,
) error {
	return exec.CommandContext(ctx, name, args...).Run()
}
