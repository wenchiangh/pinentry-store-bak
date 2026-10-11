//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestWatchClose(t *testing.T) {
	ln, path, cleanup, err := listenPrompt()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- c
	}()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	peer := <-accepted
	ch := watchClose(conn)
	peer.Close()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("peer close was not reported")
	}
}

func TestSockUI(t *testing.T) {
	ln, path, cleanup, err := listenPrompt()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		for {
			var m wire
			if err := dec.Decode(&m); err != nil {
				return
			}
			switch m.Op {
			case "ask":
				enc.Encode(wire{Op: "result", Res: &Result{Choice: ChoiceTyped, Secret: []byte("pw"), Save: true}})
			case "notice":
				if m.Text != "Could not save the passphrase." {
					return
				}
				enc.Encode(wire{Op: "ack"})
			case "bye":
				return
			}
		}
	}()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	ui := newSockUI(conn, cleanup)
	res, err := ui.Ask(context.Background(), Request{Description: "hello"})
	if err != nil || res.Choice != ChoiceTyped || string(res.Secret) != "pw" || !res.Save {
		t.Fatalf("%+v %v", res, err)
	}
	if err := ui.Notice(context.Background(), "Could not save the passphrase."); err != nil {
		t.Fatal(err)
	}
	ui.Close()
}

func TestSockUICancel(t *testing.T) {
	ln, path, cleanup, err := listenPrompt()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		dec := json.NewDecoder(conn)
		var m wire
		if err := dec.Decode(&m); err != nil {
			return
		}
		close(started)
		var extra wire
		dec.Decode(&extra)
	}()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	ui := newSockUI(conn, cleanup)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	if _, err := ui.Ask(ctx, Request{Description: "x"}); err == nil {
		t.Fatal("cancel did not unblock Ask")
	}
}
