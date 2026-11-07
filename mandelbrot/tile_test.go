package mandelbrot

import (
	"DistributedMandelbrot/task"
	"image"
	"slices"
	"testing"
)

func TestSubPixelOffsets(t *testing.T) {
	tests := []struct {
		superSampling int
		want          []float64
	}{
		{0, []float64{0}},
		{1, []float64{0}},
		{2, []float64{-0.25, 0.25}},
		{4, []float64{-0.375, -0.125, 0.125, 0.375}},
	}
	for _, tc := range tests {
		if got := subPixelOffsets(tc.superSampling); !slices.Equal(got, tc.want) {
			t.Errorf("subPixelOffsets(%d) = %v, want %v", tc.superSampling, got, tc.want)
		}
	}
}

func TestTileSamplesMatchEscapeTime(t *testing.T) {
	smooth := NewMandelbrot(Settings{Boundary: 1000, MaxIterations: 300, SmoothColoring: true})
	tile := task.Tile{Boundary: 1000, MaxIterations: 300, OriginX: -2, OriginY: -1, PixelSize: 0.5}

	t.Run("one sample per pixel at its center", func(t *testing.T) {
		tile.SuperSampling = 1
		got := TileSamples(tile, image.Rect(0, 0, 3, 2))
		// Pixel centers: x = -1.75, -1.25, -0.75 and y = -0.75, -0.25
		var want []float32
		for _, y := range []float64{-0.75, -0.25} {
			for _, x := range []float64{-1.75, -1.25, -0.75} {
				want = append(want, float32(smooth.EscapeTime(x, y)))
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("TileSamples = %v, want %v", got, want)
		}
	})

	t.Run("super samples are ordered x offset first", func(t *testing.T) {
		tile.SuperSampling = 2
		got := TileSamples(tile, image.Rect(1, 1, 2, 2))
		// The pixel at column 1, row 1 spans x -1.5..-1 and y -0.5..0; its samples are a quarter pixel from the center
		var want []float32
		for _, x := range []float64{-1.375, -1.125} {
			for _, y := range []float64{-0.375, -0.125} {
				want = append(want, float32(smooth.EscapeTime(x, y)))
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("TileSamples = %v, want %v", got, want)
		}
	})
}

func TestTileSamplesInsideTheSet(t *testing.T) {
	tile := task.Tile{Boundary: 1000, MaxIterations: 250, OriginX: -0.2, OriginY: -0.2, PixelSize: 0.1, SuperSampling: 2}
	samples := TileSamples(tile, image.Rect(0, 0, 4, 4))
	if len(samples) != 4*4*2*2 {
		t.Fatalf("got %d samples, want %d", len(samples), 4*4*2*2)
	}
	for i, sample := range samples {
		if sample != 250 {
			t.Fatalf("sample %d = %v, want MaxIterations (250) for a point in the main cardioid", i, sample)
		}
	}
}
