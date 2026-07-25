package gicodingagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/nowa/gi/gi-coding-agent/internal/shellconfig"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type configValuePart struct {
	literal string
	envName string
}

type configValueReference struct {
	command string
	parts   []configValuePart
}

type configValueCacheEntry struct {
	ready chan struct{}
	value string
	ok    bool
}

var (
	configValueCacheMu sync.Mutex
	configValueCache   = map[string]*configValueCacheEntry{}
)

// ConfigValueEnvVarName returns the environment variable name when config is
// exactly one $NAME or ${NAME} reference.
func ConfigValueEnvVarName(config string) (string, bool) {
	reference := parseConfigValueReference(config)
	if reference.command != "" || len(reference.parts) != 1 || reference.parts[0].envName == "" {
		return "", false
	}
	return reference.parts[0].envName, true
}

// ConfigValueEnvVarNames returns referenced environment variables in first-use
// order with duplicates removed.
func ConfigValueEnvVarNames(config string) []string {
	reference := parseConfigValueReference(config)
	if reference.command != "" {
		return nil
	}
	return templateEnvVarNames(reference.parts)
}

// MissingConfigValueEnvVarNames reports template variables that cannot be
// resolved. Provider-scoped values take precedence over process environment.
func MissingConfigValueEnvVarNames(config string, env llm.ProviderEnv) []string {
	names := ConfigValueEnvVarNames(config)
	missing := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := resolveEnvConfigValue(name, env); !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

// IsCommandConfigValue reports whether config requests shell command
// execution.
func IsCommandConfigValue(config string) bool {
	return strings.HasPrefix(config, "!")
}

// IsConfigValueConfigured reports whether all environment references required
// by config are available. Commands and literals are considered configured
// without executing them.
func IsConfigValueConfigured(config string, env llm.ProviderEnv) bool {
	return len(MissingConfigValueEnvVarNames(config, env)) == 0
}

// ResolveConfigValue resolves config with process environment and caches shell
// command results for the process lifetime.
func ResolveConfigValue(config string) (string, bool) {
	return ResolveConfigValueWithEnv(config, nil)
}

// ResolveConfigValueWithEnv resolves commands, environment templates, escapes,
// and literals. Credential-scoped env values take precedence over process
// environment. For backward compatibility, a bare environment-variable name
// is also resolved when present.
func ResolveConfigValueWithEnv(config string, env llm.ProviderEnv) (string, bool) {
	reference := parseConfigValueReference(config)
	if reference.command != "" {
		return resolveCommandConfigValue(reference.command)
	}
	if value, ok := resolveLegacyBareEnv(config, env); ok {
		return value, true
	}
	return resolveConfigValueTemplate(reference.parts, env)
}

// ResolveConfigValueUncached resolves config without caching shell command
// results.
func ResolveConfigValueUncached(config string) (string, bool) {
	return ResolveConfigValueUncachedWithEnv(config, nil)
}

// ResolveConfigValueUncachedWithEnv is ResolveConfigValueWithEnv without the
// process-lifetime command cache.
func ResolveConfigValueUncachedWithEnv(config string, env llm.ProviderEnv) (string, bool) {
	reference := parseConfigValueReference(config)
	if reference.command != "" {
		return executeConfigCommand(strings.TrimPrefix(reference.command, "!"))
	}
	if value, ok := resolveLegacyBareEnv(config, env); ok {
		return value, true
	}
	return resolveConfigValueTemplate(reference.parts, env)
}

// ResolveConfigValueOrError resolves a config value and describes the missing
// command or environment dependency when resolution fails.
func ResolveConfigValueOrError(config, description string, env llm.ProviderEnv) (string, error) {
	if value, ok := ResolveConfigValueUncachedWithEnv(config, env); ok {
		return value, nil
	}
	if IsCommandConfigValue(config) {
		return "", fmt.Errorf(
			"Failed to resolve %s from shell command: %s",
			description,
			strings.TrimPrefix(config, "!"),
		)
	}
	switch missing := MissingConfigValueEnvVarNames(config, env); len(missing) {
	case 1:
		return "", fmt.Errorf(
			"Failed to resolve %s from environment variable: %s",
			description,
			missing[0],
		)
	case 2:
		return "", fmt.Errorf(
			"Failed to resolve %s from environment variables: %s",
			description,
			strings.Join(missing, ", "),
		)
	default:
		if len(missing) > 2 {
			return "", fmt.Errorf(
				"Failed to resolve %s from environment variables: %s",
				description,
				strings.Join(missing, ", "),
			)
		}
		return "", fmt.Errorf("Failed to resolve %s", description)
	}
}

// ResolveConfigHeaders resolves header values and omits unresolved or empty
// entries.
func ResolveConfigHeaders(headers map[string]string, env llm.ProviderEnv) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	resolved := make(map[string]string, len(headers))
	for name, config := range headers {
		if value, ok := ResolveConfigValueWithEnv(config, env); ok && value != "" {
			resolved[name] = value
		}
	}
	if len(resolved) == 0 {
		return nil
	}
	return resolved
}

// ResolveConfigHeadersOrError resolves every header or returns a contextual
// error for the first unresolved value.
func ResolveConfigHeadersOrError(
	headers map[string]string,
	description string,
	env llm.ProviderEnv,
) (map[string]string, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	resolved := make(map[string]string, len(headers))
	for name, config := range headers {
		value, err := ResolveConfigValueOrError(
			config,
			fmt.Sprintf(`%s header %q`, description, name),
			env,
		)
		if err != nil {
			return nil, err
		}
		resolved[name] = value
	}
	return resolved, nil
}

// ClearConfigValueCache clears cached command successes and failures.
func ClearConfigValueCache() {
	configValueCacheMu.Lock()
	defer configValueCacheMu.Unlock()
	clear(configValueCache)
}

func parseConfigValueReference(config string) configValueReference {
	if strings.HasPrefix(config, "!") {
		return configValueReference{command: config}
	}
	return configValueReference{parts: parseConfigValueTemplate(config)}
}

func parseConfigValueTemplate(config string) []configValuePart {
	parts := make([]configValuePart, 0, 3)
	for index := 0; index < len(config); {
		relativeDollar := strings.IndexByte(config[index:], '$')
		if relativeDollar < 0 {
			parts = appendConfigLiteral(parts, config[index:])
			break
		}
		dollar := index + relativeDollar
		parts = appendConfigLiteral(parts, config[index:dollar])
		if dollar+1 >= len(config) {
			parts = appendConfigLiteral(parts, "$")
			break
		}

		next := config[dollar+1]
		switch next {
		case '$', '!':
			parts = appendConfigLiteral(parts, string(next))
			index = dollar + 2
		case '{':
			closeOffset := strings.IndexByte(config[dollar+2:], '}')
			if closeOffset < 0 {
				parts = appendConfigLiteral(parts, "$")
				index = dollar + 1
				continue
			}
			closeIndex := dollar + 2 + closeOffset
			name := config[dollar+2 : closeIndex]
			if isEnvVarName(name) {
				parts = append(parts, configValuePart{envName: name})
			} else {
				parts = appendConfigLiteral(parts, config[dollar:closeIndex+1])
			}
			index = closeIndex + 1
		default:
			nameLength := envVarNamePrefixLength(config[dollar+1:])
			if nameLength == 0 {
				parts = appendConfigLiteral(parts, "$")
				index = dollar + 1
				continue
			}
			parts = append(parts, configValuePart{
				envName: config[dollar+1 : dollar+1+nameLength],
			})
			index = dollar + 1 + nameLength
		}
	}
	return parts
}

func appendConfigLiteral(parts []configValuePart, literal string) []configValuePart {
	if literal == "" {
		return parts
	}
	if len(parts) > 0 && parts[len(parts)-1].envName == "" {
		parts[len(parts)-1].literal += literal
		return parts
	}
	return append(parts, configValuePart{literal: literal})
}

func templateEnvVarNames(parts []configValuePart) []string {
	names := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if part.envName == "" {
			continue
		}
		if _, ok := seen[part.envName]; ok {
			continue
		}
		seen[part.envName] = struct{}{}
		names = append(names, part.envName)
	}
	return names
}

func resolveConfigValueTemplate(parts []configValuePart, env llm.ProviderEnv) (string, bool) {
	var resolved strings.Builder
	for _, part := range parts {
		if part.envName == "" {
			resolved.WriteString(part.literal)
			continue
		}
		value, ok := resolveEnvConfigValue(part.envName, env)
		if !ok {
			return "", false
		}
		resolved.WriteString(value)
	}
	return resolved.String(), true
}

func resolveEnvConfigValue(name string, env llm.ProviderEnv) (string, bool) {
	if value := env[name]; value != "" {
		return value, true
	}
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

func resolveLegacyBareEnv(config string, env llm.ProviderEnv) (string, bool) {
	if !isEnvVarName(config) {
		return "", false
	}
	return resolveEnvConfigValue(config, env)
}

func isEnvVarName(value string) bool {
	return envVarNamePrefixLength(value) == len(value) && value != ""
}

func envVarNamePrefixLength(value string) int {
	if value == "" || !isEnvVarNameStart(value[0]) {
		return 0
	}
	index := 1
	for index < len(value) && isEnvVarNameContinue(value[index]) {
		index++
	}
	return index
}

func isEnvVarNameStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isEnvVarNameContinue(value byte) bool {
	return isEnvVarNameStart(value) || value >= '0' && value <= '9'
}

func resolveCommandConfigValue(commandConfig string) (string, bool) {
	configValueCacheMu.Lock()
	if cached, ok := configValueCache[commandConfig]; ok {
		configValueCacheMu.Unlock()
		<-cached.ready
		return cached.value, cached.ok
	}
	entry := &configValueCacheEntry{ready: make(chan struct{})}
	configValueCache[commandConfig] = entry
	configValueCacheMu.Unlock()

	entry.value, entry.ok = executeConfigCommand(strings.TrimPrefix(commandConfig, "!"))
	close(entry.ready)
	return entry.value, entry.ok
}

func executeConfigCommand(command string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := shellconfig.Resolve(
		"",
		shellconfig.ResolveOptions{},
	)
	if err != nil {
		return "", false
	}
	invocation := config.Invocation(command)
	cmd := exec.CommandContext(
		ctx,
		invocation.Command,
		invocation.Args...,
	)
	if invocation.UsesStdin {
		cmd.Stdin = strings.NewReader(invocation.Stdin)
	}
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", false
	}
	return value, true
}
