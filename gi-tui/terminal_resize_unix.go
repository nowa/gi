//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package gitui

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func startProcessResizeWatcher(onResize func()) func() {
	if onResize == nil {
		return nil
	}

	signals := make(chan os.Signal, 1)
	done := make(chan struct{})
	var once sync.Once
	signal.Notify(signals, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(signals)
		for {
			select {
			case <-signals:
				onResize()
			case <-done:
				return
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
		})
	}
}
