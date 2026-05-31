package gillmprovider

import (
	"errors"
	"strings"
	"sync"
)

type SessionResourceCleanup func(sessionID string) error

type SessionResourceCleanupError struct {
	Errors []error
}

func (e SessionResourceCleanupError) Error() string {
	if len(e.Errors) == 0 {
		return "failed to cleanup session resources"
	}
	messages := make([]string, 0, len(e.Errors))
	for _, err := range e.Errors {
		if err != nil {
			messages = append(messages, err.Error())
		}
	}
	if len(messages) == 0 {
		return "failed to cleanup session resources"
	}
	return "failed to cleanup session resources: " + strings.Join(messages, "; ")
}

func (e SessionResourceCleanupError) Unwrap() []error {
	return append([]error{}, e.Errors...)
}

var sessionResourceCleanupRegistry = struct {
	sync.Mutex
	cleanups map[*SessionResourceCleanup]SessionResourceCleanup
}{cleanups: map[*SessionResourceCleanup]SessionResourceCleanup{}}

func RegisterSessionResourceCleanup(cleanup SessionResourceCleanup) func() {
	if cleanup == nil {
		return func() {}
	}
	key := &cleanup
	sessionResourceCleanupRegistry.Lock()
	sessionResourceCleanupRegistry.cleanups[key] = cleanup
	sessionResourceCleanupRegistry.Unlock()
	return func() {
		sessionResourceCleanupRegistry.Lock()
		delete(sessionResourceCleanupRegistry.cleanups, key)
		sessionResourceCleanupRegistry.Unlock()
	}
}

func CleanupSessionResources(sessionID string) error {
	sessionResourceCleanupRegistry.Lock()
	cleanups := make([]SessionResourceCleanup, 0, len(sessionResourceCleanupRegistry.cleanups))
	for _, cleanup := range sessionResourceCleanupRegistry.cleanups {
		cleanups = append(cleanups, cleanup)
	}
	sessionResourceCleanupRegistry.Unlock()

	var errs []error
	for _, cleanup := range cleanups {
		if err := cleanup(sessionID); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return SessionResourceCleanupError{Errors: errs}
	}
	return nil
}

func resetSessionResourceCleanupsForTest() {
	sessionResourceCleanupRegistry.Lock()
	sessionResourceCleanupRegistry.cleanups = map[*SessionResourceCleanup]SessionResourceCleanup{}
	sessionResourceCleanupRegistry.Unlock()
}

func IsSessionResourceCleanupError(err error) bool {
	var cleanupErr SessionResourceCleanupError
	return errors.As(err, &cleanupErr)
}
