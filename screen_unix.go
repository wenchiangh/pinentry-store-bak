//go:build darwin || linux

package main

import (
	"os"
	"time"
)

func enableVirtualTerminal(in, out *os.File) {}

func interruptRead(f *os.File) {
	if f == nil {
		return
	}
	_ = f.SetReadDeadline(time.Now())
}

func resumeRead(f *os.File) {
	if f == nil {
		return
	}
	_ = f.SetReadDeadline(time.Time{})
}
