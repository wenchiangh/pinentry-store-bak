//go:build darwin

package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

func graphical(optDisplay, argDisplay string) bool {
	return aquaSession()
}

func aquaSession() bool {
	out, err := exec.Command("launchctl", "managername").Output()
	return err == nil && strings.TrimSpace(string(out)) == "Aqua"
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
	script := filepathJoinCommand(path)
	body := "#!/bin/sh\n" + promptScript(bin, path) + "\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		cleanup()
		return nil, err
	}
	cmd := exec.Command("open", "-a", "Terminal", script)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, err
	}
	conn, err := acceptPrompt(ctx, ln, cmd, 15*time.Second)
	if err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		cleanup()
		return nil, err
	}
	return newSockUI(conn, cleanup), nil
}
