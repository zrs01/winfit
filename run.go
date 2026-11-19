package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/zrs01/winfit/internal/place"
	"github.com/zrs01/winfit/internal/target"
	"github.com/zrs01/winfit/internal/winapi"

	"github.com/urfave/cli/v3"
)

// runMove implements the root command: resolve target, compute the final
// rect, and apply it.
func runMove(_ context.Context, c *cli.Command) error {
	spec, err := specFromFlags(c)
	if err != nil {
		return err
	}
	monitor := c.String("monitor")
	if !spec.Changed() && monitor == "" {
		if err := cli.ShowAppHelp(c); err != nil {
			return err
		}
		return cli.Exit("nothing to do: pass position (--top/--left/...) or size (--width/--height) flags", 2)
	}

	winapi.Init()
	hwnd, pid, title, err := resolveTarget(c)
	if err != nil {
		return err
	}
	slog.Debug("target window",
		"hwnd", fmt.Sprintf("0x%X", hwnd), "pid", pid, "title", title)

	if pid != 0 && pid != winapi.CurrentPID() && !winapi.OwnsSameUserSession(pid) {
		return fmt.Errorf("window is owned by another logged-in user (pid %d); refusing to move it", pid)
	}

	cur, err := winapi.WindowRect(hwnd)
	if err != nil {
		return err
	}
	ownWA, err := winapi.WorkArea(hwnd)
	if err != nil {
		return err
	}
	wa, originWA := ownWA, (*place.Rect)(nil)
	if monitor != "" {
		wa, err = monitorWorkArea(monitor, ownWA)
		if err != nil {
			return err
		}
		if !spec.HasPosition() { // carry current offsets onto the new monitor
			origin := ownWA
			originWA = &origin
		}
	}

	// Logical → physical px for pixel-valued flags.
	scale := winapi.Scale(hwnd)
	spec.ScaleLogical(scale)

	minW, minH := winapi.SystemMinSizes()
	spec.OriginWA = originWA
	res := place.Compute(wa, cur, spec, minW, minH)
	for _, w := range res.Warnings {
		slog.Warn(w)
	}
	if err := winapi.SetWindowRect(hwnd, res.Rect, res.Resized); err != nil {
		return err
	}

	// Corrective pass: the target may have refused the size via
	// WM_GETMINMAXINFO; containment is checked against the ACTUAL rect.
	if res.Resized {
		if actual, err := winapi.WindowRect(hwnd); err == nil && !contained(actual, wa) {
			fixed := place.Rect{
				X: clamp(actual.X, wa.X, wa.Right()-actual.W),
				Y: clamp(actual.Y, wa.Y, wa.Bottom()-actual.H),
				W: actual.W, H: actual.H,
			}
			slog.Debug("corrective containment pass", "actual", actual, "fixed", fixed)
			if err := winapi.SetWindowRect(hwnd, fixed, true); err != nil {
				return err
			}
		}
	}
	final, err := winapi.WindowRect(hwnd)
	if err != nil {
		return err
	}
	verb := "moved"
	if !res.Moved && !res.Resized {
		verb = "checked"
	}
	fmt.Printf("%s %q (hwnd 0x%X) to %d,%d %dx%d\n", verb, title, hwnd, final.X, final.Y, final.W, final.H)
	return nil
}

// runList implements `winfit list`: candidate windows in z-order with
// their HWND/PID/APP/CLASS/TITLE.
func runList(_ context.Context, c *cli.Command) error {
	winapi.Init()
	cands, err := winapi.Candidates(c.String("title"), 0)
	if err != nil {
		return err
	}
	app := strings.ToLower(strings.TrimSuffix(c.String("app"), ".exe"))
	imgCache := map[uint32]string{}
	appName := func(pid uint32) string {
		s, ok := imgCache[pid]
		if !ok {
			s = trimExe(winapi.ImageName(pid))
			imgCache[pid] = s
		}
		return s
	}
	out := os.Stdout
	fmt.Fprintf(out, "%-12s %-8s %-24s %-24s %s\n", "HWND", "PID", "APP", "CLASS", "TITLE")
	for _, w := range cands {
		if w.Pid == winapi.CurrentPID() {
			continue // never list our own console as a target
		}
		if app != "" && !strings.Contains(strings.ToLower(appName(w.Pid)), app) {
			continue
		}
		fmt.Fprintf(out, "0x%08X  %-8d %-24s %-24s %s\n", w.Hwnd, w.Pid, appName(w.Pid), w.Class, w.Title)
	}
	return nil
}

// resolveTarget maps the selector flags to a window, following the
// precedence: --hwnd → --title → --app → self.
func resolveTarget(c *cli.Command) (hwnd uintptr, pid uint32, title string, err error) {
	if h := c.String("hwnd"); h != "" {
		v, perr := strconv.ParseUint(strings.TrimPrefix(strings.TrimPrefix(h, "0x"), "0X"), 16, 64)
		if perr != nil {
			return 0, 0, "", fmt.Errorf("invalid --hwnd %q (expected hex like 0x001A2B3C)", h)
		}
		hwnd = uintptr(v)
		if !winapi.IsWindow(hwnd) {
			return 0, 0, "", fmt.Errorf("hwnd 0x%X is not a live window (handles change per launch; re-run `winfit list`)", hwnd)
		}
		for _, w := range mustCandidates("") {
			if w.Hwnd == hwnd {
				return hwnd, w.Pid, w.Title, nil
			}
		}
		// Live but not a UI candidate (hidden/tool window): still movable,
		// pid unknown to us.
		return hwnd, 0, winapiTitleOf(hwnd), nil
	}
	opts := target.Opts{Title: c.String("title"), App: c.String("app"), Topmost: c.Bool("topmost")}
	if opts.Selectors() {
		cands := mustCandidates(opts.Title)
		windows := make([]target.Window, 0, len(cands))
		for _, w := range cands {
			windows = append(windows, target.Window{Hwnd: w.Hwnd, Pid: w.Pid, Title: w.Title})
		}
		imgCache := map[uint32]string{}
		l := target.List{
			Windows: windows,
			Image: func(p uint32) string {
				s, ok := imgCache[p]
				if !ok {
					s = winapi.ImageName(p)
					imgCache[p] = s
				}
				return s
			},
			Parent:        winapi.ParentOf,
			ForegroundPid: fgPid(),
		}
		w, err := target.Select(l, opts)
		if err != nil {
			var amb *target.AmbiguousError
			if !errors.As(err, &amb) {
				return 0, 0, "", err
			}
			w, err = chooseInteractively(amb)
			if err != nil {
				return 0, 0, "", err
			}
		}
		return w.Hwnd, w.Pid, w.Title, nil
	}
	hwnd, err = winapi.SelfWindow()
	if err != nil {
		return 0, 0, "", err
	}
	return hwnd, winapi.CurrentPID(), consoleTitle(), nil
}

func mustCandidates(titleSubstr string) []winapi.Info {
	cands, err := winapi.Candidates(titleSubstr, 0)
	if err != nil {
		panic(err) // surfaced via the caller's error return below
	}
	return cands
}

func fgPid() uint32 {
	fg := winapi.Foreground()
	if fg == 0 {
		return 0
	}
	for _, w := range mustCandidates("") {
		if w.Hwnd == fg {
			return w.Pid
		}
	}
	return 0
}

func winapiTitleOf(hwnd uintptr) string {
	for _, w := range mustCandidates("") {
		if w.Hwnd == hwnd {
			return w.Title
		}
	}
	return ""
}

func consoleTitle() string {
	h, err := winapi.SelfWindow()
	if err != nil {
		return ""
	}
	return winapiTitleOf(h)
}

// monitorWorkArea resolves --monitor by 1-based index or device name.
func monitorWorkArea(ref string, fallback place.Rect) (place.Rect, error) {
	mons, err := winapi.Monitors()
	if err != nil {
		return fallback, err
	}
	if n, cerr := strconv.Atoi(ref); cerr == nil {
		if n < 1 || n > len(mons) {
			return fallback, fmt.Errorf("monitor index %d out of range (1..%d)", n, len(mons))
		}
		return mons[n-1].Work, nil
	}
	for _, m := range mons {
		if strings.EqualFold(m.Name, ref) {
			return m.Work, nil
		}
	}
	names := make([]string, 0, len(mons))
	for _, m := range mons {
		names = append(names, m.Name)
	}
	return fallback, fmt.Errorf("unknown monitor %q; available: %s", ref, strings.Join(names, ", "))
}

func contained(r, wa place.Rect) bool {
	return r.X >= wa.X && r.Right() <= wa.Right() && r.Y >= wa.Y && r.Bottom() <= wa.Bottom()
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func baseName(path string) string {
	i := strings.LastIndexAny(path, `/\`)
	return strings.TrimSuffix(strings.ToLower(path[i+1:]), ".exe")
}

// trimExe is the display form: basename without .exe, case preserved.
func trimExe(path string) string {
	i := strings.LastIndexAny(path, `/\`)
	name := path[i+1:]
	if strings.HasSuffix(strings.ToUpper(name), ".EXE") {
		name = name[:len(name)-4]
	}
	return name
}
