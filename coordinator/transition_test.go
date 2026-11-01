package coordinator

import (
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
	zoomIn := transitionSettings{
		StartX: -0.5, StartY: 0, EndX: -0.75, EndY: -0.1,
		MagnificationStart: 2, MagnificationEnd: 8, MagnificationStep: 2,
	}
	zoomIn.FrameCount = zoomIn.frameCount()
	zoomOut := transitionSettings{
		StartX: -0.75, StartY: -0.1, EndX: -0.5, EndY: 0,
		MagnificationStart: 8, MagnificationEnd: 2, MagnificationStep: 2,
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
		ts := transitionSettings{StartX: 0.1, StartY: 0.2, EndX: 0.3, EndY: 0.4, MagnificationStart: 5, MagnificationEnd: 5, MagnificationStep: 1.1}
		ts.FrameCount = ts.frameCount()
		x, y, magnification := ts.frame(0)
		if x != 0.1 || y != 0.2 || magnification != 5 {
			t.Errorf("frame(0) = (%v, %v, %v), want (0.1, 0.2, 5)", x, y, magnification)
		}
	})
}
