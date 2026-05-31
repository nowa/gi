package sandboxenv

import (
	"os"
	"strings"
)

type Reader func() (string, error)

func Restore(isBun bool, env map[string]string, readEnviron Reader) bool {
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

func RestoreProcess(isBun bool) bool {
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
	restored := Restore(isBun, env, func() (string, error) {
		content, err := os.ReadFile("/proc/self/environ")
		return string(content), err
	})
	for key, value := range env {
		_ = os.Setenv(key, value)
	}
	return restored
}
