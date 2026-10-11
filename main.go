package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	if maybeChild() {
		return
	}
	os.Exit(dispatch(os.Args[1:]))
}

// launch is the pinentry command line. These values are the initial session
// state. OPTION and SETTIMEOUT replace ttyname, ttytype, and timeout later.
type launch struct {
	display string
	ttyName string
	ttyType string
	timeout int
	rest    []string
	unknown []string
}

func dispatch(args []string) int {
	ln, err := splitArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pinentry-store: %v\n", err)
		return 2
	}
	for _, name := range ln.unknown {
		fmt.Fprintf(os.Stderr, "pinentry-store: unknown option %s\n", name)
	}
	switch {
	case len(ln.rest) == 0:
		return runAssuan(ln)
	case len(ln.rest) == 1 && ln.rest[0] == "list":
		return runList(false)
	case len(ln.rest) == 2 && ln.rest[0] == "list" && ln.rest[1] == "no-prompt":
		return runList(true)
	case len(ln.rest) >= 2 && ln.rest[0] == "delete":
		return runDelete(strings.Join(ln.rest[1:], " "))
	case len(ln.rest) >= 2 && ln.rest[0] == "prompt":
		return runPrompt(strings.Join(ln.rest[1:], " "))
	default:
		fmt.Fprintln(os.Stderr, "usage: pinentry-store [options] | list [no-prompt] | delete SETKEYINFO | prompt SETKEYINFO")
		return 2
	}
}

func splitArgs(args []string) (launch, error) {
	var out launch
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			out.rest = append(out.rest, args[i+1:]...)
			break
		}
		name, inline, hasInline, isOpt := classifyOpt(tok)
		if !isOpt {
			out.rest = append(out.rest, tok)
			continue
		}
		switch name {
		case "--display", "-D":
			v, err := optVal(args, &i, hasInline, inline, name)
			if err != nil {
				return launch{}, err
			}
			out.display = v
		case "--ttyname", "-T":
			v, err := optVal(args, &i, hasInline, inline, name)
			if err != nil {
				return launch{}, err
			}
			out.ttyName = v
		case "--ttytype", "-N":
			v, err := optVal(args, &i, hasInline, inline, name)
			if err != nil {
				return launch{}, err
			}
			out.ttyType = v
		case "--timeout", "-o":
			v, err := optVal(args, &i, hasInline, inline, name)
			if err != nil {
				return launch{}, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return launch{}, fmt.Errorf("%s requires a number", name)
			}
			out.timeout = n
		case "--lc-ctype", "-C", "--lc-messages", "-M", "--parent-wid", "-W", "--colors", "-c", "--ttyalert", "-a":
			if _, err := optVal(args, &i, hasInline, inline, name); err != nil {
				return launch{}, err
			}
		case "--debug", "-d", "--no-global-grab", "-g":
			if hasInline {
				return launch{}, fmt.Errorf("%s does not take a value", name)
			}
		default:
			out.unknown = append(out.unknown, tok)
		}
	}
	return out, nil
}

func classifyOpt(tok string) (name, inline string, hasInline, isOpt bool) {
	if strings.HasPrefix(tok, "--") {
		name = tok
		if eq := strings.IndexByte(tok, '='); eq > 2 {
			name = tok[:eq]
			inline = tok[eq+1:]
			hasInline = true
		}
		return name, inline, hasInline, true
	}
	if strings.HasPrefix(tok, "-") && len(tok) >= 2 {
		name = tok[:2]
		if len(tok) > 2 {
			inline = tok[2:]
			hasInline = true
		}
		return name, inline, hasInline, true
	}
	return tok, "", false, false
}

func optVal(args []string, i *int, hasInline bool, inline, name string) (string, error) {
	if hasInline {
		return inline, nil
	}
	if *i+1 >= len(args) {
		return "", fmt.Errorf("%s requires a value", name)
	}
	*i++
	return args[*i], nil
}

func applyLaunch(a *app, ln launch) {
	a.argDisplay = ln.display
	a.sess.ttyName = ln.ttyName
	a.sess.ttyType = ln.ttyType
	a.sess.timeout = ln.timeout
}

func runAssuan(ln launch) int {
	cfg := loadConfig(configPath())
	a := &app{
		store: openStore(cfg),
		flags: &catalog{path: flagPath()},
		cfg:   cfg,
	}
	applyLaunch(a, ln)
	installStopRestore()
	a.ui = &lazyUI{open: a.openUI}
	if err := serve(os.Stdin, os.Stdout, a); err != nil {
		fmt.Fprintf(os.Stderr, "pinentry-store: %v\n", err)
		return 1
	}
	return 0
}

func runList(onlyNoPrompt bool) int {
	entries, err := (&catalog{path: flagPath()}).List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pinentry-store: %v\n", err)
		return 1
	}
	for _, e := range entries {
		if onlyNoPrompt && !e.NoPrompt {
			continue
		}
		state := "prompt"
		if e.NoPrompt {
			state = "no-prompt"
		}
		fmt.Printf("%s\t%s\n", e.ID, state)
	}
	return 0
}

func runDelete(id string) int {
	if err := openStore(loadConfig(configPath())).Delete(id); err != nil {
		fmt.Fprintf(os.Stderr, "pinentry-store: %v\n", err)
		return 1
	}
	if err := (&catalog{path: flagPath()}).Remove(id); err != nil {
		fmt.Fprintf(os.Stderr, "pinentry-store: %v\n", err)
		return 1
	}
	return 0
}

func runPrompt(id string) int {
	if err := (&catalog{path: flagPath()}).Mark(id, false, false); err != nil {
		fmt.Fprintf(os.Stderr, "pinentry-store: %v\n", err)
		return 1
	}
	return 0
}
