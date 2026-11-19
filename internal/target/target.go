// Package target decides WHICH window a command acts on. The selection
// rules are pure functions over injected data; the syscalls that produce
// that data live in internal/winapi.
package target

import (
	"fmt"
	"strings"
)

// Window is one candidate window in z-order position (0 = frontmost).
type Window struct {
	Hwnd  uintptr
	Pid   uint32
	Title string
}

// Opts carries the selector flags. Empty strings mean "not given". Both
// Title and App are case-insensitive substrings.
type Opts struct {
	Title   string // title contains this
	App     string // executable name contains this (.exe suffix trimmed)
	Topmost bool   // among several matches, pick the frontmost
}

// Selectors reports whether any selector flag was given.
func (o Opts) Selectors() bool { return o.Title != "" || o.App != "" }

// List is the injected system view used by Select.
type List struct {
	// Windows are candidates in z-order, front to back.
	Windows []Window
	// Image returns the full executable path for a pid ("" if unknown).
	Image func(pid uint32) string
	// Parent returns the parent pid of a pid (0 if unknown).
	Parent func(pid uint32) uint32
	// ForegroundPid owns the foreground window (0 if none).
	ForegroundPid uint32
}

// AmbiguousError is returned when several windows survive filtering and
// rooting. Matches are the survivors in z-order (front first), so the
// caller can offer them as an interactive choice instead of just a text
// hint. --topmost short-circuits this and never produces it.
type AmbiguousError struct {
	Matches           []Window
	Title, App        string
	RootingWasAttempt bool // --app given, rooting tried and tied
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf(
		"%d windows match (title=%q app=%q); run `winfit list` and pass --hwnd, "+
			"or pass --topmost to pick the frontmost", len(e.Matches), e.Title, e.App)
}

// Select resolves the single target window from the candidates, applying
// title/app filters, multi-process rooting, and the ambiguity policy
// (*AmbiguousError unless --topmost).
func Select(l List, o Opts) (Window, error) {
	matches := filter(l, o)
	switch len(matches) {
	case 0:
		return Window{}, fmt.Errorf(
			"no window matched (title=%q app=%q); run `winfit list` to see candidates", o.Title, o.App)
	case 1:
		return matches[0], nil
	}
	ambiguous := func(ws []Window) (Window, error) {
		return Window{}, &AmbiguousError{Matches: ws, Title: o.Title, App: o.App, RootingWasAttempt: o.App != ""}
	}
	if o.Topmost {
		return matches[0], nil // EnumWindows order is z-order: frontmost first
	}
	if o.App == "" {
		return ambiguous(matches)
	}
	pid, ok := rootProcess(l, matches, normalizeApp(o.App))
	if !ok {
		return ambiguous(matches)
	}
	// Rooting narrows the PROCESS; several windows in that one process are
	// still the user's choice, not ours.
	ws := windowsOf(matches, pid)
	if len(ws) == 1 {
		return ws[0], nil
	}
	return ambiguous(ws)
}

func filter(l List, o Opts) []Window {
	titleSub := strings.ToLower(o.Title)
	app := normalizeApp(o.App)
	var out []Window
	for _, w := range l.Windows {
		if titleSub != "" && !strings.Contains(strings.ToLower(w.Title), titleSub) {
			continue
		}
		if app != "" && !strings.Contains(baseLower(l.Image(w.Pid)), app) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// rootProcess implements multi-process rooting: when same-named
// processes collide, prefer the process NOT spawned by another process
// whose image also matches the app pattern, tie-broken by foreground
// ownership, window count, then lowest pid. ok is false when even the
// tie-breaks end tied.
func rootProcess(l List, matches []Window, app string) (uint32, bool) {
	byPid := map[uint32][]Window{}
	for _, w := range matches {
		byPid[w.Pid] = append(byPid[w.Pid], w)
	}
	var roots []uint32
	for pid := range byPid {
		if parent := l.Parent(pid); parent == 0 || !strings.Contains(baseLower(l.Image(parent)), app) {
			roots = append(roots, pid)
		}
	}
	if len(roots) == 0 {
		roots = keys(byPid) // all children (odd tree): fall through to tie-break
	}
	switch {
	case len(roots) == 1:
		return roots[0], true
	default:
		for _, r := range roots { // foreground-most pid wins
			if r == l.ForegroundPid {
				return r, true
			}
		}
		best, bestN := roots[0], len(byPid[roots[0]])
		tied := false
		for _, r := range roots[1:] {
			if n := len(byPid[r]); n > bestN {
				best, bestN, tied = r, n, false
			} else if n == bestN {
				tied = true
			}
		}
		if !tied {
			return best, true
		}
		return 0, false
	}
}

// windowsOf returns the z-ordered windows of one pid.
func windowsOf(matches []Window, pid uint32) []Window {
	var out []Window
	for _, w := range matches {
		if w.Pid == pid {
			out = append(out, w)
		}
	}
	return out
}

func normalizeApp(s string) string {
	s = strings.ToLower(s)
	return strings.TrimSuffix(s, ".exe")
}

func baseLower(path string) string {
	if path == "" {
		return ""
	}
	i := strings.LastIndexAny(path, `/\`)
	return normalizeApp(path[i+1:])
}

func keys(m map[uint32][]Window) []uint32 {
	var out []uint32
	for k := range m {
		out = append(out, k)
	}
	return out
}
