//go:build !(darwin || linux || freebsd || openbsd || netbsd || dragonfly)

package gitui

func startProcessResizeWatcher(func()) func() {
	return nil
}
