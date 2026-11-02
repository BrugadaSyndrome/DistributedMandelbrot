package misc

import (
	"image/color"
	"math"
	"testing"
)

func TestLerpFloat64(t *testing.T) {
	tests := []struct {
		v1, v2, fraction, want float64
	}{
		{0, 10, 0, 0},
		{0, 10, 1, 10},
		{0, 10, 0.25, 2.5},
		{-2, 2, 0.5, 0},
		{10, 0, 0.25, 7.5},
	}
	for _, tc := range tests {
		if got := LerpFloat64(tc.v1, tc.v2, tc.fraction); got != tc.want {
			t.Errorf("LerpFloat64(%v, %v, %v) = %v, want %v", tc.v1, tc.v2, tc.fraction, got, tc.want)
		}
	}
}

func TestLerpUint8(t *testing.T) {
	tests := []struct {
		v1, v2   uint8
		fraction float64
		want     uint8
	}{
		{0, 255, 0, 0},
		{0, 255, 1, 255},
		{0, 200, 0.5, 100},
		{200, 0, 0.25, 150},
		{0, 3, 0.5, 1}, // truncates toward zero
	}
	for _, tc := range tests {
		if got := LerpUint8(tc.v1, tc.v2, tc.fraction); got != tc.want {
			t.Errorf("LerpUint8(%v, %v, %v) = %v, want %v", tc.v1, tc.v2, tc.fraction, got, tc.want)
		}
	}
}

func TestLinearInterpolationRGB(t *testing.T) {
	c1 := color.RGBA{R: 0, G: 100, B: 200, A: 10}
	c2 := color.RGBA{R: 200, G: 100, B: 0, A: 20}
	want := color.RGBA{R: 50, G: 100, B: 150, A: 255}
	if got := LinearInterpolationRGB(c1, c2, 0.25); got != want {
		t.Errorf("LinearInterpolationRGB = %v, want %v", got, want)
	}
}

func TestEaseOutExpo(t *testing.T) {
	if got := EaseOutExpo(0); got != 0 {
		t.Errorf("EaseOutExpo(0) = %v, want 0", got)
	}
	for _, v := range []float64{1, 1.5} {
		if got := EaseOutExpo(v); got != 1 {
			t.Errorf("EaseOutExpo(%v) = %v, want 1", v, got)
		}
	}
	assertMonotonic(t, "EaseOutExpo", EaseOutExpo)
}

func TestEaseInExpo(t *testing.T) {
	for _, v := range []float64{0, -0.5} {
		if got := EaseInExpo(v); got != 0 {
			t.Errorf("EaseInExpo(%v) = %v, want 0", v, got)
		}
	}
	if got := EaseInExpo(1); got != 1 {
		t.Errorf("EaseInExpo(1) = %v, want 1", got)
	}
	assertMonotonic(t, "EaseInExpo", EaseInExpo)
}

func TestSmoothStep(t *testing.T) {
	tests := []struct{ in, want float64 }{
		{0, 0}, {0.25, 0.15625}, {0.5, 0.5}, {1, 1}, {-1, 0}, {2, 1},
	}
	for _, tc := range tests {
		if got := SmoothStep(tc.in); got != tc.want {
			t.Errorf("SmoothStep(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	// Symmetric about the middle, so easing in looks the same as easing out
	if got := SmoothStep(0.2) + SmoothStep(0.8); math.Abs(got-1) > 1e-12 {
		t.Errorf("SmoothStep(0.2) + SmoothStep(0.8) = %v, want 1", got)
	}
	assertMonotonic(t, "SmoothStep", SmoothStep)
}

// assertMonotonic checks that an easing function never decreases over [0, 1]
func assertMonotonic(t *testing.T, name string, ease func(float64) float64) {
	t.Helper()
	previous := ease(0)
	for i := 1; i <= 100; i++ {
		v := float64(i) / 100
		current := ease(v)
		if current < previous {
			t.Fatalf("%s(%v) = %v is less than the previous value %v", name, v, current, previous)
		}
		previous = current
	}
}
