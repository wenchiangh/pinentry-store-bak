//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

func deviceStat(name string) string {
	if name == "" {
		return "-"
	}
	fi, err := os.Stat(name)
	if err != nil {
		return "?"
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return "?"
	}
	return fmt.Sprintf("%o/%d/%d", st.Mode, st.Uid, st.Gid)
}

func idField() string {
	return fmt.Sprintf("%d/%d", os.Geteuid(), os.Getegid())
}
