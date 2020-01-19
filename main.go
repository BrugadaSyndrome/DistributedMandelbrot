package main

import "flag"

/*
 * TODO: Mandelbrot.GenerateMandelbrotZoom(startMag, endMag, magStep)
 * TODO: need to make a folder to store images (especially for zoom generation)
 * TODO: smooth coloring, aa
 */

func main() {
	boundary := flag.Float64("boundary", 4.0, "Boundary escape value")
	centerX := flag.Float64("centerX", 0.0, "Center x value of mandelbrot set")
	centerY := flag.Float64("centerY", 0.0, "Center y value of mandelbrot set")
	height := flag.Int("height", 1080, "Height of resulting image")
	magnification := flag.Float64("magnification", 1.0, "Zoom level")
	maxIterations := flag.Int("maxIterations", 1000, "Iterations to run to verify each point")
	width := flag.Int("width", 1920, "Width of resulting image")
	flag.Parse()

	mandelbrot := newMandelbrot(*boundary, *centerX, *centerY, *height, *magnification, *maxIterations, *width)
	mandelbrot.GenerateMandelbrot()
}
