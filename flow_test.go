package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestDeliverPIN(t *testing.T) {
	ctx := context.Background()

	t.Run("cancel", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		st.mem.Put("n/a", []byte("old"))
		if err := fl.Mark("n/a", true, true); err != nil {
			t.Fatal(err)
		}
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), false, Result{Choice: ChoiceCancel})
		if !canceled || pin != nil || st.put != 0 || st.del != 0 || !fl.Has("n/a") || !fl.Known("n/a") {
			t.Fatalf("pin=%q canceled=%v", pin, canceled)
		}
	})

	t.Run("no key", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		pin, canceled := deliverPIN(ctx, ui, st, fl, "", false, nil, false, Result{Choice: ChoiceTyped, Secret: []byte("pw"), Save: true, NoPrompt: true})
		if canceled || string(pin) != "pw" || st.get != 0 || st.put != 0 || st.del != 0 || fl.Known("") {
			t.Fatalf("pin=%q canceled=%v puts=%d known=%v", pin, canceled, st.put, fl.Known(""))
		}
	})

	t.Run("repeat save", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		st.mem.Put("n/a", []byte("old"))
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), true, Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: true, NoPrompt: true})
		got, err := st.mem.Get("n/a")
		if canceled || string(pin) != "new" || err != nil || string(got) != "new" || !fl.Has("n/a") || !fl.Known("n/a") || len(ui.notices) != 0 {
			t.Fatalf("pin=%q got=%q flag=%v notices=%v", pin, got, fl.Has("n/a"), ui.notices)
		}
		pin, _ = deliverPIN(ctx, ui, st, fl, "n/a", true, got, true, Result{Choice: ChoiceTyped, Secret: []byte("newer"), Save: true, NoPrompt: false})
		if string(pin) != "newer" || fl.Has("n/a") || !fl.Known("n/a") {
			t.Fatal("unchecked box removed the entry or left the mark")
		}
	})

	t.Run("repeat no save", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		st.mem.Put("n/a", []byte("old"))
		if err := fl.Mark("n/a", true, true); err != nil {
			t.Fatal(err)
		}
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), true, Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: false, NoPrompt: true})
		got, err := st.mem.Get("n/a")
		if canceled || string(pin) != "new" || err != nil || string(got) != "old" || !fl.Has("n/a") || !fl.Known("n/a") || st.del != 0 || len(ui.notices) != 0 {
			t.Fatalf("pin=%q got=%q err=%v known=%v del=%d notices=%v", pin, got, err, fl.Known("n/a"), st.del, ui.notices)
		}
	})

	t.Run("repeat save rejected", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		st.mem.Put("n/a", []byte("old"))
		if err := fl.Mark("n/a", true, true); err != nil {
			t.Fatal(err)
		}
		st.putErr = errors.New("full")
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), true, Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: true, NoPrompt: true})
		got, err := st.mem.Get("n/a")
		if canceled || string(pin) != "new" || err != nil || string(got) != "old" || !fl.Has("n/a") || !fl.Known("n/a") || st.del != 0 {
			t.Fatalf("pin=%q got=%q err=%v known=%v del=%d", pin, got, err, fl.Known("n/a"), st.del)
		}
		if len(ui.notices) != 1 || ui.notices[0] != "Could not save the passphrase." {
			t.Fatalf("notices %v", ui.notices)
		}
	})

	t.Run("repeat save rejected clears an unchecked mark", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		st.mem.Put("n/a", []byte("old"))
		if err := fl.Mark("n/a", true, true); err != nil {
			t.Fatal(err)
		}
		st.putErr = errors.New("full")
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), true, Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: true})
		got, err := st.mem.Get("n/a")
		if canceled || string(pin) != "new" || err != nil || string(got) != "old" || fl.Has("n/a") || !fl.Known("n/a") || st.del != 0 {
			t.Fatalf("pin=%q got=%q known=%v has=%v del=%d", pin, got, fl.Known("n/a"), fl.Has("n/a"), st.del)
		}
		if len(ui.notices) != 1 || ui.notices[0] != "Could not save the passphrase." {
			t.Fatalf("notices %v", ui.notices)
		}
	})

	t.Run("use", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		st.mem.Put("n/a", []byte("old"))
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), false, Result{Choice: ChoiceUse, Secret: []byte("nope"), NoPrompt: true})
		got, _ := st.mem.Get("n/a")
		if canceled || string(pin) != "old" || st.put != 0 || string(got) != "old" || !fl.Has("n/a") || !fl.Known("n/a") {
			t.Fatalf("pin=%q put=%d flag=%v", pin, st.put, fl.Has("n/a"))
		}
		pin[0] = 'X'
		got, _ = st.mem.Get("n/a")
		if string(got) != "old" {
			t.Fatal("returned secret aliases the store")
		}
		_, canceled = deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), false, Result{Choice: ChoiceUse, NoPrompt: false})
		if canceled || fl.Has("n/a") || !fl.Known("n/a") {
			t.Fatal("clearing the box removed the entry or left the mark")
		}
	})

	t.Run("typed save", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", false, nil, false, Result{Choice: ChoiceTyped, Secret: []byte("pw"), Save: true, NoPrompt: true})
		got, err := st.mem.Get("n/a")
		if canceled || string(pin) != "pw" || err != nil || !bytes.Equal(got, []byte("pw")) || !fl.Has("n/a") || !fl.Known("n/a") {
			t.Fatalf("pin=%q got=%q flag=%v", pin, got, fl.Has("n/a"))
		}
	})

	t.Run("typed save still prompts", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", false, nil, false, Result{Choice: ChoiceTyped, Secret: []byte("pw"), Save: true, NoPrompt: false})
		got, err := st.mem.Get("n/a")
		if canceled || string(pin) != "pw" || err != nil || string(got) != "pw" || fl.Has("n/a") || !fl.Known("n/a") {
			t.Fatalf("pin=%q got=%q has=%v known=%v", pin, got, fl.Has("n/a"), fl.Known("n/a"))
		}
	})

	t.Run("typed no save", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		st.mem.Put("n/a", []byte("old"))
		pin, _ := deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), false, Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: false, NoPrompt: true})
		got, _ := st.mem.Get("n/a")
		if string(pin) != "new" || string(got) != "old" || fl.Has("n/a") || fl.Known("n/a") || st.put != 0 {
			t.Fatalf("pin=%q got=%q known=%v", pin, got, fl.Known("n/a"))
		}
		if err := fl.Mark("n/a", true, true); err != nil {
			t.Fatal(err)
		}
		deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), false, Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: false, NoPrompt: false})
		if fl.Has("n/a") || !fl.Known("n/a") {
			t.Fatal("unchecked box removed the entry or left the mark")
		}
		got, _ = st.mem.Get("n/a")
		if string(got) != "old" {
			t.Fatal("unsaved update removed the secret")
		}
	})

	t.Run("typed save rejected", func(t *testing.T) {
		st, fl, ui := newFlow(t)
		st.mem.Put("n/a", []byte("old"))
		st.putErr = errors.New("full")
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), false, Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: true, NoPrompt: true})
		got, _ := st.mem.Get("n/a")
		if canceled || string(pin) != "new" || string(got) != "old" || fl.Has("n/a") || fl.Known("n/a") {
			t.Fatalf("pin=%q got=%q known=%v", pin, got, fl.Known("n/a"))
		}
		if len(ui.notices) != 1 || ui.notices[0] != "Could not save the passphrase." {
			t.Fatalf("notices %v", ui.notices)
		}
		if err := fl.Mark("n/a", true, true); err != nil {
			t.Fatal(err)
		}
		ui.notices = nil
		pin, canceled = deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), false, Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: true, NoPrompt: true})
		got, _ = st.mem.Get("n/a")
		if canceled || string(pin) != "new" || string(got) != "old" || !fl.Has("n/a") || !fl.Known("n/a") {
			t.Fatal("failed save dropped an existing entry or the secret")
		}
		ui.notices = nil
		pin, canceled = deliverPIN(ctx, ui, st, fl, "n/a", true, []byte("old"), false, Result{Choice: ChoiceTyped, Secret: []byte("new"), Save: true, NoPrompt: false})
		got, _ = st.mem.Get("n/a")
		if canceled || string(pin) != "new" || string(got) != "old" || fl.Has("n/a") || !fl.Known("n/a") {
			t.Fatal("failed save removed the entry or the secret")
		}
		if len(ui.notices) != 1 || ui.notices[0] != "Could not save the passphrase." {
			t.Fatalf("notices %v", ui.notices)
		}
	})

	t.Run("notice failure still returns the pin", func(t *testing.T) {
		st, fl, _ := newFlow(t)
		st.putErr = errors.New("full")
		ui := failNotice{}
		pin, canceled := deliverPIN(ctx, ui, st, fl, "n/a", false, nil, false, Result{Choice: ChoiceTyped, Secret: []byte("pw"), Save: true})
		if canceled || string(pin) != "pw" || fl.Known("n/a") {
			t.Fatalf("pin=%q canceled=%v known=%v", pin, canceled, fl.Known("n/a"))
		}
	})
}

func newFlow(t *testing.T) (*recStore, *catalog, *scriptUI) {
	t.Helper()
	return newRecStore(), &catalog{path: filepath.Join(t.TempDir(), "entries.json")}, &scriptUI{}
}

type failNotice struct{}

func (failNotice) Ask(context.Context, Request) (Result, error) {
	return Result{Choice: ChoiceCancel}, nil
}
func (failNotice) Notice(context.Context, string) error { return errors.New("no window") }
func (failNotice) Close() error                         { return nil }
