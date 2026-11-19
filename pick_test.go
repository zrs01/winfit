package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zrs01/winfit/internal/target"
)

// The picker branch in resolveTarget depends on *target.AmbiguousError
// staying recoverable from whatever wrapping the CLI layers on top.
func TestAmbiguitySurvivesWrapping(t *testing.T) {
	sentinel := &target.AmbiguousError{Matches: []target.Window{{Hwnd: 0x10, Pid: 7, Title: "A"}}, Title: "doc"}
	var got *target.AmbiguousError
	if !errors.As(fmt.Errorf("resolve target: %w", sentinel), &got) {
		t.Fatal("errors.As must see through %w wrapping or the picker never runs")
	}
	if len(got.Matches) != 1 || got.Matches[0].Hwnd != 0x10 {
		t.Fatalf("matches must carry through for the list, got %+v", got.Matches)
	}
}

func TestPromptForEchoesSelectors(t *testing.T) {
	got := promptFor(&target.AmbiguousError{
		Matches: []target.Window{{Hwnd: 1}, {Hwnd: 2}},
		Title:   "report", App: "notepad",
	})
	for _, want := range []string{"2 windows match", `title="report"`, `app="notepad"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt %q missing %q", got, want)
		}
	}
	if !strings.Contains(promptFor(&target.AmbiguousError{Matches: []target.Window{{}, {}}}), "2 windows match -") {
		t.Fatal("prompt without selectors should still count the rows")
	}
}

// Without a terminal the picker must not be attempted: the caller gets the
// original guidance error (--hwnd / --topmost) instead.
func TestNoTerminalKeepsGuidanceError(t *testing.T) {
	if interactive() {
		t.Skip("stdin is a terminal here; this path covers piped runs only")
	}
	amb := &target.AmbiguousError{Matches: []target.Window{{Hwnd: 1}, {Hwnd: 2}}, Title: "doc"}
	_, err := chooseInteractively(amb)
	if err == nil || !strings.Contains(err.Error(), "winfit list") {
		t.Fatalf("non-interactive ambiguity should keep the list/--hwnd hint, got %v", err)
	}
}
