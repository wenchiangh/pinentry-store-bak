//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func listenPrompt() (*net.UnixListener, string, func(), error) {
	dir, err := os.MkdirTemp("", "pinentry-store-")
	if err != nil {
		return nil, "", nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		os.RemoveAll(dir)
		return nil, "", nil, err
	}
	path := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", path)
	if err != nil {
		os.RemoveAll(dir)
		return nil, "", nil, err
	}
	os.Chmod(path, 0600)
	cleanup := func() {
		ln.Close()
		os.RemoveAll(dir)
	}
	return ln.(*net.UnixListener), path, cleanup, nil
}

func filepathJoinCommand(socket string) string {
	return filepath.Join(filepath.Dir(socket), "pinentry-store.command")
}

func acceptPrompt(ctx context.Context, ln *net.UnixListener, cmd *exec.Cmd, wait time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(wait)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	exitCh := make(chan error, 1)
	if cmd != nil {
		go func() { exitCh <- cmd.Wait() }()
	}
	var exitErr error
	for {
		if time.Now().After(deadline) {
			return nil, errNoUI
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		ln.SetDeadline(time.Now().Add(200 * time.Millisecond))
		conn, err := ln.Accept()
		if err == nil {
			return conn, nil
		}
		select {
		case exitErr = <-exitCh:
			if exitErr != nil {
				// The launcher failed. Give a window that is already opening
				// a moment to connect, then try the next emulator.
				deadline = time.Now().Add(300 * time.Millisecond)
			}
			exitCh = nil
		default:
		}
	}
}

type sockUI struct {
	conn    net.Conn
	enc     *json.Encoder
	dec     *json.Decoder
	cleanup func()
}

func newSockUI(conn net.Conn, cleanup func()) *sockUI {
	return &sockUI{
		conn:    conn,
		enc:     json.NewEncoder(conn),
		dec:     json.NewDecoder(conn),
		cleanup: cleanup,
	}
}

func (s *sockUI) Ask(ctx context.Context, req Request) (Result, error) {
	stop := context.AfterFunc(ctx, func() { s.conn.Close() })
	defer stop()
	if err := s.enc.Encode(wire{Op: "ask", Req: &req}); err != nil {
		return Result{}, err
	}
	var m wire
	if err := s.dec.Decode(&m); err != nil {
		return Result{}, err
	}
	if m.Res == nil {
		return Result{Choice: ChoiceCancel}, nil
	}
	return *m.Res, nil
}

func (s *sockUI) Notice(ctx context.Context, text string) error {
	stop := context.AfterFunc(ctx, func() { s.conn.Close() })
	defer stop()
	if err := s.enc.Encode(wire{Op: "notice", Text: text}); err != nil {
		return err
	}
	var m wire
	return s.dec.Decode(&m)
}

func (s *sockUI) Close() error {
	_ = s.enc.Encode(wire{Op: "bye"})
	err := s.conn.Close()
	if s.cleanup != nil {
		s.cleanup()
	}
	return err
}

// watchClose reports when the peer closes conn.
// The fd is duplicated outside Control: Control must return before any other
// use of the connection, and the watcher only peeks so the JSON reader keeps
// the bytes.
func watchClose(conn net.Conn) <-chan struct{} {
	ch := make(chan struct{})
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return ch
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return ch
	}
	dup := -1
	var dupErr error
	if err := raw.Control(func(fd uintptr) {
		dup, dupErr = unix.Dup(int(fd))
	}); err != nil || dupErr != nil || dup < 0 {
		if dup >= 0 {
			unix.Close(dup)
		}
		return ch
	}
	go func() {
		defer unix.Close(dup)
		defer close(ch)
		p := []unix.PollFd{{
			Fd:     int32(dup),
			Events: unix.POLLIN | unix.POLLHUP | unix.POLLERR | unix.POLLNVAL,
		}}
		buf := []byte{0}
		for {
			_, err := unix.Poll(p, 500)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				return
			}
			rev := p[0].Revents
			if rev&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
				return
			}
			if rev&unix.POLLIN != 0 {
				n, _, rerr := unix.Recvfrom(dup, buf, unix.MSG_PEEK|unix.MSG_DONTWAIT)
				if n == 0 || (rerr != nil && rerr != unix.EAGAIN && rerr != unix.EWOULDBLOCK) {
					return
				}
			}
		}
	}()
	return ch
}
