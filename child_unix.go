//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func maybeChild() bool {
	socket := os.Getenv("PINENTRY_STORE_SOCKET")
	if socket == "" {
		return false
	}
	runChild(socket)
	return true
}

// runChild draws the prompt in the terminal the host opened and returns
// results on the socket. The passphrase is not on the command line.
func runChild(socket string) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		os.Exit(1)
	}
	defer conn.Close()

	ui := &termUI{sc: newScreen(os.Stdin, os.Stdout), hold: true}
	defer ui.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-watchClose(conn)
		cancel()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		ui.Close()
		os.Exit(1)
	}()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	for {
		var m wire
		if err := dec.Decode(&m); err != nil {
			return
		}
		switch m.Op {
		case "bye":
			return
		case "ask":
			if m.Req == nil {
				continue
			}
			res, err := ui.Ask(ctx, *m.Req)
			if err != nil || ctx.Err() != nil {
				res = Result{Choice: ChoiceCancel}
			}
			if err := enc.Encode(wire{Op: "result", Res: &res}); err != nil {
				return
			}
		case "notice":
			_ = ui.Notice(ctx, m.Text)
			if err := enc.Encode(wire{Op: "ack"}); err != nil {
				return
			}
		}
	}
}
