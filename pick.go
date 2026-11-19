package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/zrs01/winfit/internal/picker"
	"github.com/zrs01/winfit/internal/target"
	"github.com/zrs01/winfit/internal/winapi"

	"github.com/mattn/go-isatty"
)

// chooseInteractively resolves an ambiguous match by asking the user: a
// bubbletea list of the surviving windows (enter picks, esc cancels).
// Without a terminal on both ends there is nothing to ask, so the
// ambiguity message is returned as-is (it points at --hwnd / --topmost).
func chooseInteractively(amb *target.AmbiguousError) (target.Window, error) {
	if !interactive() {
		return target.Window{}, amb // its text already points at --hwnd / --topmost
	}
	app := map[uint32]string{}
	items := make([]picker.Item, 0, len(amb.Matches))
	for _, w := range amb.Matches {
		name, ok := app[w.Pid]
		if !ok {
			name = trimExe(winapi.ImageName(w.Pid))
			app[w.Pid] = name
		}
		items = append(items, picker.Item{Hwnd: w.Hwnd, Pid: w.Pid, App: name, Title: w.Title})
	}
	slog.Debug("picker candidates", "count", len(items))

	res, err := picker.Pick(items, promptFor(amb))
	if err != nil {
		return target.Window{}, err
	}
	if res.Canceled || res.Index < 0 || res.Index >= len(amb.Matches) {
		return target.Window{}, errors.New("cancelled: no window selected")
	}
	return amb.Matches[res.Index], nil
}

func promptFor(amb *target.AmbiguousError) string {
	sel := ""
	if amb.Title != "" {
		sel += fmt.Sprintf(" title=%q", amb.Title)
	}
	if amb.App != "" {
		sel += fmt.Sprintf(" app=%q", amb.App)
	}
	return fmt.Sprintf("%d windows match%s - select one:", len(amb.Matches), sel)
}

// interactive reports whether an interactive prompt is possible at all
// (piped stdin or redirected stdout means a script, not a person).
func interactive() bool {
	return isTTY(os.Stdin.Fd()) && isTTY(os.Stdout.Fd())
}

func isTTY(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
