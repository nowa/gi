package gicodingagent

import startuptiming "github.com/nowa/gi/gi-coding-agent/internal/startuptiming"

type startupTimingEntry = startuptiming.Entry
type startupTimings = startuptiming.Timings

func newStartupTimingsFromEnv() *startupTimings {
	return startuptiming.NewFromEnv()
}

func timingEnvEnabled(value string) bool {
	return startuptiming.EnvEnabled(value)
}
