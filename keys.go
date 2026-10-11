package main

import "unicode/utf8"

type keyKind int

const (
	keyNone keyKind = iota
	keyRune
	keyEnter
	keyEsc
	keyTab
	keyBacktab
	keyUp
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyBackspace
	keyDelete
	keyCtrlC
	keyCtrlU
	keyPaste
)

type key struct {
	kind  keyKind
	r     rune
	paste string
}

type takeStatus int

const (
	takeNeed takeStatus = iota
	takeGot
)

func takeKey(buf []byte, idle bool) (key, []byte, takeStatus) {
	if len(buf) == 0 {
		return key{}, buf, takeNeed
	}
	switch buf[0] {
	case '\r', '\n':
		return key{kind: keyEnter}, buf[1:], takeGot
	case '\t':
		return key{kind: keyTab}, buf[1:], takeGot
	case 0x7f, 0x08:
		return key{kind: keyBackspace}, buf[1:], takeGot
	case 0x03:
		return key{kind: keyCtrlC}, buf[1:], takeGot
	case 0x15:
		return key{kind: keyCtrlU}, buf[1:], takeGot
	case 0x1b:
		return takeEsc(buf, idle)
	}
	r, size := utf8.DecodeRune(buf)
	if r == utf8.RuneError && size == 1 {
		if !utf8.FullRune(buf) && !idle {
			return key{}, buf, takeNeed
		}
		return key{kind: keyNone}, buf[1:], takeGot
	}
	return key{kind: keyRune, r: r}, buf[size:], takeGot
}

func takeEsc(buf []byte, idle bool) (key, []byte, takeStatus) {
	if len(buf) == 1 {
		if idle {
			return key{kind: keyEsc}, nil, takeGot
		}
		return key{}, buf, takeNeed
	}
	switch buf[1] {
	case '[':
		return takeCSI(buf, idle)
	case 'O':
		if len(buf) < 3 {
			if idle {
				return key{kind: keyEsc}, nil, takeGot
			}
			return key{}, buf, takeNeed
		}
		if k, ok := arrowKey(buf[2]); ok {
			return k, buf[3:], takeGot
		}
		return key{kind: keyEsc}, buf[1:], takeGot
	default:
		return key{kind: keyEsc}, buf[1:], takeGot
	}
}

func takeCSI(buf []byte, idle bool) (key, []byte, takeStatus) {
	i := 2
	for i < len(buf) && i < 32 {
		c := buf[i]
		if c >= 0x40 && c <= 0x7e {
			seq := string(buf[2:i])
			if c == '~' && seq == "200" {
				end := indexPasteEnd(buf[i+1:])
				if end < 0 {
					if idle {
						return key{kind: keyEsc}, nil, takeGot
					}
					return key{}, buf, takeNeed
				}
				body := buf[i+1 : i+1+end]
				rest := buf[i+1+end+len("\x1b[201~"):]
				return key{kind: keyPaste, paste: string(body)}, rest, takeGot
			}
			return csiKey(seq, c), buf[i+1:], takeGot
		}
		i++
	}
	if i >= 32 {
		return key{kind: keyEsc}, buf[1:], takeGot
	}
	if idle {
		return key{kind: keyEsc}, nil, takeGot
	}
	return key{}, buf, takeNeed
}

func indexPasteEnd(buf []byte) int {
	marker := []byte("\x1b[201~")
	for i := 0; i+len(marker) <= len(buf); i++ {
		match := true
		for j := range marker {
			if buf[i+j] != marker[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func csiKey(seq string, final byte) key {
	switch final {
	case 'A':
		return key{kind: keyUp}
	case 'B':
		return key{kind: keyDown}
	case 'C':
		return key{kind: keyRight}
	case 'D':
		return key{kind: keyLeft}
	case 'H':
		return key{kind: keyHome}
	case 'F':
		return key{kind: keyEnd}
	case 'Z':
		return key{kind: keyBacktab}
	case '~':
		switch seq {
		case "1", "7":
			return key{kind: keyHome}
		case "3":
			return key{kind: keyDelete}
		case "4", "8":
			return key{kind: keyEnd}
		}
	}
	return key{kind: keyNone}
}

func arrowKey(b byte) (key, bool) {
	switch b {
	case 'A':
		return key{kind: keyUp}, true
	case 'B':
		return key{kind: keyDown}, true
	case 'C':
		return key{kind: keyRight}, true
	case 'D':
		return key{kind: keyLeft}, true
	default:
		return key{}, false
	}
}
