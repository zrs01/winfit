// Package place computes the final window rectangle from a placement spec.
// It is deliberately syscall-free: all inputs are plain numbers so the
// geometry is unit-testable on any platform.
package place

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Value is one parsed flag value: a pixel count (Pct=false) or a percentage
// (Pct=true) of the reference monitor's work-area extent on that axis.
type Value struct {
	Pct bool
	Num float64
}

// ParseValue parses a position or size flag value: "123", "-50", "12.5",
// "10%" (suffix required, fractions allowed). Size is true for size flags
// (width/height: value must be > 0; negatives rejected).
func ParseValue(s string, size bool) (Value, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return Value{}, errors.New("empty value")
	}
	pct := strings.HasSuffix(t, "%")
	numStr := strings.TrimSuffix(t, "%")
	if pct && strings.ContainsAny(numStr, " \t") {
		return Value{}, fmt.Errorf("invalid number %q", s)
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid number %q", s)
	}
	if size && f <= 0 {
		return Value{}, fmt.Errorf("size must be positive, got %q", s)
	}
	return Value{Pct: pct, Num: f}, nil
}

// Resolve converts v to physical pixels against the given axis extent
// (work-area width or height).
func (v Value) Resolve(axisExtent int) int {
	if v.Pct {
		return int(v.Num / 100 * float64(axisExtent))
	}
	return int(v.Num)
}

// Rect is a window rectangle in physical pixels.
type Rect struct{ X, Y, W, H int }

func (r Rect) Right() int  { return r.X + r.W }
func (r Rect) Bottom() int { return r.Y + r.H }

// EdgeOffsets are the gaps from each work-area edge to the nearest window
// edge: exactly what --left/--top/--right/--bottom ask for, so a rect read
// back with `winfit get` can be reproduced by `winfit move`.
type EdgeOffsets struct{ Left, Top, Right, Bottom int }

// EdgeOffsetsFrom measures r inside wa. Values are negative when the window
// overhangs that work-area edge.
func EdgeOffsetsFrom(wa, r Rect) EdgeOffsets {
	return EdgeOffsets{
		Left:   r.X - wa.X,
		Top:    r.Y - wa.Y,
		Right:  wa.Right() - r.Right(),
		Bottom: wa.Bottom() - r.Bottom(),
	}
}

// Percentages is the flag-facing form of a rect: each edge offset and each
// dimension as a percentage of the work-area extent on that axis. Scale-
// invariant (both operands are physical px), so these are the values to
// report for `--left 3.5%`-style round-tripping.
type Percentages struct{ Left, Top, Right, Bottom, Width, Height float64 }

// PercentagesFrom expresses r relative to wa. Degenerate extents yield 0.
func PercentagesFrom(wa, r Rect) Percentages {
	o := EdgeOffsetsFrom(wa, r)
	pct := func(v, extent int) float64 {
		if extent == 0 {
			return 0
		}
		return float64(v) / float64(extent) * 100
	}
	return Percentages{
		Left:   pct(o.Left, wa.W),
		Top:    pct(o.Top, wa.H),
		Right:  pct(o.Right, wa.W),
		Bottom: pct(o.Bottom, wa.H),
		Width:  pct(r.W, wa.W),
		Height: pct(r.H, wa.H),
	}
}

// PhysicalToLogical converts a physical pixel count to the logical pixels
// that position/size flags speak (inverse of Spec.ScaleLogical).
func PhysicalToLogical(px int, scale float64) int {
	if scale <= 0 {
		return px
	}
	return int(float64(px)/scale + math.Copysign(0.5, float64(px)))
}

// Spec carries the parsed flags. Nil means "flag not given".
type Spec struct {
	Top, Bottom, Left, Right *Value // position, resolved against work area
	Width, Height            *Value // size; nil keeps current size
	KeepAspect               bool
	// OriginWA is the work area the window currently lives in. When a
	// --monitor move put the window on a different reference work area and
	// that axis has no position flag, the current edge offset from OriginWA
	// is carried over.
	OriginWA *Rect
}

// Result is the computed rectangle plus warnings worth logging.
type Result struct {
	Rect     Rect
	Resized  bool
	Moved    bool
	Warnings []string
}

// Changed reports whether any position or size flag was given.
func (s Spec) Changed() bool {
	return s.Top != nil || s.Bottom != nil || s.Left != nil || s.Right != nil ||
		s.Width != nil || s.Height != nil
}

// HasPosition reports whether any of the four position flags was given.
func (s Spec) HasPosition() bool {
	return s.Top != nil || s.Bottom != nil || s.Left != nil || s.Right != nil
}

// ScaleLogical converts pixel-valued (non-percent) entries from logical to
// physical pixels; percentages are scale-invariant.
func (s *Spec) ScaleLogical(scale float64) {
	if scale == 1 {
		return
	}
	for _, p := range []*Value{s.Top, s.Bottom, s.Left, s.Right, s.Width, s.Height} {
		if p != nil && !p.Pct {
			p.Num *= scale
		}
	}
}

// MinReachablePx is how much of a (moved-only) window must stay inside the
// work area on each axis.
const MinReachablePx = 64

// Compute derives the final rect from the work area wa, the window's current
// rect cur, and the spec. minW/minH are the system minimum tracking sizes
// (SM_CXMIN/SM_CYMIN).
func Compute(wa, cur Rect, s Spec, minW, minH int) Result {
	var r Result
	warn := func(f string, a ...any) {
		r.Warnings = append(r.Warnings, fmt.Sprintf(f, a...))
	}

	// --- horizontal ---
	left := dim{neg: s.Left, pos: s.Right, explicit: s.Width, extent: wa.W, cur: cur.W, min: minW,
		warn: warn, axis: "width"}
	w := left.compute()

	// --- vertical (same pattern) ---
	top := dim{neg: s.Top, pos: s.Bottom, explicit: s.Height, extent: wa.H, cur: cur.H, min: minH,
		warn: warn, axis: "height"}
	h := top.compute()

	if s.KeepAspect && cur.W > 0 && cur.H > 0 {
		aspect := float64(cur.W) / float64(cur.H)
		widthKnown := s.Width != nil || (s.Left != nil && s.Right != nil)
		heightKnown := s.Height != nil || (s.Top != nil && s.Bottom != nil)
		switch {
		case widthKnown && heightKnown:
			// Largest size at the aspect inside the requested box (contain fit).
			w, h = fitRatio(w, h, aspect, warn)
		case widthKnown:
			nh := int(float64(w) / aspect)
			warn("--keep-aspect: height scaled to %d px", nh)
			h = nh
		case heightKnown:
			nw := int(float64(h) * aspect)
			warn("--keep-aspect: width scaled to %d px", nw)
			w = nw
		}
		// Containment is absolute: scale the aspect-sized rect down
		// proportionally if it exceeds the work area.
		if w > wa.W {
			scale := float64(wa.W) / float64(w)
			w, h = wa.W, int(float64(h)*scale)
		}
		if h > wa.H {
			scale := float64(wa.H) / float64(h)
			w, h = int(float64(w)*scale), wa.H
		}
	}

	// Clamp sizes into the work area (containment precondition).
	cw := clampInt(w, maxInt(minW, 1), wa.W)
	ch := clampInt(h, maxInt(minH, 1), wa.H)
	if cw != w {
		warn("%s clamped to %d px to fit the work area", "width", cw)
		w = cw
	}
	if ch != h {
		warn("%s clamped to %d px to fit the work area", "height", ch)
		h = ch
	}
	// Resized means "a size change was requested" (explicit, derived, or by
	// keep-aspect), which drives the full-containment policy — even if
	// the resolved size equals the current one.
	r.Resized = s.Width != nil || s.Height != nil ||
		(s.Left != nil && s.Right != nil) || (s.Top != nil && s.Bottom != nil) ||
		w != cur.W || h != cur.H

	var x, y int
	switch {
	case s.Left != nil:
		x = wa.X + s.Left.Resolve(wa.W)
		if s.Width != nil && s.Right != nil {
			warn("--right ignored: explicit --width wins over the right offset")
		}
	case s.Right != nil:
		x = wa.Right() - s.Right.Resolve(wa.W) - w
	case s.OriginWA != nil && s.OriginWA.X != wa.X:
		x = wa.X + (cur.X - s.OriginWA.X)
	default:
		x = anchorKeep(cur, wa, w, true, warn)
	}
	switch {
	case s.Top != nil:
		y = wa.Y + s.Top.Resolve(wa.H)
		if s.Height != nil && s.Bottom != nil {
			warn("--bottom ignored: explicit --height wins over the bottom offset")
		}
	case s.Bottom != nil:
		y = wa.Bottom() - s.Bottom.Resolve(wa.H) - h
	case s.OriginWA != nil && s.OriginWA.Y != wa.Y:
		y = wa.Y + (cur.Y - s.OriginWA.Y)
	default:
		y = anchorKeep(cur, wa, h, false, warn)
	}

	// --- final clamps ---
	if r.Resized {
		// Full containment wins over every position instruction.
		x = clampContain(x, wa.X, wa.W, w)
		y = clampContain(y, wa.Y, wa.H, h)
		if s.Left != nil || s.Right != nil || s.Top != nil || s.Bottom != nil {
			warn("position adjusted to keep the resized window fully on screen")
		}
	} else {
		x = clampVisible(x, wa, w, warn, "x")
		y = clampVisible(y, wa, h, warn, "y")
	}

	r.Rect = Rect{X: x, Y: y, W: w, H: h}
	r.Moved = r.Rect.X != cur.X || r.Rect.Y != cur.Y
	return r
}

// dim is one axis's size computation helper.
type dim struct {
	neg, pos, explicit *Value
	extent, cur, min   int
	warn               func(string, ...any)
	axis               string
}

func (d dim) compute() int {
	size := d.cur
	switch {
	case d.explicit != nil:
		size = clampInt(d.explicit.Resolve(d.extent), maxInt(d.min, 1), d.extent)
		if d.neg != nil && d.pos != nil {
			d.warn("%s: explicit size wins over derived edge offsets", d.axis)
		}
	case d.neg != nil && d.pos != nil:
		size = d.extent - d.neg.Resolve(d.extent) - d.pos.Resolve(d.extent)
		if size < d.min {
			d.warn("%s: derived size clamped to minimum %d px", d.axis, d.min)
			size = d.min
		}
	}
	return size
}

// fitRatio contains a rectangle of the given aspect ratio inside (w, h),
// shrinking proportionally. Used for --keep-aspect fitting.
func fitRatio(w, h int, aspect float64, warn func(string, ...any)) (int, int) {
	if w <= 0 || h <= 0 || aspect <= 0 {
		return w, h
	}
	var nw, nh int
	if float64(w)/float64(h) > aspect {
		nw, nh = int(float64(h)*aspect), h
	} else {
		nw, nh = w, int(float64(w)/aspect)
	}
	warn("--keep-aspect: size fitted %d×%d → %d×%d", w, h, nw, nh)
	return nw, nh
}

// anchorKeep implements the resize anchor rule: keep the current position,
// except hold the far edge when the window already sits flush against it.
func anchorKeep(cur Rect, wa Rect, size int, horizontal bool, warn func(string, ...any)) int {
	if horizontal {
		if cur.Right() == wa.Right() {
			return wa.Right() - size
		}
		return cur.X
	}
	if cur.Bottom() == wa.Bottom() {
		return wa.Bottom() - size
	}
	return cur.Y
}

// clampContain keeps [pos, pos+size] inside [start, start+extent]; sizes are
// pre-clamped to the extent so start+extent-size >= start.
func clampContain(pos, start, extent, size int) int {
	return clampInt(pos, start, start+extent-size)
}

func clampVisible(pos int, wa Rect, size int, warn func(string, ...any), axis string) int {
	lo := wa.X - size + MinReachablePx
	hi := wa.Right() - MinReachablePx
	if axis == "y" {
		lo = wa.Y - size + MinReachablePx
		hi = wa.Bottom() - MinReachablePx
	}
	if pos < lo {
		warn("%s clamped to keep %d px reachable", axis, MinReachablePx)
		return lo
	}
	if pos > hi {
		warn("%s clamped to keep %d px reachable", axis, MinReachablePx)
		return hi
	}
	return pos
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
