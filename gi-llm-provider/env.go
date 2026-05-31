package gillmprovider

import "github.com/nowa/gi/gi-llm-provider/internal/envkeys"

func FindEnvKeys(provider string) []string {
	return envkeys.FindEnvKeys(provider)
}

func GetEnvAPIKey(provider string) string {
	return envkeys.GetEnvAPIKey(provider)
}
