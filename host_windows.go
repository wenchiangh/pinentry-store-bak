//go:build windows

package main

import (
	"context"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procAllocConsole             = kernel32.NewProc("AllocConsole")
	procGetConsoleWindow         = kernel32.NewProc("GetConsoleWindow")
	procGetProcessWindowStation  = user32.NewProc("GetProcessWindowStation")
	procGetUserObjectInformation = user32.NewProc("GetUserObjectInformationW")
)

func hasConsole() bool {
	r, _, _ := procGetConsoleWindow.Call()
	return r != 0
}

func allocConsole() error {
	r, _, err := procAllocConsole.Call()
	if r == 0 {
		return err
	}
	return nil
}

func callerTerminal(name string) bool {
	return hasConsole()
}

func graphical(optDisplay, argDisplay string) bool {
	return interactiveDesktop()
}

func interactiveDesktop() bool {
	h, _, _ := procGetProcessWindowStation.Call()
	if h == 0 {
		return false
	}
	var flags struct {
		inherit  int32
		reserved int32
		dwFlags  uint32
	}
	r, _, _ := procGetUserObjectInformation.Call(
		h,
		1,
		uintptr(unsafe.Pointer(&flags)),
		unsafe.Sizeof(flags),
		0,
	)
	if r == 0 {
		return false
	}
	return flags.dwFlags&1 != 0
}

func openCaller(name string) (UI, error) {
	return openConsole(false)
}

func openSystemConsole(ctx context.Context) (UI, error) {
	return openConsole(true)
}

func openConsole(alloc bool) (UI, error) {
	if alloc && !hasConsole() {
		if err := allocConsole(); err != nil {
			return nil, err
		}
	}
	in, err := os.OpenFile(`\\.\CONIN$`, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	out, err := os.OpenFile(`\\.\CONOUT$`, os.O_RDWR, 0)
	if err != nil {
		in.Close()
		return nil, err
	}
	return &termUI{sc: newScreen(in, out), files: []*os.File{in, out}, hold: alloc}, nil
}
