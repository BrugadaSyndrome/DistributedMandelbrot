package misc

import (
	"image/color"
	"math"
)

func LerpFloat64(v1 float64, v2 float64, fraction float64) float64 {
	return v1 + (v2-v1)*fraction
}

func LerpUint8(v1 uint8, v2 uint8, fraction float64) uint8 {
	v1f := float64(v1)
	v2f := float64(v2)
	return uint8(LerpFloat64(v1f, v2f, fraction))
}

func LinearInterpolationRGB(color1 color.RGBA, color2 color.RGBA, fraction float64) color.RGBA {
	var finalColor color.RGBA
	finalColor.R = uint8(LerpFloat64(float64(color1.R), float64(color2.R), fraction))
	finalColor.G = uint8(LerpFloat64(float64(color1.G), float64(color2.G), fraction))
	finalColor.B = uint8(LerpFloat64(float64(color1.B), float64(color2.B), fraction))
	finalColor.A = 255
	return finalColor
}

func EaseOutExpo(t float64) float64 {
	if t >= 1 {
		return 1
	}
	return 1 - math.Pow(2, -10*t)
}

func EaseInExpo(t float64) float64 {
	if t <= 0 {
		return 0
	}
	return math.Pow(2, 10*t-10)
}
