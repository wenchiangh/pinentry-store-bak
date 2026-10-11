//go:build linux

package main

import "testing"

func TestXfceLaunchUsesExecute(t *testing.T) {
	var prefix []string
	for _, term := range linuxTerminals {
		if term.name == "xfce4-terminal" {
			prefix = term.args
		}
	}
	cmd := terminalCmd("xfce4-terminal", prefix, "echo hi")
	if len(cmd.Args) < 5 || cmd.Args[1] != "-x" || cmd.Args[2] != "sh" || cmd.Args[3] != "-c" {
		t.Fatalf("%q", cmd.Args)
	}
}
