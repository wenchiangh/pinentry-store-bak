package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	cfg := loadConfig(path)
	if cfg.UI != "auto" || cfg.Store != "auto" {
		t.Fatalf("%+v", cfg)
	}
	body := "# comment\n\nui = gui\nstore = file\ncolor = red\nui = nope\nstore = keychain\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg = loadConfig(path)
	if cfg.UI != "gui" || cfg.Store != "file" {
		t.Fatalf("bad values stuck: %+v", cfg)
	}

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	if configPath() != filepath.Join(dir, "cfg", "pinentry-store", "config") {
		t.Fatal(configPath())
	}
	if flagPath() != filepath.Join(dir, "state", "pinentry-store", "entries.json") {
		t.Fatal(flagPath())
	}
	if secretPath() != filepath.Join(dir, "data", "pinentry-store", "secrets") {
		t.Fatal(secretPath())
	}
}

func TestFlags(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	fl := &catalog{path: filepath.Join(dir, "entries.json")}
	key := "n/a\tb%c<& >"
	if err := fl.Mark(key, true, true); err != nil {
		t.Fatal(err)
	}
	if !fl.Has(key) || !fl.Known(key) {
		t.Fatal("missing")
	}
	info, err := os.Stat(fl.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("file mode %v %v", info, err)
	}
	dinfo, err := os.Stat(dir)
	if err != nil || dinfo.Mode().Perm() != 0700 {
		t.Fatalf("dir mode %v %v", dinfo, err)
	}
	raw, err := os.ReadFile(fl.path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`\u003c`)) || bytes.Contains(raw, []byte(`\u003e`)) || bytes.Contains(raw, []byte(`\u0026`)) {
		t.Fatalf("html escaped: %s", raw)
	}
	if !bytes.Contains(raw, []byte("<& >")) || !bytes.Contains(raw, []byte("\n  {")) {
		t.Fatalf("file %s", raw)
	}
	list, err := fl.List()
	if err != nil || len(list) != 1 || list[0].ID != key || !list[0].NoPrompt {
		t.Fatalf("%+v %v", list, err)
	}
	if err := fl.Mark("n/second", false, true); err != nil {
		t.Fatal(err)
	}
	list, err = fl.List()
	if err != nil || len(list) != 2 || list[0].ID != key || list[1].ID != "n/second" || list[1].NoPrompt {
		t.Fatalf("order %+v %v", list, err)
	}
	raw, err = os.ReadFile(fl.path)
	if err != nil || !bytes.Contains(raw, []byte(`"no_prompt": false`)) {
		t.Fatalf("false mark omitted: %s %v", raw, err)
	}
	if err := fl.Mark(key, false, false); err != nil {
		t.Fatal(err)
	}
	if fl.Has(key) || !fl.Known(key) {
		t.Fatal("clear removed the line or left the mark")
	}
	if err := fl.Mark("absent", false, false); err != nil {
		t.Fatal(err)
	}
	if fl.Known("absent") {
		t.Fatal("clear created a line")
	}
	gone := &catalog{path: filepath.Join(t.TempDir(), "missing", "entries.json")}
	if err := gone.Mark("n/x", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gone.path); !os.IsNotExist(err) {
		t.Fatal("clearing a missing id created a file")
	}
	if err := fl.Ensure("n/second"); err != nil || fl.Has("n/second") || !fl.Known("n/second") {
		t.Fatal("ensure changed an existing prompting entry")
	}
	if err := fl.Ensure("n/fresh"); err != nil || fl.Has("n/fresh") || !fl.Known("n/fresh") {
		t.Fatal("ensure did not record a prompting entry")
	}
	if err := fl.Remove(key); err != nil {
		t.Fatal(err)
	}
	if fl.Known(key) {
		t.Fatal("remove left the line")
	}
	if err := fl.Remove(key); err != nil {
		t.Fatal("remove of a missing line failed")
	}

	dup := []byte("[\n  {\"id\": \"n/a\", \"no_prompt\": true},\n  {\"id\": \"n/a\", \"no_prompt\": false},\n  {\"id\": \"\", \"no_prompt\": true}\n]\n")
	dupPath := filepath.Join(t.TempDir(), "dup.json")
	if err := os.WriteFile(dupPath, dup, 0600); err != nil {
		t.Fatal(err)
	}
	dfl := &catalog{path: dupPath}
	list, err = dfl.List()
	if err != nil || len(list) != 1 || list[0].ID != "n/a" || !list[0].NoPrompt {
		t.Fatalf("duplicates %+v %v", list, err)
	}
	if dfl.Has("n/a") != true || dfl.Known("") {
		t.Fatal("empty id or later duplicate won")
	}

	blankPath := filepath.Join(t.TempDir(), "blank.json")
	if err := os.WriteFile(blankPath, []byte(" \n"), 0600); err != nil {
		t.Fatal(err)
	}
	blank := &catalog{path: blankPath}
	list, err = blank.List()
	if err != nil || len(list) != 0 || blank.Known("n/a") {
		t.Fatalf("blank %+v %v", list, err)
	}

	badPath := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badPath, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := &catalog{path: badPath}
	if _, err := bad.List(); err == nil || bad.Has("n/a") || bad.Known("n/a") {
		t.Fatal("invalid catalog was treated as data")
	}
	raw, err = os.ReadFile(badPath)
	if err != nil || string(raw) != "{" {
		t.Fatal("a read error rewrote the catalog")
	}

	if err := fl.Remove("n/second"); err != nil || fl.Remove("n/fresh") != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(fl.path)
	if err != nil || strings.TrimSpace(string(raw)) != "[]" || bytes.Contains(raw, []byte("null")) {
		t.Fatalf("empty catalog %q %v", raw, err)
	}
}

func TestFileStore(t *testing.T) {
	fs := &fileStore{path: filepath.Join(t.TempDir(), "secrets")}
	id := "n/a b"
	sec := []byte("s e\ncret")
	if err := fs.Put(id, sec); err != nil {
		t.Fatal(err)
	}
	got, err := fs.Get(id)
	if err != nil || !bytes.Equal(got, sec) {
		t.Fatalf("%q %v", got, err)
	}
	info, err := os.Stat(fs.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v %v", info, err)
	}
	if err := fs.Put(id, nil); err != nil {
		t.Fatal(err)
	}
	got, err = fs.Get(id)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty %q %v", got, err)
	}
	if err := fs.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Get(id); err == nil {
		t.Fatal("deleted secret still present")
	}
	if err := fs.Delete(id); err != nil {
		t.Fatal(err)
	}
}

func TestCommands(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	if err := os.MkdirAll(filepath.Dir(configPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath(), []byte("store=file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	st := openStore(loadConfig(configPath()))
	if _, ok := st.(*fileStore); !ok {
		t.Fatalf("store=file opened %#v", st)
	}
	code, text := captureStdout(t, func() int { return runList(false) })
	if code != 0 || text != "" {
		t.Fatalf("missing catalog list %d %q", code, text)
	}
	if _, err := os.Stat(flagPath()); !os.IsNotExist(err) {
		t.Fatal("list created the catalog")
	}
	if dispatch([]string{"prompt", "n/missing"}) != 0 {
		t.Fatal("prompt of a missing entry failed")
	}
	if _, err := os.Stat(flagPath()); !os.IsNotExist(err) {
		t.Fatal("prompt created a line")
	}
	if err := st.Put("n/ab c", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	if err := st.Put("n/unlisted", []byte("other")); err != nil {
		t.Fatal(err)
	}
	fl := &catalog{path: flagPath()}
	if err := fl.Mark("n/ab c", true, true); err != nil {
		t.Fatal(err)
	}
	if err := fl.Mark("s/plain", false, true); err != nil {
		t.Fatal(err)
	}

	code, text = captureStdout(t, func() int { return runList(false) })
	if code != 0 {
		t.Fatal(code)
	}
	if text != "n/ab c\tno-prompt\ns/plain\tprompt\n" {
		t.Fatalf("list %q", text)
	}
	code, text = captureStdout(t, func() int { return runList(true) })
	if code != 0 || text != "n/ab c\tno-prompt\n" {
		t.Fatalf("no-prompt list %d %q", code, text)
	}

	if dispatch([]string{"prompt", "n/ab", "c"}) != 0 {
		t.Fatal("prompt")
	}
	if _, err := st.Get("n/ab c"); err != nil || fl.Has("n/ab c") || !fl.Known("n/ab c") {
		t.Fatal("prompt removed the secret or the line, or left the mark")
	}
	if dispatch([]string{"delete", "n/ab", "c"}) != 0 {
		t.Fatal("delete")
	}
	if _, err := st.Get("n/ab c"); err == nil || fl.Known("n/ab c") {
		t.Fatal("delete left the secret or the line")
	}
	if _, err := st.Get("n/unlisted"); err != nil {
		t.Fatal("delete removed a different secret")
	}
	if dispatch([]string{"delete", "n/ab", "c"}) != 0 {
		t.Fatal("delete of an absent entry failed")
	}
	if dispatch([]string{"delete", "s/plain"}) != 0 || fl.Known("s/plain") {
		t.Fatal("delete of a line with no secret failed")
	}
	errOut := os.Stderr
	rerr, werr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = werr
	code = dispatch([]string{"nope"})
	werr.Close()
	os.Stderr = errOut
	io.Copy(io.Discard, rerr)
	rerr.Close()
	if code != 2 {
		t.Fatal("usage")
	}
	ln, err := splitArgs([]string{"--display", ":0", "list"})
	if err != nil || ln.display != ":0" || len(ln.rest) != 1 || ln.rest[0] != "list" {
		t.Fatalf("%+v %v", ln, err)
	}

	if err := fl.Mark("n/zzz", true, true); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(data, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", data)
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = devnull
	code = runDelete("n/zzz")
	devnull.Close()
	os.Stderr = errOut
	if code != 1 || !fl.Known("n/zzz") || !fl.Has("n/zzz") {
		t.Fatal("a failed credential delete removed the saved entry")
	}

	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	if err := st.Put("n/locked", []byte("stay")); err != nil {
		t.Fatal(err)
	}
	if err := fl.Mark("n/locked", false, true); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Dir(flagPath())
	if err := os.Chmod(stateDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(stateDir, 0700) })
	devnull, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = devnull
	code = runDelete("n/locked")
	devnull.Close()
	os.Stderr = errOut
	if err := os.Chmod(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatal("a failed catalog update should fail")
	}
	if _, err := st.Get("n/locked"); err == nil || !fl.Known("n/locked") {
		t.Fatal("catalog save failure restored the secret or dropped the line")
	}

	if err := os.WriteFile(flagPath(), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	devnull, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = devnull
	code, text = captureStdout(t, func() int { return runList(false) })
	devnull.Close()
	os.Stderr = errOut
	if code != 1 {
		t.Fatalf("bad catalog list exited %d %q", code, text)
	}
}

func captureStdout(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	code := fn()
	w.Close()
	b, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	return code, string(b)
}
