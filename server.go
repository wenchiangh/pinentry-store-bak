package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// version is reported by GETINFO version. A release build replaces it with -X.
var version = "0.1.0"

type session struct {
	title, desc, prompt, errText string
	key                          string
	repeat                       bool
	repeatLabel, repeatErr       string
	ok, notOK, cancelText        string
	timeout                      int
	ttyName, ttyType, display    string
	defaultOK                    string
	defaultCancel                string
	defaultPrompt                string
}

func (s *session) reset() {
	s.title, s.desc, s.prompt, s.errText = "", "", "", ""
	s.key = ""
	s.repeat = false
	s.repeatLabel, s.repeatErr = "", ""
	s.ok, s.notOK, s.cancelText = "", "", ""
}

type app struct {
	store      store
	flags      *catalog
	ui         UI
	cfg        config
	argDisplay string
	sess       session
}

func installStopRestore() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		callRestore()
		os.Exit(1)
	}()
}

func serve(in io.Reader, out io.Writer, app *app) error {
	a := newAssuan(in, out)
	if err := a.greet(); err != nil {
		return err
	}
	if app.ui != nil {
		defer app.ui.Close()
	}
	// Same loop as assuan_process: finish the current command and write its
	// reply before reading the next line. EOF means the client has no more
	// commands; it does not cancel the command already in progress.
	ctx := context.Background()
	for {
		line, err := a.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		bye, err := app.handle(ctx, a, line)
		if err != nil {
			return err
		}
		if bye {
			return nil
		}
	}
}

func (app *app) handle(ctx context.Context, a *assuan, line string) (bool, error) {
	if line == "" || strings.HasPrefix(line, "#") {
		return false, nil
	}
	cmd, arg, ok := parseCommand(line)
	if !ok {
		return false, a.err(gpgAssSyntax)
	}
	switch cmd {
	case "NOP":
		return false, a.ok()
	case "BYE":
		return true, a.okClosing()
	case "RESET":
		app.sess.reset()
		return false, a.ok()
	case "OPTION":
		return false, app.option(a, arg)
	case "SETDESC":
		app.sess.desc = unescapePercent(arg)
		return false, a.ok()
	case "SETPROMPT":
		app.sess.prompt = unescapePercent(arg)
		return false, a.ok()
	case "SETTITLE":
		app.sess.title = unescapePercent(arg)
		return false, a.ok()
	case "SETKEYINFO":
		if arg == "" || arg == "--clear" {
			app.sess.key = ""
		} else {
			app.sess.key = arg
		}
		return false, a.ok()
	case "SETREPEAT":
		app.sess.repeat = true
		app.sess.repeatLabel = unescapePercent(arg)
		return false, a.ok()
	case "SETREPEATERROR":
		app.sess.repeatErr = unescapePercent(arg)
		return false, a.ok()
	case "SETREPEATOK":
		return false, a.ok()
	case "SETERROR":
		app.sess.errText = unescapePercent(arg)
		return false, a.ok()
	case "SETOK":
		app.sess.ok = unescapePercent(arg)
		return false, a.ok()
	case "SETNOTOK":
		app.sess.notOK = unescapePercent(arg)
		return false, a.ok()
	case "SETCANCEL":
		app.sess.cancelText = unescapePercent(arg)
		return false, a.ok()
	case "SETTIMEOUT":
		if arg != "" {
			app.sess.timeout = atoi(arg)
		}
		return false, a.ok()
	case "SETQUALITYBAR", "SETQUALITYBAR_TT", "SETGENPIN", "SETGENPIN_TT":
		return false, a.ok()
	case "GETINFO":
		return false, app.getinfo(a, arg)
	case "CLEARPASSPHRASE":
		return false, app.clearPassphrase(a, arg)
	case "GETPIN":
		return false, app.finish(a, app.getPIN(ctx, a))
	case "MESSAGE":
		return false, app.finish(a, app.confirm(ctx, a, "--one-button"))
	case "CONFIRM":
		return false, app.finish(a, app.confirm(ctx, a, arg))
	default:
		return false, a.err(gpgAssUnknownCmd)
	}
}

func (app *app) finish(a *assuan, err error) error {
	if err == nil {
		return nil
	}
	code, ok := assuanCode(err)
	if !ok {
		return err
	}
	return a.err(code)
}

func assuanCode(err error) (int, bool) {
	switch {
	case errors.Is(err, errCanceled):
		return gpgCanceled, true
	case errors.Is(err, errTimeout):
		return gpgTimeout, true
	case errors.Is(err, errNotConfirmed):
		return gpgNotConfirmed, true
	case errors.Is(err, errNoUI):
		return gpgPinEntry, true
	default:
		return 0, false
	}
}

func (app *app) option(a *assuan, arg string) error {
	key, value, ok := parseOption(arg)
	if !ok {
		return a.err(gpgAssSyntax)
	}
	switch key {
	case "ttyname":
		app.sess.ttyName = value
	case "ttytype":
		app.sess.ttyType = value
	case "display":
		app.sess.display = value
	case "default-ok":
		app.sess.defaultOK = value
	case "default-cancel":
		app.sess.defaultCancel = value
	case "default-prompt":
		app.sess.defaultPrompt = value
	}
	// Unknown options, including no-grab, allow-external-password-cache, and putenv, are OK.
	// putenv is not applied, so variables already in this process stay as they are.
	return a.ok()
}

func (app *app) getinfo(a *assuan, what string) error {
	var payload string
	switch what {
	case "version":
		payload = version
	case "pid":
		payload = strconv.Itoa(os.Getpid())
	case "flavor":
		payload = "store"
	case "ttyinfo":
		payload = app.ttyinfo()
	default:
		return a.err(gpgAssParameter)
	}
	if err := a.data([]byte(payload)); err != nil {
		return err
	}
	return a.ok()
}

func (app *app) ttyinfo() string {
	disp := app.sess.display
	if disp == "" {
		disp = app.argDisplay
	}
	return fmt.Sprintf("%s %s %s %s %s -",
		dash(app.sess.ttyName),
		dash(app.sess.ttyType),
		dash(disp),
		deviceStat(app.sess.ttyName),
		idField(),
	)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (app *app) clearPassphrase(a *assuan, arg string) error {
	id := strings.TrimSpace(arg)
	if id == "" {
		return a.ok()
	}
	if err := app.store.Delete(id); err != nil {
		return a.err(gpgPinEntry)
	}
	if err := app.flags.Remove(id); err != nil {
		return a.err(gpgPinEntry)
	}
	return a.ok()
}

func (app *app) getPIN(ctx context.Context, a *assuan) error {
	defer func() {
		app.sess.errText = ""
		app.sess.repeat = false
	}()

	key := app.sess.key
	var secret []byte
	found := false
	if key != "" {
		b, err := app.store.Get(key)
		if err == nil {
			found = true
			secret = b
			if !app.flags.Known(key) {
				_ = app.flags.Ensure(key)
			}
		} else if secretMissing(err) && app.flags.Known(key) {
			_ = app.flags.Remove(key)
		}
	}
	noPrompt := key != "" && app.flags.Has(key)
	if found && noPrompt && !app.sess.repeat && app.sess.errText == "" {
		return sendPIN(a, secret, false)
	}

	req := app.pinRequest(found, noPrompt)
	res, err := app.ask(ctx, req)
	if err != nil {
		return err
	}
	pin, canceled := deliverPIN(ctx, app.ui, app.store, app.flags, key, found, secret, req.Repeat, res)
	if canceled {
		return errCanceled
	}
	return sendPIN(a, pin, req.Repeat)
}

func (app *app) confirm(ctx context.Context, a *assuan, arg string) error {
	defer func() { app.sess.errText = "" }()
	one := strings.Contains(arg, "--one-button")
	focus := 0
	if strings.Contains(arg, "--focus=ok") {
		focus = 1
	} else if strings.Contains(arg, "--focus=cancel") {
		focus = -1
	}
	req := app.pinRequest(false, false)
	req.Kind = KindConfirm
	req.OneButton = one
	req.Focus = focus
	req.HasSecret = false
	req.Repeat = false
	req.ErrorSet = false
	res, err := app.ask(ctx, req)
	if err != nil {
		return err
	}
	if res.Choice == ChoiceOK || one {
		return a.ok()
	}
	if res.Choice == ChoiceNotOK {
		return errNotConfirmed
	}
	return errCanceled
}

func (app *app) pinRequest(found, noPrompt bool) Request {
	return Request{
		Kind:        KindPIN,
		Title:       app.sess.title,
		Description: app.sess.desc,
		Prompt:      pick(app.sess.prompt, app.sess.defaultPrompt, "Passphrase:"),
		Error:       app.sess.errText,
		RepeatLabel: app.sess.repeatLabel,
		RepeatError: app.sess.repeatErr,
		OK:          pick(app.sess.ok, app.sess.defaultOK, "OK"),
		NotOK:       app.sess.notOK,
		Cancel:      pick(app.sess.cancelText, app.sess.defaultCancel, "Cancel"),
		HasSecret:   found,
		CanStore:    app.sess.key != "",
		Repeat:      app.sess.repeat,
		ErrorSet:    app.sess.errText != "",
		NoPrompt:    noPrompt,
	}
}

func (app *app) ask(ctx context.Context, req Request) (Result, error) {
	if app.ui == nil {
		return Result{}, errNoUI
	}
	pctx := ctx
	if app.sess.timeout > 0 {
		var cancel context.CancelFunc
		pctx, cancel = context.WithTimeout(ctx, time.Duration(app.sess.timeout)*time.Second)
		defer cancel()
	}
	res, err := app.ui.Ask(pctx, req)
	if pctx.Err() != nil {
		if errors.Is(pctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return Result{}, errTimeout
		}
		return Result{}, errCanceled
	}
	if err != nil {
		return Result{}, errNoUI
	}
	return res, nil
}

func sendPIN(a *assuan, pin []byte, repeated bool) error {
	if repeated {
		if err := a.status("PIN_REPEATED"); err != nil {
			return err
		}
	}
	if len(pin) > 0 {
		if err := a.data(pin); err != nil {
			return err
		}
	}
	return a.ok()
}

func pick(v, def, fallback string) string {
	if v != "" {
		return v
	}
	if def != "" {
		return def
	}
	return fallback
}

func atoi(s string) int {
	n := 0
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if i == 0 {
		return 0
	}
	return n
}
