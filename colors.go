package main

import "image/color"

func interpolateFloat64(v1 float64, v2 float64, fraction float64) float64 {
	return v1 + (v2-v1)*fraction
}

func interpolateUint8(v1 uint8, v2 uint8, fraction float64) uint8 {
	v1f := float64(v1)
	v2f := float64(v2)
	return uint8(interpolateFloat64(v1f, v2f, fraction))
}

func linearInterpolationRGB(color1 color.RGBA, color2 color.RGBA, fraction float64) color.RGBA {
	var finalColor color.RGBA
	finalColor.R = uint8(interpolateFloat64(float64(color1.R), float64(color2.R), fraction))
	finalColor.G = uint8(interpolateFloat64(float64(color1.G), float64(color2.G), fraction))
	finalColor.B = uint8(interpolateFloat64(float64(color1.B), float64(color2.B), fraction))
	finalColor.A = 255
	return finalColor
}

/*
// HSV //
type HSV struct {
	Hue int // 0-360
	Saturation float64 // 0-1
	Value float64 // 0-1
}

func newHSV(color color.RGBA) HSV {
	// R, G, B values need to be in the range [0, 1] on a scale of 360
	r := float64(color.R)/255
	g := float64(color.G)/255
	b := float64(color.B)/255

	// Determine the color value
	value := math.Max(math.Max(r, g), b)

	// Determine the color saturation
	min := math.Min(math.Min(r, g), b)
	chroma := value - min
	saturation := 0.0
	if value != 0.0 {
		saturation = chroma / value
	}

	// Determine the color hue
	hue := 0.0
	if min != value {
		if value == r {
			hue = math.Mod((g-b)/chroma, 6.0)
		} else if value == g {
			hue = ((b-r)/chroma) + 2.0
		} else if value == b {
			hue = ((r-g)/chroma) + 4.0
		}
		hue *= 60.0
		if hue < 0 {
			hue += 360.0
		}
	}

	return HSV{
		Hue: int(hue),
		Saturation: saturation,
		Value: value,
	}
}

func (h HSV) RGBA() color.RGBA {
	chroma := h.Value * h.Saturation
	hprime := h.Hue / 60.0
	x := chroma * (1 - math.Abs(math.Mod(float64(hprime), 2.0) - 1))

	var r, g, b float64
	if hprime >= 0 && hprime <= 1 {
		r = chroma
		g = x
	} else if hprime > 1 && hprime <= 2 {
		r = x
		g = chroma
	} else if hprime > 2 && hprime <= 3 {
		g = chroma
		b = x
	} else if hprime > 3 && hprime <= 4 {
		g = x
		b = chroma
	} else if hprime > 4 && hprime <= 5 {
		r = x
		b = chroma
	} else if hprime > 5 && hprime <= 6 {
		r = chroma
		b = x
	}

	m := h.Value - chroma
	return color.RGBA{
		R: uint8(m + r),
		G: uint8(m + g),
		B: uint8(m + b),
		A: 255,
	}
}

func linearInterpolationHSV(color1 color.RGBA, color2 color.RGBA, fraction float64) HSV {
	color1HSV := newHSV(color1)
	color2HSV := newHSV(color2)
	hue := interpolateFloat64(float64(color1HSV.Hue), float64(color2HSV.Hue), fraction)
	saturation := interpolateFloat64(color1HSV.Saturation, color2HSV.Saturation, fraction)
	value := interpolateFloat64(color1HSV.Value, color2HSV.Value, fraction)
	log.Printf("linear interpolation hsv: %+v %+v %+v %+v %+v %+v %+v", color1HSV, color1, color2HSV, color2, int(hue), saturation, value)
	return HSV{int(hue), saturation, value}
}

// HSL //
type HSL struct {
	hue int // 0-360
	saturation float64 // 0-1
	lightness float64 // 0-1
}

func newHSL(color color.RGBA) HSL {

}
*/
