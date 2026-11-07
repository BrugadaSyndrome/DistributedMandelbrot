package explorer

import (
	"DistributedMandelbrot/task"
	"image"
	"math"
	"testing"
)

func TestTileOrigin(t *testing.T) {
	tests := []struct {
		key        tileKey
		wantX      float64
		wantY      float64
		wantPixels float64
	}{
		{tileKey{0, 0, 0}, 0, 0, 4.0 / 256},
		{tileKey{0, -1, -1}, -4, -4, 4.0 / 256},
		{tileKey{2, -3, 1}, -3, 1, 1.0 / 256},
		{tileKey{-1, 1, -1}, 8, -8, 8.0 / 256},
		// Deep tiles still land exactly on their coordinates, since spans are powers of two
		{tileKey{39, -1 << 38, 3}, -2, 3 * math.Ldexp(1, -37), math.Ldexp(1, -45)},
	}
	for _, tc := range tests {
		x, y := tc.key.origin()
		if x != tc.wantX || y != tc.wantY {
			t.Errorf("origin of %s = (%v, %v), want (%v, %v)", tc.key, x, y, tc.wantX, tc.wantY)
		}
		if got := pixelSize(tc.key.Level); got != tc.wantPixels {
			t.Errorf("pixelSize(%d) = %v, want %v", tc.key.Level, got, tc.wantPixels)
		}
	}
}

func TestTileInPlane(t *testing.T) {
	tests := []struct {
		key  tileKey
		want bool
	}{
		{tileKey{0, 0, 0}, true},
		{tileKey{0, -1, -1}, true},
		{tileKey{0, 1, 0}, false},  // starts at x = 4
		{tileKey{0, -2, 0}, false}, // ends at x = -4
		{tileKey{0, 0, 1}, false},
		{tileKey{-4, -1, -1}, true}, // one huge tile around the whole plane
		{tileKey{10, 1023, -1024}, true},
		{tileKey{10, 1024, 0}, false},
	}
	for _, tc := range tests {
		if got := tc.key.inPlane(); got != tc.want {
			t.Errorf("%s.inPlane() = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestTileIDTask(t *testing.T) {
	id := tileID{tileKey: tileKey{3, -5, 2}, Boundary: 1000, MaxIterations: 500, SuperSampling: 2}
	todo := id.task(9)
	want := task.Tile{Boundary: 1000, MaxIterations: 500, OriginX: -2.5, OriginY: 1, PixelSize: 0.5 / 256, SuperSampling: 2}
	if todo.ID != 9 || todo.Bounds != image.Rect(0, 0, 256, 256) || todo.Tile == nil || *todo.Tile != want {
		t.Errorf("task = %+v with tile %+v, want ID 9, a 256x256 tile and %+v", todo, todo.Tile, want)
	}
}

// sampleSpacingUlps is how many float64 steps near planeLimit separate adjacent samples at level
func sampleSpacingUlps(level int, superSampling int) float64 {
	ulp := planeLimit - math.Nextafter(planeLimit, 0)
	return pixelSize(level) / float64(superSampling) / ulp
}

func TestMaxPreciseLevel(t *testing.T) {
	for _, superSampling := range []int{1, 2, 3, 4, 8} {
		level := maxPreciseLevel(superSampling)
		if got := sampleSpacingUlps(level, superSampling); got < minSampleSpacingUlps {
			t.Errorf("SuperSampling %d: level %d has samples %v ulps apart, want at least %d", superSampling, level, got, minSampleSpacingUlps)
		}
		if got := sampleSpacingUlps(level+1, superSampling); got >= minSampleSpacingUlps {
			t.Errorf("SuperSampling %d: level %d still has samples %v ulps apart, so %d is not the deepest", superSampling, level+1, got, level)
		}
	}
	// The documented default: 4x super sampling stops at level 39
	if got := maxPreciseLevel(4); got != 39 {
		t.Errorf("maxPreciseLevel(4) = %d, want 39", got)
	}
}
