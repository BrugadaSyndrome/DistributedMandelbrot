package mandelbrot

import (
	"image/color"
	"testing"
)

func TestSettingsVerifyDefaults(t *testing.T) {
	s := Settings{CenterX: 10, CenterY: -10}
	if err := s.Verify(); err != nil {
		t.Fatalf("Verify() returned error: %s", err)
	}

	black := color.RGBA{A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	checks := []struct {
		name      string
		got, want any
	}{
		{"Boundary", s.Boundary, 100.0},
		{"CenterX", s.CenterX, 0.0},
		{"CenterY", s.CenterY, 0.0},
		{"EscapeColor", s.EscapeColor, black},
		{"Height", s.Height, uint(1080)},
		{"Width", s.Width, uint(1920)},
		{"Magnification", s.Magnification, 2.0},
		{"MaxIterations", s.MaxIterations, uint(1000)},
		{"SuperSampling", s.SuperSampling, 1},
		{"ShorterSide", s.ShorterSide, uint(1080)},
		{"Palette length", len(s.Palette), 1},
		{"Palette[0]", s.Palette[0], white},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestSettingsVerifyKeepsValidValues(t *testing.T) {
	s := Settings{Boundary: 4, CenterX: -1.5, Height: 600, Width: 800, MaxIterations: 50, SuperSampling: 3}
	_ = s.Verify()
	if s.Boundary != 4 || s.CenterX != -1.5 || s.Height != 600 || s.Width != 800 || s.MaxIterations != 50 || s.SuperSampling != 3 {
		t.Errorf("Verify() changed valid values: %+v", s)
	}
}

func TestSettingsVerifyShorterSide(t *testing.T) {
	s := Settings{Width: 500, Height: 900}
	_ = s.Verify()
	if s.ShorterSide != 500 {
		t.Errorf("ShorterSide = %d, want 500 for a portrait image", s.ShorterSide)
	}
}

func TestSettingsVerifyDisablesSmoothColoringWithOneColor(t *testing.T) {
	s := Settings{SmoothColoring: true}
	_ = s.Verify()
	if s.SmoothColoring {
		t.Error("SmoothColoring should be disabled when the palette has a single color")
	}
}

func TestSettingsVerifyGeneratesPalette(t *testing.T) {
	s := Settings{
		Palette:        []color.RGBA{{R: 9, A: 255}}, // replaced by the generated palette
		SmoothColoring: true,
		GeneratePaletteSettings: []generatePaletteSettings{
			{StartColor: color.RGBA{A: 255}, EndColor: color.RGBA{R: 255, A: 255}, NumberColors: 3},
			{StartColor: color.RGBA{A: 255}, EndColor: color.RGBA{B: 255, A: 255}, NumberColors: 2},
		},
	}
	_ = s.Verify()
	if len(s.Palette) != 5 {
		t.Fatalf("len(Palette) = %d, want 5", len(s.Palette))
	}
	if !s.SmoothColoring {
		t.Error("SmoothColoring should stay enabled with a multi-color palette")
	}
}
