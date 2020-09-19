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

type Worker struct {
	Client    *rpc.Client
	IpAddress string
	Logger    *log.Logger
	Port      int
	Settings  TaskSettings
	Wait      *sync.WaitGroup
}

func newWorker(ipAddress string, port int, wg *sync.WaitGroup) Worker {
	worker := Worker{
		IpAddress: ipAddress,
		Logger:    log.New(os.Stdout, fmt.Sprintf("Worker[%s:%d] ", ipAddress, port), log.Ldate|log.Ltime|log.Lshortfile),
		Port:      port,
		Wait:      wg,
	}

	worker.Client = worker.connectCoordinator(coordinatorAddress)

	newRPCServer(&worker, ipAddress, port)

	return worker
}

func (w *Worker) connectCoordinator(masterAddress string) *rpc.Client {
	client, err := rpc.DialHTTP("tcp", masterAddress)
	if err != nil {
		w.Logger.Fatalf("Failed dailing address: %s - %s", masterAddress, err)
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
		w.Logger.Print("All tasks handed out")
		return err
	}

	// Unable to make the call
	w.Client.Close()
	w.Logger.Printf("ERROR - Failed to call coordinator at address: %s, method: %s, error: %v", coordinatorAddress, method, err)
	return err
}

func (w *Worker) ProcessTasks() {
	var junk Nothing
	var count int
	var startTime time.Time
	var elapsedTime time.Duration

	// Register worker with coordinator
	address := fmt.Sprintf("%s:%d", w.IpAddress, w.Port)
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
	w.Settings = settings

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
			row, column, magnification := task.NextTask()
			if row == -1 && column == -1 && magnification == -1 {
				break
			}

			task.RecordColor(w.determinePixelColor(row, column, magnification))
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
	x1, y1, x2, y2, max := 0.0, 0.0, 0.0, 0.0, float64(w.Settings.MaxIterations)
	iteration := 0.0
	for (x2+y2) <= w.Settings.Boundary && iteration < max {
		y1 = 2*x1*y1 + y
		x1 = x2 - y2 + x
		x2 = x1 * x1
		y2 = y1 * y1
		iteration++
	}

	if w.Settings.SmoothColoring && iteration < max {
		// https://en.wikipedia.org/wiki/Plotting_algorithms_for_the_Mandelbrot_set
		// Calculate the normalized iteration count when smooth coloring
		zn := math.Log(x1*x1+y1*y1) / 2
		nu := math.Log(zn/math.Log(2)) / math.Log(2)
		iteration = iteration + 1 - nu
	}

	return iteration
}

func (w *Worker) determinePixelColor(row int, column int, magnification float64) color.RGBA {
	subPixels := []float64{0}
	var finalColor color.RGBA

	// Using grid super sampling
	if w.Settings.SuperSampling > 1 {
		subPixels = []float64{}
		for i := 0; i < w.Settings.SuperSampling; i++ {
			subPixels = append(subPixels, ((0.5+float64(i))/float64(w.Settings.SuperSampling))-0.5)
		}
	}

	// Collect each sample
	var colorSamples []color.RGBA
	var iteration float64
	for _, sx := range subPixels {
		for _, sy := range subPixels {
			// Convert the (column, row) point on the image to the (x, y) point in the imaginary axis
			x := w.Settings.CenterX + ((float64(column)-float64(w.Settings.Width)/2)+sx)/(magnification*(float64(w.Settings.ShorterSide)-1))
			y := w.Settings.CenterY + ((float64(row)-float64(w.Settings.Height)/2)-sy)/(magnification*(float64(w.Settings.ShorterSide)-1))
			iteration = w.mandel(x, y)
			colorSamples = append(colorSamples, w.GetColor(iteration))
		}
	}

	// Set the final color to the only sample taken in case super sampling is not used
	finalColor = colorSamples[0]

	// Generate the final super sampled color
	if w.Settings.SuperSampling > 1 {
		var r, g, b, a int
		for _, sample := range colorSamples {
			r += int(sample.R)
			g += int(sample.G)
			b += int(sample.B)
			a += int(sample.A)
		}
		divisor := len(colorSamples)
		finalColor = color.RGBA{R: uint8(r / divisor), G: uint8(g / divisor), B: uint8(b / divisor), A: uint8(a / divisor)}
	}

	// Apply Smooth coloring if enabled
	if int(math.Floor(iteration)) != w.Settings.MaxIterations && w.Settings.SmoothColoring {
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
	if intIterations == w.Settings.MaxIterations {
		return color.RGBA{R: 0, G: 0, B: 0, A: 255}
	}
	return w.Settings.Colors[intIterations%len(w.Settings.Colors)]
}
