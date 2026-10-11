package main

import (
	"context"
	"os"
	"runtime"
	"strings"
)

type surface int

const (
	surfaceFail surface = iota
	surfaceTTY
	surfaceConsole
)

type callInfo struct {
	Mode      string
	Curses    bool
	CallerTTY bool
	Graphical bool
	Remote    bool
}

func chooseSurface(c callInfo) surface {
	if c.Curses || c.Mode == "tui" {
		if c.CallerTTY {
			return surfaceTTY
		}
		return surfaceFail
	}
	gui := c.Graphical && !c.Remote
	if c.Mode == "gui" {
		if gui {
			return surfaceConsole
		}
		if c.CallerTTY {
			return surfaceTTY
		}
		return surfaceFail
	}
	if c.CallerTTY {
		return surfaceTTY
	}
	if gui {
		return surfaceConsole
	}
	return surfaceFail
}

func useCurses() bool {
	return strings.Contains(os.Getenv("PINENTRY_USER_DATA"), "USE_CURSES=1")
}

func remoteSession() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return os.Getenv("SSH_CONNECTION") != ""
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func promptScript(bin, socket string) string {
	return "export PINENTRY_STORE_SOCKET=" + shellQuote(socket) + "; exec " + shellQuote(bin)
}

type lazyUI struct {
	open func(ctx context.Context) (UI, error)
	ui   UI
}

func (l *lazyUI) Ask(ctx context.Context, req Request) (Result, error) {
	if err := l.ensure(ctx); err != nil {
		return Result{}, err
	}
	res, err := l.ui.Ask(ctx, req)
	if err != nil {
		l.drop()
		return Result{}, err
	}
	return res, nil
}

func (l *lazyUI) Notice(ctx context.Context, text string) error {
	if l.ui == nil {
		return errNoUI
	}
	err := l.ui.Notice(ctx, text)
	if err != nil {
		l.drop()
	}
	return err
}

func (l *lazyUI) Close() error {
	return l.drop()
}

func (l *lazyUI) ensure(ctx context.Context) error {
	if l.ui != nil {
		return nil
	}
	ui, err := l.open(ctx)
	if err != nil {
		return err
	}
	l.ui = ui
	return nil
}

func (l *lazyUI) drop() error {
	if l.ui == nil {
		return nil
	}
	err := l.ui.Close()
	l.ui = nil
	return err
}

func (app *app) openUI(ctx context.Context) (UI, error) {
	mode := app.cfg.UI
	if mode == "" {
		mode = "auto"
	}
	info := callInfo{
		Mode:      mode,
		Curses:    useCurses(),
		CallerTTY: callerTerminal(app.sess.ttyName),
		Graphical: graphical(app.sess.display, app.argDisplay),
		Remote:    remoteSession(),
	}
	switch chooseSurface(info) {
	case surfaceTTY:
		return openCaller(app.sess.ttyName)
	case surfaceConsole:
		return openSystemConsole(ctx)
	default:
		return nil, errNoUI
	}
}
