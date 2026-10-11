//go:build darwin || linux

package main

import (
	"os"

	"golang.org/x/term"
)

func callerTerminal(name string) bool {
	if name == "" || name == "/dev/tty" {
		return false
	}
	f, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	return term.IsTerminal(int(f.Fd()))
}

func openCaller(name string) (UI, error) {
	f, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &termUI{sc: newScreen(f, f), files: []*os.File{f}}, nil
}
