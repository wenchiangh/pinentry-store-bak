package main

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

type screen struct {
	mu      sync.Mutex
	in      *os.File
	out     *os.File
	state   *term.State
	active  bool
	quit    chan struct{}
	done    chan struct{}
	reads   chan []byte
	readErr chan error
	pending []byte
}

func newScreen(in, out *os.File) *screen {
	enableVirtualTerminal(in, out)
	return &screen{
		in:      in,
		out:     out,
		reads:   make(chan []byte, 16),
		readErr: make(chan error, 1),
	}
}

// begin takes the tty for one dialog. The caller of a pinentry keeps using
// that tty after the dialog returns, so release gives it back.
func (s *screen) begin() error {
	s.mu.Lock()
	err := s.activate()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	setRestore(s.release)
	return nil
}

func (s *screen) activate() error {
	if s.active {
		return nil
	}
	state, err := term.MakeRaw(int(s.in.Fd()))
	if err != nil {
		return err
	}
	quit := make(chan struct{})
	done := make(chan struct{})
	s.quit = quit
	s.done = done
	go s.readLoop(quit, done)
	if _, err := io.WriteString(s.out, "\033[?1049h\033[?2004h"); err != nil {
		s.stopLoop()
		_ = term.Restore(int(s.in.Fd()), state)
		return err
	}
	s.state = state
	s.active = true
	return nil
}

func (s *screen) readLoop(quit <-chan struct{}, done chan struct{}) {
	defer close(done)
	buf := make([]byte, 256)
	for {
		n, err := s.in.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			select {
			case s.reads <- chunk:
			case <-quit:
				return
			}
		}
		if err != nil {
			select {
			case <-quit:
				return
			default:
			}
			select {
			case s.readErr <- err:
			case <-quit:
			}
			return
		}
	}
}

func (s *screen) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLoop()
	if s.active {
		_, _ = io.WriteString(s.out, "\033[?2004l\033[?25h\033[?1049l")
		if s.state != nil {
			_ = term.Restore(int(s.in.Fd()), s.state)
			s.state = nil
		}
		s.active = false
	}
	s.pending = nil
	for {
		select {
		case <-s.reads:
		case <-s.readErr:
		default:
			return
		}
	}
}

func (s *screen) stopLoop() {
	if s.quit == nil {
		return
	}
	select {
	case <-s.quit:
	default:
		close(s.quit)
	}
	interruptRead(s.in)
	<-s.done
	resumeRead(s.in)
	s.quit = nil
	s.done = nil
}

func (s *screen) draw(view string) error {
	// MakeRaw turns off ONLCR, so a bare LF leaves the cursor in the column
	// where the previous line ended. The next line then starts further right.
	_, err := io.WriteString(s.out, "\033[H\033[2J\033[?25l"+crlf(view))
	return err
}

func crlf(s string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + strings.Count(s, "\n"))
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			b.WriteByte('\r')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

var (
	restoreMu sync.Mutex
	restoreFn func()
)

func setRestore(fn func()) {
	restoreMu.Lock()
	restoreFn = fn
	restoreMu.Unlock()
}

func callRestore() {
	restoreMu.Lock()
	fn := restoreFn
	restoreMu.Unlock()
	if fn != nil {
		fn()
	}
}

func (s *screen) width() int {
	w, _, err := term.GetSize(int(s.out.Fd()))
	if err != nil || w <= 0 {
		w, _, err = term.GetSize(int(s.in.Fd()))
	}
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

func (s *screen) readKey(ctx context.Context) (key, error) {
	buf := s.pending
	idle := false
	for {
		k, rest, st := takeKey(buf, idle)
		buf = rest
		if st == takeGot {
			s.pending = buf
			return k, nil
		}
		idle = false
		timer := time.NewTimer(150 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.pending = buf
			return key{}, ctx.Err()
		case b := <-s.reads:
			timer.Stop()
			buf = append(buf, b...)
		case err := <-s.readErr:
			timer.Stop()
			s.pending = buf
			return key{}, err
		case <-timer.C:
			idle = true
		}
	}
}
