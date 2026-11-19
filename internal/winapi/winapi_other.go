//go:build !windows

package winapi

import (
	"errors"

	"github.com/zrs01/winfit/internal/place"
)

// errUnsupported: the CLI builds everywhere but
// only moves windows on Windows.
var errUnsupported = errors.New("winfit only supports moving windows on Windows")

// Init is a no-op off Windows.
func Init() {}

// EnableVT reports true off Windows: the caller installs the colored handler
// only for a character-device stderr, and POSIX terminals speak ANSI.
func EnableVT(fd uintptr) bool { return true }

// Candidates is unsupported off Windows.
func Candidates(titleSubstr string, pid uint32) ([]Info, error) { return nil, errUnsupported }

// ImageName is unsupported off Windows.
func ImageName(pid uint32) string { return "" }

// ParentOf is unsupported off Windows.
func ParentOf(pid uint32) uint32 { return 0 }

// CurrentPID returns 0 off Windows.
func CurrentPID() uint32 { return 0 }

// Foreground returns 0 off Windows.
func Foreground() uintptr { return 0 }

// OwnsSameUserSession is unsupported off Windows.
func OwnsSameUserSession(pid uint32) bool { return false }

// SelfWindow is unsupported off Windows.
func SelfWindow() (uintptr, error) { return 0, errUnsupported }

// WindowRect is unsupported off Windows.
func WindowRect(hwnd uintptr) (place.Rect, error) { return place.Rect{}, errUnsupported }

// WorkArea is unsupported off Windows.
func WorkArea(hwnd uintptr) (place.Rect, error) { return place.Rect{}, errUnsupported }

// Monitors is unsupported off Windows.
func Monitors() ([]Monitor, error) { return nil, errUnsupported }

// Scale returns 1 off Windows.
func Scale(hwnd uintptr) float64 { return 1 }

// SystemMinSizes returns zero minimums off Windows.
func SystemMinSizes() (int, int) { return 0, 0 }

// IsWindow is unsupported off Windows.
func IsWindow(hwnd uintptr) bool { return false }

// SetWindowRect is unsupported off Windows.
func SetWindowRect(hwnd uintptr, r place.Rect, resize bool) error { return errUnsupported }
