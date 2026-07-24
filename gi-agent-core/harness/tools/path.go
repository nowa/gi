package tools

import (
	"context"
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

func ResolveReadToolPath(ctx context.Context, env harnessenv.ExecutionEnv, path string) (string, error) {
	resolved := ResolveToolPath(env, path)
	variants := uniqueStrings(
		resolved,
		replaceMeridiemSpace(resolved),
		strings.ReplaceAll(resolved, "'", "\u2019"),
	)
	for _, variant := range variants {
		exists, err := env.Exists(ctx, variant)
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
