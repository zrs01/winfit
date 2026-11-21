package place

import (
	"testing"
)

func v(s string) *Value {
	val, err := ParseValue(s, false)
	if err != nil {
		panic(err)
	}
	return &val
}

func sv(s string) *Value {
	val, err := ParseValue(s, true)
	if err != nil {
		panic(err)
	}
	return &val
}

func TestParseValue(t *testing.T) {
	for _, tc := range []struct {
		in    string
		size  bool
		want  Value
		isErr bool
	}{
		{in: "100", want: Value{Num: 100}},
		{in: "-50", want: Value{Num: -50}},
		{in: "50%", want: Value{Pct: true, Num: 50}},
		{in: "33.3%", want: Value{Pct: true, Num: 33.3}},
		{in: " 20 % ", want: Value{Num: 0}, isErr: true}, // stray space before %
		{in: "edge", isErr: true},
		{in: "", isErr: true},
		{in: "100", size: true, want: Value{Num: 100}},
		{in: "0", size: true, isErr: true},
		{in: "-10", size: true, isErr: true},
	} {
		got, err := ParseValue(tc.in, tc.size)
		if tc.isErr {
			if err == nil {
				t.Errorf("ParseValue(%q,%v) want error, got %+v", tc.in, tc.size, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseValue(%q,%v) unexpected error %v", tc.in, tc.size, err)
			continue
		}
		if got.Pct != tc.want.Pct || got.Num != tc.want.Num {
			t.Errorf("ParseValue(%q,%v) = %+v, want %+v", tc.in, tc.size, got, tc.want)
		}
	}
}

func TestResolve(t *testing.T) {
	if got := (Value{Pct: true, Num: 25}).Resolve(2560); got != 640 {
		t.Errorf("25%% of 2560 = %d, want 640", got)
	}
	if got := (Value{Num: -50}).Resolve(1000); got != -50 {
		t.Errorf("absolute -50 = %d", got)
	}
}

var (
	wa1920 = Rect{X: 0, Y: 0, W: 1920, H: 1040} // work area, taskbar excluded
	cur100 = Rect{X: 300, Y: 200, W: 800, H: 600}
)

func TestComputePositions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spec    Spec
		want    Rect
		resized bool
	}{
		{
			name: "left+top absolute px",
			spec: Spec{Left: v("100"), Top: v("50")},
			want: Rect{X: 100, Y: 50, W: 800, H: 600},
		},
		{
			name: "right anchor keeps size",
			spec: Spec{Right: v("0")},
			want: Rect{X: 1920 - 800, Y: 200, W: 800, H: 600},
		},
		{
			name: "percent position",
			spec: Spec{Left: v("25%"), Top: v("10%")},
			want: Rect{X: 480, Y: 104, W: 800, H: 600},
		},
		{
			name: "derived size from left+right",
			spec: Spec{Left: v("20"), Right: v("20")},
			want: Rect{X: 20, Y: 200, W: 1880, H: 600}, resized: true,
		},
		{
			name: "mixed px and pct derived size",
			spec: Spec{Left: v("100"), Right: v("20%")},
			want: Rect{X: 100, Y: 200, W: 1920 - 100 - 384, H: 600}, resized: true,
		},
		{
			name: "explicit width wins over left+right derivation",
			spec: Spec{Left: v("10"), Right: v("10"), Width: sv("500")},
			want: Rect{X: 10, Y: 200, W: 500, H: 600}, resized: true,
		},
		{
			name: "no horizontal flags keeps x",
			spec: Spec{Top: v("0")},
			want: Rect{X: 300, Y: 0, W: 800, H: 600},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Compute(wa1920, cur100, tc.spec, 100, 100)
			if got.Rect != tc.want || got.Resized != tc.resized {
				t.Fatalf("got %+v resized=%v, want %+v resized=%v (warnings: %v)",
					got.Rect, got.Resized, tc.want, tc.resized, got.Warnings)
			}
		})
	}
}

func TestComputeSizeAndContainment(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec Spec
		want Rect
	}{
		{
			name: "width 50pct anchored left",
			spec: Spec{Width: sv("50%"), Left: v("0")},
			want: Rect{X: 0, Y: 200, W: 960, H: 600},
		},
		{
			name: "100pct fills work area",
			spec: Spec{Width: sv("100%"), Height: sv("100%")},
			want: Rect{X: 0, Y: 0, W: 1920, H: 1040},
		},
		{
			name: "size beyond work area clamps",
			spec: Spec{Width: sv("300%"), Height: sv("4000")},
			want: Rect{X: 0, Y: 0, W: 1920, H: 1040},
		},
		{
			name: "position beyond work area nudged in on resize",
			spec: Spec{Width: sv("800"), Height: sv("600"), Left: v("1500")},
			want: Rect{X: 1920 - 800, Y: 200, W: 800, H: 600},
		},
		{
			name: "bottom-right anchor holds edge on resize",
			spec: Spec{Width: sv("800")},
			want: Rect{X: 300, Y: 200, W: 800, H: 600}, // unchanged: cur not flush
		},
		{
			name: "size below system minimum clamped up",
			spec: Spec{Width: sv("1")},
			want: Rect{X: 300, Y: 200, W: 100, H: 600},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Compute(wa1920, cur100, tc.spec, 100, 100)
			if got.Rect != tc.want {
				t.Fatalf("got %+v, want %+v (warnings: %v)", got.Rect, tc.want, got.Warnings)
			}
			// Resized implies full containment inside the work area.
			if got.Resized {
				r := got.Rect
				if r.X < wa1920.X || r.Right() > wa1920.Right() || r.Y < wa1920.Y || r.Bottom() > wa1920.Bottom() {
					t.Fatalf("resized rect %+v escapes work area %+v", r, wa1920)
				}
			}
		})
	}
}

func TestComputeFlushAnchorHeld(t *testing.T) {
	// Window already flush right+bottom: resize grows leftward/upward.
	cur := Rect{X: 1920 - 800, Y: 1040 - 600, W: 800, H: 600}
	got := Compute(wa1920, cur, Spec{Width: sv("400"), Height: sv("300")}, 100, 100)
	if got.Rect != (Rect{X: 1920 - 400, Y: 1040 - 300, W: 400, H: 300}) {
		t.Fatalf("got %+v", got.Rect)
	}
}

func TestComputeMoveOnlyReachableClamp(t *testing.T) {
	// Move-only allows partial off-screen but keeps MinReachablePx inside.
	got := Compute(wa1920, cur100, Spec{Left: v("5000"), Top: v("-5000")}, 100, 100)
	wantX := wa1920.Right() - MinReachablePx
	wantY := wa1920.Y - 600 + MinReachablePx
	if got.Rect.X != wantX || got.Rect.Y != wantY {
		t.Fatalf("got %+v, want x=%d y=%d", got.Rect, wantX, wantY)
	}
	if got.Resized {
		t.Fatal("move-only flagged as resized")
	}
}

func TestComputeKeepAspect(t *testing.T) {
	// cur aspect 800/600 = 4:3.
	t.Run("both dims contain fit", func(t *testing.T) {
		got := Compute(wa1920, cur100, Spec{Width: sv("960"), Height: sv("600"), KeepAspect: true}, 100, 100)
		// box 960x600 ratio 1.6 > 1.333 → h stays 600, w = 600*4/3 = 800
		if got.Rect.W != 800 || got.Rect.H != 600 {
			t.Fatalf("got %+v", got.Rect)
		}
	})
	t.Run("single dim scales other", func(t *testing.T) {
		got := Compute(wa1920, cur100, Spec{Width: sv("400"), KeepAspect: true}, 100, 100)
		if got.Rect.W != 400 || got.Rect.H != 300 {
			t.Fatalf("got %+v, want 400x300", got.Rect)
		}
	})
	t.Run("derived width sets height then clamps into work area", func(t *testing.T) {
		got := Compute(wa1920, cur100, Spec{Left: v("0"), Right: v("0"), KeepAspect: true}, 100, 100)
		// width 1920 → h = 1440 exceeds work-area height 1040 → proportional
		// shrink preserves the 4:3 aspect.
		if got.Rect.W != 1386 || got.Rect.H != 1040 {
			t.Fatalf("got %+v, want 1386x1040", got.Rect)
		}
	})
}

func TestComputeMonitorMoveCarriesOffsets(t *testing.T) {
	wa2 := Rect{X: 2560, Y: 0, W: 2560, H: 1400}
	old := Rect{X: 0, Y: 0, W: 1920, H: 1040}
	got := Compute(wa2, cur100, Spec{OriginWA: &old}, 100, 100)
	if got.Rect.X != 2560+300 || got.Rect.Y != 200 {
		t.Fatalf("got %+v, want offsets 300/200 carried to x=2860", got.Rect)
	}
	// With an explicit position flag the flag wins over the carried offset.
	got = Compute(wa2, cur100, Spec{Left: v("10"), OriginWA: &old}, 100, 100)
	if got.Rect.X != 2570 {
		t.Fatalf("got %+v", got.Rect)
	}
}

// Property-style invariant: whenever a resize is requested the result is
// contained and >= the system minimums.
func TestInvariantContainment(t *testing.T) {
	areas := []Rect{
		{X: 0, Y: 0, W: 1920, H: 1040},
		{X: 1920, Y: -100, W: 2560, H: 1400},
		{X: 0, Y: 0, W: 800, H: 600},
	}
	reqs := []Spec{
		{Width: sv("20000"), Height: sv("20000")},
		{Width: sv("150%"), Left: v("-9999"), Top: v("-9999")},
		{Left: v("0"), Right: v("0"), Bottom: v("0")},
		{Width: sv("1"), Height: sv("1"), Right: v("5000"), KeepAspect: true},
	}
	for _, wa := range areas {
		for _, s := range reqs {
			got := Compute(wa, Rect{X: wa.X + 50, Y: wa.Y + 50, W: 500, H: 400}, s, 100, 100)
			r := got.Rect
			if !got.Resized {
				continue
			}
			if r.W < 100 || r.H < 100 {
				t.Errorf("wa %v spec %+v: size %dx%d below min", wa, s, r.W, r.H)
			}
			if r.X < wa.X || r.Right() > wa.Right() || r.Y < wa.Y || r.Bottom() > wa.Bottom() {
				t.Errorf("wa %v spec %+v: rect %+v not contained", wa, s, r)
			}
		}
	}
}

func TestEdgeOffsetsFrom(t *testing.T) {
	wa := Rect{X: 0, Y: 0, W: 1920, H: 1040}
	for _, tc := range []struct {
		wa, r Rect
		want  EdgeOffsets
	}{
		{wa, Rect{X: 0, Y: 0, W: 800, H: 600}, EdgeOffsets{Left: 0, Top: 0, Right: 1120, Bottom: 440}},
		{wa, Rect{X: 100, Y: 50, W: 800, H: 600}, EdgeOffsets{Left: 100, Top: 50, Right: 1020, Bottom: 390}},
		{Rect{X: 1920, Y: 0, W: 1920, H: 1040}, Rect{X: 1930, Y: 10, W: 100, H: 100}, EdgeOffsets{Left: 10, Top: 10, Right: 1810, Bottom: 930}},
		{wa, Rect{X: 1900, Y: 1000, W: 200, H: 200}, EdgeOffsets{Left: 1900, Top: 1000, Right: -180, Bottom: -160}},
	} {
		if got := EdgeOffsetsFrom(tc.wa, tc.r); got != tc.want {
			t.Errorf("EdgeOffsetsFrom(%v, %v) = %+v want %+v", tc.wa, tc.r, got, tc.want)
		}
	}
}

func TestPercentagesFrom(t *testing.T) {
	wa := Rect{X: 0, Y: 0, W: 2000, H: 1000}
	got := PercentagesFrom(wa, Rect{X: 100, Y: 200, W: 1000, H: 500})
	want := Percentages{Left: 5, Top: 20, Right: 45, Bottom: 30, Width: 50, Height: 50}
	if got != want {
		t.Errorf("PercentagesFrom = %+v want %+v", got, want)
	}
	// A full-work-area window reads as 100% on both axes and 0 at the edges.
	full := PercentagesFrom(wa, wa)
	if full.Width != 100 || full.Height != 100 || full.Left != 0 || full.Bottom != 0 {
		t.Errorf("a full-work-area window should read 100 size / 0 edges, got %+v", full)
	}
	// Degenerate work area must not divide by zero.
	if z := PercentagesFrom(Rect{}, Rect{W: 10, H: 10}); z != (Percentages{}) {
		t.Errorf("zero extent should be all zeros, got %+v", z)
	}
}

// The values `winfit get` prints must be the ones the flags take, so a
// measured window can be restored at 150% scaling without drift.
func TestPhysicalToLogical(t *testing.T) {
	for _, tc := range []struct {
		px    int
		scale float64
		want  int
	}{
		{150, 1.5, 100},
		{151, 1.5, 101},
		{75, 1.5, 50},
		{100, 1, 100},
		{-150, 1.5, -100},
		{100, 0, 100}, // unknown scale: report as-is
	} {
		if got := PhysicalToLogical(tc.px, tc.scale); got != tc.want {
			t.Errorf("PhysicalToLogical(%d, %v) = %d want %d", tc.px, tc.scale, got, tc.want)
		}
	}
	// Round-trip: a logical request scaled up must read back at the same size.
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		for _, logical := range []int{0, 1, 100, 800, 1920} {
			phys := int(float64(logical) * scale)
			if back := PhysicalToLogical(phys, scale); back != logical {
				t.Errorf("scale %v: %d logical -> %d px -> %d logical", scale, logical, phys, back)
			}
		}
	}
}
