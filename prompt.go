package main

import "context"

type Kind int

const (
	KindPIN Kind = iota
	KindConfirm
)

type Choice int

const (
	ChoiceCancel Choice = iota
	ChoiceUse
	ChoiceTyped
	ChoiceOK
	ChoiceNotOK
)

// Request is what the prompt is allowed to see. It does not carry the secret.
// Repeat and ErrorSet are separate: SETREPEAT decides the form, SETERROR is the text.
type Request struct {
	Kind        Kind   `json:"kind"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	Error       string `json:"error,omitempty"`
	RepeatLabel string `json:"repeatLabel,omitempty"`
	RepeatError string `json:"repeatError,omitempty"`
	OK          string `json:"ok,omitempty"`
	NotOK       string `json:"notOK,omitempty"`
	Cancel      string `json:"cancel,omitempty"`
	HasSecret   bool   `json:"hasSecret,omitempty"`
	CanStore    bool   `json:"canStore,omitempty"`
	Repeat      bool   `json:"repeat,omitempty"`
	ErrorSet    bool   `json:"errorSet,omitempty"`
	NoPrompt    bool   `json:"noPrompt,omitempty"`
	OneButton   bool   `json:"oneButton,omitempty"`
	Focus       int    `json:"focus,omitempty"`
}

type Result struct {
	Choice   Choice `json:"choice"`
	Secret   []byte `json:"secret,omitempty"`
	Save     bool   `json:"save,omitempty"`
	NoPrompt bool   `json:"noPrompt,omitempty"`
}

// UI talks to the user. Notice is the acknowledgement after the form has closed.
// The entry form is not shown again.
type UI interface {
	Ask(ctx context.Context, req Request) (Result, error)
	Notice(ctx context.Context, text string) error
	Close() error
}

type wire struct {
	Op   string   `json:"op"`
	Req  *Request `json:"req,omitempty"`
	Res  *Result  `json:"res,omitempty"`
	Text string   `json:"text,omitempty"`
}
