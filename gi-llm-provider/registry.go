package gillmprovider

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type APIProvider interface {
	Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error)
	StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error)
}

type APIProviderFuncs struct {
	StreamFunc       func(Model, Context, StreamOptions) (*AssistantMessageEventStream, error)
	StreamSimpleFunc func(Model, Context, SimpleStreamOptions) (*AssistantMessageEventStream, error)
}

func (p APIProviderFuncs) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	if p.StreamFunc != nil {
		return p.StreamFunc(model, llmContext, options)
	}
	if p.StreamSimpleFunc != nil {
		return p.StreamSimpleFunc(model, llmContext, options)
	}
	return nil, fmt.Errorf("provider does not implement stream")
}

func (p APIProviderFuncs) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	if p.StreamSimpleFunc != nil {
		return p.StreamSimpleFunc(model, llmContext, options)
	}
	return p.Stream(model, llmContext, options)
}

type APIProviderRegistration struct {
	API      string
	Provider APIProvider
	SourceID string
}

const BuiltInAPIProviderSourceID = "builtin"

type registeredAPIProvider struct {
	api      string
	provider APIProvider
	sourceID string
}

type validatingAPIProvider struct {
	api      string
	provider APIProvider
}

func (p validatingAPIProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	if model.API != "" && model.API != p.api {
		return nil, fmt.Errorf("mismatched api: %s expected %s", model.API, p.api)
	}
	return p.provider.Stream(model, llmContext, options)
}

func (p validatingAPIProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	if model.API != "" && model.API != p.api {
		return nil, fmt.Errorf("mismatched api: %s expected %s", model.API, p.api)
	}
	return p.provider.StreamSimple(model, llmContext, options)
}

var apiRegistry = struct {
	sync.RWMutex
	providers map[string]registeredAPIProvider
}{providers: map[string]registeredAPIProvider{}}

var builtInAPIRegistry = struct {
	sync.RWMutex
	providers map[string]APIProvider
}{providers: map[string]APIProvider{}}

func RegisterAPIProvider(api string, provider APIProvider) {
	RegisterAPIProviderWithSource(api, provider, "")
}

func RegisterBuiltInAPIProvider(api string, provider APIProvider) {
	builtInAPIRegistry.Lock()
	builtInAPIRegistry.providers[api] = provider
	builtInAPIRegistry.Unlock()
	RegisterAPIProviderWithSource(api, provider, BuiltInAPIProviderSourceID)
}

func RegisterBuiltInAPIProviders() {
	builtInAPIRegistry.RLock()
	apis := make([]string, 0, len(builtInAPIRegistry.providers))
	providers := make(map[string]APIProvider, len(builtInAPIRegistry.providers))
	for api, provider := range builtInAPIRegistry.providers {
		apis = append(apis, api)
		providers[api] = provider
	}
	builtInAPIRegistry.RUnlock()
	sort.Strings(apis)
	for _, api := range apis {
		RegisterAPIProviderWithSource(api, providers[api], BuiltInAPIProviderSourceID)
	}
}

func ResetAPIProviders() {
	ClearAPIProviders()
	RegisterBuiltInAPIProviders()
}

func RegisterAPIProviderWithSource(api string, provider APIProvider, sourceID string) {
	apiRegistry.Lock()
	defer apiRegistry.Unlock()
	apiRegistry.providers[api] = registeredAPIProvider{api: api, provider: provider, sourceID: sourceID}
}

func UnregisterAPIProvider(api string) {
	apiRegistry.Lock()
	defer apiRegistry.Unlock()
	delete(apiRegistry.providers, api)
}

func UnregisterAPIProviders(sourceID string) {
	apiRegistry.Lock()
	defer apiRegistry.Unlock()
	for api, entry := range apiRegistry.providers {
		if entry.sourceID == sourceID {
			delete(apiRegistry.providers, api)
		}
	}
}

func ClearAPIProviders() {
	apiRegistry.Lock()
	defer apiRegistry.Unlock()
	apiRegistry.providers = map[string]registeredAPIProvider{}
}

func GetAPIProvider(api string) APIProvider {
	apiRegistry.RLock()
	defer apiRegistry.RUnlock()
	entry, ok := apiRegistry.providers[api]
	if !ok || entry.provider == nil {
		return nil
	}
	return validatingAPIProvider{api: entry.api, provider: entry.provider}
}

func GetAPIProviders() []APIProviderRegistration {
	apiRegistry.RLock()
	defer apiRegistry.RUnlock()
	providers := make([]APIProviderRegistration, 0, len(apiRegistry.providers))
	for _, entry := range apiRegistry.providers {
		if entry.provider == nil {
			continue
		}
		providers = append(providers, APIProviderRegistration{
			API:      entry.api,
			Provider: validatingAPIProvider{api: entry.api, provider: entry.provider},
			SourceID: entry.sourceID,
		})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].API < providers[j].API })
	return providers
}

func Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	if options.Context != nil {
		select {
		case <-options.Context.Done():
			return ErrorAssistantStream(AssistantErrorMessage(options.Context.Err().Error(), model, true)), nil
		default:
		}
	}
	options = withEnvAPIKey(model, options)
	provider := GetAPIProvider(model.API)
	if provider == nil {
		return nil, fmt.Errorf("no API provider registered for api: %s", model.API)
	}
	return provider.Stream(model, llmContext, options)
}

func Complete(ctx context.Context, model Model, llmContext Context, options StreamOptions) (Message, error) {
	if options.Context == nil {
		options.Context = ctx
	}
	stream, err := Stream(model, llmContext, options)
	if err != nil {
		return Message{}, err
	}
	message, err := stream.Result(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return AssistantErrorMessage(ctx.Err().Error(), model, true), nil
		}
		return Message{}, err
	}
	return message, nil
}

func StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	if options.Context != nil {
		select {
		case <-options.Context.Done():
			return ErrorAssistantStream(AssistantErrorMessage(options.Context.Err().Error(), model, true)), nil
		default:
		}
	}
	options = withEnvAPIKey(model, options)
	provider := GetAPIProvider(model.API)
	if provider == nil {
		return nil, fmt.Errorf("no API provider registered for api: %s", model.API)
	}
	return provider.StreamSimple(model, llmContext, options)
}

func CompleteSimple(ctx context.Context, model Model, llmContext Context, options SimpleStreamOptions) (Message, error) {
	if options.Context == nil {
		options.Context = ctx
	}
	stream, err := StreamSimple(model, llmContext, options)
	if err != nil {
		return Message{}, err
	}
	message, err := stream.Result(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return AssistantErrorMessage(ctx.Err().Error(), model, true), nil
		}
		return Message{}, err
	}
	return message, nil
}
