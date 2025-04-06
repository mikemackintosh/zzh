//go:build !windows
// +build !windows

package main

import (
	"syscall"
	"unsafe"

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
func getTerminalSize(fd int) (width, height int, err error) {
	var dimensions [4]uint16
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&dimensions)),
	); errno != 0 {
		return 80, 24, syscall.Errno(errno)
	}
	return int(dimensions[1]), int(dimensions[0]), nil
}
