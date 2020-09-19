package main

import "image/color"

func interpolate(v1 uint8, v2 uint8, fraction float64) uint8 {
	return uint8(float64(v1) + float64(int(v2)-int(v1))*fraction)
}

func linearInterpolationRGB(color1 color.RGBA, color2 color.RGBA, fraction float64) color.RGBA {
	var finalColor color.RGBA
	finalColor.R = interpolate(color1.R, color2.R, fraction)
	finalColor.G = interpolate(color1.G, color2.G, fraction)
	finalColor.B = interpolate(color1.B, color2.B, fraction)
	finalColor.A = 255
	return finalColor
}
