package mandelbrot

import (
	"DistributedMandelbrot/misc"
	"DistributedMandelbrot/task"
	"image/color"
	"math"
)

type Mandelbrot struct {
	mathLog2 float64
	settings Settings
}

func NewMandelbrot(settings Settings) Mandelbrot {
	mandelbrot := Mandelbrot{
		mathLog2: math.Log(2),
		settings: settings,
	}

	return mandelbrot
}

// todo: add other methods of super sampling
func (m *Mandelbrot) GetPointsToCalculate(frame task.Frame, column int, row int) []Point {
	subPixels := make([]float64, m.settings.SuperSampling)
	subPixels[0] = 0

	if m.settings.SuperSampling > 1 {
		// Using grid super sampling
		for i := 0; i < m.settings.SuperSampling; i++ {
			subPixels[i] = ((0.5 + float64(i)) / float64(m.settings.SuperSampling)) - 0.5
		}
	}

	points := make([]Point, m.settings.SuperSampling*m.settings.SuperSampling)
	i := 0
	for _, sx := range subPixels {
		for _, sy := range subPixels {
			x, y := m.ConvertPixelCoordinateToComplexCoordinate(frame, column, row, sx, sy)
			points[i] = Point{X: x, Y: y}
			i++
		}
	}
	return points
}

func (m *Mandelbrot) EscapeTimeMultiple(points []Point) []float64 {
	iterations := make([]float64, len(points))
	for i, v := range points {
		iterations[i] = m.EscapeTime(v.X, v.Y)
	}
	return iterations
}

func (m *Mandelbrot) GetColorMultiple(iterations []float64) color.RGBA {
	colorSamples := make([]color.RGBA, m.settings.SuperSampling*m.settings.SuperSampling)
	for i, iteration := range iterations {
		colorSamples[i] = m.GetColor(iteration)
	}

	// Generate the final super sampled color
	var r, g, b int
	for _, sample := range colorSamples {
		r += int(sample.R)
		g += int(sample.G)
		b += int(sample.B)
	}
	divisor := len(colorSamples)
	return color.RGBA{R: uint8(r / divisor), G: uint8(g / divisor), B: uint8(b / divisor), A: 255}
}

func (m *Mandelbrot) GetColor(iteration float64) color.RGBA {
	if m.settings.SmoothColoring {
		return m.getSmoothColor(iteration)
	}
	return m.getPaletteColor(iteration)
}

// https://en.wikipedia.org/wiki/Plotting_algorithms_for_the_Mandelbrot_set#Optimized_escape_time_algorithms
func (m *Mandelbrot) EscapeTime(x float64, y float64) float64 {
	iteration, _ := m.escapeTime(x, y)
	return iteration
}

// escapeTime also returns the number of times the loop actually ran, which lets tests confirm that periodicity
// checking exits early for points inside the set
func (m *Mandelbrot) escapeTime(x float64, y float64) (float64, uint) {
	// Calculate the iteration value
	x1, y1, x2, y2 := 0.0, 0.0, 0.0, 0.0
	iteration, maxIterations := 0.0, float64(m.settings.MaxIterations)
	period, oldX, oldY := 0.0, 0.0, 0.0
	var loops uint
	for (x2+y2) <= m.settings.Boundary && iteration < maxIterations {
		y1 = 2*x1*y1 + y
		x1 = x2 - y2 + x
		x2 = x1 * x1
		y2 = y1 * y1
		iteration++
		loops++

		// periodicity checking can speed up detection when the maxIterations value is very large
		// https://en.wikipedia.org/wiki/Cycle_detection
		// https://en.wikipedia.org/wiki/Plotting_algorithms_for_the_Mandelbrot_set#Periodicity_checking
		// https://davidaramant.github.io/post/brents-cycle-detection-algorithm/
		// If z (x1, y1) exactly repeats a previously saved value the orbit is a cycle and will never escape. Since each
		// step is deterministic an exact match can never be a false positive.
		if x1 == oldX && y1 == oldY {
			iteration = maxIterations
			break
		}

		period++
		if period > 20 {
			period = 0
			oldX = x1
			oldY = y1
		}
	}

	// Calculate the normalized iteration count when smooth coloring
	// https://en.wikipedia.org/wiki/Plotting_algorithms_for_the_Mandelbrot_set#Continuous_(smooth)_coloring
	if m.settings.SmoothColoring && iteration < maxIterations {
		zn := math.Log(x1*x1+y1*y1) / 2
		nu := math.Log(zn/m.mathLog2) / m.mathLog2
		iteration = iteration + 1 - nu
	}

	return iteration, loops
}

func (m *Mandelbrot) ConvertPixelCoordinateToComplexCoordinate(frame task.Frame, column int, row int, xOffset float64, yOffset float64) (float64, float64) {
	/*
	 * Convert the (column, row) point on the image to the (x, y) point on the complex axis
	 *
	 * - Pixels are indexed from top left to bottom right, so we need adjust the pixel to the left by half the width and
	 *   half the height to 'center' to get the real location. This is the numerator portion of the formula.
	 * - To maintain proportions of the image (since most images are wider then they are high) we need to divide by the
	 *   shorter side of the image (mind the off by one)
	 * - To magnify the image we need to multiply the denominator by a scalar; the larger the value the more magnified
	 *   the image will be
	 */
	x := frame.CenterX + (float64(column)-(float64(m.settings.Width)/2.0)+xOffset)/(frame.Magnification*(float64(m.settings.ShorterSide)-1))
	y := frame.CenterY + (float64(row)-(float64(m.settings.Height)/2.0)-yOffset)/(frame.Magnification*(float64(m.settings.ShorterSide)-1))
	return x, y
}

func (m *Mandelbrot) getPaletteColor(iterations float64) color.RGBA {
	uintIterations := uint(math.Floor(iterations))
	if uintIterations == m.settings.MaxIterations {
		return m.settings.EscapeColor
	}
	return m.settings.Palette[int(uintIterations)%len(m.settings.Palette)]
}

// https://en.wikipedia.org/wiki/Plotting_algorithms_for_the_Mandelbrot_set#Continuous_(smooth)_coloring
func (m *Mandelbrot) getSmoothColor(iterations float64) color.RGBA {
	// The modf value is the floating point portion of the iteration value
	_, fraction := math.Modf(iterations)

	// Make the new mixed color
	color1 := m.getPaletteColor(iterations)
	color2 := m.getPaletteColor(iterations + 1)
	return misc.LinearInterpolationRGB(color1, color2, fraction)
}
