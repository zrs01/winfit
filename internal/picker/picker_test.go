package picker

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sample() []Item {
	return []Item{
		{Hwnd: 0x11, Pid: 10, App: "chrome", Class: "Chrome_WidgetWin_1", Title: "Tab A — Chrome"},
		{Hwnd: 0x22, Pid: 10, App: "chrome", Class: "Chrome_WidgetWin_1", Title: "Tab B — Chrome"},
		{Hwnd: 0x33, Pid: 20, App: "notepad", Class: "Notepad", Title: "notes.txt - Notepad"},
	}
}

func send(m *model, msg tea.Msg) tea.Cmd {
	_, cmd := m.Update(msg)
	return cmd
}

func key(s string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func named(t *testing.T, s string) tea.Msg {
	t.Helper()
	k, err := keyByName(s)
	if err != nil {
		t.Fatalf("key %q: %v", s, err)
	}
	return k
}

func keyByName(s string) (tea.Msg, error) {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}, nil
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}, nil
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}, nil
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}, nil
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}, nil
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}, nil
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}, nil
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}, nil
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}, nil
	}
	return nil, fmt.Errorf("unknown key %q", s)
}

func TestFilterNarrowsAndResetsCursor(t *testing.T) {
	m := newModel(sample(), "pick:")
	if len(m.visible) != 3 {
		t.Fatalf("no filter should show all rows, got %d", len(m.visible))
	}
	send(m, key("chrome"))
	if len(m.visible) != 2 {
		t.Fatalf("filter should leave the 2 chrome rows, got %d", len(m.visible))
	}
	send(m, named(t, "down"))
	if got := m.items[m.visible[m.cursor]].Hwnd; got != 0x22 {
		t.Fatalf("down should move to row 2, got 0x%X", got)
	}
	send(m, key("x")) // no row matches
	if len(m.visible) != 0 {
		t.Fatalf("impossible filter must empty the list, got %d", len(m.visible))
	}
	if m.cursor != 0 {
		t.Fatalf("cursor must clamp to 0 on an empty list, got %d", m.cursor)
	}
	if send(m, named(t, "enter")) == nil || m.choices != -1 {
		t.Fatal("enter on an empty list must cancel rather than choose")
	}
}

func TestTypedFilterClearsBeforeCancel(t *testing.T) {
	m := newModel(sample(), "pick:")
	send(m, key("notes"))
	send(m, named(t, "esc")) // first esc clears the filter, stays open
	if m.filter != "" || len(m.visible) != 3 || m.quitting {
		t.Fatalf("esc should only clear the filter: %q %d quitting=%v", m.filter, len(m.visible), m.quitting)
	}
	send(m, named(t, "esc")) // second esc cancels
	if !m.quitting || m.choices != -1 {
		t.Fatalf("esc with an empty filter must cancel, got choices=%d", m.choices)
	}
	if r := m.result(); !r.Canceled || r.Index != -1 {
		t.Fatalf("cancel result must be Canceled/-1, got %+v", r)
	}
}

func TestEnterChoosesByOriginalIndex(t *testing.T) {
	m := newModel(sample(), "pick:")
	send(m, named(t, "down"))
	send(m, named(t, "down"))
	send(m, named(t, "enter"))
	r := m.result()
	if r.Canceled || r.Index != 2 || r.Item.Hwnd != 0x33 {
		t.Fatalf("enter must return the z-order index of the row, got %+v", r)
	}
}

func TestFilterMatchesHandleAndClass(t *testing.T) {
	m := newModel(sample(), "pick:")
	send(m, key("0x33"))
	if len(m.visible) != 1 || m.items[m.visible[0]].Hwnd != 0x33 {
		t.Fatalf("hex handle must be filterable, got %d rows", len(m.visible))
	}
	m = newModel(sample(), "pick:")
	send(m, key("widgetwin"))
	if len(m.visible) != 2 {
		t.Fatalf("class must be filterable, got %d rows", len(m.visible))
	}
}

func TestViewScrollsAndTruncates(t *testing.T) {
	items := make([]Item, 40)
	for i := range items {
		items[i] = Item{Hwnd: uintptr(0x100 + i), App: "app", Title: longTitle(i)}
	}
	m := newModel(items, "pick:")
	m.width = 40
	send(m, named(t, "end"))
	view := m.View()
	if lines := countLines(view); lines > maxVisible+4 {
		t.Fatalf("view must stay bounded, got %d lines", lines)
	}
	if !strings.Contains(view, "above") {
		t.Fatalf("view should indicate the scrolled-off rows: %q", view)
	}
	if !strings.Contains(newModel(items, "pick:").View(), "more") {
		t.Fatalf("view should indicate the rows still below the fold")
	}
	if strings.Contains(view, longTitle(len(items)-1)) {
		t.Fatal("long titles must be truncated in the view")
	}
}

func longTitle(i int) string {
	s := "window"
	for len(s) < 200 {
		s += fmt.Sprintf(" %d ", i)
	}
	return s
}

func countLines(s string) int {
	n := 1
	for _, r := range s {
		if r == '\n' {
			n++
		}
	}
	return n
}
