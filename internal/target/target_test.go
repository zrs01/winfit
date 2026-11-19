package target

import (
	"errors"
	"strings"
	"testing"
)

func list(ws []Window, images map[uint32]string, parents map[uint32]uint32, fg uint32) List {
	return List{
		Windows:       ws,
		Image:         func(pid uint32) string { return images[pid] },
		Parent:        func(pid uint32) uint32 { return parents[pid] },
		ForegroundPid: fg,
	}
}

var (
	imgs = map[uint32]string{
		10: `C:\apps\Chrome\chrome.exe`,
		11: `C:\apps\Chrome\chrome.exe`, // child renderer
		12: `C:\apps\Chrome\chrome.exe`, // child renderer
		20: `C:\Windows\notepad.exe`,
		30: `C:\tools\other.exe`,
	}
	parents = map[uint32]uint32{11: 10, 12: 10, 10: 4, 20: 4, 30: 4, 4: 0}
)

func TestSelectTitle(t *testing.T) {
	l := list([]Window{{1, 20, "Report.txt - Notepad"}, {2, 20, "readme.md - Notepad"}}, imgs, parents, 0)

	got, err := Select(l, Opts{Title: "report"})
	if err != nil || got.Hwnd != 1 {
		t.Fatalf("substring match failed: %+v %v", got, err)
	}
	if _, err := Select(l, Opts{Title: "notepad"}); err == nil ||
		!strings.Contains(err.Error(), "winfit list") {
		t.Fatalf("ambiguity should error with list hint, got %v", err)
	}
	got, err = Select(l, Opts{Title: "notepad", Topmost: true})
	if err != nil || got.Hwnd != 1 {
		t.Fatalf("--topmost should pick frontmost match: %+v %v", got, err)
	}
	if _, err := Select(l, Opts{Title: "zzz"}); err == nil {
		t.Fatal("zero matches must error")
	}
}

func TestSelectApp(t *testing.T) {
	l := list([]Window{
		{101, 11, "Tab A"}, // renderer-owned odd window
		{100, 10, "Chrome"},
		{102, 12, "Tab B"},
		{200, 20, "Notepad"},
	}, imgs, parents, 0)

	// Main (root) process wins over same-named children.
	got, err := Select(l, Opts{App: "chrome"})
	if err != nil || got.Hwnd != 100 {
		t.Fatalf("root-process pick failed: %+v %v", got, err)
	}
	// Substring app pattern matches the same processes; rooting still
	// demotes children whose parent image also contains the pattern.
	if got, err := Select(l, Opts{App: "chr"}); err != nil || got.Hwnd != 100 {
		t.Fatalf("substring app match failed: %+v %v", got, err)
	}
	// Extensionless app names accepted.
	if got, err := Select(l, Opts{App: "chrome.exe"}); err != nil || got.Hwnd != 100 {
		t.Fatalf("app.exe form failed: %+v %v", got, err)
	}
	// --topmost ignores rooting: frontmost raw match.
	if got, err := Select(l, Opts{App: "chrome", Topmost: true}); err != nil || got.Hwnd != 101 {
		t.Fatalf("--topmost over app failed: %+v %v", got, err)
	}
	if got, err := Select(l, Opts{App: "notepad"}); err != nil || got.Hwnd != 200 {
		t.Fatalf("single-app match failed: %+v %v", got, err)
	}
}

func TestSelectAppTieBreaks(t *testing.T) {
	twoRoots := []Window{{1, 20, "A"}, {2, 30, "B"}}
	// Two distinct same-named-root pids can't both be true here (different
	// images), so both roots apply: foreground pid wins.
	l := list(twoRoots, map[uint32]string{20: `x\tool.exe`, 30: `y\tool.exe`},
		map[uint32]uint32{20: 4, 30: 4}, 30)
	got, err := Select(l, Opts{App: "tool"})
	if err != nil || got.Hwnd != 2 {
		t.Fatalf("foreground tie-break failed: %+v %v", got, err)
	}
	// No foreground, no count winner → ambiguity error.
	l.ForegroundPid = 0
	if _, err := Select(l, Opts{App: "tool"}); err == nil {
		t.Fatal("tied roots must error")
	}
	// The window-count tie-break resolves the PROCESS (pid 20, two windows);
	// which of its windows to take is left to the picker.
	l = list([]Window{{1, 20, "A"}, {2, 30, "B"}, {3, 20, "C"}},
		map[uint32]string{20: `x\tool.exe`, 30: `y\tool.exe`},
		map[uint32]uint32{20: 4, 30: 4}, 0)
	_, err = Select(l, Opts{App: "tool"})
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("count tie-break should settle on a pid and list its windows, got %v", err)
	}
	if len(amb.Matches) != 2 || amb.Matches[0].Hwnd != 1 || amb.Matches[1].Hwnd != 3 {
		t.Fatalf("list must hold only the winning pid's windows in z-order, got %+v", amb.Matches)
	}
}

func TestSelectAppAllChildren(t *testing.T) {
	// Pathological tree: every same-named pid spawned by another copy.
	l := list([]Window{{1, 11, "A"}, {2, 12, "B"}},
		map[uint32]string{11: `c\chrome.exe`, 12: `c\chrome.exe`},
		map[uint32]uint32{11: 12, 12: 11}, 0)
	if _, err := Select(l, Opts{App: "chrome"}); err == nil {
		t.Fatal("all-children ambiguity must error (or foreground-pick)")
	}
}

// The picker needs the survivors, in z-order, on the error: rooting narrows
// the process, but several windows of that one process are still a choice.
func TestAmbiguousErrorCarriesMatches(t *testing.T) {
	l := list([]Window{{5, 20, "Report - Notepad"}, {3, 20, "todo - Notepad"}, {9, 20, "notes"}},
		map[uint32]string{20: `c:\notepad.exe`}, nil, 0)
	_, err := Select(l, Opts{App: "notepad"})
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("expected *AmbiguousError, got %v", err)
	}
	if len(amb.Matches) != 3 || amb.Matches[0].Hwnd != 5 || amb.Matches[2].Hwnd != 9 {
		t.Fatalf("matches must be the filtered survivors in z-order, got %+v", amb.Matches)
	}
	if !amb.RootingWasAttempt || amb.App != "notepad" {
		t.Fatalf("error should echo the selectors for the prompt, got %+v", amb)
	}
	if got, err := Select(l, Opts{App: "notepad", Topmost: true}); err != nil || got.Hwnd != 5 {
		t.Fatalf("--topmost must bypass the list, got %+v %v", got, err)
	}
}
