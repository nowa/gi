package gitui

import (
	"os"

	"golang.org/x/term"
)

func enableProcessRawMode(file *os.File) (func() error, error) {
	if file == nil {
		return nil, nil
	}
	fd := int(file.Fd())
	if !term.IsTerminal(fd) {
		return nil, nil
	}
	previous, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() error {
		return term.Restore(fd, previous)
	}, nil
}

func processTerminalSize(file *os.File) (cols, rows int, ok bool) {
	if file == nil {
		return 0, 0, false
	}
	cols, rows, err := term.GetSize(int(file.Fd()))
	if err != nil || cols <= 0 || rows <= 0 {
		return 0, 0, false
	}
	return cols, rows, true
}
