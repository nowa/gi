package tools

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
)

const narrowNoBreakSpace = "\u202f"

func NormalizeToolPath(path string) string {
	path = strings.Map(func(r rune) rune {
		switch {
		case r == '\u00a0',
			r >= '\u2000' && r <= '\u200a',
			r == '\u202f',
			r == '\u205f',
			r == '\u3000':
			return ' '
		default:
			return r
		}
	}, path)
	return strings.TrimPrefix(path, "@")
}

func ResolveToolPath(env harnessenv.ExecutionEnv, path string) string {
	return env.AbsolutePath(NormalizeToolPath(path))
}

// PathExists keeps the execution environment as the filesystem boundary so
// local and delegated tools use the same path semantics. Unlike a bare stat
// helper, it preserves cancellation and permission errors for the caller.
func PathExists(ctx context.Context, env harnessenv.ExecutionEnv, path string) (bool, error) {
	if env == nil {
		return false, errors.New("execution environment is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return env.Exists(ctx, path)
}

func ResolveReadToolPath(ctx context.Context, env harnessenv.ExecutionEnv, path string) (string, error) {
	resolved := ResolveToolPath(env, path)
	variants := uniqueStrings(
		resolved,
		replaceMeridiemSpace(resolved),
		strings.ReplaceAll(resolved, "'", "\u2019"),
	)
	for _, variant := range variants {
		exists, err := PathExists(ctx, env, variant)
		if err != nil {
			return "", err
		}
		if exists {
			return variant, nil
		}
	}
	return filepath.Clean(resolved), nil
}

func replaceMeridiemSpace(path string) string {
	replacer := strings.NewReplacer(
		" AM.", narrowNoBreakSpace+"AM.",
		" PM.", narrowNoBreakSpace+"PM.",
		" am.", narrowNoBreakSpace+"am.",
		" pm.", narrowNoBreakSpace+"pm.",
	)
	return replacer.Replace(path)
}

func uniqueStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
