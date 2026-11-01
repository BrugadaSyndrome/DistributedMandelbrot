package mandelbrot

import (
	"image/color"
	"testing"
)

func TestGeneratePalette(t *testing.T) {
	gps := generatePaletteSettings{
		StartColor:   color.RGBA{R: 0, G: 200, B: 100, A: 10},
		EndColor:     color.RGBA{R: 200, G: 0, B: 100, A: 10},
		NumberColors: 4,
	}
	// Fractions are 0, 1/4, 2/4 and 3/4: the end color itself is excluded so chained palettes do not repeat colors
	want := []color.RGBA{
		{R: 0, G: 200, B: 100, A: 255},
		{R: 50, G: 150, B: 100, A: 255},
		{R: 100, G: 100, B: 100, A: 255},
		{R: 150, G: 50, B: 100, A: 255},
	}

	got := gps.GeneratePalette()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("palette[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestGeneratePaletteEmpty(t *testing.T) {
	gps := generatePaletteSettings{NumberColors: 0}
	if got := gps.GeneratePalette(); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
