package main

import (
	"image/color"
)

type Mandelbrot struct {
	boundary      float64
	centerX       float64
	centerY       float64
	height        int
	maxIterations int
	shorterSide   int
	width         int
}

func newMandelbrot(boundary float64, centerX float64, centerY float64, height int, maxIterations int, width int) *Mandelbrot {
	shorterSide := height
	if width < height {
		shorterSide = width
	}

	return &Mandelbrot{
		boundary:      boundary,
		centerX:       centerX,
		centerY:       centerY,
		height:        height,
		maxIterations: maxIterations,
		shorterSide:   shorterSide,
		width:         width,
	}
}

func (m *Mandelbrot) mandel(x float64, y float64) int {
	a, b, r, i, z := x, y, 0.0, 0.0, 0.0
	iteration := 0
	for (r+i) <= m.boundary && iteration < m.maxIterations {
		x := r - i + a
		y := z - r - i + b
		r = x * x
		i = y * y
		z = (x + y) * (x + y)
		iteration++
	}

	return iteration
}

func (m *Mandelbrot) calcPixelColor(column int, row int, magnification float64) color.RGBA {
	x := m.centerX + (float64(column)-float64(m.width)/2)/(magnification*(float64(m.shorterSide)-1))
	y := m.centerY + (float64(row)-float64(m.height)/2)/(magnification*(float64(m.shorterSide)-1))
	iterations := m.mandel(x, y)
	return m.getColor(iterations)
}

func (m *Mandelbrot) getColor(iterations int) color.RGBA {
	colors := []color.RGBA{
		{0, 0, 0, 0xff},
		{255, 0, 0, 0xff},
		{0, 255, 0, 0xff},
		{0, 0, 255, 0xff},
		{255, 255, 255, 0xff},
	}
	return colors[iterations%len(colors)]
}
