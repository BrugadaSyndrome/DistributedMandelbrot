package mandelbrot

import (
	"image/color"
	"mandelbrot/misc"
)

type generatePaletteSettings struct {
	StartColor   color.RGBA
	EndColor     color.RGBA
	NumberColors int
}

func (gps *generatePaletteSettings) GeneratePalette() []color.RGBA {
	palette := make([]color.RGBA, 0)
	for j := 0; j < gps.NumberColors; j++ {
		fraction := float64(j) / float64(gps.NumberColors)
		colorStep := color.RGBA{
			R: misc.LerpUint8(gps.StartColor.R, gps.EndColor.R, fraction),
			G: misc.LerpUint8(gps.StartColor.G, gps.EndColor.G, fraction),
			B: misc.LerpUint8(gps.StartColor.B, gps.EndColor.B, fraction),
			A: 255}
		palette = append(palette, colorStep)
	}
	return palette
}
