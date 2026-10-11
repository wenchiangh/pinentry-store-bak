package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestForm(t *testing.T) {
	t.Run("type enter", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN})
		res, done := press(f, runeKey('p'), runeKey('w'), keyOf(keyEnter))
		if !done || res.Choice != ChoiceTyped || string(res.Secret) != "pw" || res.Save {
			t.Fatalf("%+v done=%v", res, done)
		}
	})

	t.Run("mismatch stays", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, Repeat: true, RepeatError: "mismatch"})
		res, done := press(f, runeKey('a'), keyOf(keyEnter))
		if done || f.err != "mismatch" || f.items[f.focus] != itemRepeat {
			t.Fatalf("done=%v err=%q focus=%v", done, f.err, f.items[f.focus])
		}
		res, done = press(f, runeKey('b'), keyOf(keyEnter))
		if done {
			t.Fatalf("mismatch returned %+v", res)
		}
		if f.err != "mismatch" || f.items[f.focus] != itemRepeat {
			t.Fatalf("error %q focus %v", f.err, f.items[f.focus])
		}
		res, done = press(f, keyOf(keyBackspace), runeKey('a'), keyOf(keyEnter))
		if !done || res.Choice != ChoiceTyped || string(res.Secret) != "a" {
			t.Fatalf("%+v done=%v", res, done)
		}
	})

	t.Run("empty enter is use", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, HasSecret: true})
		res, done := press(f, keyOf(keyEnter))
		if !done || res.Choice != ChoiceUse || len(res.Secret) != 0 {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("typed enter is update", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, HasSecret: true})
		res, done := press(f, runeKey('x'), keyOf(keyEnter))
		if !done || res.Choice != ChoiceTyped || string(res.Secret) != "x" {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("esc cancels", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, HasSecret: true})
		res, done := press(f, runeKey('x'), keyOf(keyEsc))
		if !done || res.Choice != ChoiceCancel {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("no use on error or repeat", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, HasSecret: true, ErrorSet: true, CanStore: true})
		if hasItem(f, itemUse) || hasItem(f, itemUpdate) || !hasItem(f, itemSecret) || !hasItem(f, itemSave) {
			t.Fatalf("error form %v", f.items)
		}
		f = newForm(Request{Kind: KindPIN, HasSecret: true, Repeat: true, ErrorSet: true, CanStore: true})
		if hasItem(f, itemUse) || !hasItem(f, itemRepeat) || !hasItem(f, itemSkip) {
			t.Fatalf("repeat form %v", f.items)
		}
	})

	t.Run("checkboxes", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, HasSecret: true, NoPrompt: true, CanStore: true})
		if !f.skip || f.save {
			t.Fatalf("save=%v skip=%v", f.save, f.skip)
		}
		press(f, keyOf(keyTab))
		if f.items[f.focus] != itemSkip {
			t.Fatalf("focus %v", f.items[f.focus])
		}
		res, done := press(f, runeKey(' '), keyOf(keyEnter))
		if !done || res.Choice != ChoiceUse || res.Save || res.NoPrompt {
			t.Fatalf("%+v", res)
		}
		view := newForm(Request{Kind: KindPIN, HasSecret: true, CanStore: true}).view(80)
		if !strings.Contains(view, "[ OK ]") || !strings.Contains(view, "[ Cancel ]") {
			t.Fatal(view)
		}
		if strings.Contains(view, "[ Use ]") || strings.Contains(view, "[ Update ]") {
			t.Fatal(view)
		}
		if strings.Contains(view, "Use the typed passphrase") || !strings.Contains(view, "Save in the credential store") || !strings.Contains(view, "Don't ask next time") {
			t.Fatal(view)
		}
		if !strings.Contains(view, "A passphrase for this request is already saved.") || !strings.Contains(view, "Leave the field empty to use the saved passphrase.") {
			t.Fatal(view)
		}
		entry := newForm(Request{Kind: KindPIN}).view(80)
		if strings.Contains(entry, "Use the typed passphrase") || strings.Contains(entry, "Save in the credential store") || strings.Contains(entry, "Credential store") {
			t.Fatal(entry)
		}
		if !strings.Contains(entry, "Enter the passphrase for this request.") || strings.Contains(entry, "Save stores it") || strings.Contains(entry, "available after Save is checked.") {
			t.Fatal(entry)
		}
	})

	t.Run("dont ask follows save", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, CanStore: true, NoPrompt: true})
		view := f.view(80)
		if !strings.Contains(view, "\033[2m[x] Don't ask next time\033[0m") {
			t.Fatal(view)
		}
		press(f, keyOf(keyTab))
		if f.items[f.focus] != itemSave {
			t.Fatalf("focus %v", f.items[f.focus])
		}
		press(f, keyOf(keyTab))
		if f.items[f.focus] != itemOK {
			t.Fatalf("focus %v", f.items[f.focus])
		}
		press(f, keyOf(keyBacktab), runeKey(' '), keyOf(keyTab))
		if !f.save || f.items[f.focus] != itemSkip {
			t.Fatalf("save=%v focus=%v", f.save, f.items[f.focus])
		}
		res, done := press(f, runeKey(' '), keyOf(keyBacktab), runeKey(' '), keyOf(keyEnter))
		if !done || res.Save || !res.NoPrompt {
			t.Fatalf("%+v", res)
		}

		errForm := newForm(Request{Kind: KindPIN, HasSecret: true, ErrorSet: true, CanStore: true, NoPrompt: true})
		if !strings.Contains(errForm.view(80), "\033[2m[x] Don't ask next time\033[0m") {
			t.Fatal(errForm.view(80))
		}
		use := newForm(Request{Kind: KindPIN, HasSecret: true, CanStore: true, NoPrompt: true})
		if strings.Contains(use.view(80), "\033[2m[x] Don't ask next time\033[0m") || strings.Contains(use.view(80), "\033[2m[ ] Don't ask next time\033[0m") {
			t.Fatal(use.view(80))
		}
		res, done = press(use, keyOf(keyTab), runeKey(' '), keyOf(keyEnter))
		if !done || res.Choice != ChoiceUse || res.Save || res.NoPrompt {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("arrows move buttons", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, HasSecret: true})
		press(f, keyOf(keyDown))
		if f.items[f.focus] != itemOK {
			t.Fatalf("focus %v", f.items[f.focus])
		}
		press(f, keyOf(keyRight))
		if f.items[f.focus] != itemCancel {
			t.Fatalf("right %v", f.items[f.focus])
		}
		press(f, keyOf(keyRight))
		if f.items[f.focus] != itemOK {
			t.Fatalf("right wrap %v", f.items[f.focus])
		}
		press(f, keyOf(keyLeft))
		if f.items[f.focus] != itemCancel {
			t.Fatalf("left wrap %v", f.items[f.focus])
		}
		res, done := press(f, keyOf(keyEnter))
		if !done || res.Choice != ChoiceCancel {
			t.Fatalf("%+v done=%v", res, done)
		}

		field := newForm(Request{Kind: KindPIN})
		press(field, runeKey('a'), runeKey('b'), keyOf(keyLeft))
		if field.items[field.focus] != itemSecret || field.pwAt != 1 {
			t.Fatalf("cursor focus=%v at=%d", field.items[field.focus], field.pwAt)
		}
		press(field, keyOf(keyRight))
		if field.pwAt != 2 || field.items[field.focus] != itemSecret {
			t.Fatalf("cursor at=%d focus=%v", field.pwAt, field.items[field.focus])
		}
	})

	t.Run("typed passphrase", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, HasSecret: true, CanStore: true, NoPrompt: true})
		if !f.saveDisabled() {
			t.Fatal("save starts enabled")
		}
		press(f, runeKey('a'))
		if f.saveDisabled() || f.items[f.focus] != itemSecret {
			t.Fatalf("focus=%v disabled=%v", f.items[f.focus], f.saveDisabled())
		}
		press(f, keyOf(keyTab))
		if f.items[f.focus] != itemSave {
			t.Fatalf("focus %v", f.items[f.focus])
		}
		res, done := press(f, keyOf(keyEnter))
		if !done || res.Choice != ChoiceTyped || string(res.Secret) != "a" || res.Save || !res.NoPrompt {
			t.Fatalf("%+v", res)
		}

		f = newForm(Request{Kind: KindPIN, HasSecret: true, CanStore: true})
		press(f, runeKey('a'), keyOf(keyTab), runeKey(' '))
		if !f.save {
			t.Fatal("save did not check")
		}
		press(f, keyOf(keyBacktab), keyOf(keyCtrlU))
		if len(f.pw) != 0 || f.save || !f.saveDisabled() {
			t.Fatalf("pw=%q save=%v", string(f.pw), f.save)
		}
		res, done = press(f, keyOf(keyEnter))
		if !done || res.Choice != ChoiceUse || res.Save {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("operation guide keeps the caller text", func(t *testing.T) {
		use := newForm(Request{
			Kind: KindPIN, HasSecret: true, CanStore: true,
			Description: "Unlock the key.", Prompt: "Master Password:",
		}).view(100)
		if !strings.Contains(use, "Unlock the key.") || !strings.Contains(use, "Master Password:") || strings.Contains(use, "This prompt") || strings.Contains(use, "\nCredential store\n") {
			t.Fatal(use)
		}
		if !strings.Contains(use, "\033[2mA passphrase for this request is already saved.\n") {
			t.Fatal(use)
		}
		if strings.Contains(use, "Save stores what you type") || strings.Contains(use, "can be changed without typing") || strings.Contains(use, "not the text above") {
			t.Fatal(use)
		}
		if !strings.Contains(use, "Next time, return the saved passphrase without asking.") {
			t.Fatal(use)
		}
		typed := newForm(Request{Kind: KindPIN, HasSecret: true, CanStore: true})
		press(typed, runeKey('x'))
		typedPage := typed.view(100)
		if !typed.skipDisabled() || !strings.Contains(typedPage, "\033[2m[ ] Don't ask next time\033[0m") || !strings.Contains(typedPage, "This stays unchecked. It can be changed when the field is empty, or after Save is checked.") {
			t.Fatal(typedPage)
		}
		press(typed, keyOf(keyTab))
		if typed.items[typed.focus] != itemSave {
			t.Fatalf("focus %v", typed.items[typed.focus])
		}
		press(typed, keyOf(keyTab))
		if typed.items[typed.focus] != itemOK {
			t.Fatalf("focus %v", typed.items[typed.focus])
		}
		press(typed, keyOf(keyBacktab), runeKey(' '), keyOf(keyTab), runeKey(' '))
		if !typed.save || !typed.skip || typed.items[typed.focus] != itemSkip {
			t.Fatalf("save=%v skip=%v focus=%v", typed.save, typed.skip, typed.items[typed.focus])
		}
		press(typed, keyOf(keyBacktab), runeKey(' '))
		res, done := press(typed, keyOf(keyEnter))
		if typed.save || !typed.skipDisabled() || !done || res.Choice != ChoiceTyped || res.Save || res.NoPrompt || string(res.Secret) != "x" {
			t.Fatalf("%+v done=%v disabled=%v", res, done, typed.skipDisabled())
		}
		cleared := newForm(Request{Kind: KindPIN, HasSecret: true, CanStore: true})
		press(cleared, runeKey('x'), keyOf(keyCtrlU))
		if cleared.skipDisabled() || len(cleared.pw) != 0 {
			t.Fatalf("disabled=%v len=%d", cleared.skipDisabled(), len(cleared.pw))
		}
		kept := newForm(Request{Kind: KindPIN, HasSecret: true, CanStore: true, NoPrompt: true})
		press(kept, runeKey('x'))
		keptPage := kept.view(100)
		if kept.skipDisabled() || !strings.Contains(keptPage, "This stays checked for the saved passphrase. It does not apply to the text above.") {
			t.Fatal(keptPage)
		}
		press(kept, keyOf(keyTab), keyOf(keyTab), runeKey(' '))
		keptPage = kept.view(100)
		if !strings.Contains(keptPage, "Leaving this unchecked asks for the saved passphrase next time.") || strings.Contains(keptPage, "does not apply to the text above") {
			t.Fatal(keptPage)
		}
		first := newForm(Request{Kind: KindPIN, CanStore: true}).view(100)
		if strings.Contains(first, "Save stores it") || strings.Contains(first, "Don't ask next time is available") || strings.Contains(first, "Available after Save is checked.") {
			t.Fatal(first)
		}
		if !strings.Contains(first, "Stores the passphrase typed above.") || !strings.Contains(first, "This stays unchecked until Save is checked.") {
			t.Fatal(first)
		}
		marked := newForm(Request{Kind: KindPIN, CanStore: true, NoPrompt: true})
		markedPage := marked.view(100)
		if !strings.Contains(markedPage, "This stays checked. It can be changed after Save is checked.") {
			t.Fatal(markedPage)
		}
		press(marked, keyOf(keyTab), runeKey(' '))
		markedPage = marked.view(100)
		if !strings.Contains(markedPage, "Next time, return the saved passphrase without asking.") || strings.Contains(markedPage, "This stays checked.") {
			t.Fatal(markedPage)
		}
		rep := newForm(Request{
			Kind: KindPIN, Repeat: true, CanStore: true, HasSecret: true,
			Description: "Choose a new passphrase.", RepeatLabel: "Repeat passphrase:",
		}).view(100)
		if !strings.Contains(rep, "Choose a new passphrase.") || !strings.Contains(rep, "Repeat passphrase:") || !strings.Contains(rep, "Enter the passphrase twice.") || !strings.Contains(rep, "Leaving this unchecked keeps the saved passphrase.") || strings.Contains(rep, "what this request receives") {
			t.Fatal(rep)
		}
		again := newForm(Request{
			Kind: KindPIN, ErrorSet: true, HasSecret: true, Error: "Bad passphrase",
		}).view(100)
		if !strings.Contains(again, "Enter the passphrase again.") || !strings.Contains(again, "Error: Bad passphrase") || !strings.Contains(again, "The saved passphrase is not used for this request.") {
			t.Fatal(again)
		}
		msg := newForm(Request{Kind: KindConfirm, OneButton: true, Description: "Key generated."}).view(100)
		if !strings.Contains(msg, "Key generated.") || strings.Contains(msg, "This is a message.") || strings.Contains(msg, "Continue when you have read it.") || strings.Contains(msg, "Esc cancels.") || strings.Contains(msg, "move in a field") {
			t.Fatal(msg)
		}
		if !strings.Contains(msg, "Enter, Esc, or Ctrl-C continues.") {
			t.Fatal(msg)
		}
		ask := newForm(Request{Kind: KindConfirm, Description: "Overwrite the file?", OK: "Yes", Cancel: "No"}).view(100)
		if !strings.Contains(ask, "Overwrite the file?") || strings.Contains(ask, "This request needs a confirmation.") || strings.Contains(ask, "accepts it") || strings.Contains(ask, "move in a field") {
			t.Fatal(ask)
		}
		if !strings.Contains(ask, "Left and Right move between buttons.") || !strings.Contains(ask, "Esc cancels.") {
			t.Fatal(ask)
		}
		narrow := stripANSI(newForm(Request{Kind: KindPIN, CanStore: true, Repeat: true, HasSecret: true}).view(40))
		for _, line := range strings.Split(narrow, "\n") {
			if len(line) > 39 {
				t.Fatalf("line longer than the page: %q", line)
			}
		}
	})

	t.Run("enter confirms a checkbox", func(t *testing.T) {
		f := newForm(Request{Kind: KindPIN, CanStore: true})
		press(f, keyOf(keyTab))
		if f.items[f.focus] != itemSave {
			t.Fatalf("focus %v", f.items[f.focus])
		}
		res, done := press(f, keyOf(keyEnter))
		if !done || res.Choice != ChoiceTyped || res.Save || f.save {
			t.Fatalf("%+v save=%v", res, f.save)
		}
		f = newForm(Request{Kind: KindPIN, CanStore: true})
		press(f, keyOf(keyTab), runeKey(' '))
		if !f.save || f.items[f.focus] != itemSave {
			t.Fatalf("save=%v focus=%v", f.save, f.items[f.focus])
		}
	})

	t.Run("confirm", func(t *testing.T) {
		f := newForm(Request{Kind: KindConfirm, OneButton: true, Focus: -1})
		if len(f.items) != 1 || f.items[0] != itemOK {
			t.Fatalf("%v", f.items)
		}
		f = newForm(Request{Kind: KindConfirm, NotOK: "No", Focus: -1})
		res, done := press(f, keyOf(keyEnter))
		if !done || res.Choice != ChoiceCancel {
			t.Fatalf("focus cancel %+v items=%v focus=%d", res, f.items, f.focus)
		}
	})
}

func TestScreenReleaseIsIdle(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	s := newScreen(r, w)
	s.release()
	if err := s.begin(); err == nil {
		s.release()
		t.Fatal("a pipe accepted raw mode")
	}
	s.release()
}

func TestDrawCRLF(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "screen")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := &screen{out: f}
	if err := s.draw("a\nb\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("a\r\nb\r\n")) {
		t.Fatalf("newline did not return to column zero: %q", body)
	}
	if crlf("a\r\nb") != "a\r\nb" {
		t.Fatal("carriage return was doubled")
	}
}

func TestTakeKey(t *testing.T) {
	k, rest, st := takeKey([]byte("é"), false)
	if st != takeGot || k.kind != keyRune || k.r != 'é' || len(rest) != 0 {
		t.Fatalf("%#v %q %v", k, rest, st)
	}
	if _, _, st = takeKey([]byte{0x1b}, false); st != takeNeed {
		t.Fatal("lone esc should wait")
	}
	k, _, st = takeKey([]byte{0x1b}, true)
	if st != takeGot || k.kind != keyEsc {
		t.Fatalf("idle esc %#v", k)
	}
	k, _, st = takeKey([]byte{0x1b, '[', 'A'}, false)
	if k.kind != keyUp {
		t.Fatalf("up %#v", k)
	}
	k, _, st = takeKey([]byte{0x1b, '[', '3', '~'}, false)
	if k.kind != keyDelete {
		t.Fatalf("delete %#v", k)
	}
	k, _, st = takeKey([]byte{0x1b, 'O', 'D'}, false)
	if k.kind != keyLeft {
		t.Fatalf("left %#v", k)
	}
	k, _, st = takeKey([]byte{0x1b, '[', 'Z'}, false)
	if k.kind != keyBacktab {
		t.Fatalf("backtab %#v", k)
	}
	k, rest, st = takeKey([]byte("\x1b[200~a b\x1b[201~X"), false)
	if st != takeGot || k.kind != keyPaste || k.paste != "a b" || string(rest) != "X" {
		t.Fatalf("paste %#v %q", k, rest)
	}
	if _, _, st = takeKey([]byte("\x1b[200~hi"), false); st != takeNeed {
		t.Fatal("partial paste")
	}
	if _, _, st = takeKey([]byte{0xc3}, false); st != takeNeed {
		t.Fatal("partial utf-8")
	}
}

func press(f *form, keys ...key) (Result, bool) {
	var res Result
	var done bool
	for _, k := range keys {
		res, done = f.handle(k)
		if done {
			return res, true
		}
	}
	return res, false
}

func runeKey(r rune) key  { return key{kind: keyRune, r: r} }
func keyOf(k keyKind) key { return key{kind: k} }

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func hasItem(f *form, it itemKind) bool {
	for _, x := range f.items {
		if x == it {
			return true
		}
	}
	return false
}
