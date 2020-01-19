package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

type Mandelbrot struct {
	boundary      float64
	centerX       float64
	centerY       float64
	height        int
	image         *image.RGBA
	magnification float64
	maxIterations int
	shorterSide   int
	width         int
}

func newMandelbrot(boundary float64, centerX float64, centerY float64, height int, magnification float64, maxIterations int, width int) *Mandelbrot {
	rect := image.Rectangle{
		Min: image.Point{X: 0, Y: 0},
		Max: image.Point{X: width, Y: height},
	}

	shorterSide := height
	if width < height {
		shorterSide = width
	}

	return &Mandelbrot{
		boundary:      boundary,
		centerX:       centerX,
		centerY:       centerY,
		height:        height,
		image:         image.NewRGBA(rect),
		magnification: magnification,
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

func (m *Mandelbrot) calcPixelColor(column int, row int) color.RGBA {
	x := m.centerX + (float64(column)-float64(m.width)/2)/(m.magnification*(float64(m.shorterSide)-1))
	y := m.centerY + (float64(row)-float64(m.height)/2)/(m.magnification*(float64(m.shorterSide)-1))
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

func (m *Mandelbrot) GenerateMandelbrot() error {
	for r := 0; r < m.height; r++ {
		for c := 0; c < m.width; c++ {
			m.image.SetRGBA(c, r, m.calcPixelColor(c, r))
		}
	}

	name := fmt.Sprintf("X%f_Y%f_M%f_B%f_I%d_W%d_H%d.png", m.centerX, m.centerY, m.magnification, m.boundary, m.maxIterations, m.width, m.height)
	f, _ := os.Create(name)
	err := png.Encode(f, m.image)
	if err != nil {
		return err
	}
	return nil
}
