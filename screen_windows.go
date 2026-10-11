//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func enableVirtualTerminal(in, out *os.File) {
	setVT(in, windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	setVT(out, windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}

func interruptRead(f *os.File) {
	if f == nil {
		return
	}
	_ = windows.CancelIoEx(windows.Handle(f.Fd()), nil)
}

func resumeRead(f *os.File) {}

func setVT(f *os.File, flag uint32) {
	var mode uint32
	h := windows.Handle(f.Fd())
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return
	}
	windows.SetConsoleMode(h, mode|flag)
}
