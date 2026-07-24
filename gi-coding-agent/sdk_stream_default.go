package gicodingagent

import (
	core "github.com/nowa/gi/gi-agent-core"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func init() {
	// The coding-agent SDK is the provider-aware host for agent-core. Keep the
	// core package provider-neutral by installing its compatibility fallback
	// here, matching Pi's SDK boundary.
	core.SetDefaultStreamFn(llm.StreamSimple)
}
