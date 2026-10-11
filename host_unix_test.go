//go:build darwin || linux

package main

import "testing"

func TestCallerTerminalRejectsDevTty(t *testing.T) {
	if callerTerminal("") || callerTerminal("/dev/tty") {
		t.Fatal("empty and /dev/tty are not a caller terminal")
	}
}
