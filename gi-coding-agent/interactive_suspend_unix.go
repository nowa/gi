//go:build darwin || linux || freebsd || netbsd || openbsd

package gicodingagent

import (
	"errors"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
)

type interactiveSignalSubscription struct {
	ch   chan os.Signal
	once sync.Once
}

func defaultInteractiveSuspendOperations() InteractiveSuspendOperations {
	return InteractiveSuspendOperations{
		Platform:             runtime.GOOS,
		SetInterval:          interactiveSetInterval,
		ClearInterval:        interactiveClearInterval,
		OnSignal:             func(name string, handler func()) any { return subscribeInteractiveSignal(name, handler, false) },
		OnceSignal:           func(name string, handler func()) any { return subscribeInteractiveSignal(name, handler, true) },
		RemoveSignalListener: func(_ string, subscription any) { unsubscribeInteractiveSignal(subscription) },
		KillProcessGroup: func(name string) error {
			sig, ok := interactiveSignalByName(name)
			if !ok {
				return errors.New("unsupported signal " + name)
			}
			return syscall.Kill(0, sig)
		},
	}
}

func interactiveSetInterval(fn func(), interval time.Duration) any {
	stop := make(chan struct{})
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if fn != nil {
					fn()
				}
			case <-stop:
				return
			}
		}
	}()
	return stop
}

func interactiveClearInterval(handle any) {
	if stop, ok := handle.(chan struct{}); ok {
		close(stop)
	}
}

func subscribeInteractiveSignal(name string, handler func(), once bool) any {
	sig, ok := interactiveSignalByName(name)
	if !ok {
		return nil
	}
	subscription := &interactiveSignalSubscription{ch: make(chan os.Signal, 1)}
	signal.Notify(subscription.ch, sig)
	go func() {
		for range subscription.ch {
			if handler != nil {
				handler()
			}
			if once {
				subscription.stop()
				return
			}
		}
	}()
	return subscription
}

func unsubscribeInteractiveSignal(subscription any) {
	if typed, ok := subscription.(*interactiveSignalSubscription); ok && typed != nil {
		typed.stop()
	}
}

func (s *interactiveSignalSubscription) stop() {
	s.once.Do(func() {
		signal.Stop(s.ch)
		close(s.ch)
	})
}

func interactiveSignalByName(name string) (syscall.Signal, bool) {
	switch name {
	case "SIGCONT":
		return syscall.SIGCONT, true
	case "SIGINT":
		return syscall.SIGINT, true
	case "SIGTSTP":
		return syscall.SIGTSTP, true
	default:
		return 0, false
	}
}
