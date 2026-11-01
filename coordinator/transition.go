package coordinator

import (
	"DistributedMandelbrot/misc"
	"math"
)

// frameCountTolerance absorbs floating point error in the logarithms so that a whole number of zoom steps, which may
// evaluate to something like 2.0000000000000004, is not rounded up to an extra frame
const frameCountTolerance = 1e-9

type transitionSettings struct {
	EndX               float64
	EndY               float64
	FrameCount         uint
	MagnificationStart float64
	MagnificationEnd   float64
	MagnificationStep  float64
	StartX             float64
	StartY             float64
}

func (ts *transitionSettings) Verify() error {
	if ts.StartX < -4 || ts.StartX > 4 {
		ts.StartX = 0
	}
	if ts.StartY < -4 || ts.StartY > 4 {
		ts.StartY = 0
	}
	if ts.EndX < -4 || ts.EndX > 4 {
		ts.EndX = 0
	}
	if ts.EndY < -4 || ts.EndY > 4 {
		ts.EndY = 0
	}
	if ts.MagnificationEnd <= 0 {
		ts.MagnificationEnd = 1.5
	}
	if ts.MagnificationStart <= 0 {
		ts.MagnificationStart = 0.5
	}
	if ts.MagnificationStep <= 1 {
		ts.MagnificationStep = 1.1
	}
	return nil
}

// frameCount returns the number of frames needed to zoom from MagnificationStart to MagnificationEnd, changing the
// magnification by a factor of MagnificationStep each frame. The first frame is rendered at MagnificationStart and the
// last frame reaches MagnificationEnd, landing exactly on it or less than one step past it.
//
// Frame k is rendered at start * step^k, so the number of steps n needed to reach the end is:
//
//	start * step^n = end
//	        step^n = end / start
//	   n * log(step) = log(end / start)
//	             n = log(end / start) / log(step)
//
// Zooming out is the same with start and end swapped, which only flips the sign of the logarithm. Rounding n up gives
// the number of whole steps, plus one for the starting frame.
func (ts *transitionSettings) frameCount() uint {
	steps := math.Abs(math.Log(ts.MagnificationEnd/ts.MagnificationStart)) / math.Log(ts.MagnificationStep)
	return uint(math.Ceil(steps-frameCountTolerance)) + 1
}

// frame returns the center point and magnification for the zero based frame index. FrameCount must already be set.
// The first frame is at the start values and the last frame is at the end point, with the center eased between them.
func (ts *transitionSettings) frame(index uint) (x float64, y float64, magnification float64) {
	t := 0.0
	if ts.FrameCount > 1 {
		t = float64(index) / float64(ts.FrameCount-1)
	}

	ease := misc.EaseOutExpo
	magnification = ts.MagnificationStart * math.Pow(ts.MagnificationStep, float64(index))
	if ts.MagnificationStart > ts.MagnificationEnd {
		// zooming out
		ease = misc.EaseInExpo
		magnification = ts.MagnificationStart / math.Pow(ts.MagnificationStep, float64(index))
	}

	x = misc.LerpFloat64(ts.StartX, ts.EndX, ease(t))
	y = misc.LerpFloat64(ts.StartY, ts.EndY, ease(t))
	return x, y, magnification
}
