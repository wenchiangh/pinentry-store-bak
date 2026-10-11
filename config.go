package main

import (
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	UI    string
	Store string
}

func loadConfig(path string) config {
	cfg := config{UI: "auto", Store: "auto"}
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "ui":
			if v == "auto" || v == "gui" || v == "tui" {
				cfg.UI = v
			}
		case "store":
			if v == "auto" || v == "file" {
				cfg.Store = v
			}
		}
	}
	return cfg
}

func xdg(env string, parts ...string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(append([]string{home}, parts...)...)
}

func configPath() string {
	return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "pinentry-store", "config")
}

func flagPath() string {
	return filepath.Join(xdg("XDG_STATE_HOME", ".local", "state"), "pinentry-store", "entries.json")
}

func secretPath() string {
	return filepath.Join(xdg("XDG_DATA_HOME", ".local", "share"), "pinentry-store", "secrets")
}
