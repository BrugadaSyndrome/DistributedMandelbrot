package task

import (
	"image"
	"testing"
)

// assertCoverage checks that regions are non-empty, inside a width x height frame, and cover every pixel exactly once
func assertCoverage(t *testing.T, regions []image.Rectangle, width int, height int) {
	t.Helper()
	frame := image.Rect(0, 0, width, height)
	covered := make([]uint8, width*height)
	for _, r := range regions {
		if r.Empty() {
			t.Fatalf("region %v is empty", r)
		}
		if !r.In(frame) {
			t.Fatalf("region %v is outside the %v frame", r, frame)
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				covered[y*width+x]++
			}
		}
	}
	for i, n := range covered {
		if n != 1 {
			t.Fatalf("pixel (column %d, row %d) covered %d times", i%width, i/width, n)
		}
	}
}

func TestRegionsCoverFrameExactlyOnce(t *testing.T) {
	sizes := []struct {
		name          string
		width, height uint
	}{
		{"4K", 3840, 2160},
		{"not divisible by the grid", 97, 61},
		{"smaller than the grid", 7, 5},
		{"single pixel", 1, 1},
	}
	for _, generation := range []Generation{Row, Column, Image, Grid} {
		for _, size := range sizes {
			t.Run(generation.String()+"/"+size.name, func(t *testing.T) {
				assertCoverage(t, Regions(generation, size.width, size.height), int(size.width), int(size.height))
			})
		}
	}
}

func TestRegionsShape(t *testing.T) {
	const width, height = 97, 61

	t.Run("Row is one full width strip per row", func(t *testing.T) {
		regions := Regions(Row, width, height)
		if len(regions) != height {
			t.Fatalf("got %d regions, want %d", len(regions), height)
		}
		for y, r := range regions {
			if r != image.Rect(0, y, width, y+1) {
				t.Fatalf("region %d = %v, want row %d", y, r, y)
			}
		}
	})

	t.Run("Column is one full height strip per column", func(t *testing.T) {
		regions := Regions(Column, width, height)
		if len(regions) != width {
			t.Fatalf("got %d regions, want %d", len(regions), width)
		}
		for x, r := range regions {
			if r != image.Rect(x, 0, x+1, height) {
				t.Fatalf("region %d = %v, want column %d", x, r, x)
			}
		}
	})

	t.Run("Image is the whole frame", func(t *testing.T) {
		regions := Regions(Image, width, height)
		if len(regions) != 1 || regions[0] != image.Rect(0, 0, width, height) {
			t.Fatalf("got %v, want the whole frame", regions)
		}
	})

	t.Run("Grid cells absorb the remainder in the last row and column", func(t *testing.T) {
		regions := Regions(Grid, width, height)
		if len(regions) != int(GridDivisions*GridDivisions) {
			t.Fatalf("got %d regions, want %d", len(regions), GridDivisions*GridDivisions)
		}
		// Cells are 9x6 pixels; the last column is 97 - 81 = 16 wide and the last row is 61 - 54 = 7 tall
		want := map[image.Rectangle]bool{
			image.Rect(0, 0, 9, 6):     true,
			image.Rect(81, 0, 97, 6):   true,
			image.Rect(0, 54, 9, 61):   true,
			image.Rect(81, 54, 97, 61): true,
		}
		for _, r := range regions {
			delete(want, r)
		}
		if len(want) != 0 {
			t.Errorf("missing expected cells %v", want)
		}
	})

	t.Run("Grid skips empty cells in a frame smaller than the grid", func(t *testing.T) {
		regions := Regions(Grid, 7, 5)
		if len(regions) != 1 || regions[0] != image.Rect(0, 0, 7, 5) {
			t.Errorf("got %v, want a single cell covering the frame", regions)
		}
	})
}

func TestRegionsUnknownGeneration(t *testing.T) {
	if regions := Regions(Generation(99), 10, 10); regions != nil {
		t.Errorf("Regions for an unknown generation = %v, want nil", regions)
	}
}

func TestNewTask(t *testing.T) {
	frame := Frame{CenterX: -0.75, CenterY: 0.1, Magnification: 42}
	bounds := image.Rect(2, 1, 6, 4)
	tk := NewTask(7, 3, frame, bounds)

	if tk.ID != 7 || tk.ImageNumber != 3 || tk.Frame != frame || tk.Bounds != bounds {
		t.Errorf("NewTask = %+v, want ID 7, image 3, frame %+v and bounds %v", tk, frame, bounds)
	}
	if tk.Pixels != nil {
		t.Error("a new task should not carry any pixel data")
	}
	if got := tk.PixelCount(); got != 12 {
		t.Errorf("PixelCount() = %d, want 12", got)
	}
	if got := tk.ResultSize(); got != 48 {
		t.Errorf("ResultSize() = %d, want 48", got)
	}
}

func TestHasValidResult(t *testing.T) {
	tk := NewTask(0, 1, Frame{}, image.Rect(0, 0, 2, 3))
	for _, size := range []int{0, 23, 25} {
		tk.Pixels = make([]byte, size)
		if tk.HasValidResult() {
			t.Errorf("%d bytes of pixel data accepted for a task needing 24", size)
		}
	}
	tk.Pixels = make([]byte, 24)
	if !tk.HasValidResult() {
		t.Error("24 bytes of pixel data rejected for a 2x3 task")
	}
}

func TestGenerationString(t *testing.T) {
	tests := map[Generation]string{
		Row:    "Row",
		Column: "Column",
		Image:  "Image",
		Grid:   "Grid",
	}
	for g, want := range tests {
		if got := g.String(); got != want {
			t.Errorf("Generation(%d).String() = %q, want %q", int(g), got, want)
		}
	}
}
