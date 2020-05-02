package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/ioutil"
	"log"
	"math"
	"net/rpc"
	"os"
	"sync"
)

type Coordinator struct {
	Colors             []color.RGBA
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
	Wait               *sync.WaitGroup
	Workers            map[string]*rpc.Client
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
		Colors:             make([]color.RGBA, 0),
		ImageCount:         int(math.Ceil((magnificationEnd - magnificationStart) / magnificationStep)),
		ImageTasks:         make([]*ImageTask, 0),
		Logger:             log.New(os.Stdout, fmt.Sprintf("Coordinator[%s:%d] ", ipAddress, port), log.Ldate|log.Ltime|log.Lshortfile),
		MagnificationEnd:   magnificationEnd,
		MagnificationStart: magnificationStart,
		MagnificationStep:  magnificationStep,
		TasksDone:          make(chan LineTask, 100),
		TasksTodo:          make(chan LineTask, 100),
		Wait:               &sync.WaitGroup{},
		Workers:            make(map[string]*rpc.Client, 0),
	}

	coordinator.Settings = TaskSettings{
		Boundary:       boundary,
		CenterX:        centerX,
		CenterY:        centerY,
		Height:         height,
		MaxIterations:  maxIterations,
		ShorterSide:    shorterSide,
		SmoothColoring: smoothColoring,
		Width:          width,
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

func (c *Coordinator) callWorker(workerAddress string, method string, request interface{}, reply interface{}) error {
	err := c.Workers[workerAddress].Call(method, request, reply)

	// The call was a success
	if err == nil {
		return nil
	}

	// All work is done
	if err.Error() == "all tasks handed out" {
		c.Logger.Print("All tasks handed out")
		return nil
	}

	c.Workers[workerAddress].Close()
	c.Logger.Printf("ERROR - Failed to call worker at address: %s, method: %s, error: %v", coordinatorAddress, method, err)
	return err
}

func (c *Coordinator) GenerateTasks() {
	c.Logger.Print("Generating tasks")

	// for each picture to be generated
	imageNumber := 0
	for magnification := c.MagnificationStart; magnification < c.MagnificationEnd; magnification += c.MagnificationStep {

		// for each pixel in this particular image
		for row := 0; row < c.Settings.Height; row++ {
			task := LineTask{
				currentWidth:  0,
				ImageNumber:   imageNumber,
				Iterations:    make([]float64, 0),
				Magnification: magnification,
				Row:           row,
				Width:         c.Settings.Width,
			}

			c.Mutex.Lock()
			c.TasksTodo <- task
			c.Mutex.Unlock()
		}

		imageNumber++
	}
	close(c.TasksTodo)

	c.Logger.Printf("Done generating %d tasks", c.TaskCount)
}

func (c *Coordinator) GetColor(iterations float64) color.RGBA {
	intIterations := int(math.Floor(iterations))
	if intIterations == c.Settings.MaxIterations {
		return color.RGBA{0, 0, 0, 255}
	}
	return c.Colors[intIterations%len(c.Colors)]
}

func (c *Coordinator) LoadColorPalette(fileName string) {
	file, err := os.Open(fileName)
	if err != nil {
		log.Fatalf("Unable to open %s - %s", fileName, err)
	}
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("Unable to read %s - %s", fileName, err)
	}
	c.Mutex.Lock()
	err = json.Unmarshal(fileBytes, &c.Colors)
	c.Mutex.Unlock()
	if err != nil {
		log.Fatalf("Unable to unmarshal %s - %s", fileName, err)
	}
}

func (c *Coordinator) RequestTask(request Nothing, reply *LineTask) error {
	c.Mutex.Lock()
	task, more := <-c.TasksTodo
	c.Mutex.Unlock()
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
	c.TasksDone <- request
	c.Mutex.Unlock()
	return nil
}

func (c *Coordinator) GetTaskSettings(request Nothing, reply *TaskSettings) error {
	c.Mutex.Lock()
	reply.Boundary = c.Settings.Boundary
	reply.CenterX = c.Settings.CenterX
	reply.CenterY = c.Settings.CenterY
	reply.Height = c.Settings.Height
	reply.MaxIterations = c.Settings.MaxIterations
	reply.ShorterSide = c.Settings.ShorterSide
	reply.SmoothColoring = c.Settings.SmoothColoring
	reply.Width = c.Settings.Width
	c.Mutex.Unlock()
	return nil
}

func (c *Coordinator) RegisterWorker(request string, reply *Nothing) error {
	client, err := rpc.DialHTTP("tcp", request)
	if err != nil {
		c.Logger.Fatalf("Failed registering worker at address: %s - %s", request, err)
	}
	c.Logger.Printf("Opened connection to worker at %s", request)

	c.Mutex.Lock()
	c.Workers[request] = client
	c.Wait.Add(1)
	c.Mutex.Unlock()
	return nil
}

func (c *Coordinator) DeRegisterWorker(request string, reply *Nothing) error {
	err := c.Workers[request].Close()
	if err != nil {
		c.Logger.Fatalf("Failed de-registering worker at address: %s - %s", request, err)
	}
	c.Logger.Printf("Closed connection to worker at %s", request)

	c.Mutex.Lock()
	c.Wait.Done()
	c.Mutex.Unlock()
	return nil
}
