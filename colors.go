package main

import "image/color"

func linearInterpolationRGB(color1 color.RGBA, color2 color.RGBA, fraction float64) color.RGBA {
	var finalColor color.RGBA
	finalColor.R = uint8(lerpFloat64(float64(color1.R), float64(color2.R), fraction))
	finalColor.G = uint8(lerpFloat64(float64(color1.G), float64(color2.G), fraction))
	finalColor.B = uint8(lerpFloat64(float64(color1.B), float64(color2.B), fraction))
	finalColor.A = 255
	return finalColor
}
