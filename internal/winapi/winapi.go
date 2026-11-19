// Package winapi is the thin Win32 layer: window enumeration, process
// and monitor queries, DPI, and the final SetWindowPos. No placement
// policy lives here — only system calls.
package winapi

import "github.com/zrs01/winfit/internal/place"

// Info describes one candidate window: handle, owning process, window class
// (GetClassName) and title (GetWindowText).
type Info struct {
	Hwnd  uintptr
	Pid   uint32
	Class string
	Title string
}

// Monitor describes one display by device name and work-area rectangle.
type Monitor struct {
	Name string // e.g. `\\.\DISPLAY1`
	Work place.Rect
}
