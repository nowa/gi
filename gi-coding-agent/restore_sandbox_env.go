package gicodingagent

import (
	"os"
	"strings"
)

type SandboxEnvReader func() (string, error)

func RestoreSandboxEnv(isBun bool, env map[string]string, readEnviron SandboxEnvReader) bool {
	if !isBun || len(env) > 0 || readEnviron == nil {
		return false
	}
	data, err := readEnviron()
	if err != nil {
		return false
	}
	for _, entry := range strings.Split(data, "\x00") {
		index := strings.Index(entry, "=")
		if index <= 0 {
			continue
		}
		env[entry[:index]] = entry[index+1:]
	}
	return len(env) > 0
}

func RestoreProcessSandboxEnv(isBun bool) bool {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		index := strings.Index(entry, "=")
		if index > 0 {
			env[entry[:index]] = entry[index+1:]
		}
	}
	if len(env) > 0 {
		return false
	}
	restored := RestoreSandboxEnv(isBun, env, func() (string, error) {
		content, err := os.ReadFile("/proc/self/environ")
		return string(content), err
	})
	for key, value := range env {
		_ = os.Setenv(key, value)
	}
	return restored
}
