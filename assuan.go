package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Assuan line limit used by libassuan. Data lines wrap before this length.
const assuanLineLength = 1002

// gpg-error source and codes. The wire value is source<<24 | code.
// Pinentry's default source is Pinentry (5). gpg-agent reads the integer with atoi.
const (
	gpgSourcePinentry = 5 << 24

	gpgTimeout       = 62
	gpgPinEntry      = 86
	gpgCanceled      = 99
	gpgNotConfirmed  = 114
	gpgAssUnknownCmd = 275
	gpgAssSyntax     = 276
	gpgAssParameter  = 280
)

func gpgErr(code int) int { return gpgSourcePinentry | code }

func gpgErrText(code int) string {
	switch code {
	case gpgTimeout:
		return "Timeout"
	case gpgPinEntry:
		return "pinentry error"
	case gpgCanceled:
		return "Operation cancelled"
	case gpgNotConfirmed:
		return "Not confirmed"
	case gpgAssUnknownCmd:
		return "Unknown IPC command"
	case gpgAssSyntax:
		return "IPC syntax error"
	case gpgAssParameter:
		return "IPC parameter error"
	default:
		return "General error"
	}
}

type assuan struct {
	r *bufio.Reader
	w *bufio.Writer
}

func newAssuan(in io.Reader, out io.Writer) *assuan {
	return &assuan{r: bufio.NewReader(in), w: bufio.NewWriter(out)}
}

func (a *assuan) greet() error {
	if err := a.line("OK Pleased to meet you"); err != nil {
		return err
	}
	return a.w.Flush()
}

func (a *assuan) line(s string) error {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if _, err := io.WriteString(a.w, s); err != nil {
		return err
	}
	return a.w.WriteByte('\n')
}

func (a *assuan) ok() error {
	if err := a.line("OK"); err != nil {
		return err
	}
	return a.w.Flush()
}

func (a *assuan) okClosing() error {
	if err := a.line("OK closing connection"); err != nil {
		return err
	}
	return a.w.Flush()
}

func (a *assuan) status(keyword string) error {
	return a.line("S " + keyword)
}

func (a *assuan) err(code int) error {
	msg := fmt.Sprintf("ERR %d %s <Pinentry>", gpgErr(code), gpgErrText(code))
	if err := a.line(msg); err != nil {
		return err
	}
	return a.w.Flush()
}

// data writes D lines the way libassuan's assuan_send_data does.
// Only '%', CR, and LF are percent-encoded, as uppercase hex.
// Space and '+' are copied raw. The client decodes %XX and does not map '+' to space.
func (a *assuan) data(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	line := make([]byte, 0, assuanLineLength)
	flush := func() error {
		line = append(line, '\n')
		_, err := a.w.Write(line)
		line = line[:0]
		return err
	}
	for len(p) > 0 {
		if len(line) == 0 {
			line = append(line, 'D', ' ')
		}
		for len(p) > 0 && len(line) < assuanLineLength-4 {
			b := p[0]
			p = p[1:]
			if b == '%' || b == '\r' || b == '\n' {
				line = append(line, '%', hexDigit(b>>4), hexDigit(b&0x0f))
			} else {
				line = append(line, b)
			}
		}
		if len(line) >= assuanLineLength-4 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if len(line) > 0 {
		return flush()
	}
	return nil
}

func hexDigit(b byte) byte {
	if b < 10 {
		return '0' + b
	}
	return 'A' + b - 10
}

func (a *assuan) readLine() (string, error) {
	s, err := a.r.ReadString('\n')
	if !strings.HasSuffix(s, "\n") {
		if err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return strings.TrimRight(s, "\r\n"), nil
}

// parseCommand splits an Assuan command line.
// A leading space is a syntax error. The command compare is case-insensitive.
func parseCommand(line string) (cmd, arg string, ok bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return "", "", false
	}
	i := 0
	for i < len(line) && line[i] != ' ' && line[i] != '\t' {
		i++
	}
	cmd = strings.ToUpper(line[:i])
	if i < len(line) {
		arg = strings.TrimLeft(line[i+1:], " \t")
	}
	return cmd, arg, true
}

// unescapePercent decodes %XX the way pinentry's strcpy_escaped does.
// '+' stays '+'. A '%' that is not followed by two hex digits stays as written.
func unescapePercent(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			b.WriteByte(unhex(s[i+1])<<4 | unhex(s[i+2]))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

// parseOption matches libassuan's OPTION grammar:
// name, name=value, name value, and name = value.
// A leading double dash is optional. One leading dash is a syntax error.
func parseOption(arg string) (key, value string, ok bool) {
	arg = strings.TrimLeft(arg, " \t")
	if arg == "" || arg[0] == '=' {
		return "", "", false
	}
	j := 0
	for j < len(arg) && arg[j] != ' ' && arg[j] != '\t' && arg[j] != '=' {
		j++
	}
	key = arg[:j]
	rest := strings.TrimLeft(arg[j:], " \t")
	if strings.HasPrefix(rest, "=") {
		rest = strings.TrimLeft(rest[1:], " \t")
		if rest == "" {
			return "", "", false
		}
	}
	value = strings.TrimRight(rest, " \t")
	if strings.HasPrefix(key, "--") && len(key) > 2 {
		key = key[2:]
	}
	if strings.HasPrefix(key, "-") {
		return "", "", false
	}
	return key, value, true
}
