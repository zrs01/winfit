package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/zrs01/winfit/internal/place"
	"github.com/zrs01/winfit/internal/winapi"

	"github.com/urfave/cli/v3"
)

// runGet implements `winfit get`: read a target window's position and size
// without changing anything. The position/size/from blocks are the values
// the move flags take (logical px, measured from the work-area edges), so a
// `get` result can be pasted straight into a `move`.
func runGet(_ context.Context, c *cli.Command) error {
	winapi.Init()
	hwnd, pid, title, err := resolveTarget(c)
	if err != nil {
		return err
	}

	cur, err := winapi.WindowRect(hwnd)
	if err != nil {
		return err
	}
	wa, err := winapi.WorkArea(hwnd)
	if err != nil {
		return err
	}
	scale := winapi.Scale(hwnd)
	iconic, maximized := winapi.WindowState(hwnd)

	offs := place.EdgeOffsetsFrom(wa, cur)
	pct := place.PercentagesFrom(wa, cur)
	mon, monName := 0, ""
	if mons, err := winapi.Monitors(); err == nil {
		mon, monName = monitorOf(mons, wa)
	}

	view := &windowView{
		HWND:    fmt.Sprintf("0x%08X", hwnd),
		PID:     pid,
		App:     trimExe(winapi.ImageName(pid)),
		Class:   classOf(hwnd),
		Title:   title,
		Rect:    rectOf(cur),
		Pos:     edgeJSON{Left: place.PhysicalToLogical(offs.Left, scale), Top: place.PhysicalToLogical(offs.Top, scale)},
		Size:    sizeJSON{Width: place.PhysicalToLogical(cur.W, scale), Height: place.PhysicalToLogical(cur.H, scale)},
		From:    fromJSON{Right: place.PhysicalToLogical(offs.Right, scale), Bottom: place.PhysicalToLogical(offs.Bottom, scale)},
		Pct:     pctJSON(pct),
		Monitor: monitorJSON{Index: mon, Name: monName},
		Work:    rectOf(wa),
		Scale:   scale,
		Iconic:  iconic,
		Maxed:   maximized,
	}

	if c.Bool("json") {
		return json.NewEncoder(os.Stdout).Encode(view)
	}
	view.print(os.Stdout)
	return nil
}

type rectJSON struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func rectOf(r place.Rect) rectJSON {
	return rectJSON{X: r.X, Y: r.Y, Width: r.W, Height: r.H}
}

type sizeJSON struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type edgeJSON struct {
	Left int `json:"left"`
	Top  int `json:"top"`
}

type fromJSON struct {
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
}

type pctJSON struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type monitorJSON struct {
	Index int    `json:"index"` // 1-based, 0 when unknown
	Name  string `json:"name"`  // e.g. `\\.\DISPLAY2`
}

// windowView is one window measurement. rect/workArea are physical px as the
// Win32 API reports them; position/size/from are the logical px the flags
// speak; percent is scale-invariant.
type windowView struct {
	HWND    string      `json:"hwnd"`
	PID     uint32      `json:"pid"`
	App     string      `json:"app"`
	Class   string      `json:"class"`
	Title   string      `json:"title"`
	Rect    rectJSON    `json:"rect"`
	Pos     edgeJSON    `json:"position"`
	Size    sizeJSON    `json:"size"`
	From    fromJSON    `json:"from"`
	Pct     pctJSON     `json:"percent"`
	Monitor monitorJSON `json:"monitor"`
	Work    rectJSON    `json:"workArea"`
	Scale   float64     `json:"scale"`
	Iconic  bool        `json:"minimized"`
	Maxed   bool        `json:"maximized"`
}

func (v *windowView) print(out io.Writer) {
	fmt.Fprintf(out, "%q hwnd %s pid %d app %s class %s\n", v.Title, v.HWND, v.PID, v.App, v.Class)
	fmt.Fprintf(out, "  pos     left %d (%.1f%%)   top %d (%.1f%%)\n",
		v.Pos.Left, v.Pct.Left, v.Pos.Top, v.Pct.Top)
	fmt.Fprintf(out, "  size    width %d (%.1f%%)  height %d (%.1f%%)\n",
		v.Size.Width, v.Pct.Width, v.Size.Height, v.Pct.Height)
	fmt.Fprintf(out, "  from    right %d (%.1f%%)  bottom %d (%.1f%%)\n",
		v.From.Right, v.Pct.Right, v.From.Bottom, v.Pct.Bottom)
	monitor := "unknown"
	if v.Monitor.Index > 0 {
		monitor = fmt.Sprintf("%d %s", v.Monitor.Index, v.Monitor.Name)
	}
	fmt.Fprintf(out, "  rect    %d,%d %dx%d px on work area %d,%d %dx%d  monitor %s @%.0f%%\n",
		v.Rect.X, v.Rect.Y, v.Rect.Width, v.Rect.Height,
		v.Work.X, v.Work.Y, v.Work.Width, v.Work.Height, monitor, v.Scale*100)
	switch {
	case v.Iconic:
		fmt.Fprintln(out, "  note    minimized: rect is its restored geometry")
	case v.Maxed && v.Pct.Width >= maximizedFillPct && v.Pct.Height >= maximizedFillPct:
		fmt.Fprintln(out, "  note    maximized: rect is the work area plus the invisible resize border")
	case v.Maxed:
		fmt.Fprintln(out, "  note    WS_MAXIMIZE is set but the rect is smaller; a move restores it first")
	}
}

// maximizedFillPct is how much of the work area a rect must cover before
// WS_MAXIMIZE is taken at face value. A genuinely maximized window overhangs
// the work area by the invisible resize border, so coverage is >100%.
const maximizedFillPct = 95

// classOf returns the window class, "" when the window is not a listed
// candidate.
func classOf(hwnd uintptr) string {
	for _, w := range mustCandidates("") {
		if w.Hwnd == hwnd {
			return w.Class
		}
	}
	return ""
}

// monitorOf finds the monitor owning work area wa: exact work-area match
// first, then the monitor whose work area contains it (mirrored displays).
// Returns index 0 when nothing matches.
func monitorOf(mons []winapi.Monitor, wa place.Rect) (int, string) {
	for i, m := range mons {
		if m.Work == wa {
			return i + 1, m.Name
		}
	}
	for i, m := range mons {
		if contained(wa, m.Work) {
			return i + 1, m.Name
		}
	}
	return 0, ""
}
