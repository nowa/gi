package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/tooloutput"

const defaultBashOutputLineLimit = tooloutput.DefaultLineLimit

type bashOutputAccumulatorOptions = tooloutput.AccumulatorOptions
type bashOutputSnapshot = tooloutput.Snapshot
type bashOutputAccumulator = tooloutput.Accumulator

func newBashOutputAccumulator(options bashOutputAccumulatorOptions) *bashOutputAccumulator {
	return tooloutput.NewAccumulator(options)
}

func sanitizeBashOutputBytes(data []byte) string {
	return tooloutput.SanitizeBytes(data)
}
