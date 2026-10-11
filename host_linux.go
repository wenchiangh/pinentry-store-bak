//go:build linux

package main

import (
	"context"
	"os"
	"os/exec"
	"time"
)

func graphical(optDisplay, argDisplay string) bool {
	return os.Getenv("DISPLAY") != "" ||
		os.Getenv("WAYLAND_DISPLAY") != "" ||
		optDisplay != "" ||
		argDisplay != ""
}

var linuxTerminals = []struct {
	name string
	args []string
}{
	{"xdg-terminal-exec", nil},
	{"x-terminal-emulator", []string{"-e"}},
	{"kgx", []string{"--"}},
	{"gnome-terminal", []string{"--"}},
	{"konsole", []string{"-e"}},
	// -e takes one command string. -x takes the remaining arguments.
	{"xfce4-terminal", []string{"-x"}},
	{"kitty", nil},
	{"alacritty", []string{"-e"}},
	{"foot", nil},
	{"xterm", []string{"-e"}},
}

func openSystemConsole(ctx context.Context) (UI, error) {
	ln, path, cleanup, err := listenPrompt()
	if err != nil {
		return nil, err
	}
	bin, err := os.Executable()
	if err != nil {
		cleanup()
		return nil, err
	}
	script := promptScript(bin, path)
	var last error
	for _, term := range linuxTerminals {
		if _, err := exec.LookPath(term.name); err != nil {
			continue
		}
		cmd := terminalCmd(term.name, term.args, script)
		if err := cmd.Start(); err != nil {
			last = err
			continue
		}
		conn, err := acceptPrompt(ctx, ln, cmd, 5*time.Second)
		if err == nil {
			return newSockUI(conn, cleanup), nil
		}
		last = err
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	cleanup()
	if last == nil {
		last = errNoUI
	}
	return nil, last
}

func terminalCmd(name string, prefix []string, script string) *exec.Cmd {
	args := append(append([]string{}, prefix...), "sh", "-c", script)
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	return cmd
}
