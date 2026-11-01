package mandelbrot

import (
	"DistributedMandelbrot/task"
	"image/color"
	"math"
	"testing"
)

// newTestMandelbrot builds a Mandelbrot from verified settings after applying any overrides
func newTestMandelbrot(t *testing.T, override func(s *Settings)) Mandelbrot {
	t.Helper()
	s := Settings{}
	if override != nil {
		override(&s)
	}
	if err := s.Verify(); err != nil {
		t.Fatalf("Verify() returned error: %s", err)
	}
	return NewMandelbrot(s)
}

// naiveEscapeTime is the escape time algorithm without periodicity checking. It mirrors the arithmetic of escapeTime
// exactly so results can be compared for equality.
func naiveEscapeTime(m *Mandelbrot, x float64, y float64) float64 {
	x1, y1, x2, y2 := 0.0, 0.0, 0.0, 0.0
	iteration, maxIterations := 0.0, float64(m.settings.MaxIterations)
	for (x2+y2) <= m.settings.Boundary && iteration < maxIterations {
		y1 = 2*x1*y1 + y
		x1 = x2 - y2 + x
		x2 = x1 * x1
		y2 = y1 * y1
		iteration++
	}
	if m.settings.SmoothColoring && iteration < maxIterations {
		zn := math.Log(x1*x1+y1*y1) / 2
		nu := math.Log(zn/m.mathLog2) / m.mathLog2
		iteration = iteration + 1 - nu
	}
	return iteration
}

func TestEscapeTimeKnownPoints(t *testing.T) {
	m := newTestMandelbrot(t, func(s *Settings) {
		s.Boundary = 4
		s.MaxIterations = 500
	})

	tests := []struct {
		name string
		x, y float64
		want float64
	}{
		{"origin is in the set", 0, 0, 500},
		{"-1 has a period 2 orbit", -1, 0, 500},
		{"-0.1+0.1i is in the main cardioid", -0.1, 0.1, 500},
		// z: 0 -> 2 (|z|^2 = 4, still inside boundary) -> 6 (escaped)
		{"2 escapes after 2 iterations", 2, 0, 2},
		// z: 0 -> 1 -> 2 -> 5 (escaped)
		{"1 escapes after 3 iterations", 1, 0, 3},
		{"far point escapes immediately", 10, 10, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.EscapeTime(tc.x, tc.y); got != tc.want {
				t.Errorf("EscapeTime(%v, %v) = %v, want %v", tc.x, tc.y, got, tc.want)
			}
		})
	}
}

// Periodicity checking is an optimization only, so it must never change the result for any point
func TestEscapeTimeMatchesNaive(t *testing.T) {
	for _, smooth := range []bool{false, true} {
		m := newTestMandelbrot(t, func(s *Settings) {
			s.MaxIterations = 2000
			s.SmoothColoring = smooth
			s.GeneratePaletteSettings = []generatePaletteSettings{{
				StartColor:   color.RGBA{A: 255},
				EndColor:     color.RGBA{R: 255, G: 255, B: 255, A: 255},
				NumberColors: 8,
			}}
		})

		const steps = 120
		for i := range steps {
			for j := range steps {
				x := -2.0 + 2.5*float64(i)/steps
				y := -1.25 + 2.5*float64(j)/steps
				got, want := m.EscapeTime(x, y), naiveEscapeTime(&m, x, y)
				if got != want && !(math.IsNaN(got) && math.IsNaN(want)) {
					t.Fatalf("smooth=%v: EscapeTime(%v, %v) = %v, naive = %v", smooth, x, y, got, want)
				}
			}
		}
	}
}

func TestPeriodicityCheckExitsEarly(t *testing.T) {
	const maxIterations = 1_000_000
	m := newTestMandelbrot(t, func(s *Settings) {
		s.MaxIterations = maxIterations
	})

	tests := []struct {
		name string
		x, y float64
	}{
		{"origin", 0, 0},
		{"period 2 bulb", -1, 0},
		{"main cardioid", -0.1, 0.1},
		{"main cardioid near real axis", 0.2, 0},
		{"period 3 bulb", -0.122, 0.745},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iteration, loops := m.escapeTime(tc.x, tc.y)
			if iteration != maxIterations {
				t.Fatalf("escapeTime(%v, %v) iteration = %v, want %v", tc.x, tc.y, iteration, float64(maxIterations))
			}
			if loops > 10_000 {
				t.Errorf("escapeTime(%v, %v) ran %d loops; periodicity checking should exit well before %d", tc.x, tc.y, loops, maxIterations)
			}
		})
	}
}

func TestConvertPixelCoordinateToComplexCoordinate(t *testing.T) {
	m := newTestMandelbrot(t, func(s *Settings) {
		s.Width = 200
		s.Height = 101
	})
	coordinate := task.Coordinate{CenterX: -0.5, CenterY: 0.25, Magnification: 2, Column: 100, Row: 50}
	// One pixel step in the complex plane: 1 / (magnification * (shorterSide - 1))
	step := 1 / (2.0 * 100)

	t.Run("image center maps to the center point", func(t *testing.T) {
		x, y := m.ConvertPixelCoordinateToComplexCoordinate(coordinate, 0, 0)
		if !almostEqual(x, -0.5) || !almostEqual(y, 0.25-0.5*step) {
			t.Errorf("got (%v, %v), want (%v, %v)", x, y, -0.5, 0.25-0.5*step)
		}
	})

	t.Run("moving one pixel moves one step", func(t *testing.T) {
		x0, y0 := m.ConvertPixelCoordinateToComplexCoordinate(coordinate, 0, 0)
		next := coordinate
		next.Column++
		next.Row++
		x1, y1 := m.ConvertPixelCoordinateToComplexCoordinate(next, 0, 0)
		if !almostEqual(x1-x0, step) || !almostEqual(y1-y0, step) {
			t.Errorf("step = (%v, %v), want (%v, %v)", x1-x0, y1-y0, step, step)
		}
	})

	t.Run("doubling magnification halves the step", func(t *testing.T) {
		zoomed := coordinate
		zoomed.Magnification = 4
		zoomed.Column = 101
		x, _ := m.ConvertPixelCoordinateToComplexCoordinate(zoomed, 0, 0)
		if !almostEqual(x-(-0.5), step/2) {
			t.Errorf("step = %v, want %v", x-(-0.5), step/2)
		}
	})

	t.Run("offsets move x right and y up", func(t *testing.T) {
		x0, y0 := m.ConvertPixelCoordinateToComplexCoordinate(coordinate, 0, 0)
		x1, y1 := m.ConvertPixelCoordinateToComplexCoordinate(coordinate, 0.5, 0.5)
		if !almostEqual(x1-x0, step/2) || !almostEqual(y1-y0, -step/2) {
			t.Errorf("offset delta = (%v, %v), want (%v, %v)", x1-x0, y1-y0, step/2, -step/2)
		}
	})
}

func TestGetPointsToCalculate(t *testing.T) {
	coordinate := task.Coordinate{CenterX: -0.5, Magnification: 1, Column: 10, Row: 20}

	t.Run("no super sampling returns the pixel point", func(t *testing.T) {
		m := newTestMandelbrot(t, nil)
		points := m.GetPointsToCalculate(coordinate)
		x, y := m.ConvertPixelCoordinateToComplexCoordinate(coordinate, 0, 0)
		if len(points) != 1 || points[0] != (Point{X: x, Y: y}) {
			t.Errorf("got %v, want [{%v %v}]", points, x, y)
		}
	})

	t.Run("grid super sampling is centered on the pixel", func(t *testing.T) {
		m := newTestMandelbrot(t, func(s *Settings) { s.SuperSampling = 3 })
		points := m.GetPointsToCalculate(coordinate)
		if len(points) != 9 {
			t.Fatalf("got %d points, want 9", len(points))
		}
		seen := make(map[Point]bool)
		var sumX, sumY float64
		for _, p := range points {
			seen[p] = true
			sumX += p.X
			sumY += p.Y
		}
		if len(seen) != 9 {
			t.Errorf("got %d distinct points, want 9", len(seen))
		}
		x, y := m.ConvertPixelCoordinateToComplexCoordinate(coordinate, 0, 0)
		if !almostEqual(sumX/9, x) || !almostEqual(sumY/9, y) {
			t.Errorf("mean = (%v, %v), want (%v, %v)", sumX/9, sumY/9, x, y)
		}
	})
}

func TestGetColor(t *testing.T) {
	red := color.RGBA{R: 200, A: 255}
	green := color.RGBA{G: 100, A: 255}
	blue := color.RGBA{B: 50, A: 255}
	escape := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	settings := func(smooth bool) func(s *Settings) {
		return func(s *Settings) {
			s.MaxIterations = 100
			s.EscapeColor = escape
			s.Palette = []color.RGBA{red, green, blue}
			s.SmoothColoring = smooth
		}
	}

	t.Run("palette coloring", func(t *testing.T) {
		m := newTestMandelbrot(t, settings(false))
		tests := []struct {
			iteration float64
			want      color.RGBA
		}{
			{0, red},
			{1, green},
			{2.9, blue},
			{3, red}, // wraps around the palette
			{100, escape},
		}
		for _, tc := range tests {
			if got := m.GetColor(tc.iteration); got != tc.want {
				t.Errorf("GetColor(%v) = %v, want %v", tc.iteration, got, tc.want)
			}
		}
	})

	t.Run("smooth coloring blends neighbouring colors", func(t *testing.T) {
		m := newTestMandelbrot(t, settings(true))
		if got := m.GetColor(1); got != green {
			t.Errorf("GetColor(1) = %v, want %v", got, green)
		}
		want := color.RGBA{R: 100, G: 50, A: 255}
		if got := m.GetColor(0.5); got != want {
			t.Errorf("GetColor(0.5) = %v, want %v", got, want)
		}
	})
}

func TestGetColorMultipleAveragesSamples(t *testing.T) {
	m := newTestMandelbrot(t, func(s *Settings) {
		s.MaxIterations = 100
		s.Palette = []color.RGBA{{R: 200, A: 255}, {G: 100, A: 255}}
		s.SuperSampling = 2
	})
	got := m.GetColorMultiple([]float64{0, 0, 1, 1})
	want := color.RGBA{R: 100, G: 50, A: 255}
	if got != want {
		t.Errorf("GetColorMultiple = %v, want %v", got, want)
	}
}

func BenchmarkEscapeTimeInterior(b *testing.B) {
	s := Settings{MaxIterations: 100_000}
	_ = s.Verify()
	m := NewMandelbrot(s)
	for b.Loop() {
		m.EscapeTime(-0.1, 0.1)
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-12*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}
