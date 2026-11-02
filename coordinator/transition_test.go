package coordinator

import (
	"DistributedMandelbrot/misc"
	"math"
	"testing"
)

func TestTransitionFrameCount(t *testing.T) {
	tests := []struct {
		name             string
		start, end, step float64
		want             uint
	}{
		// n = log(250/1) / log(1.25) = 24.74, so 25 steps are needed to reach 250 plus the starting frame
		{"example settings", 1, 250, 1.25, 26},
		// A start other than 1 is what the original formula got wrong
		{"start of 2 lands exactly on the end", 2, 8, 2, 3},
		{"start of 10", 10, 1000, 1.1, 50},
		{"start of 50", 50, 250, 1.25, 9},
		{"zooming out", 8, 2, 2, 3},
		{"zooming out by a partial step", 250, 1, 1.25, 26},
		{"no zoom still renders one frame", 5, 5, 1.1, 1},
		// log(1000) / log(10) evaluates to 2.9999999999999996
		{"floating point error below a whole number", 1, 1000, 10, 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := transitionSettings{MagnificationStart: tc.start, MagnificationEnd: tc.end, MagnificationStep: tc.step}
			if got := ts.frameCount(); got != tc.want {
				t.Errorf("frameCount() = %d, want %d", got, tc.want)
			}
		})
	}
}

// When the end is exactly a whole number of steps from the start, floating point error in the logarithms must not
// round the frame count up or down
func TestTransitionFrameCountWholeSteps(t *testing.T) {
	for _, step := range []float64{1.01, 1.1, 1.25, 1.5, 2, 3, 10} {
		for _, start := range []float64{0.5, 1, 3, 7.25} {
			for steps := range uint(40) {
				end := start * math.Pow(step, float64(steps))
				for _, ts := range []transitionSettings{
					{MagnificationStart: start, MagnificationEnd: end, MagnificationStep: step},
					{MagnificationStart: end, MagnificationEnd: start, MagnificationStep: step},
				} {
					if got := ts.frameCount(); got != steps+1 {
						t.Fatalf("frameCount() for %v -> %v by %v = %d, want %d", ts.MagnificationStart, ts.MagnificationEnd, step, got, steps+1)
					}
				}
			}
		}
	}
}

// The last frame must reach the end magnification and the frame before it must not, so no extra frames are rendered
func TestTransitionLastFrameReachesEnd(t *testing.T) {
	const tolerance = 1e-9
	for _, step := range []float64{1.05, 1.25, 2} {
		for _, start := range []float64{0.5, 1, 2, 10, 50} {
			for _, end := range []float64{0.75, 3, 8, 250, 1000} {
				ts := transitionSettings{MagnificationStart: start, MagnificationEnd: end, MagnificationStep: step}
				ts.FrameCount = ts.frameCount()
				if ts.FrameCount < 2 {
					continue
				}
				_, _, last := ts.frame(ts.FrameCount - 1)
				_, _, previous := ts.frame(ts.FrameCount - 2)
				zoomingIn := start < end
				reached := (zoomingIn && last >= end*(1-tolerance)) || (!zoomingIn && last <= end*(1+tolerance))
				overshot := (zoomingIn && previous >= end*(1+tolerance)) || (!zoomingIn && previous <= end*(1-tolerance))
				if !reached || overshot {
					t.Errorf("%v -> %v by %v: %d frames end at %v (previous %v)", start, end, step, ts.FrameCount, last, previous)
				}
			}
		}
	}
}

func TestTransitionFrame(t *testing.T) {
	for _, path := range []string{SlidePath, PointPath} {
		t.Run(path, func(t *testing.T) {
			zoomIn := transitionSettings{
				StartX: -0.5, StartY: 0, EndX: -0.75, EndY: -0.1,
				MagnificationStart: 2, MagnificationEnd: 8, MagnificationStep: 2, Path: path,
			}
			zoomIn.FrameCount = zoomIn.frameCount()
			zoomOut := transitionSettings{
				StartX: -0.75, StartY: -0.1, EndX: -0.5, EndY: 0,
				MagnificationStart: 8, MagnificationEnd: 2, MagnificationStep: 2, Path: path,
			}
			zoomOut.FrameCount = zoomOut.frameCount()

			for _, ts := range []transitionSettings{zoomIn, zoomOut} {
				t.Run("first frame is the start", func(t *testing.T) {
					x, y, magnification := ts.frame(0)
					if x != ts.StartX || y != ts.StartY || magnification != ts.MagnificationStart {
						t.Errorf("frame(0) = (%v, %v, %v), want (%v, %v, %v)", x, y, magnification, ts.StartX, ts.StartY, ts.MagnificationStart)
					}
				})
				t.Run("last frame is the end", func(t *testing.T) {
					x, y, magnification := ts.frame(ts.FrameCount - 1)
					if x != ts.EndX || y != ts.EndY || magnification != ts.MagnificationEnd {
						t.Errorf("frame(%d) = (%v, %v, %v), want (%v, %v, %v)", ts.FrameCount-1, x, y, magnification, ts.EndX, ts.EndY, ts.MagnificationEnd)
					}
				})
			}

			t.Run("each frame changes magnification by one step", func(t *testing.T) {
				for i := range zoomIn.FrameCount - 1 {
					_, _, current := zoomIn.frame(i)
					_, _, next := zoomIn.frame(i + 1)
					if next/current != 2 {
						t.Errorf("zoom in frame %d -> %d ratio = %v, want 2", i, i+1, next/current)
					}
					_, _, current = zoomOut.frame(i)
					_, _, next = zoomOut.frame(i + 1)
					if current/next != 2 {
						t.Errorf("zoom out frame %d -> %d ratio = %v, want 2", i, i+1, current/next)
					}
				}
			})

			t.Run("a single frame transition renders the start", func(t *testing.T) {
				ts := transitionSettings{StartX: 0.1, StartY: 0.2, EndX: 0.3, EndY: 0.4, MagnificationStart: 5, MagnificationEnd: 5, MagnificationStep: 1.1, Path: path}
				ts.FrameCount = ts.frameCount()
				x, y, magnification := ts.frame(0)
				if x != 0.1 || y != 0.2 || magnification != 5 {
					t.Errorf("frame(0) = (%v, %v, %v), want (0.1, 0.2, 5)", x, y, magnification)
				}
			})
		})
	}
}

// screenOffset is how far a point is from the center of a frame, in multiples of the frame's shorter side
func screenOffset(ts transitionSettings, index uint, pointX float64, pointY float64) float64 {
	x, y, magnification := ts.frame(index)
	return math.Hypot(pointX-x, pointY-y) * magnification
}

// A point zoom must never let the target drift away on screen. Zooming in, the end point's distance from the center
// of the screen only shrinks; zooming out is the same movement in reverse, so the start point's distance only grows.
func TestPointPathNeverSlides(t *testing.T) {
	seahorse := transitionSettings{
		StartX: -0.75, StartY: 0, EndX: -0.743643887037151, EndY: 0.131825904205330,
		MagnificationStart: 0.4, MagnificationEnd: 100000, MagnificationStep: 1.05, Path: PointPath,
	}
	// The end is not a whole number of steps away, so the last frame zooms slightly past MagnificationEnd
	partialStep := transitionSettings{
		StartX: 0.3, StartY: -0.4, EndX: -1.2, EndY: 0.25,
		MagnificationStart: 1, MagnificationEnd: 250, MagnificationStep: 1.25, Path: PointPath,
	}
	for _, zoomIn := range []transitionSettings{seahorse, partialStep} {
		zoomIn.FrameCount = zoomIn.frameCount()
		zoomOut := zoomIn
		zoomOut.StartX, zoomOut.StartY, zoomOut.EndX, zoomOut.EndY = zoomIn.EndX, zoomIn.EndY, zoomIn.StartX, zoomIn.StartY
		zoomOut.MagnificationStart, zoomOut.MagnificationEnd = zoomIn.MagnificationEnd, zoomIn.MagnificationStart
		zoomOut.FrameCount = zoomOut.frameCount()

		for _, ts := range []transitionSettings{zoomIn, zoomOut} {
			if x, y, _ := ts.frame(0); x != ts.StartX || y != ts.StartY {
				t.Errorf("frame(0) center = (%v, %v), want exactly (%v, %v)", x, y, ts.StartX, ts.StartY)
			}
			if x, y, _ := ts.frame(ts.FrameCount - 1); x != ts.EndX || y != ts.EndY {
				t.Errorf("last frame center = (%v, %v), want exactly (%v, %v)", x, y, ts.EndX, ts.EndY)
			}
		}

		const tolerance = 1e-9
		for i := range zoomIn.FrameCount - 1 {
			current := screenOffset(zoomIn, i, zoomIn.EndX, zoomIn.EndY)
			next := screenOffset(zoomIn, i+1, zoomIn.EndX, zoomIn.EndY)
			if next > current*(1+tolerance) {
				t.Fatalf("zooming in to (%v, %v), the target moves away from the center from frame %d to %d: %v -> %v screens",
					zoomIn.EndX, zoomIn.EndY, i, i+1, current, next)
			}
		}
		for i := range zoomOut.FrameCount - 1 {
			current := screenOffset(zoomOut, i, zoomOut.StartX, zoomOut.StartY)
			next := screenOffset(zoomOut, i+1, zoomOut.StartX, zoomOut.StartY)
			if next < current*(1-tolerance) {
				t.Fatalf("zooming out of (%v, %v), the start point moves back toward the center from frame %d to %d: %v -> %v screens",
					zoomOut.StartX, zoomOut.StartY, i, i+1, current, next)
			}
		}
	}
}

// The slide path is what the 4K test movie showed: the target leaves the screen part way through the zoom
func TestSlidePathDriftsOffScreen(t *testing.T) {
	ts := transitionSettings{
		StartX: -0.75, StartY: 0, EndX: -0.743643887037151, EndY: 0.131825904205330,
		MagnificationStart: 0.4, MagnificationEnd: 100000, MagnificationStep: 1.05, Path: SlidePath,
	}
	ts.FrameCount = ts.frameCount()
	if offset := screenOffset(ts, ts.FrameCount*3/4, ts.EndX, ts.EndY); offset < 1 {
		t.Errorf("three quarters of the way through, the target is %v screens from the center; the slide should have left it behind", offset)
	}
}

// With the same start and end point the zoom goes straight into that point, like most Mandelbrot zoom videos
func TestPointPathWithOnePointIsAStraightZoom(t *testing.T) {
	ts := transitionSettings{
		StartX: -0.743643887037151, StartY: 0.131825904205330, EndX: -0.743643887037151, EndY: 0.131825904205330,
		MagnificationStart: 0.4, MagnificationEnd: 100000, MagnificationStep: 1.05, Path: PointPath,
	}
	ts.FrameCount = ts.frameCount()
	for i := range ts.FrameCount {
		if x, y, _ := ts.frame(i); x != ts.EndX || y != ts.EndY {
			t.Fatalf("frame(%d) center = (%v, %v), want (%v, %v)", i, x, y, ts.EndX, ts.EndY)
		}
	}
}

// Runs saved before Path existed default to the slide path, so their remaining frames must be computed exactly as
// before or a resumed movie would jump. This is a frozen copy of that formula.
func TestSlidePathIsUnchanged(t *testing.T) {
	original := func(ts transitionSettings, index uint) (float64, float64, float64) {
		t := 0.0
		if ts.FrameCount > 1 {
			t = float64(index) / float64(ts.FrameCount-1)
		}
		ease := misc.EaseOutExpo
		magnification := ts.MagnificationStart * math.Pow(ts.MagnificationStep, float64(index))
		if ts.MagnificationStart > ts.MagnificationEnd {
			ease = misc.EaseInExpo
			magnification = ts.MagnificationStart / math.Pow(ts.MagnificationStep, float64(index))
		}
		return misc.LerpFloat64(ts.StartX, ts.EndX, ease(t)), misc.LerpFloat64(ts.StartY, ts.EndY, ease(t)), magnification
	}

	for _, ts := range []transitionSettings{
		{StartX: -0.75, EndX: -0.743643887037151, EndY: 0.131825904205330, MagnificationStart: 0.4, MagnificationEnd: 100000, MagnificationStep: 1.05},
		{StartX: 0.3, StartY: -0.4, EndX: -1.2, EndY: 0.25, MagnificationStart: 250, MagnificationEnd: 1, MagnificationStep: 1.25},
	} {
		_ = ts.Verify() // a saved run without Path, loaded the way the coordinator loads it
		ts.FrameCount = ts.frameCount()
		for i := range ts.FrameCount {
			x, y, magnification := ts.frame(i)
			wantX, wantY, wantMagnification := original(ts, i)
			if x != wantX || y != wantY || magnification != wantMagnification {
				t.Fatalf("frame(%d) = (%v, %v, %v), want (%v, %v, %v)", i, x, y, magnification, wantX, wantY, wantMagnification)
			}
		}
	}
}
