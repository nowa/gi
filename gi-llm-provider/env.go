package gillmprovider

import (
	"os"

	"github.com/nowa/gi/gi-llm-provider/internal/envkeys"
)

func FindEnvKeys(provider string) []string {
	return envkeys.FindEnvKeys(provider)
}

// FindEnvKeysWithOverrides reports provider API-key variables available from
// request-scoped overrides or the ambient process environment.
func FindEnvKeysWithOverrides(provider string, env ProviderEnv) []string {
	return envkeys.FindEnvKeysWithLookup(provider, func(name string) string {
		return GetProviderEnvValue(name, env)
	})
}

func GetEnvAPIKey(provider string) string {
	return envkeys.GetEnvAPIKey(provider)
}

// GetProviderEnvValue resolves a request-scoped provider override before the
// ambient process environment. Empty overrides intentionally fall through.
func GetProviderEnvValue(name string, env ProviderEnv) string {
	if value := env[name]; value != "" {
		return value
	}
	return os.Getenv(name)
}

// GetEnvAPIKeyWithOverrides resolves provider authentication from request
// overrides before consulting the ambient process environment.
func GetEnvAPIKeyWithOverrides(provider string, env ProviderEnv) string {
	return envkeys.ResolveAPIKey(provider, func(name string) string {
		return GetProviderEnvValue(name, env)
	})
}
