package gicodingagent

import sandboxenv "github.com/nowa/gi/gi-coding-agent/internal/sandboxenv"

type SandboxEnvReader = sandboxenv.Reader

func RestoreSandboxEnv(isBun bool, env map[string]string, readEnviron SandboxEnvReader) bool {
	return sandboxenv.Restore(isBun, env, readEnviron)
}

func RestoreProcessSandboxEnv(isBun bool) bool {
	return sandboxenv.RestoreProcess(isBun)
}
