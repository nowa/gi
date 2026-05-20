package gicodingagent

import (
	"os"
	"path/filepath"
)

func resolveCLICWDAndAgentDir(options CLIOptions) (string, string, error) {
	cwd := options.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", "", err
		}
	}
	agentDir := options.AgentDir
	if agentDir == "" {
		agentDir = defaultCLIAgentDir(cwd)
	}
	return cwd, agentDir, nil
}

func newCLIReadOnlyAuthStorage(agentDir string) *AuthStorage {
	content, err := os.ReadFile(filepath.Join(agentDir, "auth.json"))
	if err != nil {
		return NewInMemoryAuthStorage(nil)
	}
	data, err := parseAuthStorageData(string(content))
	if err != nil {
		return NewInMemoryAuthStorage(nil)
	}
	return NewInMemoryAuthStorage(data)
}

func newCLIModelRegistry(options CLIOptions, writeAuth bool) (*ModelRegistry, string, string, error) {
	cwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		return nil, "", "", err
	}
	if options.ModelRegistry != nil {
		return options.ModelRegistry, cwd, agentDir, nil
	}
	authStorage := newCLIReadOnlyAuthStorage(agentDir)
	if writeAuth {
		authStorage = NewAuthStorage(filepath.Join(agentDir, "auth.json"))
	}
	return NewModelRegistry(authStorage, filepath.Join(agentDir, "models.json")), cwd, agentDir, nil
}
