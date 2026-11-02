package coordinator

import (
	"DistributedMandelbrot/misc"
	"fmt"
	"math"
	"strings"
)

// frameCountTolerance absorbs floating point error in the logarithms so that a whole number of zoom steps, which may
// evaluate to something like 2.0000000000000004, is not rounded up to an extra frame
const frameCountTolerance = 1e-9

// Values for transitionSettings.Path, which controls how the center moves while the magnification changes
const (
	// SlidePath moves the center steadily across the complex plane from the start point to the end point. Since the
	// view keeps shrinking, the end point drifts off screen part way through and the view slides over other detail.
	SlidePath = "Slide"
	// PointPath zooms into the end point (or, zooming out, away from the start point) like most Mandelbrot zoom
	// videos. The point glides to the center of the screen and never drifts away from it.
	PointPath = "Point"
)

type transitionSettings struct {
	EndX               float64
	EndY               float64
	FrameCount         uint
	MagnificationStart float64
	MagnificationEnd   float64
	MagnificationStep  float64
	Path               string
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
	// Slide is the default so settings saved before Path existed, including runs being resumed, render as before
	switch {
	case strings.EqualFold(ts.Path, PointPath):
		ts.Path = PointPath
	case ts.Path == "" || strings.EqualFold(ts.Path, SlidePath):
		ts.Path = SlidePath
	default:
		unknown := ts.Path
		ts.Path = SlidePath
		return fmt.Errorf("unknown transition Path %q, using %q (the options are %q and %q)", unknown, SlidePath, SlidePath, PointPath)
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
// The first frame is at the start values and the last frame is at the end point; Path decides how the center moves
// between them.
func (ts *transitionSettings) frame(index uint) (x float64, y float64, magnification float64) {
	t := 0.0
	if ts.FrameCount > 1 {
		t = float64(index) / float64(ts.FrameCount-1)
	}
	magnification = ts.magnification(index)

	if ts.Path == PointPath {
		x, y = ts.pointCenter(index, t)
		return x, y, magnification
	}

	ease := misc.EaseOutExpo
	if ts.zoomingOut() {
		ease = misc.EaseInExpo
	}
	x = misc.LerpFloat64(ts.StartX, ts.EndX, ease(t))
	y = misc.LerpFloat64(ts.StartY, ts.EndY, ease(t))
	return x, y, magnification
}

func (ts *transitionSettings) zoomingOut() bool {
	return ts.MagnificationStart > ts.MagnificationEnd
}

// magnification returns the magnification of the zero based frame index, one MagnificationStep further each frame
func (ts *transitionSettings) magnification(index uint) float64 {
	if ts.zoomingOut() {
		return ts.MagnificationStart / math.Pow(ts.MagnificationStep, float64(index))
	}
	return ts.MagnificationStart * math.Pow(ts.MagnificationStep, float64(index))
}

// pointCenter returns the center of a frame on the point path. The anchor is the deep end of the zoom: the end point
// when zooming in, the start point when zooming out. On screen, the distance between the anchor and the center is
// measured in multiples of the view's size, which shrinks as 1/magnification. Starting from the shallow end, that
// screen distance is shrunk smoothly to nothing, so the anchor glides to the center of the screen without ever moving
// away from it:
//
//	center = anchor + (shallow - anchor) * (magnification at the shallow end / magnification) * weight
//
// where weight eases from 1 at the shallow end to 0 at the deep end. Without the magnification ratio this would be the
// slide path, whose steps look bigger and bigger on screen as the view shrinks.
func (ts *transitionSettings) pointCenter(index uint, t float64) (x float64, y float64) {
	// Return the endpoints exactly rather than via floating point arithmetic that may land a tiny bit off
	last := ts.FrameCount - 1
	switch index {
	case 0:
		return ts.StartX, ts.StartY
	case last:
		return ts.EndX, ts.EndY
	}

	anchorX, anchorY, shallowX, shallowY := ts.EndX, ts.EndY, ts.StartX, ts.StartY
	var shallowIndex uint = 0
	weight := 1 - misc.SmoothStep(t)
	if ts.zoomingOut() {
		anchorX, anchorY, shallowX, shallowY = ts.StartX, ts.StartY, ts.EndX, ts.EndY
		shallowIndex = last
		weight = misc.SmoothStep(t)
	}

	scale := ts.magnification(shallowIndex) / ts.magnification(index) * weight
	return anchorX + (shallowX-anchorX)*scale, anchorY + (shallowY-anchorY)*scale
}
