package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"net/rpc"
	"os"
	"sync"
	"time"
)

/* WorkerSettings */
type WorkerSettings struct {
	CoordinatorAddress string
	CoordinatorPort    int
	WorkerCount        int
	WorkerAddress      string
	WorkerPort         int
}

func (ws *WorkerSettings) String() string {
	output := "\nWorker settings are: \n"
	output += fmt.Sprintf("Coordinator Address: %s\n", ws.CoordinatorAddress)
	output += fmt.Sprintf("Coordinator Port: %d\n", ws.CoordinatorPort)
	output += fmt.Sprintf("Worker Address: %s\n", ws.WorkerAddress)
	output += fmt.Sprintf("Worker Count: %d\n", ws.WorkerCount)
	output += fmt.Sprintf("Worker Port Range: %d-%d\n", ws.WorkerPort, ws.WorkerPort+ws.WorkerCount-1)
	return output
}

func (ws *WorkerSettings) Verify() error {
	if ws.CoordinatorAddress == "" {
		ws.CoordinatorAddress = getLocalAddress()
	}
	if ws.CoordinatorPort <= 0 {
		ws.CoordinatorPort = 10000
	}
	if ws.WorkerAddress == "" {
		ws.WorkerAddress = getLocalAddress()
	}
	if ws.WorkerCount <= 0 {
		ws.WorkerCount = 2
	}
	if ws.WorkerPort <= 0 {
		ws.WorkerPort = 10001
	}
	return nil
}

/* Worker */
type Worker struct {
	CoordinatorAddress string
	Client             *rpc.Client
	MyAddress          string
	Logger             *log.Logger
	MathLog2           float64
	Port               int
	TaskSettings       TaskSettings
	Wait               *sync.WaitGroup
}

func newWorker(settings WorkerSettings, portOffset int, wg *sync.WaitGroup) Worker {
	port := settings.WorkerPort + portOffset

	worker := Worker{
		CoordinatorAddress: fmt.Sprintf("%s:%d", settings.CoordinatorAddress, settings.CoordinatorPort),
		MyAddress:          settings.WorkerAddress,
		Logger:             log.New(os.Stdout, fmt.Sprintf("Worker[%s:%d] ", settings.WorkerAddress, port), log.Ldate|log.Ltime|log.Lshortfile),
		MathLog2:           math.Log(2),
		Port:               port,
		Wait:               wg,
	}

	worker.Client = worker.connectCoordinator(fmt.Sprintf("%s:%d", settings.CoordinatorAddress, settings.CoordinatorPort))

	newRPCServer(&worker, settings.WorkerAddress, port)

	return worker
}

func (w *Worker) connectCoordinator(masterAddress string) *rpc.Client {
	client, err := rpc.DialHTTP("tcp", masterAddress)
	if err != nil {
		w.Logger.Fatalf("Failed dialing address: %s - %s", masterAddress, err)
	}
	w.Logger.Printf("Opened connection to coordinator at %s", masterAddress)
	return client
}

func (w *Worker) callCoordinator(method string, request interface{}, reply interface{}) error {
	err := w.Client.Call(method, request, reply)

	// The call was a success
	if err == nil {
		return nil
	}

	// All work is done
	if err.Error() == "all tasks handed out" {
		w.Logger.Print("Coordinator says all tasks handed out")
		return err
	}

	// Unable to make the call
	w.Client.Close()
	w.Logger.Printf("ERROR - Failed to call coordinator at address: %s, method: %s, error: %v", w.CoordinatorAddress, method, err)
	return err
}

func (w *Worker) ProcessTasks() {
	var junk Nothing
	var count int
	var startTime time.Time
	var elapsedTime time.Duration

	// Register worker with coordinator
	address := fmt.Sprintf("%s:%d", w.MyAddress, w.Port)
	err := w.callCoordinator("Coordinator.RegisterWorker", address, &junk)
	if err != nil {
		w.Logger.Fatalf("Failed to register with coordinator: %s", err)
	}

	// Fetch task settings from coordinator
	var settings TaskSettings
	err = w.callCoordinator("Coordinator.GetTaskSettings", junk, &settings)
	if err != nil {
		w.Logger.Fatalf("Failed to get task settings: %s", err)
	}
	w.Logger.Printf("Got task settings from coordinator: %+v", settings)
	w.TaskSettings = settings

	w.Logger.Printf("Now processing tasks")
	startTime = time.Now()
	for {
		var task LineTask

		// Ask coordinator for a task
		err := w.callCoordinator("Coordinator.RequestTask", junk, &task)
		if err != nil {
			break
		}

		// Calculate escape value
		for {
			row, column, magnification, centerX, centerY := task.NextTask()
			if row == -1 && column == -1 && magnification == -1 && centerX == -1 && centerY == -1 {
				break
			}

			task.RecordColor(w.determinePixelColor(centerX, centerY, row, column, magnification))
		}

		// Return result to master
		err = w.callCoordinator("Coordinator.TaskFinished", task, &junk)
		if err != nil {
			w.Logger.Printf("WARNING - Coordinator.TaskFinished - %s", err)
		}
		count++
	}
	// Worker is done processing
	elapsedTime = time.Since(startTime)
	w.Logger.Printf("Done processing %d tasks in %s", count, elapsedTime)

	// Inform coordinator we are leaving and shutdown
	w.Logger.Print("Shutting down")
	w.callCoordinator("Coordinator.DeRegisterWorker", address, &junk)
	w.Client.Close()
	w.Wait.Done()
}

func (w *Worker) mandel(x float64, y float64) float64 {
	// Calculate the iteration value
	x1, y1, x2, y2, max := 0.0, 0.0, 0.0, 0.0, float64(w.TaskSettings.MaxIterations)
	iteration := 0.0
	for (x2+y2) <= w.TaskSettings.Boundary && iteration < max {
		y1 = 2*x1*y1 + y
		x1 = x2 - y2 + x
		x2 = x1 * x1
		y2 = y1 * y1
		iteration++
	}

	if w.TaskSettings.SmoothColoring && iteration < max {
		// https://en.wikipedia.org/wiki/Plotting_algorithms_for_the_Mandelbrot_set
		// Calculate the normalized iteration count when smooth coloring
		zn := math.Log(x1*x1+y1*y1) / 2
		nu := math.Log(zn/w.MathLog2) / w.MathLog2
		iteration = iteration + 1 - nu
	}

	return iteration
}

func (w *Worker) determinePixelColor(centerX float64, centerY float64, row int, column int, magnification float64) color.RGBA {
	subPixels := make([]float64, w.TaskSettings.SuperSampling)
	subPixels[0] = 0
	var finalColor color.RGBA

	// Using grid super sampling
	if w.TaskSettings.SuperSampling > 1 {
		for i := 0; i < w.TaskSettings.SuperSampling; i++ {
			subPixels[i] = ((0.5 + float64(i)) / float64(w.TaskSettings.SuperSampling)) - 0.5
		}
	}

	// Collect each sample
	colorSamples := make([]color.RGBA, w.TaskSettings.SuperSampling*w.TaskSettings.SuperSampling)
	var iteration float64
	index := 0
	for _, sx := range subPixels {
		for _, sy := range subPixels {
			// Convert the (column, row) point on the image to the (x, y) point on the complex axis
			x := centerX + ((float64(column)-float64(w.TaskSettings.Width)/2)+sx)/(magnification*(float64(w.TaskSettings.ShorterSide)-1))
			y := centerY + ((float64(row)-float64(w.TaskSettings.Height)/2)-sy)/(magnification*(float64(w.TaskSettings.ShorterSide)-1))
			iteration = w.mandel(x, y)
			colorSamples[index] = w.GetColor(iteration)
			index++
		}
	}

	// Set the final color to the only sample taken in case super sampling is not used
	finalColor = colorSamples[0]

	// Generate the final super sampled color
	if w.TaskSettings.SuperSampling > 1 {
		var r, g, b int
		for _, sample := range colorSamples {
			r += int(sample.R)
			g += int(sample.G)
			b += int(sample.B)
		}
		divisor := len(colorSamples)
		finalColor = color.RGBA{R: uint8(r / divisor), G: uint8(g / divisor), B: uint8(b / divisor), A: 255}
	}

	// Apply Smooth coloring if enabled
	if int(math.Floor(iteration)) != w.TaskSettings.MaxIterations && w.TaskSettings.SmoothColoring {
		// The modf value is the floating point portion of the iteration value
		_, fraction := math.Modf(iteration)

		// Make the new mixed color
		color1 := w.GetColor(iteration)
		color2 := w.GetColor(iteration + 1)
		finalColor = linearInterpolationRGB(color1, color2, fraction)
	}

	return finalColor
}

func (w *Worker) GetColor(iterations float64) color.RGBA {
	intIterations := int(math.Floor(iterations))
	if intIterations == w.TaskSettings.MaxIterations {
		return w.TaskSettings.EscapeColor
	}
	return w.TaskSettings.Palette[intIterations%len(w.TaskSettings.Palette)]
}
