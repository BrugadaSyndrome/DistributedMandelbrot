package mandelbrot

import (
	"image/color"
	glog "log"
	"mandelbrot/log"
)

type Settings struct {
	logger log.Logger

	Boundary                float64
	CenterX                 float64
	CenterY                 float64
	EscapeColor             color.RGBA
	GeneratePaletteSettings []generatePaletteSettings
	Height                  uint
	Magnification           float64
	MaxIterations           uint
	Palette                 []color.RGBA
	ShorterSide             uint
	SmoothColoring          bool
	SuperSampling           int
	Width                   uint
}

func (s *Settings) Verify() error {
	s.logger = log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "MandelbrotSettings", log.Normal, nil)

	if s.Boundary <= 0 {
		s.Boundary = 100
	}
	if s.CenterX > 4.0 || s.CenterX < -4.0 {
		s.CenterX = 0.0
	}
	if s.CenterY > 4.0 || s.CenterY < -4.0 {
		s.CenterY = 0.0
	}
	if s.EscapeColor == (color.RGBA{}) {
		s.EscapeColor = color.RGBA{R: 0, G: 0, B: 0, A: 255}
	}
	if len(s.GeneratePaletteSettings) > 0 {
		s.Palette = make([]color.RGBA, 0)
		for i := 0; i < len(s.GeneratePaletteSettings); i++ {
			s.Palette = append(s.Palette, s.GeneratePaletteSettings[i].GeneratePalette()...)
		}
	}
	if s.Height <= 0 {
		s.Height = 1080
	}
	if s.Magnification <= 0 {
		s.Magnification = 2
	}
	if s.MaxIterations <= 0 {
		s.MaxIterations = 1000
	}
	if len(s.Palette) == 0 {
		s.Palette = []color.RGBA{{R: 255, G: 255, B: 255, A: 255}}
	}
	// s.SmoothColoring defaults to false already
	if s.SuperSampling < 1 {
		s.SuperSampling = 1
	}
	if s.Width <= 0 {
		s.Width = 1920
	}

	// Need to determine the shorter side of the image to preserve the correct scale
	s.ShorterSide = s.Height
	if s.Height > s.Width {
		s.ShorterSide = s.Width
	}

	// Smooth coloring wont work with one color
	if len(s.Palette) == 1 && s.SmoothColoring == true {
		s.SmoothColoring = false
		s.logger.Infof("Disabling SmoothColoring since the palette only has one color.")
	}

	return nil
}
