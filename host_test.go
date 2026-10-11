package main

import (
	"strings"
	"testing"
)

func TestChooseSurface(t *testing.T) {
	cases := []struct {
		name string
		in   callInfo
		want surface
	}{
		{"auto caller", callInfo{Mode: "auto", CallerTTY: true, Graphical: true}, surfaceTTY},
		{"auto graphical", callInfo{Mode: "auto", Graphical: true}, surfaceConsole},
		{"auto remote caller", callInfo{Mode: "auto", Remote: true, Graphical: true, CallerTTY: true}, surfaceTTY},
		{"auto remote", callInfo{Mode: "auto", Remote: true, Graphical: true}, surfaceFail},
		{"auto neither", callInfo{Mode: "auto"}, surfaceFail},
		{"tui caller", callInfo{Mode: "tui", CallerTTY: true, Graphical: true}, surfaceTTY},
		{"tui no caller", callInfo{Mode: "tui", Graphical: true}, surfaceFail},
		{"gui caller and graphical", callInfo{Mode: "gui", CallerTTY: true, Graphical: true}, surfaceConsole},
		{"gui graphical", callInfo{Mode: "gui", Graphical: true}, surfaceConsole},
		{"gui caller only", callInfo{Mode: "gui", CallerTTY: true}, surfaceTTY},
		{"gui remote caller", callInfo{Mode: "gui", Remote: true, Graphical: true, CallerTTY: true}, surfaceTTY},
		{"gui remote", callInfo{Mode: "gui", Remote: true, Graphical: true}, surfaceFail},
		{"curses beats gui", callInfo{Mode: "gui", Curses: true, CallerTTY: true, Graphical: true}, surfaceTTY},
		{"curses without caller", callInfo{Mode: "gui", Curses: true, Graphical: true}, surfaceFail},
	}
	for _, tc := range cases {
		if got := chooseSurface(tc.in); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestUseCurses(t *testing.T) {
	t.Setenv("PINENTRY_USER_DATA", "foo USE_CURSES=1 bar")
	if !useCurses() {
		t.Fatal("expected curses")
	}
	t.Setenv("PINENTRY_USER_DATA", "USE_CURSES=0")
	if useCurses() {
		t.Fatal("did not expect curses")
	}
}

func TestPromptScript(t *testing.T) {
	if shellQuote("a'b") != `'a'\''b'` {
		t.Fatal(shellQuote("a'b"))
	}
	script := promptScript("/Applications/My Bin/pinentry-store", "/tmp/sock et")
	if script != `export PINENTRY_STORE_SOCKET='/tmp/sock et'; exec '/Applications/My Bin/pinentry-store'` {
		t.Fatal(script)
	}
	if strings.Contains(script, "pass") {
		t.Fatal(script)
	}
}
