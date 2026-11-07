package mandelbrot

import (
	"DistributedMandelbrot/task"
	"image"
)

// TileSamples computes the smooth iteration count of every sample in bounds of an explorer tile. Samples are ordered
// row by row and pixel by pixel; within a pixel they are ordered like GetPointsToCalculate, the x offset in the outer
// loop. A point that never escapes has the value MaxIterations.
//
// Colors are applied later in the browser, so smooth coloring is always on here: the browser can recover the banded
// palette color from a smooth value but not the other way around.
//
// Unlike the movie's mapping, a pixel's samples are spread around its center, (column + 0.5, row + 0.5), so the tiles
// of a zoom level line up seamlessly with each other.
func TileSamples(tile task.Tile, bounds image.Rectangle) []float32 {
	m := NewMandelbrot(Settings{
		Boundary:       tile.Boundary,
		MaxIterations:  tile.MaxIterations,
		SmoothColoring: true,
	})

	offsets := subPixelOffsets(tile.SuperSampling)
	samples := make([]float32, 0, bounds.Dx()*bounds.Dy()*len(offsets)*len(offsets))
	for row := bounds.Min.Y; row < bounds.Max.Y; row++ {
		for column := bounds.Min.X; column < bounds.Max.X; column++ {
			for _, sx := range offsets {
				x := tile.OriginX + (float64(column)+0.5+sx)*tile.PixelSize
				for _, sy := range offsets {
					y := tile.OriginY + (float64(row)+0.5+sy)*tile.PixelSize
					samples = append(samples, float32(m.EscapeTime(x, y)))
				}
			}
		}
	}
	return samples
}

// subPixelOffsets spreads superSampling samples evenly across a pixel, centered on 0. These are the offsets
// GetPointsToCalculate uses; one sample is at the center of the pixel.
func subPixelOffsets(superSampling int) []float64 {
	superSampling = max(superSampling, 1)
	offsets := make([]float64, superSampling)
	for i := range offsets {
		offsets[i] = ((0.5 + float64(i)) / float64(superSampling)) - 0.5
	}
	return offsets
}
