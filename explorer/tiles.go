package explorer

import (
	"DistributedMandelbrot/task"
	"fmt"
	"image"
	"math"
)

// levelZeroSpan is the width and height in complex units of a tile at zoom level 0. Each level halves it.
const levelZeroSpan = 4.0

// planeLimit bounds the part of the complex plane the explorer renders: a tile must overlap [-planeLimit, planeLimit]
// on both axes. The whole set lies within 2 of the origin, so nothing past the limit is interesting.
const planeLimit = 4.0

// lowestZoomLevel is the most zoomed out level allowed; there a tile is 64 units wide, far bigger than the set
const lowestZoomLevel = -4

// minSampleSpacingUlps is how far apart, in units of the last place of a float64 near planeLimit, adjacent samples must
// stay. Closer than this, rounding makes neighboring pixels compute the same point and the image turns blocky.
const minSampleSpacingUlps = 16

// tileKey identifies a tile: at zoom level Level, tile (X, Y) covers [X, X+1) × [Y, Y+1) tile spans of the plane. Y
// grows with the row on screen, the same orientation as the movie's pixel mapping.
type tileKey struct {
	Level int
	X     int64
	Y     int64
}

// tileID is a tile computed with particular settings. These are the only settings that change the iteration counts,
// so it is also the cache key; colors are applied in the browser.
type tileID struct {
	tileKey
	Boundary      float64
	MaxIterations uint
	SuperSampling int
}

// tileSpan is the width and height of a tile at level. It is a power of two, so tile origins are exact.
func tileSpan(level int) float64 {
	return math.Ldexp(levelZeroSpan, -level)
}

// pixelSize is the width and height of one pixel of a tile at level
func pixelSize(level int) float64 {
	return tileSpan(level) / task.TileSize
}

// origin is the complex coordinate of the tile's top left corner
func (k tileKey) origin() (x float64, y float64) {
	span := tileSpan(k.Level)
	return float64(k.X) * span, float64(k.Y) * span
}

// inPlane reports whether the tile overlaps the rendered part of the plane
func (k tileKey) inPlane() bool {
	x, y := k.origin()
	span := tileSpan(k.Level)
	return x < planeLimit && x+span > -planeLimit && y < planeLimit && y+span > -planeLimit
}

func (k tileKey) String() string {
	return fmt.Sprintf("%d/%d/%d", k.Level, k.X, k.Y)
}

// task is the worker task that computes this tile
func (id tileID) task(taskID uint) task.Task {
	x, y := id.origin()
	return task.Task{
		Bounds: image.Rect(0, 0, task.TileSize, task.TileSize),
		ID:     taskID,
		Tile: &task.Tile{
			Boundary:      id.Boundary,
			MaxIterations: id.MaxIterations,
			OriginX:       x,
			OriginY:       y,
			PixelSize:     pixelSize(id.Level),
			SuperSampling: id.SuperSampling,
		},
	}
}

// maxPreciseLevel is the deepest zoom level float64 can render cleanly with superSampling samples per pixel side:
// adjacent samples must stay at least minSampleSpacingUlps apart everywhere in the plane. Deeper zooms need arbitrary
// precision or perturbation theory, which the explorer does not do.
func maxPreciseLevel(superSampling int) int {
	ulp := planeLimit - math.Nextafter(planeLimit, 0)
	minSpacing := minSampleSpacingUlps * ulp
	level := lowestZoomLevel
	for pixelSize(level+1)/float64(max(superSampling, 1)) >= minSpacing {
		level++
	}
	return level
}
