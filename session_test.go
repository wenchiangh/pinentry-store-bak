package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-keyring/keyring"
)

func TestPassphraseBytes(t *testing.T) {
	app, ui, _ := testApp(t)
	want := []byte("a b+c%\n\r")
	ui.ask = func(Request) (Result, error) {
		return Result{Choice: ChoiceTyped, Secret: append([]byte(nil), want...)}, nil
	}
	s := startSession(t, app)
	s.ok("SETKEYINFO n/abc")
	rep := s.exchange("GETPIN")
	if !rep.ok || !bytes.Equal(rep.data, want) || rep.dlines != 1 {
		t.Fatalf("%+v %q", rep, rep.data)
	}

	long := bytes.Repeat([]byte("x"), 2000)
	ui.ask = func(Request) (Result, error) {
		return Result{Choice: ChoiceTyped, Secret: long}, nil
	}
	rep = s.exchange("GETPIN")
	if !rep.ok || !bytes.Equal(rep.data, long) || rep.dlines < 2 {
		t.Fatalf("lines=%d len=%d", rep.dlines, len(rep.data))
	}
}

func TestEmptyPassphrase(t *testing.T) {
	app, ui, _ := testApp(t)
	var got Request
	ui.ask = func(req Request) (Result, error) {
		got = req
		return Result{Choice: ChoiceTyped}, nil
	}
	s := startSession(t, app)
	s.ok("SETKEYINFO --clear")
	rep := s.exchange("GETPIN")
	if !rep.ok || rep.dlines != 0 || len(rep.status) != 0 {
		t.Fatalf("empty %+v", rep)
	}
	if got.CanStore {
		t.Fatal("cleared key offered storage")
	}
	s.ok("SETREPEAT Repeat:")
	rep = s.exchange("GETPIN")
	if !rep.ok || rep.dlines != 0 || len(rep.status) != 1 || rep.status[0] != "PIN_REPEATED" {
		t.Fatalf("empty repeat %+v", rep)
	}
}

func TestRepeatWire(t *testing.T) {
	app, ui, st := testApp(t)
	if err := st.Put("n/abc", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := app.flags.Mark("n/abc", true, true); err != nil {
		t.Fatal(err)
	}
	var got Request
	ui.ask = func(req Request) (Result, error) {
		got = req
		return Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: false, NoPrompt: true}, nil
	}
	s := startSession(t, app)
	s.ok("SETKEYINFO n/abc")
	s.ok("SETDESC label%20one")
	s.ok("SETREPEAT Repeat:")
	rep := s.exchange("GETPIN")
	if !got.Repeat || got.ErrorSet || !got.HasSecret || !got.CanStore {
		t.Fatalf("request %+v", got)
	}
	if !rep.ok || string(rep.data) != "new" || len(rep.status) != 1 || rep.status[0] != "PIN_REPEATED" {
		t.Fatalf("%+v", rep)
	}
	gotSecret, err := st.Get("n/abc")
	if err != nil || string(gotSecret) != "old" || !app.flags.Has("n/abc") || !app.flags.Known("n/abc") {
		t.Fatal("repeat without save changed the stored entry")
	}
	if len(ui.notices) != 0 {
		t.Fatalf("notices %v", ui.notices)
	}

	// Repeat is cleared. The stored passphrase and its mark remain, so the next ask is silent.
	ui.ask = func(Request) (Result, error) {
		t.Fatal("stored no-prompt entry prompted")
		return Result{}, nil
	}
	rep = s.exchange("GETPIN")
	if !rep.ok || string(rep.data) != "old" || len(rep.status) != 0 {
		t.Fatalf("next %+v", rep)
	}
}

func TestClearPassphrase(t *testing.T) {
	app, ui, st := testApp(t)
	key := "n/abc%2f"
	if err := st.Put(key, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	if err := app.flags.Mark(key, true, true); err != nil {
		t.Fatal(err)
	}
	ui.ask = func(Request) (Result, error) {
		return Result{Choice: ChoiceTyped, Secret: []byte("x")}, nil
	}
	s := startSession(t, app)
	s.ok("SETKEYINFO --clear")
	rep := s.exchange("GETPIN")
	if !rep.ok || string(rep.data) != "x" {
		t.Fatalf("clear getpin %+v", rep)
	}
	if _, err := st.Get(key); err != nil || !app.flags.Has(key) || !app.flags.Known(key) {
		t.Fatal("--clear touched the store")
	}

	s.ok("CLEARPASSPHRASE")
	s.ok("CLEARPASSPHRASE   " + key + "  ")
	if _, err := st.Get(key); err == nil {
		t.Fatal("clearpassphrase left the secret")
	}
	if app.flags.Has(key) || app.flags.Known(key) {
		t.Fatal("clearpassphrase left the entry")
	}
	s.ok("CLEARPASSPHRASE " + key)
}

func TestConfirm(t *testing.T) {
	app, ui, _ := testApp(t)
	var kind Kind
	ui.ask = func(req Request) (Result, error) {
		kind = req.Kind
		return Result{Choice: ChoiceCancel}, nil
	}
	s := startSession(t, app)
	rep := s.exchange("CONFIRM --one-button")
	if kind != KindConfirm {
		t.Fatalf("kind %v", kind)
	}
	if !rep.ok || rep.okText != "" {
		t.Fatalf("one-button %+v", rep)
	}
	rep = s.exchange("MESSAGE")
	if !rep.ok {
		t.Fatalf("message %+v", rep)
	}
	ui.ask = func(Request) (Result, error) {
		return Result{Choice: ChoiceNotOK}, nil
	}
	rep = s.exchange("CONFIRM")
	if rep.code != wireNotConfirmed || rep.errText != "Not confirmed <Pinentry>" {
		t.Fatalf("not confirmed %+v", rep)
	}
	ui.ask = func(Request) (Result, error) {
		return Result{Choice: ChoiceCancel}, nil
	}
	rep = s.exchange("CONFIRM --focus=cancel")
	if rep.code != wireCanceled {
		t.Fatalf("confirm cancel %+v", rep)
	}
	ui.ask = func(Request) (Result, error) {
		return Result{Choice: ChoiceOK}, nil
	}
	s.ok("CONFIRM --focus=ok")
}

func TestEOFAfterCommand(t *testing.T) {
	app, ui, _ := testApp(t)
	ui.ask = func(Request) (Result, error) {
		return Result{Choice: ChoiceTyped, Secret: []byte("secret")}, nil
	}
	s := startSession(t, app)
	s.writeRaw("SETTITLE rbw\nSETPROMPT Master Password\nSETDESC Log in\nGETPIN\n")
	s.cw.Close()
	for range 3 {
		rep := s.readReply()
		if !rep.ok || rep.okText != "" {
			t.Fatalf("setup %+v", rep)
		}
	}
	rep := s.readReply()
	if !rep.ok || string(rep.data) != "secret" || rep.dlines != 1 {
		t.Fatalf("%+v %q", rep, rep.data)
	}
}

func TestLaunchArgs(t *testing.T) {
	ln, err := splitArgs([]string{
		"--timeout", "0",
		"--ttyname", "/dev/ttys000",
		"--no-global-grab",
		"--display=:1",
	})
	if err != nil || ln.timeout != 0 || ln.ttyName != "/dev/ttys000" || ln.display != ":1" || len(ln.rest) != 0 || len(ln.unknown) != 0 {
		t.Fatalf("%+v %v", ln, err)
	}
	ln, err = splitArgs([]string{
		"-o", "15", "-T", "/dev/pts/4", "-Nxterm", "-g", "-d",
		"-W", "9", "--colors", "x", "--ttyalert", "none",
		"--lc-ctype", "C", "--lc-messages", "C", "list",
	})
	if err != nil || ln.timeout != 15 || ln.ttyName != "/dev/pts/4" || ln.ttyType != "xterm" || len(ln.rest) != 1 || ln.rest[0] != "list" {
		t.Fatalf("%+v %v", ln, err)
	}
	ln, err = splitArgs([]string{"--timeout=30", "--ttyname=/dev/pts/1", "--foo"})
	if err != nil || ln.timeout != 30 || ln.ttyName != "/dev/pts/1" || len(ln.unknown) != 1 || ln.unknown[0] != "--foo" {
		t.Fatalf("%+v %v", ln, err)
	}
	if _, err = splitArgs([]string{"--timeout"}); err == nil {
		t.Fatal("missing timeout value")
	}
	if _, err = splitArgs([]string{"--timeout", "nope"}); err == nil {
		t.Fatal("bad timeout")
	}

	ln, err = splitArgs([]string{"--timeout", "1", "--ttyname", "/dev/pts/9", "--ttytype", "dumb"})
	if err != nil {
		t.Fatal(err)
	}
	app := &app{
		store: new(memStore),
		flags: &catalog{path: filepath.Join(t.TempDir(), "entries.json")},
		ui:    newWaitUI(),
	}
	applyLaunch(app, ln)
	s := startSession(t, app)
	s.limit = 5 * time.Second
	rep := s.exchange("GETINFO ttyinfo")
	fields := strings.Fields(string(rep.data))
	if !rep.ok || len(fields) < 2 || fields[0] != "/dev/pts/9" || fields[1] != "dumb" {
		t.Fatalf("ttyinfo %+v %q", rep, rep.data)
	}
	s.ok("OPTION ttyname=/dev/pts/3")
	s.ok("RESET")
	rep = s.exchange("GETINFO ttyinfo")
	fields = strings.Fields(string(rep.data))
	if !rep.ok || len(fields) < 2 || fields[0] != "/dev/pts/3" || fields[1] != "dumb" {
		t.Fatalf("after reset %+v %q", rep, rep.data)
	}
	rep = s.exchange("GETPIN")
	if rep.code != wireTimeout || rep.errText != "Timeout <Pinentry>" {
		t.Fatalf("timeout %+v", rep)
	}
}

func TestTimeoutSurvivesReset(t *testing.T) {
	st := newRecStore()
	app := &app{
		store: st,
		flags: &catalog{path: filepath.Join(t.TempDir(), "entries.json")},
		ui:    newWaitUI(),
	}
	s := startSession(t, app)
	s.limit = 5 * time.Second
	s.ok("SETTIMEOUT 1")
	s.ok("SETKEYINFO n/abc")
	rep := s.exchange("GETPIN")
	if rep.code != wireTimeout || rep.errText != "Timeout <Pinentry>" {
		t.Fatalf("timeout %+v", rep)
	}
	if st.get != 1 || st.lastGet != "n/abc" {
		t.Fatalf("get %d %q", st.get, st.lastGet)
	}
	s.ok("RESET")
	rep = s.exchange("GETPIN")
	if rep.code != wireTimeout {
		t.Fatalf("reset timeout %+v", rep)
	}
	if st.get != 1 {
		t.Fatalf("RESET did not clear keyinfo, gets=%d", st.get)
	}
}

func TestNoUI(t *testing.T) {
	app, _, _ := testApp(t)
	app.ui = nil
	s := startSession(t, app)
	rep := s.exchange("GETPIN")
	if rep.code != wirePinEntry || rep.errText != "pinentry error <Pinentry>" {
		t.Fatalf("no ui %+v", rep)
	}
}

func TestTouchStore(t *testing.T) {
	st := newRecStore()
	if err := st.mem.Put("n/abc", []byte("old")); err != nil {
		t.Fatal(err)
	}
	app := &app{
		store: st,
		flags: &catalog{path: filepath.Join(t.TempDir(), "entries.json")},
		ui: &scriptUI{ask: func(Request) (Result, error) {
			return Result{Choice: ChoiceTyped, Secret: []byte("new")}, nil
		}},
	}
	if err := app.flags.Mark("n/abc", true, true); err != nil {
		t.Fatal(err)
	}
	s := startSession(t, app)
	s.ok("GETPIN")
	if st.get != 0 || st.put != 0 || st.del != 0 {
		t.Fatalf("missing key touched the store: get=%d put=%d del=%d", st.get, st.put, st.del)
	}
	s.ok("SETKEYINFO --clear")
	s.ok("GETPIN")
	if st.get != 0 || st.put != 0 || st.del != 0 {
		t.Fatal("--clear touched the store")
	}
	if !app.flags.Has("n/abc") || !app.flags.Known("n/abc") {
		t.Fatal("an ask with no stable key changed the saved entry")
	}
}

func TestCatalogReconcile(t *testing.T) {
	app, ui, st := testApp(t)
	if err := st.Put("n/old", []byte("kept")); err != nil {
		t.Fatal(err)
	}
	var got Request
	ui.ask = func(req Request) (Result, error) {
		got = req
		return Result{Choice: ChoiceCancel}, nil
	}
	s := startSession(t, app)
	s.ok("SETKEYINFO n/old")
	rep := s.exchange("GETPIN")
	if rep.code != wireCanceled {
		t.Fatalf("a secret with no saved entry was silent: %+v", rep)
	}
	if !got.HasSecret || got.NoPrompt || !got.CanStore {
		t.Fatalf("request %+v", got)
	}
	if !app.flags.Known("n/old") || app.flags.Has("n/old") {
		t.Fatal("a stored secret did not gain a prompting entry")
	}

	if err := app.flags.Mark("n/gone", true, true); err != nil {
		t.Fatal(err)
	}
	prompted := false
	ui.ask = func(req Request) (Result, error) {
		prompted = true
		got = req
		return Result{Choice: ChoiceCancel}, nil
	}
	s.ok("SETKEYINFO n/gone")
	rep = s.exchange("GETPIN")
	if !prompted || rep.code != wireCanceled {
		t.Fatalf("a line with no secret %+v prompted=%v", rep, prompted)
	}
	if got.HasSecret || got.NoPrompt {
		t.Fatalf("request %+v", got)
	}
	if app.flags.Known("n/gone") {
		t.Fatal("a line with no secret survived the ask")
	}
	if !app.flags.Known("n/old") || app.flags.Has("n/old") {
		t.Fatal("reconciling another id changed the stored entry")
	}
	if len(ui.notices) != 0 {
		t.Fatalf("notices %v", ui.notices)
	}
}

type errStore struct {
	err error
}

func (s errStore) Get(string) ([]byte, error) { return nil, s.err }
func (s errStore) Put(string, []byte) error   { return s.err }
func (s errStore) Delete(string) error        { return s.err }

func TestReconcileKeepsLineOnReadError(t *testing.T) {
	app, ui, _ := testApp(t)
	app.store = errStore{err: errors.New("locked")}
	if err := app.flags.Mark("n/keep", true, true); err != nil {
		t.Fatal(err)
	}
	prompted := false
	ui.ask = func(req Request) (Result, error) {
		prompted = true
		if req.HasSecret || !req.NoPrompt {
			t.Errorf("request %+v", req)
		}
		return Result{Choice: ChoiceCancel}, nil
	}
	s := startSession(t, app)
	s.ok("SETKEYINFO n/keep")
	rep := s.exchange("GETPIN")
	if !prompted || rep.code != wireCanceled {
		t.Fatalf("%+v prompted=%v", rep, prompted)
	}
	if !app.flags.Has("n/keep") || !app.flags.Known("n/keep") {
		t.Fatal("a failed read removed the saved entry")
	}

	app.store = errStore{err: keyring.ErrNotFound}
	if err := app.flags.Mark("n/miss", true, true); err != nil {
		t.Fatal(err)
	}
	ui.ask = func(req Request) (Result, error) {
		if req.HasSecret || req.NoPrompt {
			t.Errorf("missing secret request %+v", req)
		}
		return Result{Choice: ChoiceCancel}, nil
	}
	s.ok("SETKEYINFO n/miss")
	rep = s.exchange("GETPIN")
	if rep.code != wireCanceled {
		t.Fatalf("%+v", rep)
	}
	if app.flags.Known("n/miss") {
		t.Fatal("a missing secret left its saved entry")
	}
	if !app.flags.Has("n/keep") {
		t.Fatal("removing one id cleared another")
	}
}

func TestReconcileIgnoresCatalogErrors(t *testing.T) {
	app, ui, st := testApp(t)
	blocker := filepath.Join(t.TempDir(), "not-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	app.flags = &catalog{path: filepath.Join(blocker, "entries.json")}
	if err := st.Put("n/old", []byte("kept")); err != nil {
		t.Fatal(err)
	}
	prompted := false
	ui.ask = func(Request) (Result, error) {
		prompted = true
		return Result{Choice: ChoiceCancel}, nil
	}
	s := startSession(t, app)
	s.ok("SETKEYINFO n/old")
	rep := s.exchange("GETPIN")
	if !prompted || rep.code != wireCanceled || len(ui.notices) != 0 {
		t.Fatalf("catalog error blocked the prompt: %+v notices=%v prompted=%v", rep, ui.notices, prompted)
	}
}
