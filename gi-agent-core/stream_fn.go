package giagentcore

import (
	"errors"
	"sync"
)

var defaultStreamFnRegistry struct {
	sync.RWMutex
	streamFn StreamFn
}

// SetDefaultStreamFn configures the fallback used when an Agent or low-level
// loop caller omits its stream function. Passing nil clears the fallback.
func SetDefaultStreamFn(streamFn StreamFn) {
	defaultStreamFnRegistry.Lock()
	defaultStreamFnRegistry.streamFn = streamFn
	defaultStreamFnRegistry.Unlock()
}

// GetDefaultStreamFn returns the configured fallback stream function.
func GetDefaultStreamFn() (StreamFn, error) {
	defaultStreamFnRegistry.RLock()
	streamFn := defaultStreamFnRegistry.streamFn
	defaultStreamFnRegistry.RUnlock()
	if streamFn == nil {
		return nil, errors.New("no default stream function configured; pass StreamFn explicitly or call SetDefaultStreamFn")
	}
	return streamFn, nil
}
