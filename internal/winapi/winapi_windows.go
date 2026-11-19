//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"github.com/zrs01/winfit/internal/place"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	pEnumWindows            = user32.NewProc("EnumWindows")
	pIsWindowVisible        = user32.NewProc("IsWindowVisible")
	pGetWindowTextW         = user32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW   = user32.NewProc("GetWindowTextLengthW")
	pGetClassNameW          = user32.NewProc("GetClassNameW")
	pGetWindow              = user32.NewProc("GetWindow")
	pGetAncestor            = user32.NewProc("GetAncestor")
	pGetWindowLongPtrW      = user32.NewProc("GetWindowLongPtrW")
	pGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	pGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	pGetConsoleWindow       = kernel32.NewProc("GetConsoleWindow") // kernel32, not user32
	pAttachConsole          = kernel32.NewProc("AttachConsole")
	pFreeConsole            = kernel32.NewProc("FreeConsole")
	pGetWindowRect          = user32.NewProc("GetWindowRect")
	pSetWindowPos           = user32.NewProc("SetWindowPos")
	pIsIconic               = user32.NewProc("IsIconic")
	pShowWindow             = user32.NewProc("ShowWindow")
	pMonitorFromWindow      = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW        = user32.NewProc("GetMonitorInfoW")
	pGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	pSetProcessDPIAware     = user32.NewProc("SetProcessDPIAware")
	pGetConsoleMode         = kernel32.NewProc("GetConsoleMode")
	pSetConsoleMode         = kernel32.NewProc("SetConsoleMode")

	pProcessIdToSessionId = kernel32.NewProc("ProcessIdToSessionId")
)

const (
	swRestore                  = 9
	swpNoZOrder                = 0x0004
	swpNoActivate              = 0x0010
	swpNoSize                  = 0x0001
	gwOwner                    = 4
	gaRootOwner                = 3
	gwlExStyle                 = -20
	wsExToolwindow             = 0x00000080
	wsExNoActivate             = 0x00020000
	wsCaption                  = 0x00C00000
	wsMaximize                 = 0x00010000
	monitorDefaultToNearest    = 2
	processQueryLimitedInfo    = 0x00100000
	smCxMin                    = 44
	smCyMin                    = 45
	attachParentProcess        = uint32(0xFFFFFFFF)
	dpiCtxPerMonitorAwareV2Ptr = uintptr(0xFFFFFFFFFFFFFFFC) // (DPI_AWARENESS_CONTEXT)-4
)

// Init makes this process per-monitor-v2 DPI aware,
// falling back to system DPI awareness on Windows 10 builds before 1607.
// Called before any coordinate is requested.
func Init() {
	if p := user32.NewProc("SetProcessDpiAwarenessContext"); p.Find() == nil {
		if r, _, _ := p.Call(dpiCtxPerMonitorAwareV2Ptr); r != 0 {
			return
		}
	}
	pSetProcessDPIAware.Call()
}

// --- candidate enumeration (UI-window filter) ---

var (
	candOut   []Info
	candTitle string // optional lowercase title filter
	candPid   uint32 // optional owning-PID filter (0 = any)
)

// syscall callback: must stay a top-level func with fixed signature.
// EnableVT switches a console handle to virtual-terminal processing so ANSI
// colors render. False when fd is not a console handle or the build refuses
// (pre-Win10), which is the signal to log without escape codes.
func EnableVT(fd uintptr) bool {
	var mode uint32
	if ok, _, _ := pGetConsoleMode.Call(fd, uintptr(unsafe.Pointer(&mode))); ok == 0 {
		return false
	}
	ok, _, _ := pSetConsoleMode.Call(fd, uintptr(mode|vtProcessingEnabled))
	return ok != 0
}

// vtProcessingEnabled is ENABLE_VIRTUAL_TERMINAL_PROCESSING.
const vtProcessingEnabled = 0x0004

func enumWindowsCB(hwnd, lParam uintptr) uintptr {
	if !visibleUIWindow(hwnd) {
		return 1
	}
	if candPid != 0 {
		var pid uint32
		pGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != candPid {
			return 1
		}
	}
	if candTitle != "" && !strings.Contains(strings.ToLower(title(hwnd)), candTitle) {
		return 1
	}
	pidProc, pid := pidOf(hwnd)
	_ = pidProc
	candOut = append(candOut, Info{Hwnd: hwnd, Pid: pid, Class: class(hwnd), Title: title(hwnd)})
	return 1 // continue enumerating
}

// Candidates lists top-level, visible, unowned, captioned windows in
// z-order (front to back), optionally filtered by title substring (case-
// insensitive) and owning PID.
func Candidates(titleSubstr string, pid uint32) ([]Info, error) {
	candOut, candTitle, candPid = nil, strings.ToLower(titleSubstr), pid
	// Filter out our own console window from generic listings is the
	// target layer's job; we just enumerate.
	r, _, e := pEnumWindows.Call(syscall.NewCallback(enumWindowsCB), 0)
	if r == 0 {
		return nil, fmt.Errorf("EnumWindows failed: %v", e)
	}
	return candOut, nil
}

func visibleUIWindow(hwnd uintptr) bool {
	if v, _, _ := pIsWindowVisible.Call(hwnd); v == 0 {
		return false
	}
	// unowned top-level only
	if o, _, _ := pGetWindow.Call(hwnd, gwOwner); o != 0 {
		return false
	}
	ex, _, _ := pGetWindowLongPtrW.Call(hwnd, uptr(gwlExStyle))
	if ex&uintptr(wsExToolwindow|wsExNoActivate) != 0 {
		return false
	}
	style := getStyle(hwnd)
	if style&wsCaption == 0 {
		return false
	}
	n, _, _ := pGetWindowTextLengthW.Call(hwnd)
	return n > 0
}

func getStyle(hwnd uintptr) uintptr {
	// GWL_STYLE = -16; negative index converted at runtime (wraps to two's
	// complement, matching the Win32 signature).
	if p := user32.NewProc("GetWindowLongPtrW"); p.Find() == nil {
		r, _, _ := p.Call(hwnd, uptr(-16))
		return r
	}
	r, _, _ := user32.NewProc("GetWindowLongW").Call(hwnd, uptr(-16))
	return r
}

// uptr converts a negative Win32 index to its uintptr representation.
func uptr(i int) uintptr { return uintptr(i) }

func title(hwnd uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

func class(hwnd uintptr) string {
	buf := make([]uint16, 257)
	pGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

func pidOf(hwnd uintptr) (thread, pid uint32) {
	r, _, _ := pGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return uint32(r), pid
}

// --- process helpers (multi-process rooting) ---

// ImageName returns the target process's executable path; "" on failure
// (e.g. protected processes we may not open).
func ImageName(pid uint32) string {
	// Some Windows 10-era processes (UWP-hosted ones like Notepad) refuse
	// PROCESS_QUERY_LIMITED_INFORMATION but allow PROCESS_QUERY_INFORMATION,
	// so try the full mask first and fall back.
	for _, access := range []uint32{0x1000 /*QUERY_INFORMATION*/, processQueryLimitedInfo} {
		h, err := windows.OpenProcess(access, false, pid)
		if err != nil {
			continue
		}
		buf := make([]uint16, 32768)
		size := uint32(len(buf))
		qerr := windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
		windows.CloseHandle(h)
		if qerr == nil {
			return windows.UTF16ToString(buf[:size])
		}
	}
	return ""
}

// ParentOf returns pid's parent pid via a toolhelp snapshot; 0 on failure.
func ParentOf(pid uint32) uint32 {
	snap, _, _ := kernel32.NewProc("CreateToolhelp32Snapshot").Call(2 /*TH32CS_SNAPPROCESS*/, 0)
	if snap == ^uintptr(0) {
		return 0
	}
	defer windows.CloseHandle(windows.Handle(snap))
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	pFirst := kernel32.NewProc("Process32FirstW")
	pNext := kernel32.NewProc("Process32NextW")
	for r, _, _ := pFirst.Call(snap, uintptr(unsafe.Pointer(&pe))); r != 0; r, _, _ = pNext.Call(snap, uintptr(unsafe.Pointer(&pe))) {
		if pe.ProcessID == pid {
			return pe.ParentProcessID
		}
	}
	return 0
}

// CurrentPID is this process's PID.
func CurrentPID() uint32 { return uint32(windows.GetCurrentProcessId()) }

// Foreground returns the foreground window (0 if none).
func Foreground() uintptr {
	h, _, _ := pGetForegroundWindow.Call()
	return h
}

// OwnsSameUserSession reports whether pid runs in our session (different
// users are refused by the cross-user guard).
func OwnsSameUserSession(pid uint32) bool {
	var mine, theirs uint32
	r1, _, _ := pProcessIdToSessionId.Call(uintptr(windows.GetCurrentProcessId()), uintptr(unsafe.Pointer(&mine)))
	r2, _, _ := pProcessIdToSessionId.Call(uintptr(pid), uintptr(unsafe.Pointer(&theirs)))
	return r1 != 0 && r2 != 0 && mine == theirs
}

// --- self window (default target) ---

// SelfWindow returns the console window to move: our console window's root
// owner (under Windows Terminal this is the WindowsTerminal.exe top-level,
// not the hidden pseudo-console).
func SelfWindow() (uintptr, error) {
	h, _, _ := pGetConsoleWindow.Call()
	if h == 0 {
		// Detached: try attaching to the parent's console briefly.
		pFreeConsole.Call()
		if r, _, _ := pAttachConsole.Call(uintptr(attachParentProcess)); r == 0 {
			return 0, errors.New("cannot locate a console window; pass --title, --app or --hwnd")
		}
		h, _, _ = pGetConsoleWindow.Call()
	}
	if h == 0 {
		return 0, errors.New("cannot locate a console window; pass --title, --app or --hwnd")
	}
	root, _, _ := pGetAncestor.Call(h, gaRootOwner)
	if root != 0 {
		h = root
	}
	return h, nil
}

// --- geometry ---

// WindowRect returns the window's current rectangle (physical px).
func WindowRect(hwnd uintptr) (place.Rect, error) {
	var r rectWin
	if v, _, _ := pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); v == 0 {
		return place.Rect{}, errors.New("GetWindowRect failed (window gone?)")
	}
	return place.Rect{X: int(r.left), Y: int(r.top), W: int(r.right - r.left), H: int(r.bottom - r.top)}, nil
}

// WorkArea returns the work area of the monitor nearest hwnd (taskbar
// excluded).
func WorkArea(hwnd uintptr) (place.Rect, error) {
	mon, _, _ := pMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if mon == 0 {
		return place.Rect{}, errors.New("MonitorFromWindow failed")
	}
	return monitorWorkArea(mon)
}

// Monitors lists all active monitors by device name.
func Monitors() ([]Monitor, error) {
	pEnum := user32.NewProc("EnumDisplayMonitors")
	var mons []Monitor
	cb := syscall.NewCallback(func(hmon, _, lParam uintptr) uintptr {
		if w, err := monitorWorkArea(hmon); err == nil {
			mons = append(mons, Monitor{Name: monitorName(hmon), Work: w})
		}
		return 1
	})
	if r, _, _ := pEnum.Call(0, 0, cb, 0); r == 0 {
		return nil, errors.New("EnumDisplayMonitors failed")
	}
	return mons, nil
}

type monitorInfoEx struct {
	size  uint32
	rc    rectWin
	work  rectWin
	flags uint32
	name  [32]uint16
}

func monitorWorkArea(hmon uintptr) (place.Rect, error) {
	var mi monitorInfoEx
	mi.size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := pGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return place.Rect{}, errors.New("GetMonitorInfo failed")
	}
	return place.Rect{X: int(mi.work.left), Y: int(mi.work.top),
		W: int(mi.work.right - mi.work.left), H: int(mi.work.bottom - mi.work.top)}, nil
}

func monitorName(hmon uintptr) string {
	var mi monitorInfoEx
	mi.size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := pGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(mi.name[:])
}

// Scale returns the window's DPI scale factor (logical to physical px multiplier).
func Scale(hwnd uintptr) float64 {
	if p := user32.NewProc("GetDpiForWindow"); p.Find() == nil {
		dpi, _, _ := p.Call(hwnd, 0)
		if dpi != 0 {
			return float64(dpi) / 96
		}
	}
	return 1
}

// SystemMinSizes returns SM_CXMIN/SM_CYMIN minimum tracking sizes.
func SystemMinSizes() (int, int) {
	cx, _, _ := pGetSystemMetrics.Call(smCxMin)
	cy, _, _ := pGetSystemMetrics.Call(smCyMin)
	return int(cx), int(cy)
}

// IsWindow reports whether hwnd is still a live window.
func IsWindow(hwnd uintptr) bool {
	getIsWindow := user32.NewProc("IsWindow")
	r, _, _ := getIsWindow.Call(hwnd)
	return r != 0
}

// SetWindowRect restores the window from minimized/maximized state if
// needed, then applies the rectangle with a single SetWindowPos call
// resize=false keeps the current size.
func SetWindowRect(hwnd uintptr, r place.Rect, resize bool) error {
	if v, _, _ := pIsIconic.Call(hwnd); v != 0 {
		pShowWindow.Call(hwnd, swRestore)
	} else if getStyle(hwnd)&wsMaximize != 0 {
		pShowWindow.Call(hwnd, swRestore)
	}
	flags := uintptr(swpNoZOrder | swpNoActivate)
	h, w := uintptr(0), uintptr(0)
	if resize {
		w, h = uintptr(r.W), uintptr(r.H)
	} else {
		flags |= swpNoSize
	}
	if r, _, e := pSetWindowPos.Call(hwnd, 0, uintptr(int32(r.X)), uintptr(int32(r.Y)), w, h, flags); r == 0 {
		return fmt.Errorf("SetWindowPos failed (%v); target elevated? run elevated", e)
	}
	return nil
}

type rectWin struct {
	left, top, right, bottom int32
}
