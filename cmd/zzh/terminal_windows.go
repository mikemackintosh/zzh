//go:build windows
// +build windows

package main

import (
	"golang.org/x/crypto/ssh/terminal"
)

// setRawTerminal puts the terminal connected to the given file descriptor into raw mode
// and returns the previous state of the terminal so that it can be restored.
func setRawTerminal(fd int) (*terminal.State, error) {
	return terminal.MakeRaw(fd)
}

// restoreTerminal restores the terminal connected to the given file descriptor to a
// previous state.
func restoreTerminal(fd int, state *terminal.State) error {
	return terminal.Restore(fd, state)
}

// getTerminalSize returns the dimensions of the terminal connected to the given file
// descriptor.
func getTerminalSize(fd int) (int, int, error) {
	width, height, err := terminal.GetSize(fd)
	if err != nil {
		return 80, 24, err
	}
	return width, height, nil
}
