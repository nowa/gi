package gillmprovider

import (
	"context"
	"fmt"
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

var apiRegistry = struct {
	sync.RWMutex
	providers map[string]APIProvider
}{providers: map[string]APIProvider{}}

func RegisterAPIProvider(api string, provider APIProvider) {
	apiRegistry.Lock()
	defer apiRegistry.Unlock()
	apiRegistry.providers[api] = provider
}

func UnregisterAPIProvider(api string) {
	apiRegistry.Lock()
	defer apiRegistry.Unlock()
	delete(apiRegistry.providers, api)
}

func GetAPIProvider(api string) APIProvider {
	apiRegistry.RLock()
	defer apiRegistry.RUnlock()
	return apiRegistry.providers[api]
}

func Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	if options.Context != nil {
		select {
		case <-options.Context.Done():
			return ErrorAssistantStream(AssistantErrorMessage(options.Context.Err().Error(), model, true)), nil
		default:
		}
	}
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
