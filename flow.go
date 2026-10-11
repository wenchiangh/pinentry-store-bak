package main

import (
	"context"
	"errors"
)

var (
	errCanceled     = errors.New("canceled")
	errTimeout      = errors.New("timeout")
	errNotConfirmed = errors.New("not confirmed")
	errNoUI         = errors.New("no ui")
)

func notify(ctx context.Context, ui UI, text string) {
	if ui == nil {
		return
	}
	_ = ui.Notice(ctx, text)
}

// deliverPIN applies the store rules to a finished prompt.
// A required store change that does not happen is reported, then the typed
// passphrase is still returned. A change already made is left in place.
func deliverPIN(ctx context.Context, ui UI, st store, fl *catalog, key string, found bool, secret []byte, repeat bool, res Result) (pin []byte, canceled bool) {
	if res.Choice == ChoiceCancel {
		return nil, true
	}
	if key == "" {
		if res.Choice != ChoiceTyped {
			return nil, true
		}
		return append([]byte(nil), res.Secret...), false
	}
	if repeat && res.Choice != ChoiceTyped {
		return nil, true
	}
	if res.Choice == ChoiceUse {
		if !found {
			return nil, true
		}
		if err := fl.Mark(key, res.NoPrompt, true); err != nil {
			notify(ctx, ui, "Could not update the saved entry.")
		}
		return append([]byte(nil), secret...), false
	}
	if res.Choice != ChoiceTyped {
		return nil, true
	}
	pin = append([]byte(nil), res.Secret...)
	if res.Save {
		if err := st.Put(key, pin); err != nil {
			notify(ctx, ui, "Could not save the passphrase.")
			if !res.NoPrompt {
				if err := fl.Mark(key, false, false); err != nil {
					notify(ctx, ui, "Could not update the saved entry.")
				}
			}
			return pin, false
		}
		if err := fl.Mark(key, res.NoPrompt, true); err != nil {
			notify(ctx, ui, "Could not update the saved entry.")
		}
		return pin, false
	}
	if !res.NoPrompt {
		if err := fl.Mark(key, false, false); err != nil {
			notify(ctx, ui, "Could not update the saved entry.")
		}
	}
	return pin, false
}
