package main

import (
	"image"
)

type Coordinator struct {
	boundary           float64
	centerX            float64
	centerY            float64
	height             int
	image              *image.RGBA
	magnificationEnd   float64
	magnificationStart float64
	magnificationStep  float64
	maxIterations      int
	shorterSide        int
	width              int

	Tasks chan pixelTask
}

func newCoordinator(ipAddress string, port string) Coordinator {
	if width <= 0 {
		Error.Fatal("Width must be greater than 0")
	}
	if height <= 0 {
		Error.Fatal("Height must be greater than 0")
	}

	rect := image.Rectangle{
		Min: image.Point{X: 0, Y: 0},
		Max: image.Point{X: width, Y: height},
	}
	shorterSide := height
	if width < height {
		shorterSide = width
	}

	coordinator := Coordinator{
		boundary:           boundary,
		centerX:            centerX,
		centerY:            centerY,
		height:             height,
		image:              image.NewRGBA(rect),
		magnificationEnd:   magnificationEnd,
		magnificationStart: magnificationStart,
		magnificationStep:  magnificationStep,
		maxIterations:      maxIterations,
		shorterSide:        shorterSide,
		width:              width,

		Tasks: make(chan pixelTask, 100),
	}

	newRPCServer(coordinator, ipAddress, port)

	return coordinator
}

func (c *Coordinator) GenerateTasks() {
	// for each picture to be generated
	for magnification := c.magnificationStart; magnification <= c.magnificationEnd; magnification += c.magnificationStep {

		// for each pixel in this particular image
		for row := 0; row < c.height; row++ {
			for column := 0; column < c.width; column++ {
				c.Tasks <- pixelTask{
					Row:           row,
					Column:        column,
					Magnification: magnification,
				}

				// PROBABLY BEST DONE BY THE WORKER
				// Since each pixel is from [0, c.height] and [0, c.width] and not on the real axis we need to convert
				// the (column, row) point on the image to the (x, y) point in the real axis
				// x := c.centerX + (float64(column)-float64(c.width)/2)/(magnification*(float64(c.shorterSide)-1))
				// y := c.centerY + (float64(row)-float64(c.height)/2)/(magnification*(float64(c.shorterSide)-1))
			}
		}
	}
}

func (c *Coordinator) CollectResults() {

}

/*
func (c *Coordinator) GenerateMandelbrot() error {
	for x := 0; x < c.height; x++ {
		for y := 0; y < c.width; y++ {
			c.image.SetRGBA(y, x, c.calcPixelColor(y, x))
		}
	}

	name := fmt.Sprintf("X%f_Y%f_M%f_B%f_I%d_W%d_H%d.png", m.centerX, m.centerY, m.magnification, m.boundary, m.maxIterations, c.width, c.height)
	f, _ := os.Create(name)
	err := png.Encode(f, c.image)
	if err != nil {
		return err
	}
	return nil
}
*/
