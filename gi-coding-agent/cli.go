package gicodingagent

import (
	"fmt"
	"io"
)

type CLIOptions struct {
	Args    []string
	Stdout  io.Writer
	Stderr  io.Writer
	Startup func(stderr io.Writer) error
}

func RunCLI(options CLIOptions) int {
	args := ParseArgs(options.Args)
	if options.Startup != nil {
		if err := options.Startup(nonNilWriter(options.Stderr)); err != nil {
			writeCLIError(options.Stderr, err.Error())
			return 1
		}
	}
	if args.Help {
		WriteCLIUsage(cliHelpWriter(args, options))
		return 0
	}
	if args.Version {
		_, _ = fmt.Fprintln(nonNilWriter(options.Stdout), "gi")
		return 0
	}
	return 0
}

func WriteCLIUsage(writer io.Writer) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprintln(writer, "Usage: gi [options] [message]")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "Options:")
	_, _ = fmt.Fprintln(writer, "  -p, --print        Run in non-interactive print mode")
	_, _ = fmt.Fprintln(writer, "      --mode <mode>  Output mode: text, json, or rpc")
	_, _ = fmt.Fprintln(writer, "  -h, --help         Show help")
}

func cliHelpWriter(args Args, options CLIOptions) io.Writer {
	if args.Mode == ModeJSON || args.Print {
		return nonNilWriter(options.Stderr)
	}
	return nonNilWriter(options.Stdout)
}

func writeCLIError(writer io.Writer, message string) {
	if writer == nil || message == "" {
		return
	}
	_, _ = fmt.Fprintln(writer, message)
}

func nonNilWriter(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}
	return io.Discard
}
