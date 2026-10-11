package main

import (
	"bufio"
	"context"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	wireCanceled     = 83886179
	wireTimeout      = 83886142
	wirePinEntry     = 83886166
	wireNotConfirmed = 83886194
	wireUnknownCmd   = 83886355
	wireSyntax       = 83886356
	wireParameter    = 83886360
)

type scriptUI struct {
	ask     func(Request) (Result, error)
	notices []string
}

func (s *scriptUI) Ask(ctx context.Context, req Request) (Result, error) {
	if s.ask == nil {
		return Result{Choice: ChoiceCancel}, nil
	}
	return s.ask(req)
}

func (s *scriptUI) Notice(ctx context.Context, text string) error {
	s.notices = append(s.notices, text)
	return nil
}

func (s *scriptUI) Close() error { return nil }

type waitUI struct {
	started chan struct{}
}

func newWaitUI() *waitUI {
	return &waitUI{started: make(chan struct{})}
}

func (w *waitUI) Ask(ctx context.Context, _ Request) (Result, error) {
	select {
	case <-w.started:
	default:
		close(w.started)
	}
	<-ctx.Done()
	return Result{}, ctx.Err()
}

func (w *waitUI) Notice(context.Context, string) error { return nil }
func (w *waitUI) Close() error                         { return nil }

type recStore struct {
	mem      *memStore
	get, put int
	del      int
	putErr   error
	delErr   error
	lastGet  string
}

func newRecStore() *recStore {
	return &recStore{mem: newMemStore()}
}

func (s *recStore) Get(id string) ([]byte, error) {
	s.get++
	s.lastGet = id
	return s.mem.Get(id)
}

func (s *recStore) Put(id string, secret []byte) error {
	s.put++
	if s.putErr != nil {
		return s.putErr
	}
	return s.mem.Put(id, secret)
}

func (s *recStore) Delete(id string) error {
	s.del++
	if s.delErr != nil {
		return s.delErr
	}
	return s.mem.Delete(id)
}

type reply struct {
	data    []byte
	dlines  int
	status  []string
	ok      bool
	okText  string
	code    int
	errText string
}

type sess struct {
	t     *testing.T
	cr    *bufio.Reader
	cw    *io.PipeWriter
	limit time.Duration
}

func startSession(t *testing.T, app *app) *sess {
	t.Helper()
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	go serve(sr, sw, app)
	t.Cleanup(func() {
		cw.Close()
		sw.Close()
		sr.Close()
		cr.Close()
	})
	s := &sess{t: t, cr: bufio.NewReader(cr), cw: cw, limit: 3 * time.Second}
	if line := s.readLine(); line != "OK Pleased to meet you" {
		t.Fatalf("greeting %q", line)
	}
	return s
}

func (s *sess) readLine() string {
	s.t.Helper()
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := s.cr.ReadString('\n')
		ch <- result{line, err}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			s.t.Fatalf("read: %v", res.err)
		}
		return strings.TrimRight(res.line, "\r\n")
	case <-time.After(s.limit):
		s.t.Fatal("timed out waiting for a response")
	}
	return ""
}

func (s *sess) writeRaw(p string) {
	s.t.Helper()
	if _, err := io.WriteString(s.cw, p); err != nil {
		s.t.Fatal(err)
	}
}

func (s *sess) readReply() reply {
	s.t.Helper()
	var rep reply
	for {
		line := s.readLine()
		switch {
		case strings.HasPrefix(line, "D "):
			rep.dlines++
			rep.data = append(rep.data, decodeDataLine(line)...)
		case strings.HasPrefix(line, "S "):
			rep.status = append(rep.status, strings.TrimPrefix(line, "S "))
		case strings.HasPrefix(line, "OK"):
			rep.ok = true
			rep.okText = strings.TrimSpace(strings.TrimPrefix(line, "OK"))
			return rep
		case strings.HasPrefix(line, "ERR "):
			rest := strings.TrimPrefix(line, "ERR ")
			num, text, _ := strings.Cut(rest, " ")
			rep.code, _ = strconv.Atoi(num)
			rep.errText = text
			return rep
		default:
			s.t.Fatalf("unexpected line %q", line)
		}
	}
}

func (s *sess) exchange(cmd string) reply {
	s.t.Helper()
	s.writeRaw(cmd + "\n")
	return s.readReply()
}

func (s *sess) ok(cmd string) {
	s.t.Helper()
	rep := s.exchange(cmd)
	if !rep.ok || rep.okText != "" {
		s.t.Fatalf("%s: %+v", cmd, rep)
	}
}

// decodeDataLine reverses libassuan's client decode: percent-unescape the
// whole line, then drop the "D " prefix. '+' is not a space.
func decodeDataLine(line string) []byte {
	b := []byte(line)
	linelen := len(b)
	out := make([]byte, 0, len(b))
	for s := 0; linelen > 0; {
		if b[s] == '%' && linelen > 2 {
			out = append(out, unhex(b[s+1])<<4|unhex(b[s+2]))
			s += 3
			linelen -= 3
			continue
		}
		out = append(out, b[s])
		s++
		linelen--
	}
	if len(out) < 2 {
		return nil
	}
	return out[2:]
}

func testApp(t *testing.T) (*app, *scriptUI, *memStore) {
	t.Helper()
	st := newMemStore()
	ui := &scriptUI{}
	ui.ask = func(Request) (Result, error) {
		return Result{Choice: ChoiceCancel}, nil
	}
	app := &app{
		store: st,
		flags: &catalog{path: filepath.Join(t.TempDir(), "entries.json")},
		ui:    ui,
	}
	return app, ui, st
}
