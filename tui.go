package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type itemKind int

const (
	itemSecret itemKind = iota
	itemRepeat
	itemSave
	itemSkip
	itemUse
	itemUpdate
	itemOK
	itemNotOK
	itemCancel
)

type form struct {
	req   Request
	items []itemKind
	focus int
	pw    []rune
	pwAt  int
	rep   []rune
	repAt int
	save  bool
	skip  bool
	err   string
}

func newForm(req Request) *form {
	if req.Prompt == "" {
		req.Prompt = "Passphrase:"
	}
	if req.OK == "" {
		req.OK = "OK"
	}
	if req.Cancel == "" {
		req.Cancel = "Cancel"
	}
	if req.Repeat && req.RepeatLabel == "" {
		req.RepeatLabel = "Repeat:"
	}
	if req.RepeatError == "" {
		req.RepeatError = "Passphrases do not match."
	}
	f := &form{req: req, skip: req.NoPrompt, err: req.Error}
	if req.Kind == KindConfirm {
		f.items = []itemKind{itemOK}
		if !req.OneButton && req.NotOK != "" {
			f.items = append(f.items, itemNotOK)
		}
		if !req.OneButton {
			f.items = append(f.items, itemCancel)
		}
		if req.Focus < 0 {
			for i, it := range f.items {
				if it == itemCancel {
					f.focus = i
				}
			}
		}
		return f
	}
	entry := req.Repeat || req.ErrorSet || !req.HasSecret
	if entry {
		f.items = append(f.items, itemSecret)
		if req.Repeat {
			f.items = append(f.items, itemRepeat)
		}
		if req.CanStore {
			f.items = append(f.items, itemSave, itemSkip)
		}
		f.items = append(f.items, itemOK, itemCancel)
		return f
	}
	f.items = []itemKind{itemSecret}
	if req.CanStore {
		f.items = append(f.items, itemSave, itemSkip)
	}
	f.items = append(f.items, itemOK, itemCancel)
	return f
}

func (f *form) handle(k key) (Result, bool) {
	switch k.kind {
	case keyNone:
		return Result{}, false
	case keyCtrlC, keyEsc:
		return Result{Choice: ChoiceCancel}, true
	case keyTab, keyDown:
		f.moveFocus(1)
	case keyBacktab, keyUp:
		f.moveFocus(-1)
	case keyEnter:
		return f.activate()
	case keyRune:
		if k.r == ' ' && f.onCheck() {
			f.toggle()
			return Result{}, false
		}
		if k.r == ' ' && f.onButton() {
			return f.activate()
		}
		f.insert(k.r)
	case keyPaste:
		for _, r := range k.paste {
			if r != '\r' && r != '\n' {
				f.insert(r)
			}
		}
	case keyBackspace:
		f.backspace()
	case keyDelete:
		f.deleteAt()
	case keyLeft:
		if f.onButton() {
			f.moveButton(-1)
			break
		}
		f.move(-1)
	case keyRight:
		if f.onButton() {
			f.moveButton(1)
			break
		}
		f.move(1)
	case keyHome:
		f.moveHome()
	case keyEnd:
		f.moveEnd()
	case keyCtrlU:
		f.clearField()
	}
	return Result{}, false
}

func (f *form) onCheck() bool {
	it := f.items[f.focus]
	if f.rowDisabled(it) {
		return false
	}
	return it == itemSave || it == itemSkip
}

// A new-passphrase form keeps don't-ask visible, but it follows Save.
// On the use form, turning a mark on follows Save once the field has text.
// An existing mark can still be cleared without saving.
func (f *form) skipDisabled() bool {
	entry := f.req.Repeat || f.req.ErrorSet || !f.req.HasSecret
	if entry && !f.save {
		return true
	}
	return f.useForm() && len(f.pw) > 0 && !f.save && !f.req.NoPrompt
}

func (f *form) noPrompt() bool {
	if f.skipDisabled() {
		return f.req.NoPrompt
	}
	return f.skip
}

func (f *form) useForm() bool {
	return f.req.Kind != KindConfirm && f.req.HasSecret && !f.req.Repeat && !f.req.ErrorSet
}

// On the use form, an empty field uses the saved passphrase, so Save does not apply.
func (f *form) saveDisabled() bool {
	return f.useForm() && len(f.pw) == 0
}

func (f *form) saveOn() bool {
	if f.saveDisabled() {
		return false
	}
	return f.save
}

func (f *form) rowDisabled(it itemKind) bool {
	switch it {
	case itemSkip:
		return f.skipDisabled()
	case itemSave:
		return f.saveDisabled()
	default:
		return false
	}
}

func (f *form) afterSecretEdit() {
	if f.useForm() && len(f.pw) == 0 {
		f.save = false
	}
}

func (f *form) moveFocus(d int) {
	n := len(f.items)
	if n == 0 {
		return
	}
	for range n {
		f.focus = (f.focus + d + n) % n
		if !f.rowDisabled(f.items[f.focus]) {
			return
		}
	}
}

func (f *form) onButton() bool {
	return isButton(f.items[f.focus])
}

// moveButton moves among the buttons. Left and Right stay on that row.
func (f *form) moveButton(d int) {
	n := len(f.items)
	if n == 0 || !f.onButton() {
		return
	}
	i := f.focus
	for {
		i = (i + d + n) % n
		if i == f.focus {
			return
		}
		if isButton(f.items[i]) {
			f.focus = i
			return
		}
	}
}

func (f *form) activate() (Result, bool) {
	switch f.items[f.focus] {
	case itemCancel:
		return Result{Choice: ChoiceCancel}, true
	case itemOK:
		if f.req.Kind == KindConfirm {
			return Result{Choice: ChoiceOK}, true
		}
		return f.submitEntry()
	case itemNotOK:
		return Result{Choice: ChoiceNotOK}, true
	default:
		return f.submitEntry()
	}
}

func (f *form) submitEntry() (Result, bool) {
	if f.req.Kind == KindConfirm {
		return Result{Choice: ChoiceOK}, true
	}
	if f.useForm() && len(f.pw) == 0 {
		return Result{Choice: ChoiceUse, Save: f.saveOn(), NoPrompt: f.noPrompt()}, true
	}
	if f.req.Repeat && string(f.pw) != string(f.rep) {
		f.err = f.req.RepeatError
		for i, it := range f.items {
			if it == itemRepeat {
				f.focus = i
				f.repAt = len(f.rep)
				break
			}
		}
		return Result{}, false
	}
	return f.typed()
}

func (f *form) typed() (Result, bool) {
	return Result{
		Choice:   ChoiceTyped,
		Secret:   []byte(string(f.pw)),
		Save:     f.saveOn(),
		NoPrompt: f.noPrompt(),
	}, true
}

func (f *form) insert(r rune) {
	buf, at := f.field()
	if buf == nil {
		return
	}
	f.edited()
	*buf = append((*buf)[:*at], append([]rune{r}, (*buf)[*at:]...)...)
	*at++
	if buf == &f.pw {
		f.afterSecretEdit()
	}
}

func (f *form) backspace() {
	buf, at := f.field()
	if buf == nil || *at == 0 {
		return
	}
	f.edited()
	*buf = append((*buf)[:*at-1], (*buf)[*at:]...)
	*at--
	if buf == &f.pw {
		f.afterSecretEdit()
	}
}

func (f *form) deleteAt() {
	buf, at := f.field()
	if buf == nil || *at >= len(*buf) {
		return
	}
	f.edited()
	*buf = append((*buf)[:*at], (*buf)[*at+1:]...)
	if buf == &f.pw {
		f.afterSecretEdit()
	}
}

func (f *form) move(d int) {
	_, at := f.field()
	if at == nil {
		return
	}
	buf, _ := f.field()
	*at += d
	if *at < 0 {
		*at = 0
	}
	if *at > len(*buf) {
		*at = len(*buf)
	}
}

func (f *form) moveHome() {
	_, at := f.field()
	if at != nil {
		*at = 0
	}
}

func (f *form) moveEnd() {
	buf, at := f.field()
	if at != nil {
		*at = len(*buf)
	}
}

func (f *form) clearField() {
	buf, at := f.field()
	if buf == nil {
		return
	}
	f.edited()
	*buf = nil
	*at = 0
	if buf == &f.pw {
		f.afterSecretEdit()
	}
}

func (f *form) edited() {
	if f.err == f.req.RepeatError {
		f.err = f.req.Error
	}
}

func (f *form) toggle() {
	switch f.items[f.focus] {
	case itemSave:
		if f.saveDisabled() {
			return
		}
		f.save = !f.save
	case itemSkip:
		if f.skipDisabled() {
			return
		}
		f.skip = !f.skip
	}
}

func (f *form) field() (*[]rune, *int) {
	switch f.items[f.focus] {
	case itemSecret:
		return &f.pw, &f.pwAt
	case itemRepeat:
		return &f.rep, &f.repAt
	default:
		return nil, nil
	}
}

func (f *form) view(width int) string {
	if width < 40 {
		width = 40
	}
	var b strings.Builder
	title := f.req.Title
	if title == "" {
		title = "pinentry-store"
	}
	b.WriteString(title)
	b.WriteString("\n\n")
	if f.req.Description != "" {
		b.WriteString(wrapText(f.req.Description, width-1))
		b.WriteString("\n\n")
	}
	if lines := f.guide(); len(lines) > 0 {
		b.WriteString("\033[2m")
		for _, line := range lines {
			b.WriteString(wrapText(line, width-1))
			b.WriteByte('\n')
		}
		b.WriteString("\033[0m\n")
	}
	if f.err != "" {
		b.WriteString("Error: ")
		b.WriteString(f.err)
		b.WriteString("\n\n")
	}
	for i := 0; i < len(f.items); {
		if isButton(f.items[i]) {
			for i < len(f.items) && isButton(f.items[i]) {
				if i > 0 && isButton(f.items[i-1]) {
					b.WriteString("  ")
				}
				f.writeButton(&b, buttonLabel(f, f.items[i]), i == f.focus)
				i++
			}
			b.WriteByte('\n')
			continue
		}
		f.writeItem(&b, i, width)
		b.WriteByte('\n')
		i++
	}
	b.WriteString("\n\033[2m")
	b.WriteString(wrapText(f.keyHint(), width-1))
	b.WriteString("\033[0m\n")
	return b.String()
}

func (f *form) keyHint() string {
	if f.req.Kind == KindConfirm && f.req.OneButton {
		return "Enter, Esc, or Ctrl-C continues."
	}
	var parts []string
	if f.hasField() {
		parts = append(parts, "Up and Down move between rows. Left and Right move in a field or between buttons.")
	} else if f.buttonCount() > 1 {
		parts = append(parts, "Left and Right move between buttons.")
	}
	if f.hasCheck() {
		parts = append(parts, "Space toggles a checkbox.")
	}
	parts = append(parts, "Enter confirms. Esc cancels.")
	return strings.Join(parts, " ")
}

func (f *form) hasField() bool {
	for _, it := range f.items {
		if it == itemSecret || it == itemRepeat {
			return true
		}
	}
	return false
}

func (f *form) buttonCount() int {
	n := 0
	for _, it := range f.items {
		if isButton(it) {
			n++
		}
	}
	return n
}

func (f *form) guide() []string {
	switch {
	case f.req.Kind == KindConfirm:
		return nil
	case f.req.Repeat:
		return []string{
			"Enter the passphrase twice.",
			"The two entries must match.",
		}
	case f.req.ErrorSet:
		lines := []string{
			"Enter the passphrase again.",
			"The previous passphrase was not accepted.",
		}
		if f.req.HasSecret {
			lines = append(lines, "The saved passphrase is not used for this request.")
		}
		return lines
	case f.useForm():
		return []string{
			"A passphrase for this request is already saved.",
			"Leave the field empty to use the saved passphrase.",
			"Type a passphrase to use what you type instead.",
		}
	default:
		return []string{"Enter the passphrase for this request."}
	}
}

func (f *form) hasCheck() bool {
	for _, it := range f.items {
		if it == itemSave || it == itemSkip {
			return true
		}
	}
	return false
}

func isButton(it itemKind) bool {
	switch it {
	case itemOK, itemNotOK, itemCancel:
		return true
	default:
		return false
	}
}

func (f *form) writeItem(b *strings.Builder, i, width int) {
	on := i == f.focus
	switch f.items[i] {
	case itemSecret:
		writeMasked(b, f.req.Prompt, f.pw, f.pwAt, on)
	case itemRepeat:
		writeMasked(b, f.req.RepeatLabel, f.rep, f.repAt, on)
	case itemSave:
		if f.saveDisabled() {
			writeDisabledCheck(b, "Save in the credential store", false)
			writeHint(b, "Type a passphrase before this can be checked.", width)
			break
		}
		writeCheck(b, "Save in the credential store", f.save, on)
		if f.req.Repeat && f.req.HasSecret {
			writeHint(b, "Stores the passphrase typed above. Leaving this unchecked keeps the saved passphrase.", width)
			break
		}
		writeHint(b, "Stores the passphrase typed above.", width)
	case itemSkip:
		if f.skipDisabled() {
			writeDisabledCheck(b, "Don't ask next time", f.req.NoPrompt)
		} else {
			writeCheck(b, "Don't ask next time", f.skip, on)
		}
		writeHint(b, f.skipHint(), width)
	default:
		f.writeButton(b, buttonLabel(f, f.items[i]), on)
	}
}

func buttonLabel(f *form, it itemKind) string {
	switch it {
	case itemOK:
		return f.req.OK
	case itemNotOK:
		return f.req.NotOK
	case itemCancel:
		return f.req.Cancel
	default:
		return ""
	}
}

func (f *form) writeButton(b *strings.Builder, label string, on bool) {
	if on {
		b.WriteString("\033[7m[ ")
		b.WriteString(label)
		b.WriteString(" ]\033[0m")
		return
	}
	b.WriteString("[ ")
	b.WriteString(label)
	b.WriteString(" ]")
}

func (f *form) skipHint() string {
	if f.skipDisabled() {
		if f.req.NoPrompt {
			return "This stays checked. It can be changed after Save is checked."
		}
		if f.useForm() {
			return "This stays unchecked. It can be changed when the field is empty, or after Save is checked."
		}
		return "This stays unchecked until Save is checked."
	}
	if f.useForm() && len(f.pw) > 0 && !f.save {
		if f.skip {
			return "This stays checked for the saved passphrase. It does not apply to the text above."
		}
		return "Leaving this unchecked asks for the saved passphrase next time. The text above is not stored."
	}
	return "Next time, return the saved passphrase without asking."
}

func writeHint(b *strings.Builder, text string, width int) {
	inner := width - 3
	for _, line := range strings.Split(wrapText(text, inner), "\n") {
		b.WriteString("\n\033[2m  ")
		b.WriteString(line)
		b.WriteString("\033[0m")
	}
}

func writeDisabledCheck(b *strings.Builder, label string, checked bool) {
	box := "[ ]"
	if checked {
		box = "[x]"
	}
	fmt.Fprintf(b, "\033[2m%s %s\033[0m", box, label)
}

func writeCheck(b *strings.Builder, label string, checked, on bool) {
	box := "[ ]"
	if checked {
		box = "[x]"
	}
	if on {
		fmt.Fprintf(b, "\033[7m%s\033[0m %s", box, label)
		return
	}
	fmt.Fprintf(b, "%s %s", box, label)
}

func writeMasked(b *strings.Builder, label string, pw []rune, at int, on bool) {
	b.WriteString(label)
	b.WriteByte(' ')
	for i := range pw {
		if on && i == at {
			b.WriteString("\033[7m*\033[0m")
		} else {
			b.WriteByte('*')
		}
	}
	if on && at == len(pw) {
		b.WriteString("\033[7m \033[0m")
	}
}

func wrapText(s string, width int) string {
	if width < 20 {
		width = 20
	}
	var b strings.Builder
	for pi, para := range strings.Split(s, "\n") {
		if pi > 0 {
			b.WriteByte('\n')
		}
		line := 0
		for _, word := range strings.Fields(para) {
			if line > 0 && line+1+len(word) > width {
				b.WriteByte('\n')
				line = 0
			}
			if line > 0 {
				b.WriteByte(' ')
				line++
			}
			b.WriteString(word)
			line += len(word)
		}
	}
	return b.String()
}

type termUI struct {
	sc    *screen
	files []*os.File
	// hold keeps the tty for the life of a console window.
	// The caller's own tty is released when each dialog returns.
	hold bool
}

func (t *termUI) grab() error {
	return t.sc.begin()
}

func (t *termUI) release() {
	if t.hold {
		return
	}
	t.sc.release()
	setRestore(nil)
}

func (t *termUI) Ask(ctx context.Context, req Request) (Result, error) {
	if err := t.grab(); err != nil {
		return Result{}, err
	}
	defer t.release()
	f := newForm(req)
	for {
		if err := t.sc.draw(f.view(t.sc.width())); err != nil {
			return Result{}, err
		}
		k, err := t.sc.readKey(ctx)
		if err != nil {
			return Result{Choice: ChoiceCancel}, err
		}
		if res, done := f.handle(k); done {
			return res, nil
		}
	}
}

func (t *termUI) Notice(ctx context.Context, text string) error {
	if err := t.grab(); err != nil {
		return err
	}
	defer t.release()
	width := t.sc.width()
	if width < 40 {
		width = 40
	}
	var b strings.Builder
	if text != "" {
		b.WriteString(wrapText(text, width-1))
	}
	b.WriteString("\n\n\033[2m")
	b.WriteString(wrapText("Press Enter, Esc, or Ctrl-C to continue.", width-1))
	b.WriteString("\033[0m\n")
	view := b.String()
	for {
		if err := t.sc.draw(view); err != nil {
			return err
		}
		k, err := t.sc.readKey(ctx)
		if err != nil {
			return err
		}
		switch k.kind {
		case keyEnter, keyEsc, keyCtrlC:
			return nil
		}
	}
}

func (t *termUI) Close() error {
	t.sc.release()
	setRestore(nil)
	var err error
	for _, f := range t.files {
		if e := f.Close(); err == nil {
			err = e
		}
	}
	return err
}
