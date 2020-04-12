package main

import (
	"errors"
	"fmt"
	"image"
	"sync"
)

type Coordinator struct {
	boundary           float64
	centerX            float64
	centerY            float64
	height             int
	magnificationEnd   float64
	magnificationStart float64
	magnificationStep  float64
	maxIterations      int
	shorterSide        int
	width              int

	ImageCount int
	Images     []*image.RGBA
	Name       string
	TaskCount  int
	TasksDone  chan LineTask
	TasksTodo  chan LineTask

	Mutex sync.Mutex
}

func newCoordinator(ipAddress string, port int) Coordinator {
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
		magnificationEnd:   magnificationEnd,
		magnificationStart: magnificationStart,
		magnificationStep:  magnificationStep,
		maxIterations:      maxIterations,
		shorterSide:        shorterSide,
		width:              width,

		Name:      fmt.Sprintf("Coordinator[%s:%d]", ipAddress, port),
		TasksDone: make(chan LineTask, 100),
		TasksTodo: make(chan LineTask, 100),
	}

	coordinator.ImageCount = int(((magnificationEnd - magnificationStart) + 1) / magnificationStep)
	coordinator.Images = make([]*image.RGBA, 0)
	coordinator.TaskCount = height * coordinator.ImageCount
	for c := 0; c < coordinator.ImageCount; c++ {
		coordinator.Images = append(coordinator.Images, image.NewRGBA(rect))
	}

	newRPCServer(&coordinator, ipAddress, port)

	return coordinator
}

func (c *Coordinator) GenerateTasks() {
	Info.Printf("%s - is generating tasks", c.Name)

	// for each picture to be generated
	number := 0
	for magnification := c.magnificationStart; magnification <= c.magnificationEnd; magnification += c.magnificationStep {

		// for each pixel in this particular image
		for row := 0; row < c.height; row++ {
			// for column := 0; column < c.width; column++ {
			task := LineTask{
				currentWidth:  0,
				ImageNumber:   number,
				Iterations:    make([]int, 0),
				Magnification: magnification,
				Row:           row,
				Width:         c.width,
			}

			c.Mutex.Lock()
			c.TasksTodo <- task
			c.Mutex.Unlock()
			// }
		}

		number++
	}
	close(c.TasksTodo)

	Info.Printf("%s - is done generating %d tasks", c.Name, c.TaskCount)
}

func (c *Coordinator) RequestTask(request Nothing, reply *LineTask) error {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	task, more := <-c.TasksTodo
	if !more {
		reply = nil
		Info.Print("All tasks handed out")
		return errors.New("all tasks handed out")
	}
	*reply = task
	return nil
}

func (c *Coordinator) TaskFinished(request LineTask, reply *Nothing) error {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	c.TasksDone <- request
	return nil
}
