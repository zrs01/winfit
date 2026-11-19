package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/zrs01/winfit/internal/winapi"
)

// setupLogging installs the slog default logger on stderr, so stdout stays
// clean for scripts parsing `winfit list`. LOG_LEVEL selects the level
// (error|warn|info|debug; "trace" means debug). An attached console gets
// the human handler below; redirected or piped output gets slog's text
// format with timestamps, because there the log is read after the fact.
func setupLogging() {
	lvl := levelFromEnv()
	if onConsole(os.Stderr) {
		slog.SetDefault(slog.New(&consoleHandler{w: os.Stderr, lvl: lvl, color: ansiOK(os.Stderr)}))
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
}

func levelFromEnv() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "trace", "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}

func onConsole(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// ansiOK reports whether escape codes can be rendered: asking Windows to
// switch the handle to VT processing is what makes colors safe there.
func ansiOK(f *os.File) bool { return winapi.EnableVT(f.Fd()) }

// consoleHandler renders records as short colored sentences for a person
// reading a terminal:
//
//	error: no window matched (title="zzz"); run `winfit list` to see candidates
//	debug: target window hwnd=0x1E8738 pid=1234 title="notes.txt"
//
// No timestamps, no key=value soup; attributes become trailing details.
type consoleHandler struct {
	w     io.Writer
	lvl   slog.Level
	color bool
	attrs []slog.Attr
}

func (h *consoleHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.lvl }

func (h *consoleHandler) Handle(_ context.Context, r slog.Record) error {
	tag, col := levelTag(r.Level)
	var b strings.Builder
	if h.color {
		fmt.Fprintf(&b, "\x1b[%sm%s \x1b[0m", col, tag)
	} else {
		fmt.Fprintf(&b, "%s ", tag)
	}
	b.WriteString(r.Message)
	for _, a := range h.attrs {
		writeAttr(&b, a)
	}
	r.Attrs(func(a slog.Attr) bool { writeAttr(&b, a); return true })
	b.WriteByte('\n')
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *consoleHandler) WithAttrs(as []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr{}, h.attrs...), as...)
	return &c
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	c := *h
	if name != "" {
		c.attrs = append(append([]slog.Attr{}, h.attrs...), slog.String("group", name))
	}
	return &c
}

func levelTag(l slog.Level) (tag, color string) {
	switch {
	case l >= slog.LevelError:
		return "error:", "31"
	case l >= slog.LevelWarn:
		return "warn:", "33"
	case l >= slog.LevelInfo:
		return "info:", "36"
	default:
		return "debug:", "90"
	}
}

// writeAttr appends " key=value", quoting values that contain blanks so the
// line stays readable at a glance.
func writeAttr(b *strings.Builder, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	v := a.Value.Resolve()
	if v.Kind() == slog.KindString && needsQuote(v.String()) {
		fmt.Fprintf(b, " %s=%q", a.Key, v.String())
		return
	}
	fmt.Fprintf(b, " %s=%v", a.Key, v.Any())
}

func needsQuote(s string) bool {
	return s == "" || strings.ContainsAny(s, " \t\"=")
}
