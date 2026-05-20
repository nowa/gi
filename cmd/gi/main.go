package main

import (
	"os"

	gicodingagent "github.com/nowa/gi/gi-coding-agent"
)

func main() {
	os.Setenv("GI_CODING_AGENT", "true")
	os.Exit(gicodingagent.RunCLI(gicodingagent.CLIOptions{
		Args:        os.Args[1:],
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		PackageName: gicodingagent.DefaultCodingAgentPackageName,
		Version:     gicodingagent.DefaultCodingAgentVersion,
	}))
}
