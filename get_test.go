package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zrs01/winfit/internal/place"
	"github.com/zrs01/winfit/internal/winapi"
)

func sampleView() *windowView {
	return &windowView{
		HWND: "0x001A2B3C", PID: 1234, App: "notepad", Class: "Notepad", Title: "untitled",
		Rect:    rectJSON{X: 100, Y: 50, Width: 150, Height: 150},
		Pos:     edgeJSON{Left: 100, Top: 50},
		Size:    sizeJSON{Width: 100, Height: 100},
		From:    fromJSON{Right: 1020, Bottom: 390},
		Pct:     pctJSON{Left: 5.2, Top: 4.8, Right: 53.1, Bottom: 37.5, Width: 5.2, Height: 9.6},
		Monitor: monitorJSON{Index: 2, Name: `\\.\DISPLAY2`},
		Work:    rectJSON{X: 0, Y: 0, Width: 1920, Height: 1040},
		Scale:   1.5,
	}
}

// The human report must carry the numbers a move takes back as flags.
func TestGetPrintReportsFlagValues(t *testing.T) {
	var buf bytes.Buffer
	sampleView().print(&buf)
	out := buf.String()
	for _, want := range []string{
		`"untitled"`, "0x001A2B3C", "pid 1234", "app notepad", "class Notepad",
		"left 100", "top 50", "width 100", "height 100", "right 1020", "bottom 390",
		"5.2%", `monitor 2 \\.\DISPLAY2`, "1920", "@150%",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "  note") {
		t.Errorf("ordinary window should carry no state note:\n%s", out)
	}
}

func TestGetPrintNotesWindowState(t *testing.T) {
	v := sampleView()
	v.Iconic = true
	var buf bytes.Buffer
	v.print(&buf)
	if !strings.Contains(buf.String(), "minimized") {
		t.Errorf("minimized window should say the rect is the restored geometry:\n%s", buf.String())
	}

	// WS_MAXIMIZE with a work-area-sized rect: the overhang is the invisible
	// resize border, so report it as a plain maximize.
	full := sampleView()
	full.Maxed = true
	full.Pct.Width, full.Pct.Height = 100.6, 101.1
	buf.Reset()
	full.print(&buf)
	if !strings.Contains(buf.String(), "maximized") || !strings.Contains(buf.String(), "work area") {
		t.Errorf("true maximize should be reported as filling the work area:\n%s", buf.String())
	}

	// WS_MAXIMIZE with a small rect (seen after SetWindowPos on Win11): the
	// note must not claim the rect spans the work area.
	partial := sampleView()
	partial.Maxed = true
	buf.Reset()
	partial.print(&buf)
	if !strings.Contains(buf.String(), "restore") {
		t.Errorf("stale WS_MAXIMIZE should warn that a move restores first:\n%s", buf.String())
	}
}

// Piping (`winfit get --json | jq`) depends on stable, flag-shaped keys.
func TestGetJSONShape(t *testing.T) {
	raw, err := json.Marshal(sampleView())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"hwnd", "pid", "app", "class", "title", "rect",
		"position", "size", "from", "percent", "monitor", "workArea", "scale"} {
		if _, ok := m[k]; !ok {
			t.Errorf("json missing key %q in %s", k, raw)
		}
	}
	pos := m["position"].(map[string]any)
	if pos["left"] != float64(100) || pos["top"] != float64(50) {
		t.Errorf("position should be the flag values, got %v", pos)
	}
	size := m["size"].(map[string]any)
	if size["width"] != float64(100) || size["height"] != float64(100) {
		t.Errorf("size should be logical px, got %v", size)
	}
	rect := m["rect"].(map[string]any)
	if rect["width"] != float64(150) {
		t.Errorf("rect should stay physical px, got %v", rect)
	}
}

func TestMonitorOf(t *testing.T) {
	mons := []winapi.Monitor{
		{Name: `\\.\DISPLAY1`, Work: place.Rect{X: 0, Y: 0, W: 1920, H: 1040}},
		{Name: `\\.\DISPLAY2`, Work: place.Rect{X: 1920, Y: 0, W: 2560, H: 1400}},
	}
	if i, n := monitorOf(mons, place.Rect{X: 1920, Y: 0, W: 2560, H: 1400}); i != 2 || n != `\\.\DISPLAY2` {
		t.Errorf("exact work-area match should win, got %d %q", i, n)
	}
	if i, n := monitorOf(mons, place.Rect{X: 1920, Y: 0, W: 2560, H: 1360}); i != 2 || n != `\\.\DISPLAY2` {
		t.Errorf("contained work area should fall back to its monitor, got %d %q", i, n)
	}
	if i, _ := monitorOf(mons, place.Rect{X: -5000, Y: 0, W: 10, H: 10}); i != 0 {
		t.Errorf("unknown work area should report monitor 0, got %d", i)
	}
}
