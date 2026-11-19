// Package picker renders the interactive "which window?" list shown when a
// selector matches several windows. It is UI only: no Win32 calls, no
// geometry — callers hand it ready-made rows.
package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item is one row of the list, pre-formatted by the caller (HWND/PID/app
// come from internal/winapi, z-order is preserved).
type Item struct {
	Hwnd  uintptr
	Pid   uint32
	App   string
	Class string
	Title string
}

// Label is the row text: everything the user needs to tell windows apart.
func (i Item) Label() string {
	return fmt.Sprintf("0x%08X  %s  %s", i.Hwnd, orDash(i.App), i.Title)
}

// Haystack is what the typed filter matches against (case-insensitive).
func (i Item) Haystack() string {
	return strings.ToLower(fmt.Sprintf("%s %s %s 0x%x", i.Title, i.App, i.Class, i.Hwnd))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Result carries the outcome: exactly one of Item / Canceled is meaningful.
type Result struct {
	Canceled bool
	Index    int // index into the slice passed to Pick, -1 when canceled
	Item     Item
}

// Pick shows items (front to back) and blocks until the user presses enter
// (choice) or esc / ctrl-c (Result.Canceled). It needs a terminal; callers
// check for one before calling (see run.go's terminal check).
func Pick(items []Item, prompt string) (Result, error) {
	m := newModel(items, prompt)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return Result{Canceled: true, Index: -1}, err
	}
	return m.result(), nil
}

const maxVisible = 12 // rows drawn at once; the rest scroll

type model struct {
	items    []Item
	visible  []int // indices into items after filtering, z-order kept
	prompt   string
	filter   string
	cursor   int // index into visible
	choices  int // index into items of the accepted row (-1 = canceled)
	width    int
	height   int
	quitting bool
}

func newModel(items []Item, prompt string) *model {
	m := &model{items: items, choices: -1, width: 80, height: maxVisible + 3}
	m.prompt = prompt
	m.refilter()
	return m
}

func (m *model) result() Result {
	if m.choices < 0 {
		return Result{Canceled: true, Index: -1}
	}
	return Result{Index: m.choices, Item: m.items[m.choices]}
}

// refilter rebuilds `visible` from the typed filter and clamps the cursor.
func (m *model) refilter() {
	m.visible = m.visible[:0]
	f := strings.ToLower(m.filter)
	for i, it := range m.items {
		if f == "" || strings.Contains(it.Haystack(), f) {
			m.visible = append(m.visible, i)
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = 0
	}
}

var (
	styleCursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleTitle  = lipgloss.NewStyle().Bold(true)
	styleErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if msg.String() == "esc" && m.filter != "" {
				m.filter = ""
				m.cursor = 0
				m.refilter()
				return m, nil
			}
			m.quitting, m.choices = true, -1
			return m, tea.Quit

		case "enter":
			if len(m.visible) > 0 {
				m.quitting, m.choices = true, m.visible[m.cursor]
			}
			return m, tea.Quit

		case "backspace":
			if m.filter == "" {
				return m, nil
			}
			r := []rune(m.filter)
			m.filter = string(r[:len(r)-1])
			m.cursor = 0
			m.refilter()
			return m, nil

		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup", "ctrl+u":
			m.move(-maxVisible)
		case "pgdown", "ctrl+d":
			m.move(maxVisible)
		case "home", "ctrl+a":
			m.cursor = 0
		case "end", "ctrl+e":
			m.cursor = len(m.visible) - 1

		default:
			if msg.Type == tea.KeyRunes { // typed text is the filter
				m.filter += string(msg.Runes)
				m.cursor = 0
				m.refilter()
			}
		}
	}
	return m, nil
}

func (m *model) move(d int) {
	n := len(m.visible)
	if n == 0 {
		return
	}
	m.cursor += d
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
}

func (m *model) View() string {
	head := fmt.Sprintf("%s  %s", styleTitle.Render(m.prompt), m.filter)
	if m.quitting {
		return head + "\n"
	}
	var b strings.Builder
	b.WriteString(head + "\n")
	b.WriteString(styleDim.Render("up/down move | type to filter | enter pick | esc cancel") + "\n")
	if len(m.visible) == 0 {
		b.WriteString(styleErr.Render("no row matches the filter") + "\n")
		return b.String()
	}
	// Scroll so the cursor stays inside the drawn window, and say how many
	// rows the user cannot currently see (above and below).
	top := max(m.cursor-(maxVisible-1), 0)
	if top > len(m.visible)-maxVisible {
		top = len(m.visible) - maxVisible
	}
	if top < 0 {
		top = 0
	}
	if top > 0 {
		fmt.Fprintf(&b, "  %s\n", styleDim.Render(fmt.Sprintf("^ %d above", top)))
	}
	for i := top; i < len(m.visible) && i < top+maxVisible; i++ {
		it := m.items[m.visible[i]]
		label := truncate(it.Label(), m.width-4)
		if i == m.cursor {
			fmt.Fprintf(&b, "%s %s\n", styleCursor.Render(">"), label)
		} else {
			fmt.Fprintf(&b, "  %s\n", styleDim.Render(label))
		}
	}
	if hidden := len(m.visible) - (min(top+maxVisible, len(m.visible))); hidden > 0 {
		fmt.Fprintf(&b, "  %s\n", styleDim.Render(fmt.Sprintf("v %d more", hidden)))
	}
	return b.String()
}

func truncate(s string, n int) string {
	if n < 8 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "~"
}
