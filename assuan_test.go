package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestGreetingBeforeRead(t *testing.T) {
	app, _, _ := testApp(t)
	// startSession fails unless the server writes the greeting before it
	// needs a client command.
	startSession(t, app)
}

func TestDataLine(t *testing.T) {
	raw := encodeData(t, []byte("a b+c%\n\r"))
	if raw != "D a b+c%25%0A%0D\n" {
		t.Fatalf("wire %q", raw)
	}
	if !bytes.Equal(decodeDataLine(strings.TrimRight(raw, "\n")), []byte("a b+c%\n\r")) {
		t.Fatal("round trip")
	}

	plain := bytes.Repeat([]byte("a"), 996)
	if strings.Count(encodeData(t, plain), "\n") != 1 {
		t.Fatal("996 bytes should be one data line")
	}
	if strings.Count(encodeData(t, bytes.Repeat([]byte("a"), 997)), "\n") != 2 {
		t.Fatal("997 bytes should wrap")
	}

	var long []byte
	for i := 0; i < 2000; i++ {
		long = append(long, byte(i%256))
		if i%50 == 0 {
			long = append(long, '%', '\n', '\r', '+', ' ')
		}
	}
	got := decodeAll(t, encodeData(t, long))
	if !bytes.Equal(got, long) {
		t.Fatalf("wrapped round trip %d vs %d", len(got), len(long))
	}
}

func encodeData(t *testing.T, p []byte) string {
	t.Helper()
	var buf bytes.Buffer
	a := newAssuan(bytes.NewReader(nil), &buf)
	if err := a.data(p); err != nil {
		t.Fatal(err)
	}
	if err := a.w.Flush(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func decodeAll(t *testing.T, raw string) []byte {
	t.Helper()
	var out []byte
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "D ") {
			t.Fatalf("not a data line %q", line)
		}
		out = append(out, decodeDataLine(line)...)
	}
	return out
}

func TestProtocol(t *testing.T) {
	app, _, _ := testApp(t)
	s := startSession(t, app)

	s.writeRaw("\n# comment\nNOP\n")
	if s.readLine() != "OK" {
		t.Fatal("blank line or comment produced a reply")
	}
	bye := s.exchange("BYE")
	if !bye.ok || bye.okText != "closing connection" {
		t.Fatalf("BYE %+v", bye)
	}

	app, ui, _ := testApp(t)
	s = startSession(t, app)

	rep := s.exchange(" GETPIN")
	if rep.code != wireSyntax || rep.errText != "IPC syntax error <Pinentry>" {
		t.Fatalf("leading space %+v", rep)
	}
	rep = s.exchange("NO-SUCH")
	if rep.code != wireUnknownCmd || rep.errText != "Unknown IPC command <Pinentry>" {
		t.Fatalf("unknown %+v", rep)
	}
	s.ok("NOP")

	rep = s.exchange("GETINFO no-such")
	if rep.code != wireParameter || rep.errText != "IPC parameter error <Pinentry>" {
		t.Fatalf("getinfo %+v", rep)
	}
	rep = s.exchange("GETINFO flavor")
	if !rep.ok || string(rep.data) != "store" || rep.dlines != 1 {
		t.Fatalf("flavor %+v", rep)
	}
	rep = s.exchange("GETINFO version")
	if string(rep.data) != version {
		t.Fatalf("version %q", rep.data)
	}
	rep = s.exchange("GETINFO pid")
	if string(rep.data) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("pid %q", rep.data)
	}

	name := "PINENTRY_STORE_PUTENV_TEST"
	if os.Getenv(name) != "" {
		t.Fatal("test variable already set")
	}
	s.ok("OPTION putenv=" + name + "=1")
	if os.Getenv(name) != "" {
		t.Fatal("putenv was applied")
	}
	t.Setenv(name, "keep")
	s.ok("OPTION putenv " + name + "=changed")
	if os.Getenv(name) != "keep" {
		t.Fatalf("putenv changed %q", os.Getenv(name))
	}
	s.ok("OPTION allow-external-password-cache")
	s.ok("OPTION no-grab")
	rep = s.exchange("OPTION -ttyname=/dev/pts/1")
	if rep.code != wireSyntax {
		t.Fatalf("one dash %+v", rep)
	}
	s.ok("OPTION --ttyname=/dev/pts/9")
	s.ok("OPTION ttytype=xterm")
	s.ok("OPTION display=:1")

	s.ok("SETQUALITYBAR Quality:")
	s.ok("SETQUALITYBAR_TT tip")
	s.ok("SETGENPIN label")
	s.ok("SETGENPIN_TT tip")
	s.ok("SETREPEATERROR does not match")
	s.ok("SETREPEATOK match")

	var got Request
	ui.ask = func(req Request) (Result, error) {
		got = req
		return Result{Choice: ChoiceCancel}, nil
	}
	s.ok("SETDESC hello%20there+plus")
	s.ok("SETKEYINFO n/abc%2f")
	rep = s.exchange("GETPIN")
	if rep.code != wireCanceled || rep.errText != "Operation cancelled <Pinentry>" {
		t.Fatalf("cancel %+v", rep)
	}
	if got.Description != "hello there+plus" {
		t.Fatalf("description %q", got.Description)
	}
	if app.sess.key != "n/abc%2f" {
		t.Fatalf("key was decoded to %q", app.sess.key)
	}

	s.ok("RESET")
	if app.sess.key != "" {
		t.Fatal("RESET kept keyinfo")
	}
	rep = s.exchange("GETINFO ttyinfo")
	fields := strings.Fields(string(rep.data))
	if len(fields) != 6 || fields[0] != "/dev/pts/9" || fields[1] != "xterm" || fields[2] != ":1" || fields[5] != "-" {
		t.Fatalf("ttyinfo %q", rep.data)
	}

	app.argDisplay = ":9"
	app.sess.display = ""
	info := app.ttyinfo()
	if !strings.Contains(info, ":9") {
		t.Fatalf("argv display missing: %s", info)
	}
}

func TestPINWire(t *testing.T) {
	app, ui, st := testApp(t)
	if err := st.Put("n/abc", []byte("stored-secret-value")); err != nil {
		t.Fatal(err)
	}
	if err := app.flags.Mark("n/abc", true, true); err != nil {
		t.Fatal(err)
	}
	prompted := false
	ui.ask = func(req Request) (Result, error) {
		prompted = true
		return Result{Choice: ChoiceCancel}, nil
	}
	s := startSession(t, app)
	s.ok("OPTION allow-external-password-cache")
	s.ok("SETKEYINFO n/abc")
	rep := s.exchange("GETPIN")
	if prompted {
		t.Fatal("no-prompt entry opened a prompt")
	}
	if !rep.ok || string(rep.data) != "stored-secret-value" || len(rep.status) != 0 {
		t.Fatalf("silent %+v %q", rep.status, rep.data)
	}

	var got Request
	ui.ask = func(req Request) (Result, error) {
		got = req
		return Result{Choice: ChoiceTyped, Secret: []byte("typed-secret"), NoPrompt: true}, nil
	}
	s.ok("SETERROR bad passphrase")
	rep = s.exchange("GETPIN")
	if !got.ErrorSet || got.Repeat || got.HasSecret != true || !got.CanStore {
		t.Fatalf("request %+v", got)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("stored-secret-value")) {
		t.Fatalf("request carried the secret: %s", b)
	}
	if !rep.ok || string(rep.data) != "typed-secret" {
		t.Fatalf("seterror returned %q", rep.data)
	}
	still, err := st.Get("n/abc")
	if err != nil || string(still) != "stored-secret-value" {
		t.Fatalf("stored secret changed: %q %v", still, err)
	}

	// Error text is cleared after GETPIN, so the same no-prompt entry is silent again.
	prompted = false
	ui.ask = func(Request) (Result, error) {
		prompted = true
		return Result{Choice: ChoiceCancel}, nil
	}
	rep = s.exchange("GETPIN")
	if prompted || string(rep.data) != "stored-secret-value" {
		t.Fatalf("second get %+v prompted=%v", rep, prompted)
	}

	ui.ask = func(req Request) (Result, error) {
		got = req
		return Result{Choice: ChoiceUse, Secret: []byte("not-this"), NoPrompt: true}, nil
	}
	// The flag is still set, so another GETPIN would be silent. Clear it so
	// the Use result is what the wire returns.
	if err := app.flags.Mark("n/abc", false, false); err != nil {
		t.Fatal(err)
	}
	rep = s.exchange("getpin")
	if !rep.ok || string(rep.data) != "stored-secret-value" {
		t.Fatalf("use %+v %q", rep, rep.data)
	}
	if got.Error != "" || got.ErrorSet || got.Repeat {
		t.Fatalf("stale error %+v", got)
	}
	if !app.flags.Has("n/abc") {
		t.Fatal("use did not set the no-prompt flag")
	}
	still, err = st.Get("n/abc")
	if err != nil || string(still) != "stored-secret-value" {
		t.Fatal("use rewrote the secret")
	}
}
