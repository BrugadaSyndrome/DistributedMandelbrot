package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"sync"
)

type Coordinator struct {
	ImageCount         int
	ImageTasks         []*ImageTask
	Logger             *log.Logger
	MagnificationEnd   float64
	MagnificationStart float64
	MagnificationStep  float64
	Mutex              sync.Mutex
	Settings           TaskSettings
	TaskCount          int
	TasksDone          chan LineTask
	TasksTodo          chan LineTask
}

type ImageTask struct {
	Generated  bool
	Image      *image.RGBA
	PixelsLeft int
}

func newCoordinator(ipAddress string, port int) Coordinator {
	if width <= 0 {
		log.Fatal("Width must be greater than 0")
	}
	if height <= 0 {
		log.Fatal("Height must be greater than 0")
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
		ImageCount:         int(((magnificationEnd - magnificationStart) + 1) / magnificationStep),
		ImageTasks:         make([]*ImageTask, 0),
		Logger:             log.New(os.Stdout, fmt.Sprintf("Coordinator[%s:%d] ", ipAddress, port), log.Ldate|log.Ltime|log.Lshortfile),
		MagnificationEnd:   magnificationEnd,
		MagnificationStart: magnificationStart,
		MagnificationStep:  magnificationStep,
		TasksDone:          make(chan LineTask, 100),
		TasksTodo:          make(chan LineTask, 100),
	}

	coordinator.Settings = TaskSettings{
		Boundary:      boundary,
		CenterX:       centerX,
		CenterY:       centerY,
		Height:        height,
		MaxIterations: maxIterations,
		ShorterSide:   shorterSide,
		Width:         width,
	}
	coordinator.TaskCount = height * coordinator.ImageCount
	pixelCount := height * width
	for c := 0; c < coordinator.ImageCount; c++ {
		imageTask := &ImageTask{
			Generated:  false,
			Image:      image.NewRGBA(rect),
			PixelsLeft: pixelCount,
		}
		coordinator.ImageTasks = append(coordinator.ImageTasks, imageTask)
	}

	newRPCServer(&coordinator, ipAddress, port)

	return coordinator
}

func (c *Coordinator) GenerateTasks() {
	c.Logger.Print("generating tasks")

	// for each picture to be generated
	number := 0
	for magnification := c.MagnificationStart; magnification <= c.MagnificationEnd; magnification += c.MagnificationStep {

		// for each pixel in this particular image
		for row := 0; row < c.Settings.Height; row++ {
			task := LineTask{
				currentWidth:  0,
				ImageNumber:   number,
				Iterations:    make([]int, 0),
				Magnification: magnification,
				Row:           row,
				Width:         c.Settings.Width,
			}

			c.Mutex.Lock()
			c.TasksTodo <- task
			c.Mutex.Unlock()
		}

		number++
	}
	close(c.TasksTodo)

	c.Logger.Printf("done generating %d tasks", c.TaskCount)
}

func (c *Coordinator) GetColor(iterations int) color.RGBA {
	if iterations == c.Settings.MaxIterations {
		return color.RGBA{0, 0, 0, 255}
	}
	colors := []color.RGBA{
		{25, 0, 0, 255},
		{50, 0, 0, 255},
		{75, 0, 0, 255},
		{100, 0, 0, 255},
		{125, 0, 0, 255},
		{150, 0, 0, 255},
		{175, 0, 0, 255},
		{200, 0, 0, 255},
		{225, 0, 0, 255},
		{255, 0, 0, 255},
	}
	return colors[iterations%len(colors)]
}

func (c *Coordinator) RequestTask(request Nothing, reply *LineTask) error {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	task, more := <-c.TasksTodo
	if !more {
		reply = nil
		c.Logger.Print("All tasks handed out")
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

func (c *Coordinator) TaskSettings(request Nothing, reply *TaskSettings) error {
	reply.Boundary = c.Settings.Boundary
	reply.CenterX = c.Settings.CenterX
	reply.CenterY = c.Settings.CenterY
	reply.Height = c.Settings.Height
	reply.MaxIterations = c.Settings.MaxIterations
	reply.ShorterSide = c.Settings.ShorterSide
	reply.Width = c.Settings.Width
	return nil
}
