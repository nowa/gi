package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const modelCatalogRefreshTimeout = 15 * time.Second

// ModelCatalogRefreshFunc is the command boundary for an explicit catalog
// refresh. The agent directory and refresh policy are values so callers can
// test the command without installing process-global network hooks.
type ModelCatalogRefreshFunc func(
	ctx context.Context,
	agentDir string,
	options ModelRegistryRefreshOptions,
) (llm.ModelsRefreshResult, error)

func refreshModelCatalogs(
	ctx context.Context,
	agentDir string,
	refresh ModelCatalogRefreshFunc,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if refresh == nil {
		refresh = defaultModelCatalogRefresh
	}
	refreshCtx, cancel := context.WithTimeout(
		ctx,
		modelCatalogRefreshTimeout,
	)
	defer cancel()

	result, err := refresh(
		refreshCtx,
		agentDir,
		ModelRegistryRefreshOptions{
			AllowNetwork: true,
			Force:        true,
			Timeout:      modelCatalogRefreshTimeout,
		},
	)
	if result.Aborted ||
		errors.Is(refreshCtx.Err(), context.DeadlineExceeded) ||
		errors.Is(err, context.DeadlineExceeded) {
		return errors.New("Model catalog refresh timed out.")
	}
	if err != nil {
		return err
	}
	if len(result.Errors) == 0 {
		return nil
	}

	providers := make([]string, 0, len(result.Errors))
	for provider := range result.Errors {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	details := make([]string, 0, len(providers))
	for _, provider := range providers {
		details = append(
			details,
			fmt.Sprintf("%s: %v", provider, result.Errors[provider]),
		)
	}
	return fmt.Errorf(
		"Could not refresh model catalogs: %s",
		strings.Join(details, "; "),
	)
}

func cliModelCatalogRefresh(options CLIOptions) ModelCatalogRefreshFunc {
	if options.ModelCatalogRefresh != nil {
		return options.ModelCatalogRefresh
	}
	if options.ModelRegistry != nil {
		return func(
			ctx context.Context,
			_ string,
			refreshOptions ModelRegistryRefreshOptions,
		) (llm.ModelsRefreshResult, error) {
			return options.ModelRegistry.RefreshModels(
				ctx,
				refreshOptions,
			), nil
		}
	}
	return defaultModelCatalogRefresh
}

func defaultModelCatalogRefresh(
	ctx context.Context,
	agentDir string,
	options ModelRegistryRefreshOptions,
) (llm.ModelsRefreshResult, error) {
	runtime, err := NewModelRuntime(ctx, ModelRuntimeOptions{
		ModelRegistryOptions: ModelRegistryOptions{
			AuthStorage:         NewAuthStorage(filepath.Join(agentDir, "auth.json")),
			ModelsJSONPath:      filepath.Join(agentDir, "models.json"),
			ModelRefreshTimeout: options.Timeout,
		},
	})
	if err != nil {
		return llm.ModelsRefreshResult{}, err
	}
	return runtime.Refresh(ctx, options)
}
